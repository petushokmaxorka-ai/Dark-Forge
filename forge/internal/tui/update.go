// Package tui — Bubble Tea update (event handling).
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/model"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
)

// Update handles incoming events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := 3
		footerH := 1
		inputH := 3
		vpHeight := msg.Height - headerH - footerH - inputH
		if vpHeight < 5 {
			vpHeight = 5
		}
		vpWidth := msg.Width - 2
		if vpWidth < 20 {
			vpWidth = 20
		}
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		// Batch edit mode intercepts keys
		if m.batchMode && m.batch != nil {
			switch msg.String() {
			case "y":
				m.batchMode = false
				m.AppendEntry("system", "", "applying batch edit...")
				return m, func() tea.Msg {
					// Re-run with dry_run=false using same edits
					result, err := m.client.ApplyBatch(m.batch.Edits, false)
					return BatchEditMsg{Result: result, Err: err}
				}
			case "n", "r":
				m.batch = nil
				m.batchMode = false
				m.AppendEntry("system", "", "batch edit rejected")
				return m, nil
			}
		}

		// Plan mode intercepts keys
		if m.planMode && m.plan != nil {
			switch msg.String() {
			case "a":
				m.planMode = false
				m.planExec = model.NewPlanExecutor()
				m.planExec.ApproveAll()
				m.AppendEntry("system", "", "plan approved (all steps)")
				return m, nil
			case "s":
				m.planMode = false
				m.planExec = model.NewPlanExecutor()
				m.planExec.ApproveStep()
				m.AppendEntry("system", "", "plan approved (step by step)")
				return m, nil
			case "r":
				m.plan = nil
				m.planMode = false
				m.planExec = nil
				m.AppendEntry("system", "", "plan rejected")
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			m.AppendEntry("system", "", "«Never fade away.»")
			return m, tea.Quit

		case tea.KeyEnter:
			// Submit shortcuts: OpenCode uses Enter=newline,
			// Shift/Ctrl/Alt+Enter=submit. Intercept via msg.String()
			// before the textarea sees the key, so the bubble doesn't
			// insert a newline.
			switch msg.String() {
			case "shift+enter", "ctrl+enter", "alt+enter":
				if !m.paletteMode {
					newM, cmd := m.submitInput()
					return newM, cmd
				}
			}
			// Command palette: enter executes the selected command
			if m.paletteMode {
				entries := m.filteredPalette()
				if len(entries) == 0 {
					m.paletteMode = false
					m.paletteFilter = ""
					m.paletteInput.SetValue("")
					return m, nil
				}
				action := entries[0].Action
				m.paletteMode = false
				m.paletteFilter = ""
				m.paletteInput.SetValue("")
				if action != nil {
					return m, action(&m)
				}
				return m, nil
			}
			// In textarea: bare Enter inserts a newline (OpenCode
			// convention: Enter=newline, Shift+Enter=submit).
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd

		case tea.KeyCtrlL:
			// Ctrl+L clears the chat (unless palette is open, then types 'l')
			if m.paletteMode {
				var cmd tea.Cmd
				m.paletteInput, cmd = m.paletteInput.Update(msg)
				return m, cmd
			}
			m.entries = []ChatEntry{}
			m.refreshViewport()
			return m, nil

		case tea.KeyCtrlS:
			m.pending = true
			m.AppendEntry("system", "", "creating new session...")
			cmds = append(cmds, createSession(m.client))

		case tea.KeyCtrlP:
			// Ctrl+P opens/closes the command palette
			m.paletteMode = !m.paletteMode
			if m.paletteMode {
				m.paletteInput.Focus()
				m.AppendEntry("system", "", "command palette: type to filter, Enter to run, Esc to close")
			} else {
				m.paletteInput.Blur()
				m.paletteFilter = ""
				m.paletteInput.SetValue("")
			}
			return m, nil

		case tea.KeyTab:
			// Tab cycles agents (unless in palette, then autocomplete)
			if m.paletteMode {
				return m, nil // skip — palette uses input
			}
			m.cycleAgent()
			m.AppendEntry("system", "", "→ agent: "+m.activeAgent())
			return m, nil

		default:
			// If palette is open, route the keystroke to the palette input
			if m.paletteMode {
				var cmd tea.Cmd
				m.paletteInput, cmd = m.paletteInput.Update(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}

	case TickMsg:
		cmds = append(cmds, probeStatus(m.client), tickCmd())

	case VersionMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.AppendEntry("error", "", "version probe failed: "+msg.Err.Error())
		} else {
			m.version = msg.Version
			if msg.Version.Version != "" {
				m.AppendEntry("system", "", fmt.Sprintf("connected to %s v%s", msg.Version.Name, msg.Version.Version))
			}
		}

	case StatusMsg:
		if msg.Err == nil {
			m.status = msg.Status
			if msg.Status.Session != "" {
				m.sessionID = msg.Status.Session
			}
		}

	case HistoryLoadedMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", "history load failed: "+msg.Err.Error())
		} else {
			if msg.SessionID != "" {
				m.sessionID = msg.SessionID
			}
			if len(msg.Entries) > 0 {
				m.entries = append(m.entries, msg.Entries...)
				m.refreshViewport()
				m.AppendEntry("system", "", fmt.Sprintf("loaded %d messages from session %s", len(msg.Entries), m.sessionID))
			}
		}

	case SessionsLoadedMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", "sessions list failed: "+msg.Err.Error())
		} else {
			m.AppendEntry("tool", "", "──── /sessions ────")
			for _, line := range strings.Split(formatSessions(msg.Sessions), "\n") {
				m.AppendEntry("tool", "", line)
			}
		}

	case CerebrumStatusMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", "cerebrum.status: "+msg.Err.Error())
		} else {
			m.AppendEntry("tool", "", fmt.Sprintf(
				"cerebrum: indexing=%v  jobs=%d  points=%d  queue=%s",
				msg.Data.Indexing, msg.Data.Jobs, msg.Data.Points, msg.Data.Queue))
		}

	case SearchResultsMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", "obsidian.search: "+msg.Err.Error())
		} else {
			m.AppendEntry("tool", "", "──── /search/semantic?q="+msg.Query+" ────")
			for _, r := range msg.Results {
				text := r.Text
				if len(text) > 80 {
					text = text[:77] + "..."
				}
				m.AppendEntry("tool", "", fmt.Sprintf(
					"  %.4f  %s  ·  %s",
					r.Score, r.FilePath, text))
			}
			if len(msg.Results) == 0 {
				m.AppendEntry("tool", "", "  (no matches)")
			}
		}

	case GoalsMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", "goals.list: "+msg.Err.Error())
		} else {
			m.AppendEntry("tool", "", "──── /goals/ ────")
			if len(msg.Goals) == 0 {
				m.AppendEntry("tool", "", "  (no goals)")
			}
			for _, g := range msg.Goals {
				m.AppendEntry("tool", "", fmt.Sprintf("  %v", g))
			}
		}

	case MemoriaMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", "memoria.search: "+msg.Err.Error())
		} else {
			m.AppendEntry("tool", "", "──── /memoria/search ────")
			if len(msg.Episodes) == 0 {
				m.AppendEntry("tool", "", "  (no matches)")
			}
			for _, ep := range msg.Episodes {
				q := ep.Query
				if len(q) > 60 {
					q = q[:57] + "..."
				}
				m.AppendEntry("tool", "", fmt.Sprintf("  %.4f  %s", ep.Score, q))
			}
		}

	case ChatResultMsg:
		m.pending = false
		if msg.Err != nil {
			m.AppendEntry("error", "", msg.Err.Error())
		} else if msg.Resp.Error != "" {
			m.AppendEntry("error", "", msg.Resp.Error)
		} else {
			agent := msg.Resp.Agent
			if agent == "" {
				agent = "magos"
			}
			body := msg.Resp.Response
			if body == "" {
				body = "НЕИЗВЕСТНО."
			}
			m.AppendEntry("assistant", agent, body)
			if msg.Resp.Applied != "" {
				m.AppendEntry("tool", "", "applied: "+msg.Resp.Applied)
			}
			if msg.Resp.Committed != "" {
				m.AppendEntry("tool", "", "committed: "+msg.Resp.Committed)
			}
		}

	case ChatStreamMsg:
		switch msg.Kind {
		case "status":
			// thinking/start indicator — append a tool line if agent changed
			if msg.Agent != "" {
				m.streamAgent = msg.Agent
			}
			m.pending = true
			return m, nil

		case "token":
			m.pending = false
			m.streaming = true
			m.streamBuf.WriteString(msg.Token)
			// Live-update the last assistant entry, or create one.
			m.upsertStreamingAssistant(m.streamAgent, m.streamBuf.String())
			return m, continueStream(m.client, m.streamReq, m.streamCtx)

		case "tool_call", "tool_result":
			m.pending = false
			m.streaming = true
			m.AppendEntry("tool", "", msg.Token)
			return m, continueStream(m.client, m.streamReq, m.streamCtx)

		case "done":
			m.pending = false
			m.streaming = false
			if m.streamCancel != nil {
				m.streamCancel()
				m.streamCancel = nil
			}
			m.streamBuf.Reset()
			// Flush any residual content as final assistant entry
			return m, nil

		case "error":
			m.pending = false
			m.streaming = false
			if m.streamCancel != nil {
				m.streamCancel()
				m.streamCancel = nil
			}
			if msg.Err != nil {
				m.AppendEntry("error", "", msg.Err.Error())
			} else {
				m.AppendEntry("error", "", "stream error")
			}
			m.streamBuf.Reset()
			return m, nil
		}
		return m, nil

	case AgentsMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", msg.Err.Error())
		} else {
			m.AppendEntry("tool", "", formatAgents(msg.Agents))
		}

	case CommandOutputMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", msg.Err.Error())
		} else {
			header := fmt.Sprintf("──── /%s ────", msg.Title)
			m.AppendEntry("tool", "", header)
			for _, line := range strings.Split(msg.Body, "\n") {
				m.AppendEntry("tool", "", line)
			}
		}

	case SessionCreatedMsg:
		m.pending = false
		if msg.Err != nil {
			m.AppendEntry("error", "", msg.Err.Error())
		} else {
			id := msg.Result["id"]
			if id == "" {
				id = msg.Result["session_id"]
			}
			m.sessionID = id
			m.AppendEntry("system", "", "session started: "+id)
		}

	case PlanReceivedMsg:
		m.pending = false
		if msg.Err != nil {
			m.AppendEntry("error", "", "plan error: "+msg.Err.Error())
		} else if err := msg.Plan.Validate(); err != nil {
			m.AppendEntry("error", "", "invalid plan: "+err.Error())
		} else {
			m.plan = &msg.Plan
			m.planMode = true
			m.AppendEntry("system", "", "plan received — review and approve")
		}

	case PlanApprovedMsg:
		if m.planExec != nil && m.plan != nil && m.planExec.HasNext(m.plan) {
			step := m.planExec.CurrentStepData(m.plan)
			if step != nil {
				m.AppendEntry("tool", "", fmt.Sprintf("▶ [%s] %s %s", step.Risk, step.Action, step.Target))
			}
			m.planExec.Advance()
			m.AppendEntry("tool", "", "✓ step done")
		}

	case PlanRejectedMsg:
		m.plan = nil
		m.planMode = false
		m.planExec = nil
		m.AppendEntry("system", "", "plan rejected")

	case PlanStepDoneMsg:
		if msg.Err != nil {
			m.AppendEntry("error", "", fmt.Sprintf("step %d failed: %s", msg.Index+1, msg.Err.Error()))
		} else {
			m.AppendEntry("tool", "", fmt.Sprintf("✓ step %d: %s", msg.Index+1, msg.Result))
		}

		case BatchEditMsg:
		m.pending = false
		if msg.Err != nil {
			m.AppendEntry("error", "", "batch edit: "+msg.Err.Error())
		} else {
			m.batch = &msg.Batch
			m.batchResult = &msg.Result
			m.batchMode = true
			m.AppendEntry("system", "", renderBatchPreview(msg.Result))
		}

	case DelegationMsg:
		// Show delegation in chat: ⚒ Magos → ✦ Kimi: reason
		from := msg.From
		if from == "" {
			from = "magos"
		}
		m.AppendEntry("system", "", fmt.Sprintf("⚒ %s → %s: %s", from, msg.To, msg.Reason))

	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// submitInput parses and dispatches the current input.
// Returns the updated model so streaming state propagates back to Update.
func (m Model) submitInput() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.SetValue("")
	m.pending = true

	// Commands
	if strings.HasPrefix(text, "/") {
		m.pending = false
		return m, handleCommand(m, text)
	}

	// Chat (streaming SSE)
	m.AppendEntry("user", "heretic", text)
	m.streamBuf.Reset()
	m.streaming = true
	m.streamAgent = "magos"
	m.streamReq = ChatRequest{Message: text}

	ctx, cancel := context.WithCancel(context.Background())
	m.streamCtx = ctx
	m.streamCancel = cancel
	return m, startStreamChat(m.client, m.streamReq, ctx)
}

// upsertStreamingAssistant updates the last assistant entry in-place if it
// exists, otherwise appends a new one. Used for live SSE token updates.
func (m *Model) upsertStreamingAssistant(agent, content string) {
	if agent == "" {
		agent = "magos"
	}
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Role == "assistant" {
			m.entries[i].Agent = agent
			m.entries[i].Content = content
			m.entries[i].Time = time.Now()
			m.refreshViewport()
			return
		}
	}
	m.AppendEntry("assistant", agent, content)
}

// formatAgents renders the agent roster.
func formatAgents(agents []AgentInfo) string {
	if len(agents) == 0 {
		return theme.DimStyle.Render("(no agents)")
	}
	var b strings.Builder
	for _, a := range agents {
		active := theme.DimStyle.Render("○")
		if a.Active {
			active = theme.SuccessStyle.Render("✓")
		}
		fmt.Fprintf(&b, "  %s %-12s %-10s %-20s ctx=%s\n",
			active,
			theme.AgentGlyph(a.Display),
			theme.DimStyle.Render(a.Type),
			a.Model,
			a.Context,
		)
	}
	return b.String()
}
