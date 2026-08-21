// Package tui — Bubble Tea model.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/config"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/edit"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/model"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
)

// ChatEntry is one message in the chat log.
type ChatEntry struct {
	Role    string // "user", "assistant", "system", "tool"
	Agent   string // agent name when role=assistant
	Content string
	Time    time.Time
}

// Model is the Bubble Tea root model.
type Model struct {
	client  *Client
	apiURL  string
	version VersionInfo
	status  StatusInfo

	// UI components
	viewport viewport.Model
	input    textarea.Model

	// State
	entries      []ChatEntry
	pending      bool
	streaming    bool        // SSE stream currently open
	streamReq    ChatRequest // request that opened the stream
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamBuf    strings.Builder // accumulator for current assistant message
	streamAgent  string          // agent reported by status event
	width        int
	height       int
	ready        bool
	quitting     bool
	sessionID    string
	lastError    string

	// Plan mode
	plan     *model.PlanResponse
	planMode bool // true = waiting for user to approve/reject plan
	planExec *model.PlanExecutor

	// Batch edit mode
	batch       *edit.BatchEdit
	batchResult *edit.BatchResult
	batchMode   bool

	// Custom commands from commands.yaml
	customCommands []config.CustomCommand
	repoPath       string

	// Command palette (Ctrl+P)
	paletteMode   bool // true = palette is open
	paletteInput   textinput.Model
	paletteEntries []CommandItem
	paletteFilter  string

	// Agent cycling (Tab)
	agents       []string // ordered list of available agents
	currentAgent string

	// Token tracking (for footer)
	usage *UsageInfo

	// Toast notifications
	toast      string
	toastUntil time.Time
}

// UsageInfo tracks token usage and cost for a chat turn.
type UsageInfo struct {
	InputTokens  int
	OutputTokens int
	ContextLimit int
	Cost         float64
}

// ContextPct returns the context usage percentage as a string like "45%".
func (u *UsageInfo) ContextPct() string {
	if u == nil || u.ContextLimit == 0 {
		return ""
	}
	used := u.InputTokens + u.OutputTokens
	if used == 0 {
		return ""
	}
	pct := (used * 100) / u.ContextLimit
	return fmt.Sprintf("%d%%", pct)
}

// FormatTokens renders the input/output tokens as a human-readable string.
func (u *UsageInfo) FormatTokens() string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf("%d in / %d out", u.InputTokens, u.OutputTokens)
}

// FormatCost renders the cost as USD.
func (u *UsageInfo) FormatCost() string {
	if u == nil || u.Cost == 0 {
		return ""
	}
	return fmt.Sprintf("$%.4f", u.Cost)
}

// CommandItem is one entry in the command palette.
type CommandItem struct {
	Name        string
	Description string
	Shortcut    string
	Action      func(m *Model) tea.Cmd
}

// TickMsg forces a status refresh every 5s.
type TickMsg time.Time

// ChatResultMsg is delivered when a /api/chat response arrives (blocking).
type ChatResultMsg struct {
	Resp ChatResponse
	Err  error
}

// BatchEditMsg is delivered when a batch edit preview is ready.
type BatchEditMsg struct {
	Batch  edit.BatchEdit
	Result edit.BatchResult
	Err    error
}

// ChatStreamMsg is delivered for each SSE event from /api/chat/stream.
// Kind is one of "status", "token", "tool_call", "tool_result", "done", "error".
// For Kind == "token", Token is the raw word; for others, Raw holds the JSON.
type ChatStreamMsg struct {
	Kind  string
	Agent string
	Token string
	Raw   string
	Err   error
}

// StatusMsg is delivered when a status poll completes.
type StatusMsg struct {
	Status StatusInfo
	Err    error
}

// VersionMsg is delivered after the initial version probe.
type VersionMsg struct {
	Version VersionInfo
	Err     error
}

// AgentsMsg carries the agent roster.
type AgentsMsg struct {
	Agents []AgentInfo
	Err    error
}

// SessionCreatedMsg is delivered after /api/session/new.
type SessionCreatedMsg struct {
	Result map[string]string
	Err    error
}

// HistoryLoadedMsg is delivered when prior session history is loaded.
type HistoryLoadedMsg struct {
	Entries   []ChatEntry
	SessionID string
	Err       error
}

// SessionsLoadedMsg is delivered when the session list is fetched.
type SessionsLoadedMsg struct {
	Sessions []SessionInfo
	Err      error
}

// CommandOutputMsg carries arbitrary command output for the viewport.
type CommandOutputMsg struct {
	Title string
	Body  string
	Err   error
}

// PlanReceivedMsg is delivered when the agent returns a plan.
type PlanReceivedMsg struct {
	Plan model.PlanResponse
	Err  error
}

// PlanApprovedMsg signals user approved the plan.
type PlanApprovedMsg struct {
	AutoApprove bool // true = approve all safe steps
}

// PlanRejectedMsg signals user rejected the plan.
type PlanRejectedMsg struct{}

// PlanStepDoneMsg signals one plan step completed.
type PlanStepDoneMsg struct {
	Index  int
	Result string
	Err    error
}

// DelegationMsg signals Magos delegated to a sub-agent.
type DelegationMsg struct {
	From   string // "magos"
	To     string // "glm", "kimi", "mimo", "minimax"
	Reason string // why delegated
}

// Initial returns the seed model. Probe is performed via Init() Cmd.
func Initial(apiURL string, customCommands []config.CustomCommand, repoPath string) Model {
	// Multi-line textarea (OpenCode-style: Enter=newline, Shift+Enter=submit).
	ti := textarea.New()
	ti.Placeholder = "Type a message… (try @magos hello, /help)"
	ti.Focus()
	ti.SetWidth(80)
	ti.SetHeight(3)
	ti.CharLimit = 16384
	ti.ShowLineNumbers = false

	vp := viewport.New(80, 20)
	vp.SetContent(renderWelcome())

	return Model{
		client:         NewClient(apiURL),
		apiURL:         apiURL,
		input:          ti,
		viewport:       vp,
		entries:        []ChatEntry{},
		customCommands: customCommands,
		repoPath:       repoPath,
		agents:         []string{"magos", "kimi", "mimo", "qwen", "minimax"},
		currentAgent:   "magos",
		paletteInput:   buildPaletteInput(),
		paletteEntries: defaultPaletteEntries(),
	}
}

// Init kicks off async probes and loads the last session history.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		probeVersion(m.client),
		probeStatus(m.client),
		loadInitialHistory(m.client),
		tickCmd(),
	)
}

// AppendEntry adds a chat entry to the log.
func (m *Model) AppendEntry(role, agent, content string) {
	m.entries = append(m.entries, ChatEntry{
		Role:    role,
		Agent:   agent,
		Content: content,
		Time:    time.Now(),
	})
	m.refreshViewport()
}

// refreshViewport re-renders the chat viewport.
func (m *Model) refreshViewport() {
	m.viewport.SetContent(renderEntries(m.entries))
	m.viewport.GotoBottom()
}

// activeAgent returns the current agent name.  Falls back to "magos"
// (the canonical Anathemetron core) when no agent is set.
func (m Model) activeAgent() string {
	if m.currentAgent != "" {
		return m.currentAgent
	}
	if m.streamAgent != "" {
		return m.streamAgent
	}
	return "magos"
}

// cycleAgent advances to the next agent in the cycle.
func (m *Model) cycleAgent() {
	if len(m.agents) == 0 {
		return
	}
	for i, a := range m.agents {
		if a == m.currentAgent {
			m.currentAgent = m.agents[(i+1)%len(m.agents)]
			return
		}
	}
	m.currentAgent = m.agents[0]
}

// buildPaletteInput creates the input for the command palette (Ctrl+P).
func buildPaletteInput() textinput.Model {
	pi := textinput.New()
	pi.Placeholder = "Type a command…"
	pi.CharLimit = 200
	pi.Prompt = "❯ "
	pi.PromptStyle = theme.AgentStyle
	return pi
}

// defaultPaletteEntries returns the default command palette items.
// Each command wires to a real existing backend endpoint or TUI action.
func defaultPaletteEntries() []CommandItem {
	return []CommandItem{
		{
			Name:        "session.new",
			Description: "Start a new session (POST /commune/new)",
			Shortcut:    "ctrl+n",
			Action: func(m *Model) tea.Cmd {
				m.pending = true
				m.AppendEntry("system", "", "creating new session…")
				return createSession(m.client)
			},
		},
		{
			Name:        "session.list",
			Description: "Recall past episodes via /animus/recall",
			Shortcut:    "ctrl+l",
			Action: func(m *Model) tea.Cmd {
				return loadInitialHistory(m.client)
			},
		},
		{
			Name:        "agent.cycle",
			Description: "Cycle to next agent (magos → kimi → mimo → qwen → minimax)",
			Shortcut:    "tab",
			Action: func(m *Model) tea.Cmd {
				m.cycleAgent()
				m.AppendEntry("system", "", "→ agent: "+m.activeAgent())
				return nil
			},
		},
		{
			Name:        "cerebrum.status",
			Description: "Show Cognee cerebrum indexer status",
			Shortcut:    "ctrl+b",
			Action: func(m *Model) tea.Cmd {
				return probeCerebrumStatus()
			},
		},
		{
			Name:        "obsidian.search",
			Description: "Semantic search the indexed obsidian vault",
			Shortcut:    "ctrl+f",
			Action: func(m *Model) tea.Cmd {
				return runObsidianSearch(m)
			},
		},
		{
			Name:        "goals.list",
			Description: "List active goals (GET /goals/)",
			Shortcut:    "ctrl+g",
			Action: func(m *Model) tea.Cmd {
				return loadGoals(m)
			},
		},
		{
			Name:        "memoria.search",
			Description: "Search memoria (POST /memoria/search)",
			Shortcut:    "ctrl+m",
			Action: func(m *Model) tea.Cmd {
				return runMemoriaSearch(m)
			},
		},
		{
			Name:        "session.clear",
			Description: "Clear current session messages",
			Shortcut:    "ctrl+shift+c",
			Action: func(m *Model) tea.Cmd {
				m.entries = []ChatEntry{}
				m.refreshViewport()
				return nil
			},
		},
		{
			Name:        "help",
			Description: "Show all available commands (this palette)",
			Shortcut:    "F1",
			Action: func(m *Model) tea.Cmd {
				m.AppendEntry("system", "", "Available palette commands: "+
					"session.new, session.list, agent.cycle, cerebrum.status, "+
					"obsidian.search, goals.list, memoria.search, session.clear, "+
					"help, exit.  Each wires to a real backend endpoint.")
				return nil
			},
		},
		{
			Name:        "exit",
			Description: "Exit the application",
			Shortcut:    "ctrl+c",
			Action: func(m *Model) tea.Cmd {
				m.quitting = true
				m.AppendEntry("system", "", "«Never fade away.»")
				return tea.Quit
			},
		},
	}
}

// filteredPalette returns the palette entries filtered by the input text.
func (m Model) filteredPalette() []CommandItem {
	if m.paletteFilter == "" {
		return m.paletteEntries
	}
	filter := strings.ToLower(m.paletteFilter)
	out := make([]CommandItem, 0, len(m.paletteEntries))
	for _, e := range m.paletteEntries {
		if strings.Contains(strings.ToLower(e.Name), filter) ||
			strings.Contains(strings.ToLower(e.Description), filter) {
			out = append(out, e)
		}
	}
	return out
}

// probeCerebrumStatus hits GET /cerebrum/status and returns a status msg.
func probeCerebrumStatus() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		baseURL := os.Getenv("FORGE_URL")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:9091"
		}
		resp, err := http.Get(baseURL + "/cerebrum/status")
		if err != nil {
			return CerebrumStatusMsg{Err: err}
		}
		defer resp.Body.Close()
		var data struct {
			Indexing bool   `json:"indexing"`
			Jobs     int    `json:"jobs"`
			Points   int    `json:"points"`
			Queue    string `json:"queue"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&data)
		return CerebrumStatusMsg{Data: data, Err: nil}
	})
}

// runObsidianSearch searches the indexed obsidian vault via /search/semantic.
func runObsidianSearch(m *Model) tea.Cmd {
	q := strings.TrimSpace(m.paletteInput.Value())
	if q == "" {
		m.AppendEntry("error", "", "obsidian.search: type a query first")
		return nil
	}
	return tea.Cmd(func() tea.Msg {
		baseURL := os.Getenv("FORGE_URL")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:9091"
		}
		u, _ := url.Parse(baseURL + "/search/semantic")
		q := u.Query()
		q.Set("q", m.paletteInput.Value())
		q.Set("limit", "5")
		u.RawQuery = q.Encode()
		resp, err := http.Get(u.String())
		if err != nil {
			return SearchResultsMsg{Err: err}
		}
		defer resp.Body.Close()
		var data struct {
			Query   string `json:"query"`
			Results []struct {
				Score     float64 `json:"score"`
				FilePath  string   `json:"file_path"`
				Title     string   `json:"title"`
				Text      string   `json:"text"`
			} `json:"results"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&data)
		return SearchResultsMsg{Query: m.paletteInput.Value(), Results: data.Results, Err: nil}
	})
}

// loadGoals hits GET /goals/ to list active goals.
func loadGoals(m *Model) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		baseURL := os.Getenv("FORGE_URL")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:9091"
		}
		resp, err := http.Get(baseURL + "/goals/")
		if err != nil {
			return GoalsMsg{Err: err}
		}
		defer resp.Body.Close()
		var data []map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&data)
		return GoalsMsg{Goals: data, Err: nil}
	})
}

// runMemoriaSearch hits POST /memoria/search.
func runMemoriaSearch(m *Model) tea.Cmd {
	q := strings.TrimSpace(m.paletteInput.Value())
	if q == "" {
		m.AppendEntry("error", "", "memoria.search: type a query first")
		return nil
	}
	return tea.Cmd(func() tea.Msg {
		baseURL := os.Getenv("FORGE_URL")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:9091"
		}
		body := strings.NewReader(fmt.Sprintf(`{"query":%q,"limit":5}`, q))
		resp, err := http.Post(baseURL+"/memoria/search", "application/json", body)
		if err != nil {
			return MemoriaMsg{Err: err}
		}
		defer resp.Body.Close()
		var data struct {
			Episodes []struct {
				Query     string  `json:"query"`
				Response  string  `json:"response"`
				Score     float64 `json:"score"`
				Timestamp string  `json:"timestamp"`
			} `json:"episodes"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&data)
		return MemoriaMsg{Episodes: data.Episodes, Err: nil}
	})
}

// CerebrumStatusMsg is delivered when /cerebrum/status responds.
type CerebrumStatusMsg struct {
	Data struct {
		Indexing bool   `json:"indexing"`
		Jobs     int    `json:"jobs"`
		Points   int    `json:"points"`
		Queue    string `json:"queue"`
	}
	Err error
}

// SearchResultsMsg is delivered when /search/semantic responds.
type SearchResultsMsg struct {
	Query   string
	Results []struct {
		Score     float64 `json:"score"`
		FilePath  string   `json:"file_path"`
		Title     string   `json:"title"`
		Text      string   `json:"text"`
	}
	Err error
}

// GoalsMsg is delivered when /goals/ responds.
type GoalsMsg struct {
	Goals []map[string]any
	Err  error
}

// MemoriaMsg is delivered when /memoria/search responds.
type MemoriaMsg struct {
	Episodes []struct {
		Query     string  `json:"query"`
		Response  string  `json:"response"`
		Score     float64 `json:"score"`
		Timestamp string  `json:"timestamp"`
	}
	Err error
}

// StatusLine returns the bottom status bar text.
func (m Model) StatusLine() string {
	ver := "?"
	if m.version.Version != "" {
		ver = m.version.Version
	}
	model := "?"
	if m.status.Model != "" {
		model = m.status.Model
	}
	agents := 0
	if m.status.Agents > 0 {
		agents = m.status.Agents
	}
	sess := m.sessionID
	if sess == "" {
		sess = "default"
	}
	pending := ""
	if m.pending {
		pending = theme.WarningStyle.Render(" [thinking]")
	}
	return fmt.Sprintf(
		"⚒ %s | %s | %d agents | session: %s | %s%s",
		ver,
		model,
		agents,
		sess,
		theme.DimStyle.Render(m.apiURL),
		pending,
	)
}

// tickCmd pings status every 5s.
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// probeVersion fetches /api/version.
func probeVersion(c *Client) tea.Cmd {
	return func() tea.Msg {
		v, err := c.Version()
		return VersionMsg{Version: v, Err: err}
	}
}

// probeStatus fetches /api/status.
func probeStatus(c *Client) tea.Cmd {
	return func() tea.Msg {
		s, err := c.Status()
		return StatusMsg{Status: s, Err: err}
	}
}

// fetchAgents returns the agent roster.
func fetchAgents(c *Client) tea.Cmd {
	return func() tea.Msg {
		a, err := c.Agents()
		return AgentsMsg{Agents: a, Err: err}
	}
}

// fetchDiff returns the diff text.
func fetchDiff(c *Client) tea.Cmd {
	return func() tea.Msg {
		s, err := c.Diff()
		return CommandOutputMsg{Title: "diff", Body: s, Err: err}
	}
}

// fetchLog returns the recent git log.
func fetchLog(c *Client) tea.Cmd {
	return func() tea.Msg {
		s, err := c.Log()
		return CommandOutputMsg{Title: "log", Body: s, Err: err}
	}
}

// fetchRepoMap returns the repo map.
func fetchRepoMap(c *Client) tea.Cmd {
	return func() tea.Msg {
		s, err := c.RepoMap()
		return CommandOutputMsg{Title: "repo-map", Body: s, Err: err}
	}
}

// fetchSessions lists sessions.
func fetchSessions(c *Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := c.FetchSessions()
		return SessionsLoadedMsg{Sessions: sessions, Err: err}
	}
}

// createSession starts a new session.
func createSession(c *Client) tea.Cmd {
	return func() tea.Msg {
		r, err := c.NewSession()
		return SessionCreatedMsg{Result: r, Err: err}
	}
}

// loadInitialHistory fetches the session list, picks the current (or last)
// session, and returns its messages so the TUI resumes where it left off.
func loadInitialHistory(c *Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := c.FetchSessions()
		if err != nil {
			return HistoryLoadedMsg{Err: err}
		}
		if len(sessions) == 0 {
			return HistoryLoadedMsg{Entries: []ChatEntry{}}
		}

		currentID := ""
		for _, s := range sessions {
			if s.Current {
				currentID = s.ID
				break
			}
		}
		if currentID == "" {
			currentID = sessions[len(sessions)-1].ID
		}

		history, err := c.FetchHistory(currentID)
		if err != nil {
			return HistoryLoadedMsg{SessionID: currentID, Err: err}
		}

		entries := make([]ChatEntry, 0, len(history))
		for _, h := range history {
			role := h.Role
			if role == "model" {
				role = "assistant"
			}
			entries = append(entries, ChatEntry{
				Role:    role,
				Agent:   h.Agent,
				Content: h.Content,
				Time:    h.Time,
			})
		}
		return HistoryLoadedMsg{Entries: entries, SessionID: currentID}
	}
}

// sendChat dispatches a chat message to the server (blocking).
func sendChat(c *Client, req ChatRequest) tea.Cmd {
	m := *c
	return func() tea.Msg {
		r, err := m.Chat(req)
		return ChatResultMsg{Resp: r, Err: err}
	}
}

// startStreamChat opens an SSE stream and emits ChatStreamMsg events.
// It is intentionally minimal: returns a single Cmd that reads the first
// event and yields a ChatStreamMsg. The Update loop re-invokes itself via
// readNextStreamEvent to drain the rest of the channel.
func startStreamChat(c *Client, req ChatRequest, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		events, errc := c.StreamChat(ctx, req)
		ev, ok := <-events
		if !ok {
			// Channel closed before any event — surface errc if available
			select {
			case e := <-errc:
				if e != nil {
					return ChatStreamMsg{Kind: "error", Err: e}
				}
			default:
			}
			return ChatStreamMsg{Kind: "done"}
		}
		return streamEventToMsg(ev)
	}
}

// streamEventToMsg converts a raw SSE event into a ChatStreamMsg.
func streamEventToMsg(ev StreamEvent) ChatStreamMsg {
	msg := ChatStreamMsg{Kind: ev.Event, Raw: string(ev.Data)}
	switch ev.Event {
	case "token":
		// token data is a JSON string like "word "
		var tok string
		if err := json.Unmarshal(ev.Data, &tok); err == nil {
			msg.Token = tok
		} else {
			msg.Token = string(ev.Data)
		}
	case "status", "done":
		// object with agent + status fields
		var obj struct {
			Agent  string `json:"agent"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(ev.Data, &obj); err == nil {
			msg.Agent = obj.Agent
		}
	case "tool_call":
		var obj struct {
			Tool string `json:"tool"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal(ev.Data, &obj); err == nil {
			msg.Token = obj.Tool + " " + obj.Path
		}
	case "tool_result":
		var obj struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(ev.Data, &obj); err == nil {
			msg.Token = obj.Content
		}
	case "error":
		var obj struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(ev.Data, &obj); err == nil {
			msg.Err = fmt.Errorf("%s", obj.Error)
		} else {
			msg.Err = fmt.Errorf("%s", string(ev.Data))
		}
	}
	return msg
}

// continueStream reads the next SSE event from an existing stream.
// The stream is held open across Bubble Tea ticks via a per-stream channel
// stored on the model. See internal/tui/update.go for the dispatch loop.
func continueStream(c *Client, req ChatRequest, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		events, errc := c.StreamChat(ctx, req)
		ev, ok := <-events
		if !ok {
			select {
			case e := <-errc:
				if e != nil {
					return ChatStreamMsg{Kind: "error", Err: e}
				}
			default:
			}
			return ChatStreamMsg{Kind: "done"}
		}
		return streamEventToMsg(ev)
	}
}

// formatSessions renders the session list as a friendly table.
func formatSessions(sessions []SessionInfo) string {
	if len(sessions) == 0 {
		return theme.DimStyle.Render("(no sessions)")
	}
	var b strings.Builder
	for i, s := range sessions {
		marker := "  "
		if s.Current {
			marker = theme.SuccessStyle.Render("> ")
		}
		fmt.Fprintf(&b, "%s[%d] id=%s msgs=%d updated=%s\n", marker, i, s.ID, s.Messages, s.Updated.Format("01-02 15:04"))
	}
	return b.String()
}

// renderWelcome is the initial chat viewport content.
func renderWelcome() string {
	var b strings.Builder
	b.WriteString(theme.BannerStyle.Render(
		"\n" + theme.GlyphBuild + " DARK FORGE v1.0\nDark Mechanicus Coding Terminal\n«Per Ignis, per Ferrum, per Codicem.»\n",
	))
	b.WriteString("\n")
	b.WriteString(theme.DimStyle.Render("Connected to Forge server. Type /help for commands, @agent to delegate."))
	b.WriteString("\n\n")
	return b.String()
}

// renderEntries formats the chat log for the viewport.
func renderEntries(entries []ChatEntry) string {
	if len(entries) == 0 {
		return renderWelcome()
	}
	var b strings.Builder
	for _, e := range entries {
		switch e.Role {
		case "user":
			fmt.Fprintf(&b, "%s %s\n",
				theme.UserStyle.Render("❯ heretic"),
				e.Content,
			)
		case "assistant":
			label := theme.AgentStyle.Render(theme.AgentGlyph(e.Agent))
			fmt.Fprintf(&b, "\n%s\n", label)
			for _, line := range strings.Split(e.Content, "\n") {
				fmt.Fprintf(&b, "  %s\n", theme.AssistantStyle.Render(line))
			}
		case "tool":
			fmt.Fprintf(&b, "  %s\n",
				theme.ToolCallStyle.Render("[tool] "+e.Content),
			)
		case "system":
			fmt.Fprintf(&b, "%s\n",
				theme.DimStyle.Render("• "+e.Content),
			)
		case "error":
			fmt.Fprintf(&b, "%s\n",
				theme.ErrorStyle.Render("✗ "+e.Content),
			)
		}
	}
	return b.String()
}
