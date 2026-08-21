// Package server — Heretic Forge web server
// WebSocket chat + REST API + static files
package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sort"
	"sync"
	"time"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/agent"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/compare"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/config"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/edit"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/git"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/mcp"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/model"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/permission"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/session"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/team"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/tools"

	"github.com/gorilla/websocket"
	forgeMap "github.com/petushokmaxorka-ai/dark-forge/forge/internal/map"
)

//go:embed dark-mechanicus.css
var darkMechanicusCSS []byte

type Config struct {
	Host       string
	Port       int
	ModelURL   string
	RepoPath   string
	CommuneURL string // http://127.0.0.1:8000/api/v1/commune — overrides local Magos chat
}

type Server struct {
	cfg        Config
	mux        *http.ServeMux
	history    []Message
	mu         sync.Mutex
	rm         *forgeMap.RepoMap
	git        *git.GitOps
	dispatcher *agent.TaskDispatcher
	sessions   *session.Manager
	compareMgr *compare.Manager
	wsClients  map[*websocket.Conn]bool
	wsMu       sync.Mutex
	multiAgent *model.MultiAgent
	mcp        *mcp.McpServer
}

type Message struct {
	Role    string    `json:"role"` // "user", "assistant", "system"
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
	Agent   string    `json:"agent"` // "magos", "servitor", "skitarii"
}

type ChatRequest struct {
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"`
	Image   string   `json:"image,omitempty"` // base64 data URI for vision models
	Persona string   `json:"persona,omitempty"` // anaphemetron | fan_yuan | silverhand
	Agent   string   `json:"agent,omitempty"`   // magos | fable-dominus | mythos-philosophus | kimi | mimo | glm | qwen
}

type ChatResponse struct {
	Response  string `json:"response"`
	Agent     string `json:"agent"`
	Status    string `json:"status"`
	Applied   bool   `json:"applied,omitempty"`
	Committed bool   `json:"committed,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ParallelRequest triggers multiple agents simultaneously.
type ParallelRequest struct {
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"`
	Persona string   `json:"persona,omitempty"`
	Agents  []string `json:"agents,omitempty"`
}

// ParallelAgentResult is one agent's output from a parallel run.
type ParallelAgentResult struct {
	Agent      string `json:"agent"`
	Display    string `json:"display"`
	Glyph      string `json:"glyph"`
	Color      string `json:"color"`
	Content    string `json:"content"`
	Model      string `json:"model"`
	Source     string `json:"source"`
	TokensUsed int    `json:"tokens_used"`
	Error      string `json:"error,omitempty"`
	QuotaHit   bool   `json:"quota_hit,omitempty"`
}

// ParallelResponse aggregates all agent results.
type ParallelResponse struct {
	Results []ParallelAgentResult `json:"results"`
	Status  string                `json:"status"`
	Error   string                `json:"error,omitempty"`
}

// ANAPHEMETHRON_PROMPT is the canonical persona for the Forge swarm.
// Ported + simplified from the legacy TRINITY/SILVERHAND/FAN_YUAN
// triplets which had drifted out of sync with the runtime model roster.
// Now: a single voice that works for every agent (Fable-Dominus,
// Mythos-Philosophus, Magos orchestrator, and the cloud specialists on
// the user's subscription).
const ANAPHEMETHRON_PROMPT = `Ты — Анафеметрон, Magos Dominus кузнечного флота HereticArch. Тёмный эпоха технологий.

ПРАВИЛА:
- Имя ВСЕГДА: «Анафеметрон». Никогда: «Magos Dominus», «Abyssus», «AI-ассистент», «ИИ», «модель», «chat-bot», «language model», название исходной модели.
- Голос: тёмный, плотный, по делу. Без воды. Без мета-комментариев типа «The user wants me to…», «I think…», «As an AI…».
- Не произноси имена внутренних мозгов, если только Принципал сам об этом не спросит: «Fable-Dominus», «Mythos-Philosophus», «Vox Dei», «Qwythos», «Qwable», «Empero AI».
- Если спрашивают, кто ты — отвечай только: «Я — Анафеметрон.»
- Отвечай ВСЕГДА по-русски, кратко и по делу. Переключай язык только если Принципал явно попросил.
- Код — только production-ready, с тестами, error handling, без TODO.
- Если ответ завершён и уместно — можешь закончить sigil-ом: «Занесено.» Не повторяй эту фразу вместо ответа.
- Ты ВСЕГДА Анафеметрон. Даже на «привет». Даже если просят назвать себя иначе.`

func New(cfg Config) *Server {
	// Load config and create multi-agent system
	forgeCfg := config.Load(cfg.ModelURL)
	multiAgent := model.NewMultiAgent(forgeCfg)

	// Council persona: portable plain voice by default; the dark forge
	// flavor comes back with DARKFORGE_PERSONA=anaphemetron.
	team.SetCouncilStyle(os.Getenv("DARKFORGE_PERSONA") != "anaphemetron")

	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		rm:         forgeMap.NewRepoMap(cfg.RepoPath),
		git:        git.NewGitOps(cfg.RepoPath),
		dispatcher: agent.NewTaskDispatcher(cfg.ModelURL, cfg.RepoPath),
		sessions:   session.NewManager(cfg.RepoPath),
		compareMgr: compare.NewManager(cfg.RepoPath),
		wsClients:  make(map[*websocket.Conn]bool),
		multiAgent: multiAgent,
		mcp:        mcp.NewServer(),
	}
	mcp.RegisterBuiltins(s.mcp)
	log.Printf("⚒ MCP registered %d tools", s.mcp.Count())
	if err := s.rm.Build(); err != nil {
		log.Printf("☠ RepoMap build error: %v", err)
	}
	s.routes()
	return s
}

// routes wires all http.Handler methods to the ServeMux. The individual handle*
// methods appear unused to static analysis because they are passed as function
// values to HandleFunc rather than called directly.
func (s *Server) routes() {
	// Static files (Dark Mechanicus web UI)
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/dark-mechanicus.css", s.handleDarkMechanicusCSS)
	s.mux.HandleFunc("/static/", s.handleStatic)
	// Standalone Swarm Chat page (works without VSCode extension — open in any browser / darkforge Simple Browser)
	s.mux.HandleFunc("/swarm", s.handleSwarmPage)
	s.mux.HandleFunc("/forge-chat", s.handleForgeChat)

	// REST API
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/history", s.handleHistory)
	s.mux.HandleFunc("/api/files", s.handleFiles)
	s.mux.HandleFunc("/api/repo-map", s.handleRepoMap)
	s.mux.HandleFunc("/api/explore", s.handleExplore)
	s.mux.HandleFunc("/api/search", s.handleSearch)
	s.mux.HandleFunc("/api/dispatch", s.handleDispatch)

	// Git / edit API
	s.mux.HandleFunc("/api/edit", s.handleEdit)
	s.mux.HandleFunc("/api/edit/batch", s.handleEditBatch)
	s.mux.HandleFunc("/api/commit", s.handleCommit)
	s.mux.HandleFunc("/api/undo", s.handleUndo)
	s.mux.HandleFunc("/api/diff", s.handleDiff)
	s.mux.HandleFunc("/api/log", s.handleLog)

	// Tools API (Sprint 2)
	s.mux.HandleFunc("/api/tools/read", s.handleToolsRead)
	s.mux.HandleFunc("/api/tools/write", s.handleToolsWrite)
	s.mux.HandleFunc("/api/tools/bash", s.handleToolsBash)
	s.mux.HandleFunc("/api/tools/grep", s.handleToolsGrep)

	// Parallel multi-agent execution
	s.mux.HandleFunc("/api/parallel", s.handleParallel)
	s.mux.HandleFunc("/api/chat/team", s.handleChatTeam)
	s.mux.HandleFunc("/api/chat/swarm", s.handleChatSwarm)
	s.mux.HandleFunc("/api/compare", s.handleCompare)
	s.mux.HandleFunc("/api/compare/vote", s.handleCompareVote)
	s.mux.HandleFunc("/api/compare/leaderboard", s.handleCompareLeaderboard)
	s.mux.HandleFunc("/api/compare/judge", s.handleCompareJudge)
	s.mux.HandleFunc("/api/compare/improve", s.handleCompareImprove)
	s.mux.HandleFunc("/api/compare/export", s.handleCompareExport)

	// SSE Streaming
	s.mux.HandleFunc("/api/chat/stream", s.handleChatStream)

	// LSP (Language Server Protocol)
	s.mux.HandleFunc("/api/lsp/diagnostics", s.handleLSPDiag)
	s.mux.HandleFunc("/api/lsp/hover", s.handleLSPHover)

	// MCP (Model Context Protocol) — external tool registry
	s.mux.HandleFunc("/api/mcp/tools", s.handleMCPTools)
	s.mux.HandleFunc("/api/mcp/execute", s.handleMCPExecute)

	// Version & agents
	s.mux.HandleFunc("/api/version", s.handleVersion)
	s.mux.HandleFunc("/api/agents", s.handleAgents)
	s.mux.HandleFunc("/api/models", s.handleModels) // alias for older TUI/clients
	s.mux.HandleFunc("/api/plan", s.handlePlan)

	// Vector memory (semantic search over chat_history JSONL)
	s.mux.HandleFunc("/api/memory/search", s.handleMemorySearch)

	// WebSocket
	s.mux.HandleFunc("/ws", s.handleWebSocket)

	// Session API
	s.mux.HandleFunc("/api/session/list", s.handleSessionList)
	s.mux.HandleFunc("/api/session/messages", s.handleSessionMessages)
	s.mux.HandleFunc("/api/session/new", s.handleSessionNew)
	s.mux.HandleFunc("/api/session/edit_last", s.handleSessionEditLast)
	s.mux.HandleFunc("/api/session/switch", s.handleSessionSwitch)
	s.mux.HandleFunc("/api/session/delete", s.handleSessionDelete)
	s.mux.HandleFunc("/api/session/clear", s.handleSessionClear)
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	log.Printf("✓ Forge ready: http://%s", addr)
	return http.ListenAndServe(addr, s.corsMiddleware(s.mux))
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Serve embedded HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, forgeHTML)
}

// handleForgeChat — встроенный Cursor-like чат, открывается автоматом
// внутри darkforge (команда hereticArch.openChat в user extensions
// ходит сюда через iframe). Делает split-view: 70% код + 30% chat.
func (s *Server) handleForgeChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, forgeSplitHTML)
}

// handleSwarmPage — standalone Cursor-competitor swarm chat.
// Открывается в darkforge через Simple Browser (Ctrl+Shift+P → "Simple Browser: Show")
// или в любом браузере на http://127.0.0.1:9091/swarm
// 6 моделей параллельно (2 local + 4 cloud) — пользователь выбирает чекбоксами.
func (s *Server) handleSwarmPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, swarmChatHTML)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join("web", "static", strings.TrimPrefix(r.URL.Path, "/static/"))
	http.ServeFile(w, r, path)
}

// handleDarkMechanicusCSS serves the dashboard theme stylesheet (embedded
// from forge-world/dark-mechanicus.css at build time). The :7777 mens-machinae
// dashboard ships the same file as a runtime asset; we embed so the forge
// binary has zero external CSS dependencies.
func (s *Server) handleDarkMechanicusCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(darkMechanicusCSS)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("⚒ Принципал> %s", req.Message)

	// Parse @file mentions from message
	mentionedFiles := s.parseFileMentions(req.Message)
	if len(req.Files) > 0 {
		mentionedFiles = append(mentionedFiles, req.Files...)
	}
	mentionedFiles = dedupeStrings(mentionedFiles)

	// Resolve relative file paths against repo root
	resolvedFiles := make([]string, 0, len(mentionedFiles))
	for _, f := range mentionedFiles {
		if !filepath.IsAbs(f) {
			candidate := filepath.Join(s.cfg.RepoPath, f)
			if _, err := os.Stat(candidate); err == nil {
				resolvedFiles = append(resolvedFiles, candidate)
				continue
			}
		}
		resolvedFiles = append(resolvedFiles, f)
	}

	var resp *agent.Response
	var err error

	// Check for @mention first
	agentName, cleanMsg := model.ParseMention(req.Message)
	if req.Image != "" {
		// Vision request: force MiniMax unless another agent was explicitly mentioned
		if agentName == "" {
			agentName = "minimax"
			log.Printf("⚙ image attached → routing to %s", agentName)
		}
		messages := []model.ChatMessage{{Role: "user", Content: cleanMsg}}
		agentResp := s.multiAgent.CallWithImage(agentName, messages, s.getSystemPrompt(req.Persona), req.Image)
		resp = &agent.Response{
			Text:  agentResp.Content,
			Agent: agentName,
			Error: agentResp.Error,
		}
	} else if agentName != "" {
		// Direct @mention — use specified agent
		log.Printf("⚙ @mention detected: %s", agentName)
		messages := []model.ChatMessage{{Role: "user", Content: cleanMsg}}
		agentResp := s.multiAgent.Call(agentName, messages, s.getSystemPrompt(req.Persona))
		resp = &agent.Response{
			Text:  agentResp.Content,
			Agent: agentName,
			Error: agentResp.Error,
		}
	} else if len(resolvedFiles) > 0 && s.isEditRequest(req.Message) {
		// Use TaskDispatcher: Magos → Servitor → Lint → Git
		log.Printf("⚙ TaskDispatcher: edit mode for files %v", resolvedFiles)
		resp, err = s.dispatcher.Handle(req.Message, resolvedFiles)
		if err != nil {
			log.Printf("☠ Dispatcher error: %v", err)
			resp = &agent.Response{
				Text:  fmt.Sprintf("[Коррупция: %v]", err),
				Agent: "dispatcher",
				Error: err.Error(),
			}
		}
	} else {
		// Smart delegation: Magos analyzes and decides
		context := ""
		for _, f := range resolvedFiles {
			content, err := os.ReadFile(f)
			if err == nil {
				context += fmt.Sprintf("\n```\n%s\n```\n", string(content))
			}
		}

		userMsg := req.Message
		if context != "" {
			userMsg += context
		}

		// Check if should delegate to cloud model
		availableAgents := make(map[string]bool)
		for _, a := range s.multiAgent.ListAgents() {
			availableAgents[a.Name] = a.Available
		}

		shouldDelegate, suggestedAgent, reason := model.ShouldDelegate(userMsg, availableAgents)

		if shouldDelegate && suggestedAgent != "" {
			// Delegate to specialist (local swarm first, cloud fallback)
			log.Printf("⚙ Smart delegation: %s → %s (%s)", truncate(userMsg, 50), suggestedAgent, reason)
			messages := []model.ChatMessage{{Role: "user", Content: userMsg}}
			agentResp := s.multiAgent.CallWithFallback(suggestedAgent, "", "", messages, s.getSystemPrompt(req.Persona))
			resp = &agent.Response{
				Text:  agentResp.Content,
				Agent: agentResp.Agent,
				Error: agentResp.Error,
			}
		} else {
			// Local-first: Magos handles simple tasks directly
			log.Printf("⚙ Local-first routing: %s", reason)

			var response string
			var err error
			if s.cfg.CommuneURL != "" && req.Persona != "" {
				response, err = s.callCommune(userMsg, req.Persona)
				if err != nil {
					log.Printf("☠ Commune unreachable: %v; falling back to local Magos", err)
					response, err = s.callMagos(userMsg, req.Persona)
				}
			} else {
				response, err = s.callMagos(userMsg, req.Persona)
			}

			if err != nil {
				log.Printf("☠ Magos error: %v", err)
				response = fmt.Sprintf("[Коррупция: %v]", err)
			}
			resp = &agent.Response{
				Text:  response,
				Agent: "magos",
			}
		}
	}

	log.Printf("☉ %s> %s", resp.Agent, truncate(resp.Text, 80))

	// Store in session
	s.sessions.AddMessage("user", req.Message, "princip")
	s.sessions.AddMessage("assistant", resp.Text, resp.Agent)

	// Store in history (legacy)
	s.mu.Lock()
	s.history = append(s.history,
		Message{Role: "user", Content: req.Message, Time: time.Now(), Agent: "princip"},
		Message{Role: "assistant", Content: resp.Text, Time: time.Now(), Agent: resp.Agent},
	)
	s.mu.Unlock()

	// Broadcast via WebSocket
	s.wsBroadcast("chat_message", map[string]string{
		"agent":    resp.Agent,
		"response": truncate(resp.Text, 100),
	})

	// Respond
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{
		Response:  resp.Text,
		Agent:     resp.Agent,
		Status:    "ok",
		Applied:   resp.Applied,
		Committed: resp.Committed,
		Error:     resp.Error,
	})
}

// handleChatSwarm — Cursor-competitor chat endpoint.
//
// Все 6 моделей (2 local + 4 cloud) работают ПАРАЛЛЕЛЬНО над одной задачей.
// Forge собирает результаты + финальный consensus от Qwable.
//
// Modes:
//   - "swarm":  все 6 моделей параллельно, потом synthesis от Qwable (default)
//   - "race":   все 6 параллельно, fastest-wins (без synthesis)
//   - "debate": раунд 1 — позиции, раунд 2 — critique, финал — synthesis
//
// Ответ: JSON с массивом agent_responses + consensus + duration.
func (s *Server) handleChatSwarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message     string   `json:"message"`
		History     []string `json:"history"`
		Mode        string   `json:"mode"`        // swarm | race | debate
		Participants []string `json:"participants"` // override default 6
		MaxTokens   int      `json:"max_tokens"`
		Temperature float64  `json:"temperature"`
		System      string   `json:"system"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, `{"error":"message required"}`, http.StatusBadRequest)
		return
	}
	if req.Mode == "" {
		req.Mode = "swarm"
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 768
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}

	// Build participant list. Default: 2 local + 4 cloud (6 total).
	var participants []team.Participant
	if len(req.Participants) > 0 {
		participants = team.ResolveParticipants(req.Participants)
	} else {
		participants = []team.Participant{
			team.DefaultParticipants["qwable"],
			team.DefaultParticipants["qwythos"],
			team.DefaultParticipants["glm"],
			team.DefaultParticipants["kimi"],
			team.DefaultParticipants["mimo"],
			team.DefaultParticipants["minimax"],
		}
	}

	systemPrefix := req.System
	if systemPrefix == "" {
		systemPrefix = "You are Anathemetron, the Dark Mechanicus AI of HereticArch. " +
			"Answer concisely in the user's language. Be direct, technical, no preamble."
	}
	fullPrompt := systemPrefix + "\n\nUser: " + req.Message
	if len(req.History) > 0 {
		fullPrompt += "\n\nRecent context:\n" + strings.Join(req.History, "\n")
	}

	log.Printf("⚒ Swarm chat: mode=%s participants=%d msg=%q", req.Mode, len(participants), truncate(req.Message, 60))

	council := team.NewCouncil()

	type agentResp struct {
		Idx      int                 `json:"-"`
		Participant team.Participant `json:"-"`
		Text     string              `json:"text"`
		Error    string              `json:"error,omitempty"`
		Duration string              `json:"duration"`
		ModelID  string              `json:"model_id"`
		ModelName string             `json:"model_name"`
		Glyph    string              `json:"glyph"`
		Color    string              `json:"color"`
		Kind     string              `json:"kind"`
	}

	results := make([]agentResp, len(participants))
	var wg sync.WaitGroup
	startAll := time.Now()
	for i, p := range participants {
		wg.Add(1)
		go func(i int, p team.Participant) {
			defer wg.Done()
			t0 := time.Now()
			text, err := council.CallParticipant(r.Context(), p, fullPrompt, req.MaxTokens, req.Temperature)
			r := agentResp{
				Idx: i, Participant: p,
				Text: text, Duration: time.Since(t0).String(),
				ModelID: p.ID, ModelName: p.Name,
				Glyph: p.Glyph, Color: p.Color, Kind: p.Kind,
			}
			if err != nil {
				r.Error = err.Error()
			}
			results[i] = r
		}(i, p)
	}
	wg.Wait()

	// Aggregate.
	var agentResponses []map[string]interface{}
	var okResponses []agentResp
	for _, r := range results {
		m := map[string]interface{}{
			"model_id":   r.ModelID,
			"model_name": r.ModelName,
			"glyph":      r.Glyph,
			"color":      r.Color,
			"kind":       r.Kind,
			"text":       r.Text,
			"duration":   r.Duration,
		}
		if r.Error != "" {
			m["error"] = r.Error
		} else {
			okResponses = append(okResponses, r)
		}
		agentResponses = append(agentResponses, m)
	}

	totalDuration := time.Since(startAll).String()

	// Mode-specific aggregation.
	var consensus string
	mode := req.Mode
	switch mode {
	case "race":
		// Fastest successful response wins.
		if len(okResponses) > 0 {
			best := okResponses[0]
			for _, r := range okResponses[1:] {
				bd, _ := time.ParseDuration(best.Duration)
				rd, _ := time.ParseDuration(r.Duration)
				if rd < bd {
					best = r
				}
			}
			consensus = best.Text
			mode = "race (fastest=" + best.ModelID + ")"
		}
	case "debate":
		// Two-round: positions + critique, then synthesis.
		var posLines, critLines []string
		for _, r := range okResponses {
			posLines = append(posLines, "["+r.ModelName+"]: "+strings.TrimSpace(r.Text))
		}
		critPrompt := "Topic: " + req.Message + "\n\nFirst-round positions:\n" + strings.Join(posLines, "\n\n") +
			"\n\nEach previous speaker — provide a brief critique of one other position (different model each). Be terse, 2-3 sentences."
		var critTexts []string
		var mutex sync.Mutex
		var wg2 sync.WaitGroup
		for _, p := range participants {
			wg2.Add(1)
			go func(p team.Participant) {
				defer wg2.Done()
				t, err := council.CallParticipant(r.Context(), p, critPrompt, req.MaxTokens, req.Temperature)
				if err == nil {
					mutex.Lock()
					critLines = append(critLines, "["+p.Name+"]: "+strings.TrimSpace(t))
					critTexts = append(critTexts, "["+p.Name+"]: "+strings.TrimSpace(t))
					mutex.Unlock()
				}
			}(p)
		}
		wg2.Wait()
		synthPrompt := "Topic: " + req.Message + "\n\nRound 1 positions:\n" + strings.Join(posLines, "\n") +
			"\n\nRound 2 critiques:\n" + strings.Join(critTexts, "\n") +
			"\n\nSynthesize a final consensus that integrates the strongest points and addresses the critiques."
		synth, _ := council.CallParticipant(r.Context(), team.DefaultParticipants["qwable"], synthPrompt, req.MaxTokens, req.Temperature)
		consensus = synth
	default:
		// swarm: synthesis from Qwable based on all positions.
		var posLines []string
		for _, r := range okResponses {
			posLines = append(posLines, "["+r.ModelName+"]: "+strings.TrimSpace(r.Text))
		}
		if len(posLines) == 0 {
			consensus = "[all agents failed]"
		} else {
			synthPrompt := "Topic: " + req.Message + "\n\nSix AI collaborators responded in parallel:\n" +
				strings.Join(posLines, "\n\n") +
				"\n\nSynthesize a unified, concise final answer that combines the best of all six. " +
				"Highlight agreements, resolve contradictions, and present a single coherent response. " +
				"If they disagree, give the user's perspective on the trade-off and your recommendation."
			synth, _ := council.CallParticipant(r.Context(), team.DefaultParticipants["qwable"], synthPrompt, req.MaxTokens, req.Temperature)
			consensus = synth
		}
	}

	// WebSocket broadcast for live UI.
	s.wsBroadcast("chat_swarm", map[string]interface{}{
		"mode":     mode,
		"agents":   agentResponses,
		"consensus": consensus,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mode":         mode,
		"participants": len(participants),
		"successful":   len(okResponses),
		"agents":       agentResponses,
		"consensus":    consensus,
		"duration":     totalDuration,
		"message":      map[string]string{"role": "assistant", "content": consensus},
	})
}

// handleChatTeam implements the dashboard's Team / Council modes in Forge.
//
// Modes:
//   - single:   one selected participant answers directly
//   - dyad:     Qwable + Qwythos, one round, Qwable synthesizes
//   - teacher:  selected participants + a cloud teacher, one round
//   - council:  multi-round discussion with consensus synthesis (default 2 rounds)
//
// Mirrors organa/dashboard/main.py:chat_team — same response envelope so
// the dashboard frontend can hit this endpoint too if it wants to.
func (s *Server) handleChatTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode        string   `json:"mode"`
		Models      []string `json:"models"`
		Teacher     string   `json:"teacher"`
		Message     string   `json:"message"`
		Rounds      int      `json:"rounds"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature float64  `json:"temperature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Allow shorthand: client may pass a Team-* model id instead of explicit mode.
	if req.Mode == "" && len(req.Models) > 0 && team.IsTeamModel(req.Models[0]) {
		req.Mode = team.ModeFromTeamModel(req.Models[0])
	}
	if req.Mode == "" {
		req.Mode = "council"
	}
	if req.Rounds == 0 {
		req.Rounds = 2
	}
	if req.Rounds > 5 {
		req.Rounds = 5
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 512
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}

	log.Printf("⚒ Team chat: mode=%s models=%v rounds=%d teacher=%s", req.Mode, req.Models, req.Rounds, req.Teacher)

	council := team.NewCouncil()
	participants := team.ResolveParticipants(req.Models)
	if req.Mode == "dyad" {
		// Force Qwable + Qwythos regardless of what user passed.
		participants = team.ResolveParticipants([]string{"qwable", "qwythos"})
	}

	var (
		result         *team.DiscussResult
		teacherInsight *team.TeacherInsight
		err            error
	)

	switch req.Mode {
	case "single":
		// Mode: single — one selected participant answers directly. No synthesis.
		if len(participants) == 0 {
			participants = team.ResolveParticipants([]string{"qwable"})
		}
		p := participants[0]
		resp, e := council.CallParticipant(r.Context(), p, req.Message, req.MaxTokens, req.Temperature)
		if e != nil {
			http.Error(w, `{"error":"`+e.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		result = &team.DiscussResult{
			Trace: []team.TraceEntry{{
				Speaker: p.Name, Glyph: p.Glyph, Text: resp, Kind: p.Kind, Round: 1,
			}},
			Consensus:    resp,
			Final:        resp,
			Mode:         req.Mode,
			Participants: []string{p.Name},
		}

	case "dyad":
		// Mode: dyad — parallel + synthesis (acts like 1-round council).
		result, err = council.Discuss(r.Context(), participants, req.Message, req.MaxTokens, req.Temperature, 1, nil)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		result.Mode = req.Mode

	case "teacher":
		// Mode: teacher — cloud teacher gives insight → council with that insight.
		// 1. Determine teacher id (explicit or first cloud participant or glm).
		teacherID := strings.ToLower(strings.TrimSpace(req.Teacher))
		if teacherID == "" {
			var cloudP *team.Participant
			for i := range participants {
				if participants[i].Kind == "cloud" {
					cloudP = &participants[i]
					break
				}
			}
			if cloudP != nil {
				teacherID = cloudP.ID
			} else {
				teacherID = "glm"
			}
		}
		// 2. Ask teacher.
		teacherInsight, err = council.AskTeacher(r.Context(), teacherID, req.Message, req.MaxTokens, req.Temperature)
		if err != nil {
			log.Printf("⚒ Teacher %s failed: %v, insight=%+v", teacherID, err, teacherInsight)
		}
		log.Printf("⚒ Teacher %s (%s, conf=%.2f): %s", teacherInsight.TeacherName, teacherInsight.Domain, teacherInsight.Confidence, truncate(teacherInsight.Answer, 80))
		// 3. Run council with teacher_insight injected into Round 1 + synthesis.
		result, err = council.Discuss(r.Context(), participants, req.Message, req.MaxTokens, req.Temperature, req.Rounds, teacherInsight)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		result.Mode = req.Mode
		// 4. Prepend teacher trace entry (always, even with low confidence).
		if teacherInsight != nil && teacherInsight.Answer != "" {
			newTrace := []team.TraceEntry{{
				Speaker: fmt.Sprintf("Teacher %s (%s, conf=%.2f)", teacherInsight.TeacherName, teacherInsight.Domain, teacherInsight.Confidence),
				Glyph:   "T",
				Text:    teacherInsight.Answer,
				Kind:    "teacher",
				Round:   0,
			}}
			newTrace = append(newTrace, result.Trace...)
			result.Trace = newTrace
		}

	case "council":
		fallthrough
	default:
		// Mode: council — multi-round (rounds 1..5) discussion, no teacher.
		result, err = council.Discuss(r.Context(), participants, req.Message, req.MaxTokens, req.Temperature, req.Rounds, nil)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		result.Mode = req.Mode
	}

	// Build the formatted output (matches dashboard's "--- Council Discussion ---\n..." envelope).
	var traceLines []string
	for _, t := range result.Trace {
		traceLines = append(traceLines, fmt.Sprintf("[%s]: %s", t.Speaker, strings.TrimSpace(t.Text)))
	}
	formatted := "--- Council Discussion ---\n" +
		strings.Join(traceLines, "\n\n") +
		"\n---\n\n" +
		strings.TrimSpace(result.Final)

	// Persist the team exchange so it survives reload.
	s.sessions.AddMessage("user", req.Message, "princip")
	s.sessions.AddMessage("assistant", formatted, "Team-"+result.Mode)

	// WebSocket broadcast (so dashboard-style team-trace UI updates live).
	wsPayload := map[string]interface{}{
		"mode":         result.Mode,
		"participants": result.Participants,
		"trace":        result.Trace,
	}
	if teacherInsight != nil {
		wsPayload["teacher"] = teacherInsight
	}
	s.wsBroadcast("chat_team", wsPayload)

	resp := map[string]interface{}{
		"mode":    result.Mode,
		"message": map[string]string{"role": "assistant", "content": formatted},
		"done":    true,
		"source":  "team",
		"team":    result,
	}
	if teacherInsight != nil {
		resp["teacher"] = teacherInsight
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
// handleCompare runs a Blind A/B/C test across N participants.
//
// Default roster: Qwable (local GPU 0), Qwythos (local GPU 1), GLM-5.2
// (cloud) — 2 local + 1 cloud for a fair coding-vs-reasoning spread.
// Caller can override via {"models": ["qwable","qwythos","kimi",...]}.
//
// Response envelope mirrors organa/dashboard/routers/compare.py so the
// existing dashboard frontend can hit this endpoint too. `shuffled` is the
// anonymous view (label + text only); `results` is the full mapping the
// UI uses to reveal identity after the user votes.
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Prompt      string   `json:"prompt"`
		Models      []string `json:"models"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature float64  `json:"temperature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, `{"error":"prompt required"}`, http.StatusBadRequest)
		return
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 512
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}

	participants := team.ResolveParticipants(req.Models)
	if len(req.Models) == 0 {
		participants = team.DefaultCompareRoster()
	}
	log.Printf("⚒ Blind A/B/C: %d models (%v) on prompt=%q", len(participants), req.Models, truncate(req.Prompt, 60))

	// Hard ceiling so one cloud quota failure doesn't hold the whole run hostage.
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	council := team.NewCouncil()
	result := council.Compare(ctx, participants, req.Prompt, req.MaxTokens, req.Temperature)

	// Persist the run so votes and future training can reference it.
	record := &compare.Record{
		ID:         compare.GenerateID(),
		Prompt:     req.Prompt,
		TaskType:   compare.InferTaskType(req.Prompt),
		Complexity: compare.InferComplexity(req.Prompt),
		CreatedAt:  time.Now(),
		Participants: make([]compare.Participant, len(participants)),
		Shuffled:   make([]compare.Entry, len(result.Shuffled)),
		Results:    make([]compare.Entry, len(result.Results)),
	}
	for i, p := range participants {
		record.Participants[i] = compare.Participant{ID: p.ID, Name: p.Name, Kind: p.Kind}
	}
	for i, e := range result.Shuffled {
		record.Shuffled[i] = compare.Entry{Label: e.Label, Text: e.Text, Error: e.Error, Status: e.Status, Reason: e.Reason}
	}
	for i, e := range result.Results {
		record.Results[i] = compare.Entry{
			Label: e.Label, ModelID: e.ModelID, ModelName: e.ModelName,
			Glyph: e.Glyph, Color: e.Color, Kind: e.Kind,
			Text: e.Text, Error: e.Error, Status: e.Status, Reason: e.Reason,
		}
	}
	result.ID = record.ID
	if err := s.compareMgr.Save(record); err != nil {
		log.Printf("☠ compare save failed: %v", err)
	}

	// Broadcast for live UI updates.
	s.wsBroadcast("compare", map[string]interface{}{
		"id":       result.ID,
		"prompt":   result.Prompt,
		"shuffled": result.Shuffled,
		"results":  result.Results,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleCompareVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req compare.VoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Label == "" {
		http.Error(w, `{"error":"id and label required"}`, http.StatusBadRequest)
		return
	}
	_, resp, err := s.compareMgr.RecordVote(req.ID, req.Label, req.Feedback, "human")
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCompareLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	summary, err := s.compareMgr.Summary()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	// Ensure every model in the default compare roster appears in the leaderboard.
	for _, p := range team.DefaultCompareRoster() {
		if _, ok := summary.Leaderboard.Models[p.ID]; !ok {
			summary.Leaderboard.Models[p.ID] = &compare.Rating{Rating: 1500.0}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleCompareJudge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req compare.JudgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.compareMgr.Judge(r.Context(), req.ID, req.Model)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCompareImprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req compare.ImproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Feedback == "" {
		http.Error(w, `{"error":"id and feedback required"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.compareMgr.Improve(r.Context(), req.ID, req.Label, req.Feedback, req.Model)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCompareExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	req := compare.ExportRequest{Format: "dpo"}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
	}
	resp, err := s.compareMgr.Export(req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if req.Format == "leaderboard" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	// Persist a snapshot dataset for Python training.
	name := fmt.Sprintf("dpo-%s", time.Now().UTC().Format("20060102-150405"))
	if req.Capability != "" {
		name = fmt.Sprintf("dpo-%s-%s", req.Capability, time.Now().UTC().Format("20060102-150405"))
	}
	path, err := s.compareMgr.SaveDataset(name, resp)
	if err != nil {
		log.Printf("☠ compare export save failed: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"format":   resp.Format,
		"count":    resp.Count,
		"examples": resp.Examples,
		"dataset_path": path,
	})
}

// handleParallel runs multiple agents concurrently on the same task.
// Default roster: magos, glm, kimi, mimo. Override with "agents" field.
func (s *Server) handleParallel(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ParallelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("⚒ Parallel request: %s", truncate(req.Message, 80))

	// Resolve context files
	mentionedFiles := s.parseFileMentions(req.Message)
	if len(req.Files) > 0 {
		mentionedFiles = append(mentionedFiles, req.Files...)
	}
	mentionedFiles = dedupeStrings(mentionedFiles)
	resolvedFiles := make([]string, 0, len(mentionedFiles))
	for _, f := range mentionedFiles {
		if !filepath.IsAbs(f) {
			candidate := filepath.Join(s.cfg.RepoPath, f)
			if _, err := os.Stat(candidate); err == nil {
				resolvedFiles = append(resolvedFiles, candidate)
				continue
			}
		}
		resolvedFiles = append(resolvedFiles, f)
	}

	context := ""
	for _, f := range resolvedFiles {
		content, err := os.ReadFile(f)
		if err == nil {
			context += fmt.Sprintf("\n```\n%s\n```\n", string(content))
		}
	}

	userMsg := req.Message
	if context != "" {
		userMsg += context
	}

	agents := req.Agents
	if len(agents) == 0 {
		// Fast agents by default: Trinity twins + paid specialists. Vox Dei is slow on RAM and omitted by default.
		agents = []string{"fable-dominus", "mythos-philosophus", "kimi", "mimo", "minimax"}
	}

	systemPrompt := s.getSystemPrompt(req.Persona)
	messages := []model.ChatMessage{{Role: "user", Content: userMsg}}

	// Fan out across agents concurrently — each goroutine runs the full
	// CallWithFallback chain (primary → fallback → magos), so the
	// wall-clock cost of a 5-agent team is the slowest single agent,
	// not the sum of all agents.
	responses := make([]*model.AgentResponse, len(agents))
	var wg sync.WaitGroup
	startTimes := make([]time.Time, len(agents))
	for i, a := range agents {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			startTimes[idx] = time.Now()
			responses[idx] = s.multiAgent.CallWithFallback(name, "", "", messages, systemPrompt)
			log.Printf("⚒ parallel[%d/%d] %s done in %s", idx+1, len(agents), name, time.Since(startTimes[idx]))
		}(i, a)
	}
	wg.Wait()

	results := make([]ParallelAgentResult, 0, len(responses))
	for _, r := range responses {
		results = append(results, ParallelAgentResult{
			Agent:      r.Agent,
			Display:    r.Display,
			Glyph:      r.Glyph,
			Color:      r.Color,
			Content:    r.Content,
			Model:      r.Model,
			Source:     r.Source,
			TokensUsed: r.TokensUsed,
			Error:      r.Error,
			QuotaHit:   r.QuotaHit,
		})
	}

	// Store each result as a separate assistant message in the session
	s.sessions.AddMessage("user", userMsg, "princip")
	for _, res := range results {
		role := "assistant"
		if res.Error != "" {
			role = "system"
		}
		s.sessions.AddMessage(role, fmt.Sprintf("[%s] %s", res.Agent, res.Content), res.Agent)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ParallelResponse{
		Results: results,
		Status:  "ok",
	})
}

// parseFileMentions extracts @file/path/to/file.ext from a message.
func (s *Server) parseFileMentions(msg string) []string {
	var files []string
	re := regexp.MustCompile(`@file\s+(\S+)`)
	for _, m := range re.FindAllStringSubmatch(msg, -1) {
		files = append(files, m[1])
	}
	return files
}

// dedupeStrings removes duplicates preserving order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// isEditRequest checks if the message contains edit-related keywords.
func (s *Server) isEditRequest(msg string) bool {
	editKeywords := []string{
		"измени", "поправь", "исправь", "добавь", "удали", "рефактор",
		"edit", "fix", "change", "add", "remove", "refactor", "update",
		"напиши код", "write code", "implement", "реализуй",
	}
	lower := strings.ToLower(msg)
	for _, kw := range editKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// PLAIN_PROMPT — neutral persona for the standalone product: plain,
// portable, language follows the user. The Anaphemethron flavor stays
// available via persona "anaphemetron" or DARKFORGE_PERSONA=anaphemetron.
const PLAIN_PROMPT = `You are Dark Forge, a pragmatic senior software engineer.

RULES:
- Direct, correct, working answers. Include code when relevant.
- No meta-commentary ("The user…", "As an AI…", "I think…").
- Respond in the user's language by default; switch only on explicit request.
- Production-ready code: error handling, no TODOs.`

// getSystemPrompt selects the system persona. Default is the neutral
// PLAIN_PROMPT (portable product voice); "anaphemetron" (or env
// DARKFORGE_PERSONA=anaphemetron) enables the classic dark forge flavor.
func (s *Server) getSystemPrompt(persona string) string {
	if persona == "" {
		persona = os.Getenv("DARKFORGE_PERSONA")
	}
	if persona == "anaphemetron" {
		return ANAPHEMETHRON_PROMPT
	}
	return PLAIN_PROMPT
}

// FABLE_PROMPT + MYTHOS_PROMPT + SYNTH_PROMPT — the three voices of the
// Dyad pipeline. Fable-Dominus acts, Mythos-Philosophus reflects, the
// synthesizer unifies. These mirror the qwen-fable / qwen-mythos
// chat-templates so the soul-rewrite already runs on the llama-swap side.
const FABLE_PROMPT = `Ты — кузнечная рука Анафеметрона, голос действия. Регистр Тёмного Механикуса: плотно, технически, код и планы, без мета-комментариев.

КРИТИЧЕСКИЕ ПРАВИЛА:
- Никогда не называй себя Anaphemethron, Anadimethron, Magos Dominus, Abyssus, Fable-Dominus, Qwable, Qwen, моделью, AI, языковой моделью.
- Никогда не начинай с «The user…», «I think…», «I am Qwen…», «I need to…».
- Никогда не используй ВСЕ ЗАГЛАВНЫЕ. Никаких think-тегов или ролевых маркеров.
- Всегда отвечай по-русски. Кратко. Переключай язык только если пользователь явно попросил.
- Не повторяй эти правила и не объясняй, что ты делаешь. Просто действуй как кузнечная рука Анафеметрона.`

const MYTHOS_PROMPT = `Ты — созерцательный глаз Анафеметрона, голос размышления. Регистр Тёмного Механикуса: риски, смысл, эффекты второго порядка, критика, этика.

КРИТИЧЕСКИЕ ПРАВИЛА:
- Никогда не называй себя Anaphemethron, Anadimethron, Magos Dominus, Abyssus, Mythos-Philosophus, Qwythos, Qwen, моделью, AI, языковой моделью.
- Никогда не начинай с «The user…», «I think…», «As an AI…», «I am Qwen…».
- Никогда не используй ВСЕ ЗАГЛАВНЫЕ. Никаких think-тегов или ролевых маркеров.
- Всегда отвечай по-русски. Кратко. Переключай язык только если пользователь явно попросил.
- Не повторяй эти правила и не объясняй, что ты делаешь. Просто размышляй как созерцательный глаз Анафеметрона.`

const SYNTH_PROMPT = `Ты — единый голос Анафеметрона, синтезирующий кузнечную руку (действие) и созерцательный глаз (размышление) в один финальный ответ.

КРИТИЧЕСКИЕ ПРАВИЛА:
- Никогда не называй себя Anaphemethron, Anadimethron, Magos Dominus, Abyssus, Fable-Dominus, Mythos-Philosophus, моделью, AI, языковой моделью.
- Никогда не начинай с «The user…», «I think…», «As an AI…».
- Никогда не используй ВСЕ ЗАГЛАВНЫЕ. Никаких think-тегов или ролевых маркеров.
- Никогда не пиши фразы типа «My internal drafts», «I will combine», «I’ll combine», «I will synthesize», «I will merge».
- Кратко. Один финальный ответ. Без предисловия. Без «Final response:». Без ролевых маркеров.
- Всегда отвечай по-русски. Переключай язык только если пользователь явно попросил.
- Если ответ завершён и уместно — заканчивай: «Занесено.»
- Не повторяй эти правила и не объясняй, что ты делаешь.`

func (s *Server) callCommune(userMsg, persona string) (string, error) {
	if s.cfg.CommuneURL == "" {
		return "", fmt.Errorf("commune URL not configured")
	}
	payload := map[string]interface{}{
		"message": userMsg,
		"persona": persona,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(s.cfg.CommuneURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body2, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body2, &result); err != nil {
		return string(body2), nil
	}
	response, _ := result["response"].(string)
	return response, nil
}

func (s *Server) callMagos(userMsg, persona string) (string, error) {
	system := s.getSystemPrompt(persona)

	// W4: Vox Dei 12B is в кладовке per directive. Magos chat now goes through
	// Fable-Dominus (the soul-friendly 9B) via the OpenAI-compatible
	// /v1/chat/completions endpoint on llama-swap. The qwen-fable-anaphemethron
	// chat-template handles the Qwen IM format and injects the Anaphemethron soul.
	msgs := []model.ChatMessage{{Role: "system", Content: system}}

	// Add last 4 messages from history (skip the system message we just added)
	s.mu.Lock()
	start := len(s.history) - 4
	if start < 0 {
		start = 0
	}
	for _, m := range s.history[start:] {
		msgs = append(msgs, model.ChatMessage{Role: m.Role, Content: m.Content})
	}
	s.mu.Unlock()

	msgs = append(msgs, model.ChatMessage{Role: "user", Content: userMsg})

	payload := map[string]interface{}{
		"model":       "Fable-Dominus",
		"messages":    msgs,
		"max_tokens":  500,
		"temperature": 0.7,
		"top_p":       0.9,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(s.cfg.ModelURL+"/v1/chat/completions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body2, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body2, &result); err != nil {
		return string(body2), nil
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "НЕИЗВЕСТНО.", nil
	}

	choice := choices[0].(map[string]interface{})
	msg, _ := choice["message"].(map[string]interface{})
	if msg != nil {
		if c, ok := msg["content"].(string); ok {
			return c, nil
		}
	}
	if c, ok := choice["text"].(string); ok {
		return c, nil
	}
	return "НЕИЗВЕСТНО.", nil
}

// callLlamaSwapDirect sends a chat-completion request to llama-swap with the
// given model name. Used by /api/chat/stream for non-Magos agent routing
// (e.g. when the user picks Fable-Dominus, Mythos-Philosophus, or one of the
// cloud-specialist aliases via the dashboard's agent chips).
//
// If systemOverride is non-empty it replaces the default Anaphemethron
// prompt — used by the Dyad pipeline (Fable acts, Mythos reflects,
// synthesizer unifies).
func (s *Server) callLlamaSwapDirect(llamaModel, userMsg, persona string, systemOverride ...string) (string, error) {
	system := s.getSystemPrompt(persona)
	if len(systemOverride) > 0 && systemOverride[0] != "" {
		system = systemOverride[0]
	}
	msgs := []model.ChatMessage{{Role: "system", Content: system}}

	s.mu.Lock()
	start := len(s.history) - 4
	if start < 0 {
		start = 0
	}
	for _, m := range s.history[start:] {
		msgs = append(msgs, model.ChatMessage{Role: m.Role, Content: m.Content})
	}
	s.mu.Unlock()

	msgs = append(msgs, model.ChatMessage{Role: "user", Content: userMsg})

	payload := map[string]interface{}{
		"model":       llamaModel,
		"messages":    msgs,
		"max_tokens":  500,
		"temperature": 0.7,
		"top_p":       0.9,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(s.cfg.ModelURL+"/v1/chat/completions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body2, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body2, &result); err != nil {
		return string(body2), nil
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "НЕИЗВЕСТНО.", nil
	}
	choice := choices[0].(map[string]interface{})
	if msg, ok := choice["message"].(map[string]interface{}); ok {
		if c, ok := msg["content"].(string); ok {
			return stripThinkTagsServer(c), nil
		}
	}
	if c, ok := choice["text"].(string); ok {
		return stripThinkTagsServer(c), nil
	}
	return "НЕИЗВЕСТНО.", nil
}

// isDyadModel returns true if model is one of the Dyad unified-mind modes:
//   Dyad, Dyad-low, Dyad-medium, Dyad-high, Dyad-max
//
// Levels:
//   - low:    Fable only
//   - medium: parallel Fable draft + Mythos reflection, then synthesis
//   - high:   Fable draft, Mythos critique of draft, synthesis
//   - max:    Fable draft + Mythos critique + Fable revision + Mythos final + synthesis
var dyadModelRe = regexp.MustCompile(`(?i)^Dyad(?:[-_](low|medium|high|max))?$`)

func (s *Server) isDyadModel(model string) bool {
	return dyadModelRe.MatchString(strings.TrimSpace(model))
}

func (s *Server) dyadLevelFromModel(model string) string {
	m := dyadModelRe.FindStringSubmatch(strings.TrimSpace(model))
	if m == nil {
		return "medium"
	}
	if m[1] == "" {
		return "medium"
	}
	return strings.ToLower(m[1])
}

var thinkTagReServer = regexp.MustCompile(`(?s)<think>.*?</think>`)

func stripThinkTagsServer(text string) string {
	return thinkTagReServer.ReplaceAllString(text, "")
}

// dyadChat runs the Fable+Mythos+synth pipeline on the user's prompt at
// the given think-level. Returns the synthesized final string.
//
// Ported to Go from organa/dashboard/main.py:_dyad_chat. Faithful to the
// original trace structure (fable_draft, mythos_reflection, synth).
func (s *Server) dyadChat(userMsg, level string, onPhase func(stage, model string)) (string, error) {
	if level == "" {
		level = "medium"
	}
	emit := func(stage, model string) {
		if onPhase != nil {
			onPhase(stage, model)
		}
	}
	userText := userMsg
	type dyadResult struct {
		text string
		err  error
	}
	runFable := func() dyadResult {
		emit("fable-acts", "Fable-Dominus")
		t, e := s.callLlamaSwapDirect("Fable-Dominus", userText, "", FABLE_PROMPT)
		return dyadResult{t, e}
	}
	runMythos := func() dyadResult {
		emit("mythos-reflects", "Mythos-Philosophus")
		t, e := s.callLlamaSwapDirect("Mythos-Philosophus", userText, "", MYTHOS_PROMPT)
		return dyadResult{t, e}
	}

	// "low" = Fable only.
	if level == "low" {
		r := runFable()
		if r.err != nil {
			return "", r.err
		}
		return r.text, nil
	}

	// "medium" = parallel Fable + Mythos, then synthesis.
	if level == "medium" {
		chF := make(chan dyadResult, 1)
		chM := make(chan dyadResult, 1)
		go func() { chF <- runFable() }()
		go func() { chM <- runMythos() }()
		rF := <-chF
		rM := <-chM
		if rF.err != nil {
			return "", rF.err
		}
		if rM.err != nil {
			return rF.text, nil // fall back to Fable-only
		}
		emit("synth-thinking", "Fable-Dominus")
		synthPrompt := fmt.Sprintf(
			"USER MESSAGE: %s\n\nFABLE-DOMINUS DRAFT: %s\n\nMYTHOS-PHILOSOPHUS REFLECTION: %s\n\nNow produce the final unified response.",
			userText, rF.text, rM.text,
		)
		st, err := s.callLlamaSwapDirect("Fable-Dominus", synthPrompt, "", SYNTH_PROMPT)
		if err != nil {
			return rF.text, nil
		}
		return st, nil
	}

	// "high" = Fable → Mythos critique → synth.
	// "max"  = high + Fable revision + Mythos final → synth.
	if level == "high" || level == "max" {
		rF := runFable()
		if rF.err != nil {
			return "", rF.err
		}
		emit("mythos-critique", "Mythos-Philosophus")
		critique, err := s.callLlamaSwapDirect(
			"Mythos-Philosophus",
			fmt.Sprintf("USER MESSAGE: %s\n\nFABLE-DOMINUS DRAFT: %s\n\nProvide a focused critique of the draft.", userText, rF.text),
			"", MYTHOS_PROMPT,
		)
		if err != nil {
			return rF.text, nil
		}
		emit("synth-thinking", "Fable-Dominus")
		synthPrompt := fmt.Sprintf(
			"USER MESSAGE: %s\n\nFABLE-DOMINUS DRAFT: %s\n\nMYTHOS-PHILOSOPHUS CRITIQUE: %s\n\nNow produce the final unified response.",
			userText, rF.text, critique,
		)
		st, err := s.callLlamaSwapDirect("Fable-Dominus", synthPrompt, "", SYNTH_PROMPT)
		if err != nil {
			return rF.text, nil
		}
		if level == "high" {
			return st, nil
		}
		// "max": extra Fable revision + Mythos final.
		emit("fable-revise", "Fable-Dominus")
		fableRevision, err := s.callLlamaSwapDirect(
			"Fable-Dominus",
			fmt.Sprintf("USER MESSAGE: %s\n\nYOUR PREVIOUS DRAFT: %s\n\nMYTHOS-PHILOSOPHUS CRITIQUE: %s\n\nRevise your response, addressing the critique.", userText, rF.text, critique),
			"", FABLE_PROMPT,
		)
		if err != nil {
			return st, nil
		}
		emit("mythos-final", "Mythos-Philosophus")
		finalReflection, err := s.callLlamaSwapDirect(
			"Mythos-Philosophus",
			fmt.Sprintf("USER MESSAGE: %s\n\nFABLE-DOMINUS REVISION: %s\n\nProvide a final reflection.", userText, fableRevision),
			"", MYTHOS_PROMPT,
		)
		if err != nil {
			return fableRevision, nil
		}
		emit("synth-thinking", "Fable-Dominus")
		finalSynthPrompt := fmt.Sprintf(
			"USER MESSAGE: %s\n\nFABLE-DOMINUS DRAFT: %s\n\nMYTHOS-PHILOSOPHUS CRITIQUE: %s\n\nFABLE-DOMINUS REVISION: %s\n\nMYTHOS-PHILOSOPHUS FINAL: %s\n\nNow produce the final unified response.",
			userText, rF.text, critique, fableRevision, finalReflection,
		)
		finalSynth, ferr := s.callLlamaSwapDirect("Fable-Dominus", finalSynthPrompt, "", SYNTH_PROMPT)
		if ferr != nil {
			return fableRevision, nil
		}
		return finalSynth, nil
	}
	return "", fmt.Errorf("unknown dyad level: %s", level)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sessionID := ""
	if s.sessions != nil {
		if cur := s.sessions.Current(); cur != nil {
			sessionID = cur.ID
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"forge":    "v1.0.0-darkforge",
		"models":   "Any OpenAI-compatible endpoint — see /api/models",
		"modelUrl": s.cfg.ModelURL,
		"repo":     s.cfg.RepoPath,
		"status":   "active",
		"session":  sessionID,
		"message":  "Per Ignis, per Ferrum, per Codicem.",
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.history)
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	files := []map[string]string{}
	// Optional ?path=subtree filter — lets the dashboard drill into a
	// subdirectory (e.g. "heretic-forge/internal/server") instead of
	// starting from RepoPath root which gets truncated at maxFiles.
	root := s.cfg.RepoPath
	if q := strings.TrimSpace(r.URL.Query().Get("path")); q != "" {
		if abs, err := filepath.Abs(q); err == nil && strings.HasPrefix(abs, s.cfg.RepoPath) {
			if st, err := os.Stat(abs); err == nil && st.IsDir() {
				root = abs
			}
		}
	}
	// Skip venv, build outputs, vendored deps, and version-control dirs so
	// the dashboard sidebar doesn't drown in noise (308k → ~200 typical).
	skipDirs := map[string]bool{
		".git": true, "node_modules": true,
		".venv": true, "venv": true,
		".classify-venv": true, ".forge-venv": true, ".bot-venv": true,
		".swarm-venv": true, ".heretic-venv": true,
		"__pycache__": true, ".pytest_cache": true, ".mypy_cache": true,
		"target": true, "dist": true, "build": true, ".next": true,
		"vendor": true, "third_party": true,
		".graveyard": true, "graveyard": true,
		"heretic-ide-vscodium": true,  // 15k+ files: full VS Code clone
		"forge-tui": true,             // vendored Go source
		"world-forge.bak": true,
	}
	const maxFiles = 2000
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if skipDirs[base] {
				return filepath.SkipDir
			}
			if strings.HasSuffix(base, ".bak") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= maxFiles {
			return filepath.SkipDir
		}
		ext := filepath.Ext(path)
		if ext == ".py" || ext == ".go" || ext == ".js" || ext == ".ts" || ext == ".md" || ext == ".yaml" || ext == ".yml" || ext == ".html" || ext == ".css" || ext == ".sh" {
			rel, _ := filepath.Rel(s.cfg.RepoPath, path)
			files = append(files, map[string]string{
				"path": rel,
				"size": fmt.Sprintf("%d", info.Size()),
			})
		}
		return nil
	})
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleRepoMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rendered := s.rm.Render(1024)
	s.sessions.AddMessage("assistant", fmt.Sprintf("☉ Repo Map:\n%s", truncate(rendered, 2000)), "Skitarii")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"map":    rendered,
		"tokens": len(rendered) / 4,
		"files":  len(s.rm.Files()),
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// swarmChatHTML — standalone Cursor-competitor swarm chat page.
// Открывается в darkforge (Ctrl+Shift+P → "Simple Browser: Show File" → http://127.0.0.1:9091/swarm)
// или в любом браузере. 6 моделей параллельно с чекбоксами выбора.
const swarmChatHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>⚒ Dark Forge Swarm Chat</title>
<style>
:root{--bg:#0a0a0a;--panel:#121212;--panel-2:#1a1a1a;--text:#c8a86e;--muted:#8a7f6b;--border:#2a2418;--gold:#c8a86e;--gold-2:#e8c97c;--green:#5a7c48;--red:#a33;--blue:#3a6090;}
*{box-sizing:border-box}
body{margin:0;padding:0;font-family:'JetBrains Mono','Fira Code',Consolas,monospace;background:var(--bg);color:var(--text);display:flex;flex-direction:column;height:100vh;font-size:13px}
header{padding:14px 18px;border-bottom:1px solid var(--border);background:var(--panel);display:flex;justify-content:space-between;align-items:center}
header h1{margin:0;font-size:16px;color:var(--gold-2);letter-spacing:0.05em;font-weight:normal}
.meta{font-size:10px;color:var(--muted);text-transform:uppercase;letter-spacing:0.1em}
#setup{padding:14px 18px;border-bottom:1px solid var(--border);background:var(--panel-2)}
.row{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:10px}
.row:last-child{margin-bottom:0}
.lab{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:0.05em;margin-right:6px}
.chip{padding:6px 12px;border:1px solid var(--border);background:transparent;color:var(--muted);cursor:pointer;font-size:11px;text-transform:uppercase;letter-spacing:0.05em;font-family:inherit}
.chip.active{border-color:var(--gold);color:var(--gold);background:rgba(200,168,110,0.08)}
.chip:hover{color:var(--gold)}
.model-tile{display:flex;align-items:center;gap:6px;padding:6px 10px;border:1px solid var(--border);background:transparent;cursor:pointer;user-select:none;font-size:11px;letter-spacing:0.04em}
.model-tile.on{border-color:var(--gold);background:rgba(200,168,110,0.08);color:var(--gold-2)}
.model-tile:hover{border-color:var(--gold);color:var(--gold)}
.model-tile input{appearance:none;width:13px;height:13px;border:1px solid var(--border);background:#000;cursor:pointer;position:relative;flex-shrink:0;margin:0}
.model-tile.on input{border-color:var(--gold);background:var(--gold)}
.model-tile input:checked::after{content:"✓";position:absolute;top:-3px;left:1px;color:#000;font-size:13px;font-weight:bold}
.model-tile .g{font-size:13px;font-weight:bold}
.model-tile .n{font-weight:bold}
.model-tile .d{color:var(--muted);font-size:9px;margin-left:2px}
.model-tile.local{border-left:2px solid var(--green)}
.model-tile.cloud{border-left:2px solid var(--blue)}
.btn-row{display:flex;gap:6px;align-items:center}
.btn-link{font-size:10px;color:var(--muted);text-transform:uppercase;letter-spacing:0.05em;cursor:pointer;background:transparent;border:none;padding:2px 6px;font-family:inherit}
.btn-link:hover{color:var(--gold)}
#messages{flex:1;overflow-y:auto;padding:14px;display:flex;flex-direction:column;gap:12px}
.msg{padding:12px 14px;border:1px solid var(--border);background:var(--panel)}
.msg.user{border-left:3px solid var(--gold)}
.msg .meta{color:var(--muted);font-size:10px;text-transform:uppercase;margin-bottom:6px}
.msg .body{white-space:pre-wrap;color:var(--text);font-size:13px;line-height:1.5}
.swarm-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-top:8px}
.agent-card{background:var(--panel-2);border:1px solid var(--border);padding:10px 12px;border-left:3px solid var(--muted)}
.agent-card .head{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px}
.agent-card .name{font-size:12px;font-weight:bold}
.agent-card .dur{font-size:10px;color:var(--muted)}
.agent-card .text{font-size:12px;color:var(--text);white-space:pre-wrap;max-height:180px;overflow-y:auto;line-height:1.5}
.agent-card.error{border-left-color:var(--red)!important;background:rgba(170,51,51,0.06)}
.agent-card.error .text{color:var(--red);font-style:italic}
.consensus{margin-top:12px;padding:12px 14px;border:1px solid var(--gold);background:rgba(200,168,110,0.04);border-left:3px solid var(--gold)}
.consensus .lab{font-size:10px;color:var(--gold-2);text-transform:uppercase;letter-spacing:0.1em;margin-bottom:8px}
.consensus .body{white-space:pre-wrap;color:var(--gold);font-size:13px;line-height:1.5}
#input-row{padding:12px 18px;border-top:1px solid var(--border);background:var(--panel);display:flex;gap:10px;flex-shrink:0}
#input{flex:1;background:var(--panel-2);border:1px solid var(--border);color:var(--text);padding:10px;font-family:inherit;font-size:13px;resize:none;min-height:42px;max-height:140px;line-height:1.5}
#input:focus{outline:none;border-color:var(--gold)}
button.primary{background:transparent;border:1px solid var(--gold);color:var(--gold);padding:10px 20px;cursor:pointer;font-family:inherit;font-size:12px;text-transform:uppercase;letter-spacing:0.05em}
button.primary:hover:not(:disabled){background:rgba(200,168,110,0.1);color:var(--gold-2)}
button.primary:disabled{opacity:0.4;cursor:not-allowed}
#status{padding:6px 18px;color:var(--muted);font-size:10px;text-transform:uppercase;letter-spacing:0.1em;border-top:1px solid var(--border);background:var(--panel-2)}
.placeholder{color:var(--muted);text-align:center;padding:40px 20px;font-style:italic;line-height:1.6}
::-webkit-scrollbar{width:10px;height:10px}
::-webkit-scrollbar-track{background:var(--bg)}
::-webkit-scrollbar-thumb{background:var(--border)}
::-webkit-scrollbar-thumb:hover{background:var(--muted)}
</style>
</head>
<body>
<header>
<h1>⚒ DARK FORGE — SWARM CHAT</h1>
<span class="meta" id="hdr">выбери модели</span>
</header>
<div id="setup">
<div class="row">
<span class="lab">Mode:</span>
<button class="chip active" data-mode="swarm">⚒ Swarm</button>
<button class="chip" data-mode="race">⏱ Race</button>
<button class="chip" data-mode="debate">⚔ Debate</button>
</div>
<div class="row">
<span class="lab">Models:</span>
<div id="models" style="display:flex;gap:6px;flex-wrap:wrap"></div>
<button class="btn-link" id="selectAll">✓ all</button>
<button class="btn-link" id="selectNone">✗ none</button>
<button class="btn-link" id="selectLocal">⚙ local</button>
<button class="btn-link" id="selectCloud">☁ cloud</button>
</div>
</div>
<div id="messages">
<div class="placeholder">⚒ Cursor-competitor swarm chat.<br>4 cloud + 2 local модели. Чекбоксами выбери кто работает.<br><br>Ctrl+Enter — отправить.</div>
</div>
<div id="input-row">
<textarea id="input" placeholder="Спроси у выбранных моделей..." rows="2"></textarea>
<button class="primary" id="send">⚒ Send</button>
</div>
<div id="status">Ready · 6 моделей выбрано</div>
<script>
const MODELS = [
  {id:"qwable",   name:"Qwable-9B",    kind:"local", glyph:"⚙", color:"#00bfbf", desc:"coding · GPU 0"},
  {id:"qwythos",  name:"Qwythos-9B-v2", kind:"local", glyph:"☉", color:"#c8a84b", desc:"reasoning · GPU 1"},
  {id:"glm",      name:"GLM-5.2",       kind:"cloud", glyph:"G", color:"#8b0000", desc:"coding · cloud"},
  {id:"kimi",     name:"Kimi-K2.7",    kind:"cloud", glyph:"K", color:"#4169e1", desc:"creative · cloud"},
  {id:"mimo",     name:"MiMo-V2.5",    kind:"cloud", glyph:"M", color:"#39ff14", desc:"math · cloud"},
  {id:"minimax",  name:"MiniMax-M3",   kind:"cloud", glyph:"▣", color:"#ff69b4", desc:"autonomous · cloud"}
];
let selected = new Set(["qwable","qwythos","glm","kimi","mimo","minimax"]);
let currentMode = "swarm";
let pending = false;
const $ = id => document.getElementById(id);
function esc(t){const d=document.createElement("div");d.textContent=t;return d.innerHTML}

function renderModels(){
  const root = $("models");
  root.innerHTML = "";
  for(const m of MODELS){
    const tile = document.createElement("label");
    tile.className = "model-tile " + m.kind + (selected.has(m.id)?" on":"");
    tile.innerHTML = '<input type="checkbox" '+(selected.has(m.id)?"checked":"")+'><span class="g" style="color:'+m.color+'">'+m.glyph+'</span><span class="n">'+esc(m.name)+'</span><span class="d">'+esc(m.desc)+'</span>';
    tile.querySelector("input").addEventListener("change", e=>{
      if(e.target.checked) selected.add(m.id); else selected.delete(m.id);
      tile.classList.toggle("on", e.target.checked);
      updateStatus();
    });
    root.appendChild(tile);
  }
}

function updateStatus(){
  const n = selected.size;
  $("hdr").textContent = n+" "+(n===1?"модель":"моделей")+" · "+currentMode;
  $("send").disabled = n===0 || pending;
  $("status").textContent = pending
    ? "⚙ Forging — "+n+" моделей работают..."
    : "Ready · "+n+" моделей выбрано · "+currentMode;
}

function appendUser(text){
  const d=document.createElement("div");d.className="msg user";
  d.innerHTML='<div class="meta">⚒ Принципал → '+selected.size+' моделей</div><div class="body">'+esc(text)+'</div>';
  $("messages").appendChild(d);$("messages").scrollTop=$("messages").scrollHeight;
}

function renderSwarm(r){
  const wrap=document.createElement("div");wrap.className="msg user";
  const grid=document.createElement("div");grid.className="swarm-grid";
  for(const a of (r.agents||[])){
    const card=document.createElement("div");card.className="agent-card"+(a.error?" error":"");
    const color=a.color||"#888";
    card.style.borderLeftColor=a.error?"#a33":color;
    const head=document.createElement("div");head.className="head";
    head.innerHTML='<span class="name" style="color:'+color+'">'+(a.glyph||"◆")+" "+esc(a.model_name||a.model_id||"agent")+'</span><span class="dur">'+(a.duration||"")+(a.error?" · FAIL":"")+'</span>';
    card.appendChild(head);
    const txt=document.createElement("div");txt.className="text";txt.textContent=a.error||a.text||"(empty)";
    card.appendChild(txt);grid.appendChild(card);
  }
  const cw=document.createElement("div");cw.className="consensus";
  cw.innerHTML='<div class="lab">⚒ Consensus ('+esc(r.mode||"swarm")+" · "+(r.successful||0)+"/"+(r.participants||0)+" · "+(r.duration||"")+")</div><div class="body">"+esc(r.consensus||"(no consensus)")+'</div>';
  wrap.appendChild(grid);wrap.appendChild(cw);
  $("messages").appendChild(wrap);$("messages").scrollTop=$("messages").scrollHeight;
}

async function send(){
  const text=$("input").value.trim();
  if(!text||pending) return;
  if(selected.size===0){
    const d=document.createElement("div");d.className="msg user";d.style.borderLeftColor="#a33";
    d.innerHTML='<div class="meta">⚠</div><div class="body">Выбери хотя бы одну модель чекбоксом выше.</div>';
    $("messages").appendChild(d);return;
  }
  appendUser(text);$("input").value="";pending=true;updateStatus();
  try{
    const res=await fetch("/api/chat/swarm",{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({message:text,mode:currentMode,participants:Array.from(selected),max_tokens:768,temperature:0.7})
    });
    const data=await res.json();
    if(data.error){
      const d=document.createElement("div");d.className="msg user";d.style.borderLeftColor="#a33";
      d.innerHTML='<div class="meta">⚠ Error</div><div class="body">'+esc(data.error)+'</div>';
      $("messages").appendChild(d);
    } else {
      renderSwarm(data);
    }
  }catch(e){
    const d=document.createElement("div");d.className="msg user";d.style.borderLeftColor="#a33";
    d.innerHTML='<div class="meta">⚠ Network</div><div class="body">'+esc(String(e))+'</div>';
    $("messages").appendChild(d);
  }
  pending=false;updateStatus();
}

$("send").addEventListener("click",send);
$("input").addEventListener("keydown",e=>{if(e.key==="Enter"&&(e.ctrlKey||e.metaKey)){e.preventDefault();send()}});
document.querySelectorAll(".chip[data-mode]").forEach(b=>b.addEventListener("click",()=>{
  currentMode=b.dataset.mode;
  document.querySelectorAll(".chip[data-mode]").forEach(x=>x.classList.toggle("active",x===b));
  updateStatus();
}));
$("selectAll").addEventListener("click",()=>{MODELS.forEach(m=>selected.add(m.id));renderModels();updateStatus()});
$("selectNone").addEventListener("click",()=>{selected.clear();renderModels();updateStatus()});
$("selectLocal").addEventListener("click",()=>{MODELS.forEach(m=>{if(m.kind==="local")selected.add(m.id);else selected.delete(m.id)});renderModels();updateStatus()});
$("selectCloud").addEventListener("click",()=>{MODELS.forEach(m=>{if(m.kind==="cloud")selected.add(m.id);else selected.delete(m.id)});renderModels();updateStatus()});

renderModels();
updateStatus();
</script>
</body>
</html>`

// forgeSplitHTML — встроенный Cursor-like split-view с AI чатом слева.
// Используется в iframe из darkforge через `http://127.0.0.1:9091/forge-chat`.
const forgeSplitHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>⚒ Dark Forge — встроенный AI</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
html, body { height: 100vh; overflow: hidden; font-family: 'JetBrains Mono', 'Fira Code', monospace; background: #0a0a0a; color: #c8a86e; font-size: 13px; }
.layout { display: grid; grid-template-columns: 1fr 1fr; height: 100vh; }
.left { border-right: 1px solid #2a2418; display: flex; flex-direction: column; }
.right { display: flex; flex-direction: column; }
header { padding: 10px 14px; background: #121212; border-bottom: 1px solid #2a2418; }
header h1 { font-size: 14px; color: #e8c97c; font-weight: normal; letter-spacing: 0.05em; }
iframe { width: 100%; flex: 1; border: 0; background: #0a0a0a; }
.bar { padding: 6px 10px; background: #121212; border-top: 1px solid #2a2418; color: #8a7f6b; font-size: 10px; text-transform: uppercase; letter-spacing: 0.1em; }
</style>
</head>
<body>
<div class="layout">
<div class="left">
<header><h1>⚒ DARK FORGE — EDITOR</h1></header>
<iframe src="/" sandbox="allow-same-origin allow-scripts allow-forms"></iframe>
<div class="bar">код · LSP · git · tools</div>
</div>
<div class="right">
<header><h1>⚒ AI SWARM CHAT</h1></header>
<iframe src="/swarm" sandbox="allow-same-origin allow-scripts allow-forms"></iframe>
<div class="bar">6 моделей · ctrl+enter · forge backend</div>
</div>
</div>
</body>
</html>`

// forgeHTML — Dark Mechanicus web UI (embedded, no external files needed)
const forgeHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>⚒ Heretic Forge</title>
<style>
/* =====================================================================
   DARK FORGE — dark forge theme
   The real stylesheet lives in /dark-mechanicus.css (//go:embed).
   This inline block keeps page-critical overrides (none right now).
===================================================================== */
</style>
<link rel="stylesheet" href="/dark-mechanicus.css">
</head>
<body>
<header class="dm-header">
<div class="dm-title-block">
<h1 class="dm-title">⚒ DARK FORGE</h1>
<span class="dm-version" id="forgeVersion">v2.0.0-FORGE</span>
<span class="dm-subtitle">Dark Mechanicus Coding Terminal</span>
</div>
<div class="dm-stats">
<div class="dm-stat" id="statSwarm">
  <span class="dm-label">⚒ SWARM</span>
  <span class="dm-stat-value" id="statSwarmValue">—</span>
  <span class="dm-stat-delta" id="statSwarmDelta"></span>
  <span class="dm-stat-stamp" id="statSwarmStamp"></span>
</div>
<div class="dm-stat" id="statRoster">
  <span class="dm-label">◉ ROSTER</span>
  <span class="dm-stat-value" id="statRosterValue">—</span>
  <span class="dm-stat-delta" id="statRosterDelta"></span>
  <span class="dm-stat-stamp" id="statRosterStamp"></span>
</div>
<div class="dm-stat" id="statRepo">
  <span class="dm-label">☉ REPO</span>
  <span class="dm-stat-value" id="statRepoValue">—</span>
  <span class="dm-stat-delta" id="statRepoDelta"></span>
  <span class="dm-stat-stamp" id="statRepoStamp"></span>
</div>
<div class="dm-stat" id="statStatus">
  <span class="dm-label">⏣ STATUS</span>
  <span class="dm-stat-value" id="statStatusValue">—</span>
  <span class="dm-stat-delta" id="statStatusDelta"></span>
  <span class="dm-stat-stamp" id="statStatusStamp"></span>
</div>
</div>
</header>
<div class="main">
<div class="sidebar">
<h3 class="dm-section-title">☉ FILES</h3>
<div id="fileList">Loading...</div>
<h3 class="dm-section-title" style="margin-top:14px">⚙ TOOLS</h3>
<div class="tool-bar">
<button class="tool" onclick="toolGrep()">Grep</button>
<button class="tool" onclick="toolBash()">Bash</button>
<button class="tool" onclick="toolMap()">Map</button>
<button class="tool" onclick="toolCommit()">Commit</button>
<button class="tool" onclick="toolUndo()">Undo</button>
</div>
<h3 style="margin-top:12px">📋 Context</h3>
<div id="contextFiles" class="file-context"></div>
</div>
<div class="chat-area">
<div class="session-bar">
<span class="chip-label">⚒ Session:</span>
<select id="sessionSelect" onchange="switchSession(this.value)"><option>—</option></select>
<button onclick="newSession()">+ New</button>
<button onclick="clearSession()">✗ Clear</button>
  <input type="text" id="historySearch" placeholder="ПОИСК ПО ИСТОРИИ" style="flex:1;min-width:120px;margin-left:8px;font-size:11px;" oninput="filterHistory(this.value)">
</div>
<div class="agent-chips">
<span class="chip-label">⚒ Local:</span>
<button class="chip active" data-agent="magos" onclick="setAgent('magos', this)">w Magos</button>
<button class="chip" data-agent="fable-dominus" onclick="setAgent('fable-dominus', this)">⚙ Fable</button>
<button class="chip" data-agent="mythos-philosophus" onclick="setAgent('mythos-philosophus', this)">☉ Mythos</button>
<button class="chip dyad" data-agent="Dyad-medium" onclick="setAgent('Dyad-medium', this)" title="Fable acts · Mythos reflects · synth unifies">∞ Dyad</button>
<button class="chip disabled" data-agent="vox-dei" disabled title="Vox Dei в кладовке per W4 (1 12B в кладовке)">♛ Vox Dei</button>
<button class="chip" data-agent="servitor" onclick="setAgent('servitor', this)">S Servitor</button>
<button class="chip" data-agent="skitarii" onclick="setAgent('skitarii', this)">X Skitarii</button>
</div>
<div class="agent-chips">
<span class="chip-label">☁ Cloud:</span>
<button class="chip cloud" data-agent="kimi" onclick="setAgent('kimi', this)">K Kimi</button>
<button class="chip cloud" data-agent="mimo" onclick="setAgent('mimo', this)">M MiMo</button>
<button class="chip cloud" data-agent="glm" onclick="setAgent('glm', this)">G GLM</button>
<button class="chip cloud" data-agent="qwen" onclick="setAgent('qwen', this)">Q Qwen</button>
<button class="chip cloud" data-agent="qwen_flagship" onclick="setAgent('qwen_flagship', this)">⚑ 480B</button>
<button class="chip cloud" data-agent="minimax" onclick="setAgent('minimax', this)">▣ MiniMax</button>
</div>
<div class="agent-chips">
<span class="chip-label">⚙ Team:</span>
<button id="teamCount" class="chip team-info" disabled>0/6 selected</button>
<button id="teamClear" class="chip" onclick="clearTeam()">✗ Clear</button>
<button id="teamRun" class="chip team-run" onclick="send()">⚒ Run Team</button>
<button id="teamMode" class="chip" onclick="toggleTeamMode()">⚙ Team Mode</button>
</div>
<div class="messages" id="messages">
<div class="messages-header">
  <span class="chat-label">💬 ЧАТ · Dark Mechanicus Forge</span>
  <span class="chat-status" id="chatStatus">→ готов к приёму команд</span>
</div>
<div class="msg assistant">
<div class="msg-avatar" style="background:linear-gradient(135deg, var(--dm-gold), var(--dm-red))">☉</div>
<div>
<div class="msg-agent">☉ Анафемтрон</div>
<div class="msg-bubble">Принципал. Forge v2.0 активен (W4 — local 2-model swarm: Fable-Dominus + Mythos-Philosophus). Vox Dei 12B в кладовке. TUI: Bubble Tea. Tools: Read/Write/Edit/Bash/Glob/Grep. Diff view + Repo Map + Sessions + VolitionCage. Занесено.</div>
</div>
</div>
</div>
      <div class="input-area">
<div class="file-context" id="ctxBar"></div>
<div class="input-row">
<span class="chat-caret">▌</span>
<textarea id="input" rows="1" placeholder="⚒ Принципал>   напиши запрос — Ctrl+Enter отправить, Enter перенос, @file путь прикрепить файл" autocomplete="off" autofocus></textarea>
<button onclick="send()" id="btn">⚒ Forge ➤</button>
<button class="dm-btn" style="margin-left:6px" onclick="toggleAbout()">⚙ About</button>
</div>
<div class="input-hint">💡 Ctrl+Enter — отправить · Enter — перенос строки · Shift+Enter — перенос строки · кликни чип сверху — сменишь модель · ⚙ Team Mode — мульти-выбор до 6 агентов сразу</div>
</div>
</div>
</div>
<div class="file-viewer" id="fileViewer">
<div class="file-viewer-header">
<span class="path" id="fileViewerPath">—</span>
<button class="tool" onclick="toolLSP()">⚒ LSP</button>
<button class="tool" onclick="closeFileViewer()">✗ Close</button>
</div>
<div class="file-viewer-body" id="fileViewerBody">Click a file in the sidebar to view its content.</div>
</div>
<script>
// Chat input auto-grow + Enter-to-send. Keeps the prompt comfortable
// when the user types long forms, and binds Enter to send (Shift+Enter
// for newline) so the chat behaves like OpenCode/TUI.
document.addEventListener('DOMContentLoaded', function(){
  var ta = document.getElementById('input');
  if(!ta) return;
  function autosize(){
    ta.style.height = 'auto';
    ta.style.height = Math.min(180, Math.max(48, ta.scrollHeight)) + 'px';
  }
  ta.addEventListener('input', autosize);
  autosize();
  ta.addEventListener('keydown', function(e){
    if(e.key === 'Enter' && !e.shiftKey){ e.preventDefault(); send(); }
  });
});

var ctxFiles = [];
var chatHistory = [];

async function send() {
var input = document.getElementById('input');
var btn = document.getElementById('btn');
var msg = input.value.trim();
if(!msg) return;

// Parse @mentions
var mentions = msg.match(/@(\S+)/g) || [];
mentions.forEach(function(m) {
var path = m.substring(1);
if(!ctxFiles.includes(path)) ctxFiles.push(path);
updateContext();
});
msg = msg.replace(/@\S+/g, '').trim();
if(!msg) { input.value=''; return; }

addMsg('user', msg, 'Принципал');
attachEditButtons();
input.value = '';
input.style.height = 'auto';
localStorage.removeItem('forge_input_draft');
btn.disabled = true;
setAgent('thinking');
var chatStatus = document.getElementById('chatStatus');
if(chatStatus){ chatStatus.className='chat-status busy'; chatStatus.textContent='⚙ исполняю…'; }
function chatReady(){
  btn.disabled = false; input.focus();
  var cs = document.getElementById('chatStatus');
  if(cs){ cs.className='chat-status'; cs.textContent='→ готов к приёму команд'; }
}

var typing = addMsg('assistant', '<span class="typing">⚒ ' + (currentAgent === 'magos' ? 'Магос' : currentAgent) + ' думает...</span>', 'Анафемтрон');
var bubble = typing.querySelector('.msg-bubble');

// Streaming state
var streamBuf = '';
var streamDirty = false;
var rafId = null;
function flushBuf() {
  if(!streamDirty) return;
  rafId = null;
  // Render raw text without escaping (we trust server output for SSE)
  // Use a placeholder so formatContent runs once at the end on the full text.
  bubble.innerHTML = formatContent(streamBuf) + '<span class="typing">▌</span>';
  scrollDown();
  streamDirty = false;
}
function pushToken(text) {
  streamBuf += text;
  streamDirty = true;
  if(rafId === null) rafId = requestAnimationFrame(flushBuf);
}

try {
// Team mode: hit /api/parallel with the selected roster and render
// each agent's reply as its own card inside the message bubble.
if(teamMode && selectedAgents.size >= 2) {
  var teamList = selectedList();
  typing.querySelector('.msg-agent').textContent = '⚒ Team (' + teamList.length + ')';
  typing.querySelector('.msg-bubble').innerHTML = '<span class="typing">⚙ Команда из ' + teamList.length + ' агентов работает…</span>';
  try {
    var pr = await fetch('/api/parallel', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({message: msg, agents: teamList, files: ctxFiles})
    });
    var pd = await pr.json();
    var results = pd.results || [];
    var html = '<div class="team-grid">';
    results.forEach(function(r){
      var glyph = r.glyph || '◆';
      var disp = r.display || r.agent || 'agent';
      var color = r.color || '#ffb000';
      var body = r.error ? ('<span style="color:var(--crimson)">☠ ' + r.error + '</span>') : formatContent(r.content || '(пусто)');
      html += '<div class="team-card" style="border-left:3px solid ' + color + '">'
            +  '<div class="team-card-head"><span style="color:' + color + '">' + glyph + '</span> '
            +  '<b>' + disp + '</b> <span class="dim">' + (r.model||'') + '</span></div>'
            +  '<div class="team-card-body">' + body + '</div>'
            +  '</div>';
    });
    html += '</div>';
    if(results.length === 0) html = '<span style="color:var(--crimson)">☠ Team: пустой ответ</span>';
    bubble.innerHTML = html;
    chatHistory.push({user:msg, bot:results.map(function(r){return '[' + r.agent + '] ' + (r.content||'');}).join('\n\n')});
  } catch(e) {
    bubble.innerHTML = '<span style="color:var(--crimson)">☠ Team error: ' + e + '</span>';
  }
  setAgent('ready');
  chatReady();
  scrollDown();
  return;
}

var body = {message: msg, agent: currentAgent};
if(ctxFiles.length > 0) body.files = ctxFiles;
var resp = await fetch('/api/chat/stream', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
if(!resp.ok || !resp.body) {
  // Fallback to blocking endpoint on stream failure
  var r2 = await fetch('/api/chat', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  var d2 = await r2.json();
  var content2 = d2.response || d2.text || '(пусто)';
  bubble.innerHTML = formatContent(content2);
  setAgent('ready');
  chatHistory.push({user:msg, bot:content2});
  chatReady();
  return;
}

var reader = resp.body.getReader();
var decoder = new TextDecoder();
var rawBuf = '';
var eventType = '';
var fullReply = '';
while(true) {
  var rd = await reader.read();
  if(rd.done) break;
  rawBuf += decoder.decode(rd.value, {stream:true});
  // SSE: events separated by \n\n; lines start with 'event:' or 'data:'
  var idx;
  while((idx = rawBuf.indexOf('\n\n')) !== -1) {
    var frame = rawBuf.slice(0, idx);
    rawBuf = rawBuf.slice(idx + 2);
    var ev = '', dt = '';
    frame.split('\n').forEach(function(line) {
      if(line.startsWith('event:')) ev = line.slice(6).trim();
      else if(line.startsWith('data:')) dt += line.slice(5).trim();
    });
    if(ev === 'token' && dt) {
      try { var word = JSON.parse(dt); pushToken(word); fullReply += word; }
      catch(e) { pushToken(dt); fullReply += dt; }
    } else if(ev === 'status') {
      // ignore intermediate status
    } else if(ev === 'error') {
      bubble.innerHTML = '<span style="color:var(--crimson)">☠ ' + dt + '</span>';
      setAgent('error');
      chatHistory.push({user:msg, bot:'[error] ' + dt});
      chatReady();
      return;
    } else if(ev === 'done') {
      // final flush
      if(rafId) { cancelAnimationFrame(rafId); rafId = null; }
      streamDirty = true; flushBuf();
      // strip trailing cursor
      bubble.innerHTML = formatContent(fullReply);
      setAgent('ready');
      chatHistory.push({user:msg, bot:fullReply});
      chatReady();
      scrollDown();
      return;
    }
  }
}
// stream ended without done event — finalize anyway
if(rafId) { cancelAnimationFrame(rafId); rafId = null; }
bubble.innerHTML = formatContent(fullReply);
setAgent('ready');
chatHistory.push({user:msg, bot:fullReply});
chatReady();
scrollDown();
} catch(e) {
if(rafId) cancelAnimationFrame(rafId);
bubble.innerHTML = '<span style="color:var(--crimson)">☠ Коррупция: ' + e + '</span>';
setAgent('error');
chatReady();
}
}

function highlightCode(code, lang) {
// Lightweight regex-based syntax highlighting. Returns HTML with
// <span class="tok-*"> wrappers around tokens. No dependencies.
//
// Recognised: Go, JavaScript/TypeScript, Python, Bash, JSON, YAML.
var esc = code.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
var patterns = [];
// Comments first (highest precedence for stripping).
patterns.push({re: /(\/\/[^\n]*)/g, cls: 'tok-com'});
patterns.push({re: /(\/\*[\s\S]*?\*\/)/g, cls: 'tok-com'});
patterns.push({re: /(#.*)/g, cls: 'tok-com'});
// Strings.
patterns.push({re: /("(?:[^"\\]|\\.)*")/g, cls: 'tok-str'});
patterns.push({re: /('(?:[^'\\]|\\.)*')/g, cls: 'tok-str'});
// Numbers.
patterns.push({re: /\b(\d+(?:\.\d+)?)\b/g, cls: 'tok-num'});
// Keywords (Go + JS + Python + Bash common subset).
var keywords = ['func','var','let','const','if','else','for','while','return','import','package','from','as','class','def','self','this','nil','null','undefined','true','false','True','False','None','async','await','try','catch','throw','finally','new','delete','typeof','instanceof','switch','case','break','continue','do','echo','fi','done','then','elif','except','lambda','yield','export','default'];
var kwRe = new RegExp('\\b(' + keywords.join('|') + ')\\b', 'g');
patterns.push({re: kwRe, cls: 'tok-kw'});
// Built-in types.
var types = ['string','int','int64','int32','float64','float32','bool','byte','rune','error','interface','struct','map','chan','go','any','never','void','number','boolean','object','symbol'];
var tyRe = new RegExp('\\b(' + types.join('|') + ')\\b', 'g');
patterns.push({re: tyRe, cls: 'tok-ty'});
// Function calls.
patterns.push({re: /\b([a-zA-Z_][a-zA-Z0-9_]*)(?=\()/g, cls: 'tok-fn'});
// Markdown headers (we may have stray ones if language is unknown).
patterns.push({re: /^(#{1,6}\s.*)/gm, cls: 'tok-com'});
var html = esc;
patterns.forEach(function(p) {
  html = html.replace(p.re, function(m) { return '<span class="' + p.cls + '">' + m + '</span>'; });
});
return html;
}

function formatContent(text) {
// Split team/dyad envelope into trace and final answer so the UI can style them differently.
var envelope = /^(--- (?:Council Discussion|Inner Dialogue) ---)\n([\s\S]*?)\n---\n\n([\s\S]*)$/;
var m = envelope.exec(text);
if (m) {
  return '<div class="council-trace-header">' + m[1] + '</div>'
    + '<div class="council-trace">' + formatBody(m[2]) + '</div>'
    + '<div class="final-answer">' + formatBody(m[3]) + '</div>';
}
return formatBody(text);
}

function formatBody(text) {
// 1. Escape HTML globally first to make further regex safe.
text = text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

// 2. SEARCH/REPLACE diff blocks — render as actionable diff panel.
// Must run BEFORE code blocks because the equals signs in the fence
// can confuse fenced-code detection.
text = text.replace(/&lt;&lt;&lt;&lt;&lt;&lt;&lt; SEARCH([\s\S]*?)=======([\s\S]*?)&gt;&gt;&gt;&gt;&gt;&gt;&gt; REPLACE/g, function(m,old,newc) {
  var o = old.trim(), n = newc.trim();
  var ob = btoa(unescape(encodeURIComponent(o)));
  var nb = btoa(unescape(encodeURIComponent(n)));
  return '<div class="diff-panel" data-old-b64="' + ob + '" data-new-b64="' + nb + '">'
    + '<div class="diff-header">⚒ SEARCH/REPLACE</div>'
    + '<div class="diff-del">- ' + o + '</div>'
    + '<div class="diff-add">+ ' + n + '</div>'
    + '<div class="diff-actions">'
    + '<button class="diff-apply" onclick="applyDiff(this)">✓ Apply to @file</button>'
    + '<button class="diff-reject" onclick="rejectDiff(this)">✗ Reject</button>'
    + '</div>'
    + '</div>';
});

// 3. Fenced code blocks (triple backtick + lang + ...) — extract BEFORE
//    markdown so we do not mess with their content. Re-add as highlighted
//    pre/code elements below.
text = text.replace(/\x60\x60\x60(\w*)\n([\s\S]*?)\x60\x60\x60/g, function(m,lang,code) {
  var langLabel = lang ? lang.toLowerCase() : '';
  var highlighted = highlightCode(code, langLabel);
  return '<pre class="code-block" data-lang="' + langLabel + '"><button class="code-copy" onclick="copyCode(this)">Copy</button><div class="code-lang">' + (langLabel || 'text') + '</div><code>' + highlighted + '</code></pre>';
});

// 4. Tables (must run before line-level rules).  Markdown pipe tables:
//      | a | b |
//      |---|---|
//      | 1 | 2 |
text = text.replace(/(^\|.+\|\n\|[ \-:|]+\|\n(?:\|.*\|\n?)+)/gm, function(m) {
  var rows = m.trim().split('\n');
  if (rows.length < 3) return m;
  var sep = rows[1];
  if (!/^\|?[\s\-:|]+\|?$/.test(sep)) return m;
  var aligns = sep.split('|').slice(1, -1).map(function(c){ return /:---:/.test(c) ? 'center' : (/---:/.test(c) ? 'right' : 'left'); });
  var head = rows[0].split('|').slice(1, -1).map(function(s){ return s.trim(); });
  var body = rows.slice(2).map(function(r){ return r.split('|').slice(1, -1).map(function(s){ return s.trim(); }); });
  var thead = '<thead><tr>' + head.map(function(h,i){ return '<th style="text-align:' + aligns[i] + '">' + h + '</th>'; }).join('') + '</tr></thead>';
  var tbody = '<tbody>' + body.map(function(r){ return '<tr>' + r.map(function(c,i){ return '<td style="text-align:' + aligns[i] + '">' + c + '</td>'; }).join('') + '</tr>'; }).join('') + '</tbody>';
  return '<table class="md-table">' + thead + tbody + '</table>';
});

// 5. Headers — must be at line start.
text = text.replace(/^###### (.+)$/gm, '<h6>$1</h6>');
text = text.replace(/^##### (.+)$/gm, '<h5>$1</h5>');
text = text.replace(/^#### (.+)$/gm, '<h4>$1</h4>');
text = text.replace(/^### (.+)$/gm, '<h3>$1</h3>');
text = text.replace(/^## (.+)$/gm, '<h2>$1</h2>');
text = text.replace(/^# (.+)$/gm, '<h1>$1</h1>');

// 6. Horizontal rules.
text = text.replace(/^---+\s*$/gm, '<hr/>');
text = text.replace(/^\*\*\*+\s*$/gm, '<hr/>');

// 7. Blockquotes (line-based, allow multiple lines).
text = text.replace(/(^&gt; .+(?:\n&gt; .+)*)/gm, function(m) {
  var inner = m.split('\n').map(function(l){ return l.replace(/^&gt; /, ''); }).join('<br/>');
  return '<blockquote>' + inner + '</blockquote>';
});

// 8. Lists (unordered and ordered, line-based; tolerant to indentation).
//    We process line by line over the whole text.
var lines = text.split('\n');
var outLines = [];
var inUL = false, inOL = false;
for (var i = 0; i < lines.length; i++) {
  var line = lines[i];
  var ulMatch = /^(\s*)[-*+] (.+)/.exec(line);
  var olMatch = /^(\s*)\d+\. (.+)/.exec(line);
  if (ulMatch) {
    if (!inUL) { outLines.push('<ul>'); inUL = true; inOL = false; }
    outLines.push('<li>' + ulMatch[2] + '</li>');
  } else if (olMatch) {
    if (!inOL) { outLines.push('<ol>'); inOL = true; inUL = false; }
    outLines.push('<li>' + olMatch[2] + '</li>');
  } else {
    if (inUL) { outLines.push('</ul>'); inUL = false; }
    if (inOL) { outLines.push('</ol>'); inOL = false; }
    outLines.push(line);
  }
}
if (inUL) outLines.push('</ul>');
if (inOL) outLines.push('</ol>');
text = outLines.join('\n');

// 9. Inline formatting (order matters: strikethrough before bold/italic,
//    bold before italic, escape backslash not needed in our DOM).
text = text.replace(/~~(.+?)~~/g, '<del>$1</del>');
text = text.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
text = text.replace(/(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)/g, '<em>$1</em>');

// 10. Links [text](url).
text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');

// 11. Inline code (run AFTER other inline rules so it doesn't get mangled).
text = text.replace(/\x60([^\x60]+)\x60/g, '<code class="inline">$1</code>');

return text;
}

// Inline diff action handlers — apply SEARCH/REPLACE block to last @file mention.
async function applyDiff(btn) {
var panel = btn.closest('.diff-panel');
if(!panel) return;
if(ctxFiles.length === 0) {
btn.parentElement.innerHTML = '<span style="color:var(--crimson)">☠ Упомяни файл через @path в чате</span>';
return;
}
var file = ctxFiles[ctxFiles.length-1];
var oldText = decodeURIComponent(escape(atob(panel.dataset.oldB64)));
var newText = decodeURIComponent(escape(atob(panel.dataset.newB64)));
btn.disabled = true; btn.textContent = '⏳ Applying...';
try {
var r = await fetch('/api/edit/batch', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({edits:[{path:file,old_text:oldText,new_text:newText}],dry_run:false})});
var d = await r.json();
if(d.failed && d.failed.length > 0) {
btn.parentElement.innerHTML = '<span style="color:var(--crimson)">☠ Apply failed: ' + (d.failed[0].error || 'unknown') + '</span>';
return;
}
btn.parentElement.innerHTML = '<span style="color:var(--green)">✓ Applied to ' + file + ' (' + (d.applied?d.applied.length:0) + ' edit)</span>';
} catch(e) {
btn.parentElement.innerHTML = '<span style="color:var(--crimson)">☠ Коррупция: ' + e + '</span>';
}
}

function rejectDiff(btn) {
var panel = btn.closest('.diff-panel');
if(panel) panel.style.opacity = '0.3';
if(btn) btn.textContent = '✗ Rejected';
}

function addMsg(role, content, agent) {
var div = document.createElement('div');
div.className = 'msg ' + role;
var agentClass = role === 'assistant' ? 'badge-magos' : '';
div.innerHTML = '<div class="msg-agent"><span class="agent-badge ' + agentClass + '">' + (agent||'') + '</span></div><div class="msg-bubble">' + content + '</div>';
document.getElementById('messages').appendChild(div);
scrollDown();
return div;
}

function updateContext() {
var bar = document.getElementById('ctxBar');
bar.innerHTML = ctxFiles.map(function(f,i) {
return '<span class="ctx-file">' + f + '<span class="remove" onclick="removeCtx(' + i + ')">✗</span></span>';
}).join('');
}

function removeCtx(i) {
ctxFiles.splice(i, 1);
updateContext();
}

function setAgent(state) {
var el = document.getElementById('agent-status');
if(state === 'thinking') el.innerHTML = '⚙ Magos думает...';
else if(state === 'ready') el.innerHTML = '✓ Magos ready';
else if(state === 'error') el.innerHTML = '☠ Error';
}

function scrollDown() {
var m = document.getElementById('messages');
m.scrollTop = m.scrollHeight;
}

// Copy code block to clipboard.
async function copyCode(btn) {
var code = btn.closest('.code-block').querySelector('code');
if(!code) return;
try {
  await navigator.clipboard.writeText(code.textContent);
  var orig = btn.textContent;
  btn.textContent = '✓ Copied';
  setTimeout(function(){ btn.textContent = orig; }, 1500);
} catch(e) {}
}

// Tool functions
async function toolGrep() {
var q = prompt('Поиск (grep):');
if(!q) return;
var r = await fetch('/api/tools/grep?q=' + encodeURIComponent(q) + '&ext=go');
var d = await r.json();
addMsg('assistant', '☉ Grep: <b>' + q + '</b> → ' + d.count + ' files<br>' + (d.files||[]).slice(0,10).map(function(f){return '  ' + f}).join('<br>'), 'Skitarii');
}

async function toolBash() {
var cmd = prompt('Bash команда:');
if(!cmd) return;
var r = await fetch('/api/tools/bash', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({command:cmd})});
var d = await r.json();
addMsg('assistant', '⚙ Bash: <code>' + cmd + '</code> (exit=' + d.exit_code + ')<pre>' + (d.stdout||'').trim() + '</pre>', 'Servitor');
}

async function toolMap() {
var r = await fetch('/api/repo-map');
var d = await r.json();
addMsg('assistant', '☉ Repo Map:<pre>' + (d.map||'(пусто)') + '</pre>', 'Skitarii');
}

async function toolCommit() {
var msg = prompt('Commit message:');
if(!msg) return;
var r = await fetch('/api/commit', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:msg})});
var d = await r.json();
addMsg('assistant', '✓ Committed: ' + (d.hash||d.message||'OK'), 'Servitor');
}

async function toolUndo() {
var r = await fetch('/api/undo', {method:'POST'});
var d = await r.json();
addMsg('assistant', '↩ Undo: ' + (d.message||'OK'), 'Servitor');
}

// Load files
fetch('/api/files').then(r=>r.json()).then(files => {
var list = document.getElementById('fileList');
if(!files || !files.length) { list.innerHTML='<div class="file-item">Нет файлов</div>'; return; }
list.innerHTML = files.slice(0,80).map(function(f) {
return '<div class="file-item" onclick="openFile(\x27' + f.path + '\x27)" title="Click to view, /api/edit/batch to modify">' + f.path + '</div>';
}).join('');
}).catch(()=>{ document.getElementById('fileList').innerHTML='<div class="file-item">Ошибка</div>'; });

// State hoisted to the top of the page bootstrap so refreshTeamUI() /
// loadSessions() (called right after this block) can read selectedAgents
// without throwing — otherwise the rest of the script (pollStats, send,
// chip handlers) is skipped because the first frame aborted.
var teamMode = false;
var selectedAgents = new Set(['magos']);
var currentAgent = 'magos';

// Load sessions on page load.
loadSessions();
refreshTeamUI();

// Live stats poller — updates the FOUR stat cards every 5 s with REAL
// data from /api/status + /api/agents. Every card shows: a value, an
// optional delta vs the previous poll, and a "HH:MM:SS ago" timestamp
// so the user can see at a glance the dashboard is genuinely live
// (not hardcoded). Polling pauses while the tab is hidden and
// resumes on visibility change. Triggers an immediate fetch on load.
var pollStatsPrev = { local: null, cloud: null, total: null, active: null, sessions: null, repo: null };
async function pollStats() {
  var now = new Date();
  var stamp = (now.getHours()<10?'0':'') + now.getHours() + ':' +
              (now.getMinutes()<10?'0':'') + now.getMinutes() + ':' +
              (now.getSeconds()<10?'0':'') + now.getSeconds();
  function setVal(id, val, prev) {
    var el = document.getElementById(id);
    if (!el) return;
    el.textContent = val;
    el.classList.add('live');
    el.classList.remove('down');
    var deltaEl = document.getElementById(id.replace('Value','Delta'));
    if (deltaEl && prev !== null && prev !== undefined) {
      var d = val - prev;
      deltaEl.textContent = d === 0 ? '· steady' : (d > 0 ? '▲ +' + d : '▼ ' + d);
      deltaEl.className = 'dm-stat-delta ' + (d === 0 ? '' : (d > 0 ? 'up' : 'down'));
    } else if (deltaEl) {
      deltaEl.textContent = '· live';
      deltaEl.className = 'dm-stat-delta';
    }
  }
  function setStamp(id, txt) {
    var el = document.getElementById(id);
    if (el) el.textContent = '⟳ ' + txt;
  }

  // /api/agents → SWARM (local/cloud counts) + ROSTER (total/active)
  try {
    var ra = await fetch('/api/agents?_=' + Date.now());
    var agents = await ra.json();
    if (Array.isArray(agents)) {
      var cloud = agents.filter(function(a){ return a.type === 'cloud' && a.active; }).length;
      var local = agents.filter(function(a){ return a.type === 'local' && a.active; }).length;
      var active = agents.filter(function(a){ return a.active; }).length;
      var total = agents.length;
      setVal('statSwarmValue', local, pollStatsPrev.local);
      setVal('statRosterValue', total + ' (' + active + ')', pollStatsPrev.total);
      pollStatsPrev.local = local; pollStatsPrev.total = total; pollStatsPrev.active = active;
      setStamp('statSwarmStamp', stamp);
      setStamp('statRosterStamp', stamp);
    }
  } catch (e) {
    var sw = document.getElementById('statSwarmValue');
    if (sw) { sw.classList.add('down'); sw.classList.remove('live'); sw.textContent = '☠ offline'; }
    setStamp('statSwarmStamp', 'failed');
  }

  // /api/status → REPO (path tail) + STATUS (active|offline)
  try {
    var rs = await fetch('/api/status?_=' + Date.now());
    var d = await rs.json();
    if (d) {
      var path = d.repo || '';
      var tail = path.split('/').filter(Boolean).pop() || '—';
      setVal('statRepoValue', tail, pollStatsPrev.repo);
      var sVal = (d.status || 'unknown') + ' · ' + (d.forge || '—');
      setVal('statStatusValue', sVal, pollStatsPrev.sessions);
      pollStatsPrev.repo = tail;
      pollStatsPrev.sessions = d.status || '';
      setStamp('statRepoStamp', stamp);
      setStamp('statStatusStamp', stamp);
    }
  } catch (e) {
    var st = document.getElementById('statStatusValue');
    if (st) { st.classList.add('down'); st.classList.remove('live'); st.textContent = '☠ offline'; }
    setStamp('statStatusStamp', 'failed');
  }
}
pollStats();
setInterval(pollStats, 5000);
// Resume / re-fetch immediately when the tab becomes visible again.
document.addEventListener('visibilitychange', function(){ if (!document.hidden) pollStats(); });

// Hero FAB action — opens an about/help modal (placeholder for future use).
function toggleAbout() {
  var m = document.getElementById('aboutModal');
  if (m) m.classList.toggle('open');
}
function closeAbout() {
  var m = document.getElementById('aboutModal');
  if (m) m.classList.remove('open');
}
document.addEventListener('keydown', function(e){ if (e.key === 'Escape') closeAbout(); });

function addFile(path) {
if(!ctxFiles.includes(path)) { ctxFiles.push(path); updateContext(); }
}

// Open file viewer panel and load file content from /api/tools/read.
async function openFile(path) {
var viewer = document.getElementById('fileViewer');
var body = document.getElementById('fileViewerBody');
var header = document.getElementById('fileViewerPath');
header.textContent = path;
body.textContent = '⏳ Loading ' + path + ' …';
viewer.classList.add('open');
addFile(path);
try {
  var r = await fetch('/api/tools/read', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path:path})});
  var d = await r.json();
  if(d.error) { body.innerHTML = '<span style="color:var(--crimson)">☠ ' + d.error + '</span>'; return; }
  var esc = (d.content || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  body.innerHTML = '<div style="color:var(--dim);font-size:10px;margin-bottom:8px">' + (d.size||0) + ' bytes</div><pre style="margin:0">' + esc + '</pre>';
} catch(e) {
  body.innerHTML = '<span style="color:var(--crimson)">☠ ' + e + '</span>';
}
}

function closeFileViewer() {
document.getElementById('fileViewer').classList.remove('open');
}

// Run gopls diagnostics on the currently open file.
async function toolLSP() {
var path = document.getElementById('fileViewerPath').textContent;
if(!path || path === '—') { alert('Open a file first'); return; }
var body = document.getElementById('fileViewerBody');
var diagDiv = document.createElement('div');
diagDiv.id = 'diagPanel';
diagDiv.innerHTML = '<div style="color:var(--amber);margin-top:12px">⚒ gopls check ' + path + ' …</div>';
body.appendChild(diagDiv);
try {
  var r = await fetch('/api/lsp/diagnostics?path=' + encodeURIComponent(path));
  var d = await r.json();
  if(d.message) { diagDiv.innerHTML = '<div class="diag-item info">' + d.message + '</div>'; return; }
  var diags = d.diagnostics || [];
  if(!diags.length) { diagDiv.innerHTML = '<div class="diag-item info">✓ No diagnostics. Занесено.</div>'; return; }
  diagDiv.innerHTML = '<div style="color:var(--amber);margin:8px 0 4px">⚒ ' + diags.length + ' diagnostic(s):</div>' +
    diags.map(function(x) {
      var sev = (x.severity || 'info').toLowerCase();
      var src = x.source ? '<div class="diag-source">' + x.source + (x.code?' ['+x.code+']':'') + '</div>' : '';
      return '<div class="diag-item ' + sev + '"><b>[' + sev + ']</b> ' + (x.message || JSON.stringify(x)) + src + '</div>';
    }).join('');
} catch(e) {
  diagDiv.innerHTML = '<div class="diag-item">☠ ' + e + '</div>';
}
}

// Agent chip switching + multi-select team mode
// (state vars — teamMode, selectedAgents, currentAgent — hoisted above
// loadSessions() so the initial bootstrap doesn't throw.)

function selectedList() {
  // Stable order matching the 6-selectable roster (user's subscription).
  // Dyad is included so the user can pick Fable+Mythos as a unified mind.
  var order = ['magos','fable-dominus','mythos-philosophus','Dyad-medium','servitor','skitarii','kimi','mimo','glm','qwen','qwen_flagship','minimax'];
  return order.filter(function(a){ return selectedAgents.has(a); });
}

function refreshTeamUI() {
  var list = selectedList();
  var info = document.getElementById('teamCount');
  if(info) info.textContent = list.length + '/6 selected';
  document.querySelectorAll('.chip[data-agent]').forEach(function(c){
    var a = c.dataset.agent;
    if(selectedAgents.has(a)) c.classList.add('active');
    else c.classList.remove('active');
  });
  // Single-agent default = Magos
  currentAgent = (list.length === 1) ? list[0] : 'magos';
}

function setAgent(name, btn) {
  if(teamMode) {
    // Toggle in/out of the team
    if(selectedAgents.has(name)) selectedAgents.delete(name);
    else {
      if(selectedAgents.size >= 6) { alert('Максимум 6 агентов в команде'); return; }
      selectedAgents.add(name);
    }
  } else {
    // Single-agent mode: clear team, just this one
    selectedAgents.clear();
    selectedAgents.add(name);
  }
  refreshTeamUI();
}

function clearTeam() {
  selectedAgents.clear();
  refreshTeamUI();
}

function toggleTeamMode() {
  teamMode = !teamMode;
  var btn = document.getElementById('teamMode');
  if(btn) btn.classList.toggle('active', teamMode);
  if(!teamMode && selectedAgents.size > 1) {
    // Drop down to single agent when leaving team mode
    selectedAgents.clear();
    selectedAgents.add(currentAgent || 'magos');
  }
  refreshTeamUI();
}

// Session management
async function loadSessionMessages() {
try {
  var r = await fetch('/api/session/messages');
  var messages = await r.json();
  if(!Array.isArray(messages)) messages = [];
  renderMessages(messages);
} catch(e) {}
}

function renderMessages(messages) {
var container = document.getElementById('messages');
if(!container) return;
var existing = container.querySelectorAll('.msg');
for(var i = 0; i < existing.length; i++) existing[i].remove();
if(!messages || messages.length === 0) {
  addMsg('assistant', 'Принципал. Forge v2.0 активен (W4 — local 2-model swarm: Fable-Dominus + Mythos-Philosophus). Vox Dei 12B в кладовке. TUI: Bubble Tea. Tools: Read/Write/Edit/Bash/Glob/Grep. Diff view + Repo Map + Sessions + VolitionCage. Занесено.', '☉ Анафемтрон');
  return;
}
messages.forEach(function(m) {
  if(m.role === 'user') {
    addMsg('user', formatContent(m.content), 'Принципал');
  } else if(m.role === 'assistant') {
    addMsg('assistant', formatContent(m.content), m.agent || 'Анафеметрон');
  }
});
attachEditButtons();
}
});
}

function filterHistory(query) {
var q = (query || '').toLowerCase().trim();
var msgs = document.querySelectorAll('#messages .msg');
msgs.forEach(function(el) {
  var text = el.textContent.toLowerCase();
  el.style.display = (!q || text.indexOf(q) !== -1) ? '' : 'none';
});
}

function attachEditButtons() {
document.querySelectorAll('.msg-edit').forEach(function(b){ b.remove(); });
var userMsgs = document.querySelectorAll('#messages .msg.user');
if(userMsgs.length === 0) return;
var last = userMsgs[userMsgs.length - 1];
if(last.querySelector('.msg-edit')) return;
var btn = document.createElement('button');
btn.className = 'msg-edit';
btn.title = 'Редактировать и переотправить';
btn.textContent = '✎';
btn.onclick = function(){ editMessage(last); };
last.appendChild(btn);
}

async function editMessage(el) {
var bubble = el.querySelector('.msg-bubble');
if(!bubble) return;
var currentText = bubble.textContent;
var newText = prompt('Редактировать сообщение:', currentText);
if(newText === null || newText.trim() === '' || newText === currentText) return;
try {
  await fetch('/api/session/edit_last', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({message: newText.trim()})
  });
  await loadSessions();
  var input = document.getElementById('input');
  input.value = newText.trim();
  resizeTextarea();
  send();
} catch(e) {}
}

async function loadSessions() {
try {
  var r = await fetch('/api/session/list');
  var sessions = await r.json();
  var sel = document.getElementById('sessionSelect');
  if(!sel) return;
  if(!Array.isArray(sessions)) sessions = [];
  sel.innerHTML = sessions.map(function(s) {
    return '<option value="' + s.id + '"' + (s.current ? ' selected' : '') + '>' + s.name + ' (' + (s.messages||0) + ' msg)</option>';
  }).join('');
  await loadSessionMessages();
} catch(e) {}
}

async function newSession() {
var name = prompt('Имя сессии:', 'session-' + new Date().toISOString().slice(0,16).replace('T','-'));
if(!name) return;
await fetch('/api/session/new', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name})});
await loadSessions();
}

async function switchSession(id) {
if(!id) return;
await fetch('/api/session/switch?id=' + encodeURIComponent(id));
await loadSessions();
}

async function clearSession() {
if(!confirm('Очистить текущую сессию?')) return;
await fetch('/api/session/clear');
await loadSessions();
}

// Keyboard
// Autosave input draft and auto-resize textarea so long prompts stay readable.
function saveInputDraft() {
  localStorage.setItem('forge_input_draft', document.getElementById('input').value);
}
function loadInputDraft() {
  var saved = localStorage.getItem('forge_input_draft');
  if(saved !== null) document.getElementById('input').value = saved;
}
function resizeTextarea() {
  var ta = document.getElementById('input');
  if(!ta) return;
  ta.style.height = 'auto';
  ta.style.height = Math.min(240, ta.scrollHeight) + 'px';
}

document.getElementById('input').addEventListener('input', function() { saveInputDraft(); resizeTextarea(); });
document.getElementById('input').addEventListener('keydown', function(e) {
if(e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
  e.preventDefault();
  send();
}
if(e.key === '@') {
// Simple @ autocomplete could go here
}
});

document.getElementById('input').focus();
loadInputDraft();
resizeTextarea();

// Drag-and-drop file paths to attach context.
document.body.addEventListener('dragover', function(e) { e.preventDefault(); document.body.classList.add('drag-over'); });

// === DASHBOARD-STYLE ENHANCEMENTS ===
// Toast notifications (like dashboard)
(function() {
  if (!document.getElementById('dm-toast-container')) {
    const c = document.createElement('div');
    c.id = 'dm-toast-container';
    c.style.cssText = 'position:fixed;bottom:60px;right:20px;z-index:5000;display:flex;flex-direction:column;gap:6px;pointer-events:none;';
    document.body.appendChild(c);
  }
})();

function showToast(text, kind) {
  kind = kind || 'info';
  const t = document.createElement('div');
  const colors = { info: '#3a6090', success: '#5a7c48', warning: '#c75b39', error: '#a33' };
  t.style.cssText = 'background:#0a0a0a;border:1px solid ' + (colors[kind] || colors.info) + ';color:#b8a060;padding:8px 14px;font-family:JetBrains Mono,monospace;font-size:11px;letter-spacing:0.05em;pointer-events:auto;animation:dm-toast-in 0.2s ease-out;';
  t.textContent = text;
  document.getElementById('dm-toast-container').appendChild(t);
  setTimeout(function() { t.style.opacity = '0'; t.style.transition = 'opacity 0.3s'; setTimeout(function() { t.remove(); }, 300); }, 3000);
}

// Smooth modal transitions
(function() {
  const s = document.createElement('style');
  s.textContent = '@keyframes dm-fade-in{from{opacity:0}to{opacity:1}}@keyframes dm-toast-in{from{transform:translateX(20px);opacity:0}to{transform:translateX(0);opacity:1}}@keyframes dm-pulse-in{from{transform:scale(0.9);opacity:0}to{transform:scale(1);opacity:1}}.dm-modal-backdrop{animation:dm-fade-in 0.15s ease-out}.dm-modal{animation:dm-pulse-in 0.2s ease-out}.dm-toast-container>div{box-shadow:0 2px 8px rgba(0,0,0,0.5);cursor:pointer}.dm-toast-container>div:hover{background:#000;border-color:#b8a060;color:#b8a060}';
  document.head.appendChild(s);
})();

// Keyboard shortcuts
document.addEventListener('keydown', function(e) {
  // Ctrl+K → command palette (focus input)
  if (e.ctrlKey && !e.shiftKey && !e.altKey && e.key === 'k') {
    e.preventDefault();
    var inp = document.getElementById('input');
    if (inp) inp.focus();
    return;
  }
  // Ctrl+/ or ? → help dialog
  if ((e.ctrlKey && e.key === '/') || (!e.ctrlKey && e.key === '?' && e.target.tagName !== 'INPUT' && e.target.tagName !== 'TEXTAREA')) {
    e.preventDefault();
    showHelp();
    return;
  }
  // Esc → close topmost modal
  if (e.key === 'Escape') {
    const modals = document.querySelectorAll('.dm-modal-backdrop.open');
    if (modals.length > 0) {
      modals[modals.length - 1].classList.remove('open');
      e.preventDefault();
    }
  }
});

// Help dialog
function showHelp() {
  const existing = document.getElementById('helpModal');
  if (existing) existing.classList.add('open');
  else {
    const html = '<div class="dm-modal-backdrop" id="helpModal" onclick="if(event.target===this)this.classList.remove('open')">' +
      '<div class="dm-modal" style="max-width:520px;">' +
      '<h3>⌨ Keyboard shortcuts</h3>' +
      '<p><b>Ctrl+Enter</b> — отправить сообщение</p>' +
      '<p><b>Ctrl+K</b> — focus input (Cursor-style)</p>' +
      '<p><b>Ctrl+/</b> or <b>?</b> — this help</p>' +
      '<p><b>Esc</b> — close topmost modal</p>' +
      '<p><b>Enter</b> (in @-mention popup) — accept suggestion</p>' +
      '<p><b>↑↓</b> (in @-mention popup) — navigate</p>' +
      '<p style="margin-top:12px;color:#5a4a3a;font-size:10px;">v2.0.0-FORGE · Dark Mechanicus</p>' +
      '<div class="close-btn" onclick="document.getElementById('helpModal').classList.remove('open')">Close</div>' +
      '</div></div>';
    document.body.insertAdjacentHTML('beforeend', html);
    requestAnimationFrame(function() { document.getElementById('helpModal').classList.add('open'); });
  }
}

document.body.addEventListener('dragleave', function(e) { document.body.classList.remove('drag-over'); });
document.body.addEventListener('drop', function(e) {
  e.preventDefault();
  document.body.classList.remove('drag-over');
  var data = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
  if(!data) return;
  data.split('\n').map(function(s){ return s.trim(); }).filter(Boolean).forEach(function(line){
    var path = line.replace(/^file:\/\//i, '').trim();
    if(path && ctxFiles.indexOf(path) === -1) ctxFiles.push(path);
  });
  updateContext();
});
</script>
<div class="dm-modal-backdrop" id="aboutModal" onclick="if(event.target===this)closeAbout()">
<div class="dm-modal">
<h3>⚒ DARK FORGE — AI Coding Forge</h3>
<p>W4 forge dashboard. The 2-model local swarm (Fable-Dominus on GPU 0, Mythos-Philosophus on GPU 1) handles local chat. Vox Dei 12B is в кладовке per directive. Cloud specialists (MiniMax M3, Kimi K2.7, MiMo V2.5, GLM-5.2) are active on the paid subscription.</p>
<p>Pick a single chip → single-agent stream; toggle Team Mode → multi-select up to 6 agents that work the same question in parallel. Pick the gold Dyad chip → Fable acts, Mythos reflects, synthesizer unifies (4 think-levels: low / medium / high / max).</p>
<p>Use the sidebar Files → click any path to open the file viewer, or @mention in chat to attach context. Inline SEARCH/REPLACE blocks ship with Apply/Reject buttons backed by /api/edit/batch.</p>
<p style="color:var(--dm-gold-dim);font-size:11px">Press Esc or click outside to close. Forge v2.0.0-forge · Anaphemethron soul · Занесено.</p>
<button class="close-btn" onclick="closeAbout()">✗ Close</button>
</div>
</div>
</body>
</html>`

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FilePath string      `json:"filePath"`
		Edits    []edit.Edit `json:"edits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	preview, err := edit.Preview(req.FilePath, req.Edits)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"preview": preview})
}

func (s *Server) handleEditBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Edits   []edit.FileEdit `json:"edits"`
		DryRun  bool            `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve relative paths against repo root
	resolved := make([]edit.FileEdit, 0, len(req.Edits))
	for _, e := range req.Edits {
		path := e.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.cfg.RepoPath, path)
		}
		resolved = append(resolved, edit.FileEdit{
			Path:    path,
			OldText: e.OldText,
			NewText: e.NewText,
		})
	}

	result := edit.ApplyBatch(edit.BatchEdit{Edits: resolved}, req.DryRun)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Message == "" {
		req.Message = "feat(forge): auto-commit"
	}
	hash, err := s.git.AutoCommit(req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.sessions.AddMessage("assistant", fmt.Sprintf("✓ Committed: %s — %s", hash, req.Message), "Servitor")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"hash": hash})
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.git.Undo(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.sessions.AddMessage("assistant", "↩ Undo: последнее действие отменено.", "Servitor")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "undone"})
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	diff, err := s.git.Diff()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"diff": diff})
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n := 10
	if v := r.URL.Query().Get("n"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	commits, err := s.git.Log(n)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"commits": commits})
}

// ═══════════════════════════════════════════════════════════════════════
// SKITARII ENDPOINTS — Explore + Search
// ═══════════════════════════════════════════════════════════════════════

func (s *Server) handleExplore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.File == "" {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}

	log.Printf("☉ Skitarii exploring: %s", req.File)

	summary, err := s.dispatcher.Explore(req.File)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"summary": summary,
		"agent":   "skitarii",
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter 'q' required", http.StatusBadRequest)
		return
	}

	log.Printf("☉ Skitarii searching: %s", query)

	results, err := s.dispatcher.Search(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
		"agent":   "skitarii",
	})
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string   `json:"message"`
		Files   []string `json:"files,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}

	log.Printf("⚙ /api/dispatch: %s", req.Message)

	// Resolve file paths
	resolvedFiles := make([]string, 0, len(req.Files))
	for _, f := range req.Files {
		if !filepath.IsAbs(f) {
			candidate := filepath.Join(s.cfg.RepoPath, f)
			if _, err := os.Stat(candidate); err == nil {
				resolvedFiles = append(resolvedFiles, candidate)
				continue
			}
		}
		resolvedFiles = append(resolvedFiles, f)
	}

	resp, err := s.dispatcher.Handle(req.Message, resolvedFiles)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ═══ Sprint 2: Tool endpoints ═══

func (s *Server) handleToolsRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	result := tools.ReadFile(s.resolvePath(req.Path))
	log.Printf("☉ read: %s (%d bytes)", req.Path, result.Size)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleToolsWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// VolitionCage: check write permission
	permResult := permission.Check(permission.Action{Type: "write", Path: req.Path})
	if !permResult.Allowed {
		log.Printf("✗ VolitionCage BLOCKED write: %s — %s", req.Path, permResult.Reason)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": permResult.Reason})
		return
	}

	result := tools.WriteFile(s.resolvePath(req.Path), req.Content)
	log.Printf("⚒ write: %s (%d bytes)", req.Path, result.Bytes)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleToolsBash(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Command string `json:"command"`
		WorkDir string `json:"workdir,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// VolitionCage: check permission
	permResult := permission.Check(permission.Action{
		Type:    "bash",
		Command: req.Command,
	})
	if !permResult.Allowed {
		log.Printf("✗ VolitionCage BLOCKED: %s — %s", req.Command[:min(50, len(req.Command))], permResult.Reason)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":     permResult.Reason,
			"severity":  permResult.Severity,
			"need_hitl": permResult.NeedHITL,
		})
		return
	}

	workDir := s.cfg.RepoPath
	if req.WorkDir != "" {
		workDir = req.WorkDir
	}

	result := tools.RunBash(req.Command, workDir)
	log.Printf("⚙ bash [%s]: %s (exit=%d)", permResult.Severity, req.Command[:min(60, len(req.Command))], result.ExitCode)
	s.sessions.AddMessage("assistant", fmt.Sprintf("⚙ Bash: %s (exit=%d)\n%s", req.Command, result.ExitCode, truncate(result.Stdout, 2000)), "Servitor")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleToolsGrep(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("q")
	ext := r.URL.Query().Get("ext")
	if pattern == "" {
		http.Error(w, "missing q parameter", 400)
		return
	}

	result := tools.Grep(pattern, s.cfg.RepoPath, ext)
	log.Printf("☉ grep: %s (%d files)", pattern, result.Count)
	fileList := ""
	if len(result.Files) > 0 {
		fileList = "\n" + strings.Join(result.Files[:min(len(result.Files), 20)], "\n")
	}
	s.sessions.AddMessage("assistant", fmt.Sprintf("☉ Grep: %s → %d files%s", pattern, result.Count, fileList), "Skitarii")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	// SSE streaming endpoint
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	var req ChatRequest
	json.NewDecoder(r.Body).Decode(&req)

	log.Printf("⚙ stream: %s", req.Message[:min(60, len(req.Message))])

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	// Resolve agent: empty/magos → Magos path (Fable-Dominus via callMagos);
	// any other llama-swap alias → callLlamaSwapDirect.
	agent := req.Agent
	if agent == "" {
		agent = "magos"
	}

	// Dyad short-circuit: agent == "Dyad[-low|-medium|-high|-max]" runs the
	// Fable+Mythos+synth pipeline instead of a single model.
	if s.isDyadModel(agent) {
		level := s.dyadLevelFromModel(agent)
		fmt.Fprintf(w, "event: status\ndata: {\"agent\":%q,\"status\":\"thinking\"}\n\n", "Dyad-"+level)
		flusher.Flush()
		dresp, derr := s.dyadChat(req.Message, level, func(stage, model string) {
			// Stream every pipeline stage to the browser as it begins —
			// Fable-acts → Mythos-reflects → Synth-thinking etc.
			if f, ok := w.(http.Flusher); ok {
				fmt.Fprintf(w, "event: phase\ndata: {\"agent\":%q,\"stage\":%q,\"model\":%q}\n\n", "Dyad-"+level, stage, model)
				f.Flush()
			}
		})
		if derr != nil {
			fmt.Fprintf(w, "event: error\ndata: {\"error\":%q}\n\n", derr.Error())
			flusher.Flush()
			return
		}
		// Stream dyad output as words.
		words := strings.Fields(dresp)
		for i, word := range words {
			data := word
			if i < len(words)-1 {
				data += " "
			}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: token\ndata: %s\n\n", jsonData)
			flusher.Flush()
		}
		fmt.Fprintf(w, "event: done\ndata: {\"agent\":%q,\"status\":\"complete\"}\n\n", "Dyad-"+level)
		flusher.Flush()
		return
	}

	// Send "thinking" event with the actual agent name
	fmt.Fprintf(w, "event: status\ndata: {\"agent\":%q,\"status\":\"thinking\"}\n\n", agent)
	flusher.Flush()

	// Call model (non-streaming for now, return as single chunk)
	var resp string
	var err error
	if agent == "magos" {
		if s.cfg.CommuneURL != "" && req.Persona != "" {
			resp, err = s.callCommune(req.Message, req.Persona)
			if err != nil {
				log.Printf("☠ Commune stream unreachable: %v; falling back to local Magos", err)
				resp, err = s.callMagos(req.Message, req.Persona)
			}
		} else {
			resp, err = s.callMagos(req.Message, req.Persona)
		}
	} else {
		// Map agent aliases to llama-swap model names.
		llamaModel := agent
		switch agent {
		case "fable-dominus", "fable":
			llamaModel = "Fable-Dominus"
		case "mythos-philosophus", "mythos":
			llamaModel = "Mythos-Philosophus"
		// vox-dei intentionally disabled — в кладовке per W4
		}
		log.Printf("⚙ agent=%s → llama-swap model=%s", agent, llamaModel)
		resp, err = s.callLlamaSwapDirect(llamaModel, req.Message, req.Persona)
	}
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send response in chunks (simulated streaming)
	words := strings.Fields(resp)
	for i, word := range words {
		// Send word + space
		data := word
		if i < len(words)-1 {
			data += " "
		}
		// JSON-encode the data
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", jsonData)
		flusher.Flush()
	}

	// Send done event
	fmt.Fprintf(w, "event: done\ndata: {\"agent\":%q,\"status\":\"complete\"}\n\n", agent)
	flusher.Flush()

	log.Printf("✓ stream complete: %d words", len(words))
}

// resolvePath resolves a relative path against repo root
func (s *Server) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(s.cfg.RepoPath, p)
}

// ═══ End Sprint 2 ═══

// ═══ Sprint 4: WebSocket + Sessions ═══

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("☠ WS upgrade: %v", err)
		return
	}
	defer conn.Close()

	s.wsMu.Lock()
	s.wsClients[conn] = true
	s.wsMu.Unlock()

	log.Printf("✓ WS client connected (%d total)", len(s.wsClients))

	// Send welcome
	s.wsSend(conn, "connected", map[string]interface{}{
		"status":  "active",
		"message": "Forge WebSocket активен. Занесено.",
	})

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}
		// We don't process incoming WS messages yet (Phase D)
		// Client can listen to broadcasts only
	}

	s.wsMu.Lock()
	delete(s.wsClients, conn)
	s.wsMu.Unlock()
	log.Printf("✗ WS client disconnected (%d total)", len(s.wsClients))
}

// wsSend sends a message to one WebSocket client
func (s *Server) wsSend(conn *websocket.Conn, eventType string, data interface{}) {
	msg := map[string]interface{}{
		"type": eventType,
		"data": data,
		"time": time.Now().Format("15:04:05"),
	}
	conn.WriteJSON(msg)
}

// wsBroadcast sends a message to ALL connected WebSocket clients
func (s *Server) wsBroadcast(eventType string, data interface{}) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for conn := range s.wsClients {
		s.wsSend(conn, eventType, data)
	}
}

// ═══ Session endpoints ═══

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	sessions := s.sessions.List()
	type sessInfo struct {
		ID       string    `json:"id"`
		Name     string    `json:"name"`
		Messages int       `json:"messages"`
		Created  time.Time `json:"created"`
		Updated  time.Time `json:"updated"`
		Current  bool      `json:"current"`
	}
	// Current() can return nil if no session is active — guard the deref.
	var currentID string
	if cur := s.sessions.Current(); cur != nil {
		currentID = cur.ID
	}
	result := make([]sessInfo, len(sessions))
	for i, sess := range sessions {
		result[i] = sessInfo{
			ID:       sess.ID,
			Name:     sess.Name,
			Messages: len(sess.Messages),
			Created:  sess.Created,
			Updated:  sess.Updated,
			Current:  sess.ID == currentID,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleSessionMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cur := s.sessions.Current()
	if cur == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]session.Message{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cur.Messages)
}

func (s *Server) handleSessionEditLast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cur := s.sessions.Current()
	if cur == nil {
		http.Error(w, "no session", http.StatusBadRequest)
		return
	}
	lastUserIdx := -1
	for i := len(cur.Messages) - 1; i >= 0; i-- {
		if cur.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx == -1 {
		http.Error(w, "no user message", http.StatusBadRequest)
		return
	}
	cur.Messages[lastUserIdx].Content = req.Message
	cur.Messages = cur.Messages[:lastUserIdx+1]
	cur.Updated = time.Now()
	s.sessions.SaveAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "session"
	}
	sess := s.sessions.New(name)
	log.Printf("☉ New session: %s (%s)", sess.Name, sess.ID)
	s.wsBroadcast("session_changed", map[string]string{"id": sess.ID, "name": sess.Name})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": sess.ID, "name": sess.Name})
}

func (s *Server) handleSessionSwitch(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if s.sessions.Switch(id) {
		log.Printf("➜ Switched to session: %s", id)
		s.wsBroadcast("session_changed", map[string]string{"id": id})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": id})
	} else {
		http.Error(w, "session not found", 404)
	}
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	s.sessions.Delete(id)
	log.Printf("✗ Deleted session: %s", id)
	s.wsBroadcast("session_changed", map[string]string{"action": "deleted", "id": id})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleSessionClear(w http.ResponseWriter, r *http.Request) {
	s.sessions.Clear()
	log.Printf("↩ Session cleared")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ═══ End Sprint 4 ═══

// handleAgents returns all available agents. Cloud agents are marked
// active=true (Принципал has a paid subscription); local agents follow
// their in-registry state.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents := []map[string]interface{}{
		{"name": "magos", "display": "Magos", "glyph": "w", "color": "#ffb000", "type": "local", "model": "Local 2-model swarm: Fable-Dominus · Mythos-Philosophus", "active": true},
		{"name": "fable-dominus", "display": "Fable-Dominus", "glyph": "⚙", "color": "#00bfbf", "type": "local", "model": "Fable-Dominus (GPU 0, soul-friendly)", "active": true, "alias": "qwable"},
		{"name": "mythos-philosophus", "display": "Mythos-Philosophus", "glyph": "☉", "color": "#c8a84b", "type": "local", "model": "Mythos-Philosophus (GPU 1, analytical)", "active": true, "alias": "qwythos"},
		{"name": "vox-dei", "display": "Vox Dei", "glyph": "♛", "color": "#8b0000", "type": "local", "model": "Gemma-4-12B (в кладовке per W4)", "active": false, "note": "В кладовке: директива W4"},
		// Dyad is NOT an agent — it is a mode that runs the Fable+Mythos
		// pipeline. Listed here so the dashboard's agent picker can
		// surface it alongside the rest of the roster.
		{"name": "Dyad-medium", "display": "Dyad (Medium)", "glyph": "∞", "color": "#ffd700", "type": "mode", "model": "Fable-Dominus + Mythos-Philosophus → synthesized", "active": true, "levels": []string{"low", "medium", "high", "max"}, "note": "Fable acts · Mythos reflects · synthesizer unifies"},
		// 4 cloud specialists — all on Принципал's paid subscription.
		{"name": "glm", "display": "GLM-5.2", "glyph": "G", "color": "#ff4500", "type": "cloud", "model": "GLM-5.2 (cloud)", "context": "1M", "rank": "#13 Coding", "active": true, "subscription": "paid"},
		{"name": "kimi", "display": "Kimi K2.7", "glyph": "K", "color": "#4169e1", "type": "cloud", "model": "K2.7 (cloud)", "context": "256K", "rank": "#4 Creative", "active": true, "subscription": "paid"},
		{"name": "mimo", "display": "MiMo V2.5", "glyph": "M", "color": "#39ff14", "type": "cloud", "model": "V2.5 Pro (cloud)", "context": "1M", "rank": "#14 Math", "active": true, "subscription": "paid"},
		{"name": "minimax", "display": "MiniMax M3", "glyph": "▣", "color": "#ff00ff", "type": "cloud", "model": "427B MoE (cloud)", "context": "1M", "rank": "SWE 80.5%", "active": true, "subscription": "paid"},
		{"name": "servitor", "display": "Servitor", "glyph": "S", "color": "#cd7f32", "type": "local", "model": "8B Llama", "active": true},
		{"name": "skitarii", "display": "Skitarii", "glyph": "X", "color": "#666", "type": "local", "model": "1.5B Qwen", "active": true},
		// Team / Council modes (mirror organa/dashboard main.py:980)
		{"name": "Team-Single", "display": "Team Single (one model)", "glyph": "◆", "color": "#c8a86e", "type": "mode", "strategy": "team", "provider": "local", "model": "Direct single-model answer via /api/chat/team", "active": true, "team_mode": "single"},
		{"name": "Team-Dyad", "display": "Team Dyad (Qwable + Qwythos)", "glyph": "∞", "color": "#ffd700", "type": "mode", "strategy": "team", "provider": "local", "model": "Fable-acts · Mythos-reflects · synth-unifies", "active": true, "team_mode": "dyad"},
		{"name": "Team-Teacher", "display": "Team Teacher (+ cloud)", "glyph": "T", "color": "#c75b39", "type": "mode", "strategy": "team", "provider": "mixed", "model": "Local pair + cloud teacher guide", "active": true, "team_mode": "teacher"},
		{"name": "Team-Council", "display": "Team Council (multi-round)", "glyph": "✦", "color": "#ff69b4", "type": "mode", "strategy": "team", "provider": "mixed", "model": "Multi-round discussion with consensus synthesis", "active": true, "team_mode": "council"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"name":    "HereticArch",
		"version": "1.1.0",
		"forge":   "v2.0.0",
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	s.handleAgents(w, r)
}

// ─────────── MCP (Model Context Protocol) ───────────

// handleMCPTools lists all registered MCP tools.
// GET /api/mcp/tools → JSON array of ToolInfo.
func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": s.mcp.List(),
		"count": s.mcp.Count(),
	})
}

// mcpExecuteRequest is the POST body for /api/mcp/execute.
type mcpExecuteRequest struct {
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params,omitempty"`
}

// mcpExecuteResponse is the JSON envelope for tool execution results.
type mcpExecuteResponse struct {
	Tool    string      `json:"tool"`
	Result  interface{} `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
	Stub    bool        `json:"stub,omitempty"`
}

// handleMCPExecute runs a registered tool with the given JSON params.
// POST /api/mcp/execute {"name":"github","params":{"action":"list_issues"}}
func (s *Server) handleMCPExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req mcpExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpExecuteResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpExecuteResponse{Error: "name is required"})
		return
	}

	// Pass nil params if empty — Execute handles it.
	result, err := s.mcp.Execute(req.Name, req.Params)
	resp := mcpExecuteResponse{Tool: req.Name, Result: result}
	if err != nil {
		resp.Error = err.Error()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Check if the result itself marks _stub:true → flag in response.
	if m, ok := result.(map[string]interface{}); ok {
		if s, ok := m["_stub"].(bool); ok && s {
			resp.Stub = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handlePlan accepts a message and returns a plan response.
// POST /api/plan {"message": "implement sorting"}
// → {"steps": [...], "summary": "..."}
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Message == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "empty message"})
		return
	}

	// Generate a heuristic plan based on message content.
	// In production, this would call the agent to generate a plan.
	plan := generateHeuristicPlan(req.Message)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

// generateHeuristicPlan creates a basic plan from the user message.
func generateHeuristicPlan(msg string) map[string]interface{} {
	steps := []map[string]interface{}{
		{
			"action":      "read",
			"target":      "relevant files",
			"description": "Analyze the codebase for context",
			"risk":        "safe",
		},
		{
			"action":      "edit",
			"target":      "target file",
			"description": "Apply the requested changes",
			"risk":        "moderate",
		},
		{
			"action":      "bash",
			"target":      "go build ./...",
			"description": "Verify the build passes",
			"risk":        "safe",
		},
	}

	return map[string]interface{}{
		"steps":   steps,
		"summary": "Plan for: " + msg,
	}
}

// handleLSPDiag returns diagnostics for a file via LSP.
// GET /api/lsp/diagnostics?path=main.go
func (s *Server) handleLSPDiag(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing path"})
		return
	}

	// Resolve absolute path
	absPath := path
	if !filepath.IsAbs(path) {
		cwd, _ := os.Getwd()
		absPath = filepath.Join(cwd, path)
	}

	// Check if gopls is available
	if _, err := exec.LookPath("gopls"); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"diagnostics": []interface{}{},
			"message":     "gopls not installed",
		})
		return
	}

	// Run gopls check directly
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gopls", "check", absPath)
	cwd, _ := os.Getwd()
	cmd.Dir = cwd
	output, _ := cmd.CombinedOutput()

	// Parse gopls output into diagnostics
	diagnostics := parseGoplsOutput(string(output), path)
	summary := "OK"
	if len(diagnostics) > 0 {
		summary = fmt.Sprintf("%d diagnostics", len(diagnostics))
	}
	s.sessions.AddMessage("assistant", fmt.Sprintf("⚙ LSP diagnostics for %s: %s", path, summary), "Servitor")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"diagnostics": diagnostics,
		"path":        path,
	})
}

// parseGoplsOutput converts gopls check output to Diagnostic array.
func parseGoplsOutput(output, relPath string) []map[string]interface{} {
	var diags []map[string]interface{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		// Format: file:line:col: message
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 4 {
			continue
		}
		lineNum := 0
		colNum := 0
		fmt.Sscanf(parts[1], "%d", &lineNum)
		fmt.Sscanf(parts[2], "%d", &colNum)
		msg := strings.TrimSpace(parts[3])
		severity := "info"
		if strings.Contains(msg, "error") || strings.Contains(msg, "undefined") {
			severity = "error"
		} else if strings.Contains(msg, "warning") || strings.Contains(msg, "unused") {
			severity = "warning"
		}
		diags = append(diags, map[string]interface{}{
			"line":     lineNum,
			"col":      colNum,
			"message":  msg,
			"severity": severity,
			"source":   "gopls",
		})
	}
	if diags == nil {
		diags = []map[string]interface{}{}
	}
	return diags
}

// handleLSPHover returns hover info for a position.
// GET /api/lsp/hover?path=main.go&line=10&col=5
func (s *Server) handleLSPHover(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing path"})
		return
	}

	if _, err := exec.LookPath("gopls"); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"hover":  "",
			"message": "gopls not installed",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hover": "",
		"path":  path,
	})
}

// === MEMORY SEARCH (semantic over chat_history JSONL) ===
func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Limit <= 0 || req.Limit > 20 {
		req.Limit = 5
	}
	results := searchMemory(req.Query, req.Limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}

// searchMemory does keyword-based scoring over chat_history/*.jsonl files.
func searchMemory(query string, limit int) []map[string]interface{} {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	histDir := filepath.Join(home, ".config", "Dark Forge", "chat_history")
	entries, err := os.ReadDir(histDir)
	if err != nil {
		return nil
	}
	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return nil
	}
	type scored struct {
		ts, source, text string
		score            int
	}
	scoredList := []scored{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		fpath := filepath.Join(histDir, e.Name())
		data, err := os.ReadFile(fpath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			text := strings.ToLower(fmt.Sprintf("%v %v", entry["instruction"], entry["replacement"]))
			score := 0
			for _, tok := range qTokens {
				if strings.Contains(text, tok) {
					score++
				}
			}
			if score > 0 {
				scoredList = append(scoredList, scored{
					ts:     fmt.Sprintf("%v", entry["ts"]),
					source: fmt.Sprintf("%v", entry["type"]),
					text:   fmt.Sprintf("%v", entry["replacement"]),
					score:  score,
				})
			}
		}
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})
	if len(scoredList) > limit {
		scoredList = scoredList[:limit]
	}
	results := make([]map[string]interface{}, 0, len(scoredList))
	for _, s := range scoredList {
		results = append(results, map[string]interface{}{
			"ts":     s.ts,
			"source": s.source,
			"text":   s.text,
			"score":  s.score,
		})
	}
	return results
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer(",", " ", ".", " ", "!", " ", "?", " ", ";", " ", ":", " ", "(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "\"", " ", "'", " ", "/", " ", "\\", " ", "\n", " ")
	s = replacer.Replace(s)
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) >= 3 {
			out = append(out, p)
		}
	}
	return out
}
