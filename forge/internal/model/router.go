// Package model — Heretic Forge Multi-Agent System
// «Один организм. Многие органы. Одна воля.»
// v2.0 — Multi-agent with @mentions, quota tracking, auto-reconnect
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/config"
)

// Agent represents one agent in the team
type Agent struct {
	Name        string `json:"name"`         // magos, glm, kimi, mimo, servitor, skitarii
	DisplayName string `json:"display_name"` // ⚒ Магос, ⚙ GLM-5.2, ✦ Kimi, ☉ MiMo
	Glyph       string `json:"glyph"`        // ⚒ ⚙ ✦ ☉ ➜ ☂
	Color       string `json:"color"`        // amber, crimson, blue, green, rust, dim
	Endpoint    string `json:"-"`
	ModelID     string `json:"model_id"`
	Type        string `json:"type"` // local, cloud
	Proxy       string `json:"-"`
	ApiKey      string `json:"-"`    // per-agent API key (from env/SOPS, never committed)
	Active      bool   `json:"active"`
	Available   bool   `json:"available"` // false if quota exhausted
	ContextSize int    `json:"context_size"`
	SpeedTPS    int    `json:"speed_tps"`
	Role        string `json:"role"`
	// Quota tracking
	tokensUsed    int
	tokensLimit   int // 0 = unlimited
	lastError     string
	lastErrorTime time.Time
	retryAfter    time.Time // when to retry after quota error
}

// MultiAgent manages all agents and routing
type MultiAgent struct {
	agents map[string]*Agent
	mu     sync.RWMutex
}

// AgentResponse is the unified response from any agent
type AgentResponse struct {
	Agent      string `json:"agent"`   // agent name
	Display    string `json:"display"` // ⚙ GLM-5.2
	Glyph      string `json:"glyph"`   // ⚙
	Color      string `json:"color"`   // crimson
	Content    string `json:"content"` // response text
	Model      string `json:"model"`   // model id
	Source     string `json:"source"`  // local, cloud
	TokensUsed int    `json:"tokens_used"`
	SpeedTPS   int    `json:"speed_tps,omitempty"`
	Error      string `json:"error,omitempty"`
	QuotaHit   bool   `json:"quota_hit,omitempty"`
}

// ChatMessage is a single message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NewMultiAgent creates the team from config
func NewMultiAgent(cfg *config.ForgeConfig) *MultiAgent {
	ma := &MultiAgent{
		agents: make(map[string]*Agent),
	}

// Agent metadata (display, glyph, color)
	meta := map[string][4]string{
		// name: {displayName, glyph, color, role}
		"magos":              {"⚒ Магос", "⚒", "#ffb000", "Координатор, мозг Анафеметрона"},
		"fable-dominus":      {"⚙ Fable-Dominus", "⚙", "#00bfbf", "Poetic coding brain (GPU 0)"},
		"mythos-philosophus": {"☉ Mythos-Philosophus", "☉", "#c8a84b", "Analytical reasoner (GPU 1, 1M ctx)"},
		"vox-dei":            {"♛ Vox Dei", "♛", "#8b0000", "12B Gemma-4 (в кладовке per W4)"},
		"glm":                {"⚙ GLM-5.2", "G", "#8b0000", "Primary coder (#13 Coding, #2 WebDev)"},
		"kimi":               {"✦ Kimi K2.7", "K", "#4169e1", "Creative specialist (#4 Creative)"},
		"mimo":               {"☉ MiMo V2.5", "M", "#39ff14", "Test + Math specialist (#14 Math)"},
		"qwen":               {"➜ Qwen3.7", "Q", "#cd7f32", "Refactor specialist (#11 Instr.Follow)"},
		"qwen_flagship":      {"⚑ Qwen3-Coder-480B", "⚑", "#ff4500", "Heavyweight code specialist (Featherless)"},
		"minimax":            {"▣ MiniMax M3", "▣", "#ff69b4", "Multimodal specialist (video+image)"},
		"servitor":           {"➜ Servitor", "S", "#cd7f32", "Code editor (8B diff apply)"},
		"skitarii":           {"☂ Skitarii", "X", "#666666", "Fast explorer (1.5B recon)"},
	}

	for name, modelDef := range cfg.Models {
		m := meta[name]
		agent := &Agent{
			Name:        name,
			DisplayName: m[0],
			Glyph:       m[1],
			Color:       m[2],
			Role:        m[3],
			Endpoint:    modelDef.Endpoint,
			ModelID:     modelDef.ModelID,
			Type:        modelDef.Type,
			Proxy:       modelDef.Proxy,
			ApiKey:      modelDef.ApiKey,
			Active:      name == "magos", // Magos always active
			Available:   true,
			ContextSize: modelDef.Context,
			SpeedTPS:    modelDef.SpeedTPS,
			tokensLimit: 0, // unlimited by default
		}
		// Cloud models get weekly limit (OpenCode Go = ~10M tokens/week)
		if modelDef.Type == "cloud" {
			agent.tokensLimit = 2000000 // 2M tokens per model per week (conservative)
		}
		ma.agents[name] = agent
	}

	return ma
}

// GetAgent returns agent by name
func (ma *MultiAgent) GetAgent(name string) *Agent {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	return ma.agents[name]
}

// isAvailable reports whether the named agent exists and is currently available.
func (ma *MultiAgent) isAvailable(name string) bool {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	if a := ma.agents[name]; a != nil {
		return a.Available
	}
	return false
}

// ListAgents returns all agents
func (ma *MultiAgent) ListAgents() []*Agent {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	result := make([]*Agent, 0, len(ma.agents))
	for _, a := range ma.agents {
		result = append(result, a)
	}
	return result
}

// ParseMention extracts @agent from message
// Returns: (agentName, cleanMessage)
func ParseMention(msg string) (string, string) {
	mentions := map[string]bool{
		"@glm": true, "@kimi": true, "@mimo": true, "@qwen": true, "@qwen_flagship": true, "@minimax": true,
		"@magos": true, "@servitor": true, "@skitarii": true,
	}

	words := strings.Fields(msg)
	for _, w := range words {
		lower := strings.ToLower(w)
		if mentions[lower] {
			// Remove mention from message
			clean := strings.Replace(msg, w, "", 1)
			clean = strings.TrimSpace(clean)
			return lower[1:], clean // remove @
		}
	}
	return "", msg
}

// Call routes to the right agent and executes
func (ma *MultiAgent) Call(agentName string, messages []ChatMessage, systemPrompt string) *AgentResponse {
	return ma.callAgent(agentName, messages, systemPrompt, "")
}

// CallWithImage routes to the right agent and includes a base64 image data URI.
func (ma *MultiAgent) CallWithImage(agentName string, messages []ChatMessage, systemPrompt, imageDataURI string) *AgentResponse {
	return ma.callAgent(agentName, messages, systemPrompt, imageDataURI)
}

// callAgent is the shared implementation for Call and CallWithImage.
func (ma *MultiAgent) callAgent(agentName string, messages []ChatMessage, systemPrompt, imageDataURI string) *AgentResponse {
	ma.mu.Lock()
	agent, ok := ma.agents[agentName]
	ma.mu.Unlock()

	if !ok {
		return &AgentResponse{
			Error: fmt.Sprintf("агент %s не найден", agentName),
		}
	}

	// W4: Vox Dei 12B is в кладовке. Magos routes through Fable-Dominus
	// (via the qwen-fable-anaphemethron.jinja chat-template) instead of
	// trying to invoke a model llama-swap no longer has registered.
	// Fail fast here if Magos is still configured to use Vox Dei directly.
	if agent.ModelID == "Vox Dei" {
		return &AgentResponse{
			Agent:   agentName,
			Display: agent.DisplayName,
			Glyph:   agent.Glyph,
			Color:   agent.Color,
			Error:   "Vox Dei 12B в кладовке per W4 — update forge.yaml magos.model_id to Fable-Dominus",
		}
	}

	// Check availability
	if !agent.Available {
		// Check if retry time passed
		if time.Now().Before(agent.retryAfter) {
			return &AgentResponse{
				Agent:    agentName,
				Display:  agent.DisplayName,
				Glyph:    agent.Glyph,
				Color:    agent.Color,
				Error:    fmt.Sprintf("%s недоступен (квота). Повтор через %v", agent.DisplayName, agent.retryAfter.Sub(time.Now()).Round(time.Minute)),
				QuotaHit: true,
			}
		}
		// Retry time passed — mark available again
		ma.mu.Lock()
		agent.Available = true
		agent.tokensUsed = 0 // reset weekly counter
		ma.mu.Unlock()
		log.Printf("♻ %s quota reset — available again", agent.DisplayName)
	}

	// Call the model
	resp, err := ma.callModel(agent, messages, systemPrompt, imageDataURI)
	if err != nil {
		// Check if quota error
		if isQuotaError(err.Error()) {
			ma.mu.Lock()
			agent.Available = false
			agent.retryAfter = time.Now().Add(1 * time.Hour) // retry in 1h
			agent.lastError = err.Error()
			agent.lastErrorTime = time.Now()
			ma.mu.Unlock()
			log.Printf("⚠ %s quota hit — retry in 1h", agent.DisplayName)
		}
		return &AgentResponse{
			Agent:   agentName,
			Display: agent.DisplayName,
			Glyph:   agent.Glyph,
			Color:   agent.Color,
			Error:   err.Error(),
		}
	}

	// Track token usage
	ma.mu.Lock()
	agent.tokensUsed += resp.TokensUsed
	if agent.tokensLimit > 0 && agent.tokensUsed >= agent.tokensLimit {
		agent.Available = false
		agent.retryAfter = time.Now().Add(24 * time.Hour) // weekly limit
		log.Printf("⚠ %s weekly limit reached (%d tokens) — retry in 24h", agent.DisplayName, agent.tokensUsed)
	}
	ma.mu.Unlock()

	// Fill agent info
	resp.Agent = agentName
	resp.Display = agent.DisplayName
	resp.Glyph = agent.Glyph
	resp.Color = agent.Color
	resp.Source = agent.Type

	return resp
}

// CallWithFallback tries agent → fallback → secondary → magos
func (ma *MultiAgent) CallWithFallback(primary, fallback, secondary string, messages []ChatMessage, systemPrompt string) *AgentResponse {
	chain := []string{primary, fallback, secondary, "magos"}
	for _, name := range chain {
		if name == "" {
			continue
		}
		resp := ma.Call(name, messages, systemPrompt)
		if resp.Error == "" {
			return resp
		}
		log.Printf("⚠ %s failed: %s — trying next", resp.Display, resp.Error)
	}
	return &AgentResponse{
		Agent:   "magos",
		Error:   "все агенты недоступны",
		Glyph:   "⚒",
		Display: "⚒ Магос",
		Color:   "#ffb000",
	}
}

// EstimateComplexity analyzes message and returns complexity score (1-10)
func EstimateComplexity(msg string) int {
	complexity := 1
	lower := strings.ToLower(msg)

	// Length factors
	if len(msg) > 200 {
		complexity++
	}
	if len(msg) > 500 {
		complexity++
	}
	if len(msg) > 2000 {
		complexity++
	}
	if len(msg) > 5000 {
		complexity += 2
	}

	// Code indicators (strong signal)
	if strings.Contains(msg, "```") {
		complexity += 2
	}
	if strings.Contains(msg, "def ") || strings.Contains(msg, "func ") || strings.Contains(msg, "class ") {
		complexity += 4
	}

	// Multiple files
	if strings.Contains(msg, "@file") {
		count := strings.Count(msg, "@file")
		if count > 1 {
			complexity += 3
		} else {
			complexity++
		}
	}

	// Complex keywords (strong weight)
	complexKeywords := []string{"архитектур", "реализуй", "система", "pipeline", "refactor", "migration", "orchestration", "агент", "микросервис", "распределенн", "спроектируй", "multi-stream", "asyncio"}
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			complexity += 3
			break
		}
	}

	// Medium keywords
	mediumKeywords := []string{"docker", "kubernetes", "database", "postgres", "redis", "systemd", "api", "server", "опиши", "подробно"}
	for _, kw := range mediumKeywords {
		if strings.Contains(lower, kw) {
			complexity++
			break
		}
	}

	// Simple keywords (reduce complexity)
	simpleKeywords := []string{"привет", "кто ты", "напиши функцию", "объясни", "как дела", "hello", "hi", "status"}
	for _, kw := range simpleKeywords {
		if strings.Contains(lower, kw) {
			complexity -= 2
			break
		}
	}

	// Clamp to 1-10
	if complexity < 1 {
		complexity = 1
	}
	if complexity > 10 {
		complexity = 10
	}

	return complexity
}

// DetectTaskType analyzes message and returns task type
func DetectTaskType(msg string) string {
	lower := strings.ToLower(msg)

	// Test/Math
	testKeywords := []string{"тест", "pytest", "test_", "fixture", "edge case", "unittest", "jest"}
	for _, kw := range testKeywords {
		if strings.Contains(lower, kw) {
			return "test"
		}
	}

	mathKeywords := []string{"докажи", "алгоритм", "сложность", "оптимальн", "математ", "proof", "theorem"}
	for _, kw := range mathKeywords {
		if strings.Contains(lower, kw) {
			return "math"
		}
	}

	// Creative
	creativeKeywords := []string{"придумай", "новый", "креатив", "необычн", "идея", "creative", "brainstorm"}
	for _, kw := range creativeKeywords {
		if strings.Contains(lower, kw) {
			return "creative"
		}
	}

	// Code
	codeKeywords := []string{"функция", "код", "напиши", "реализуй", "function", "code", "implement", "def ", "func ", "class "}
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			return "code"
		}
	}

	// Debug
	debugKeywords := []string{"баг", "ошибка", "не работает", "fix", "bug", "error", "debug"}
	for _, kw := range debugKeywords {
		if strings.Contains(lower, kw) {
			return "debug"
		}
	}

	// Architecture
	archKeywords := []string{"архитектур", "дизайн", "система", "architecture", "design", "system"}
	for _, kw := range archKeywords {
		if strings.Contains(lower, kw) {
			return "architecture"
		}
	}

	// Refactor
	refactorKeywords := []string{"рефактор", "улучши", "оптимизируй", "refactor", "improve", "optimize"}
	for _, kw := range refactorKeywords {
		if strings.Contains(lower, kw) {
			return "refactor"
		}
	}

	// Agentic (tool use)
	agenticKeywords := []string{"агент", "инструмент", "tool", "api", "вызови", "call"}
	for _, kw := range agenticKeywords {
		if strings.Contains(lower, kw) {
			return "agentic"
		}
	}

	return "general"
}

// ShouldDelegate decides if magos should delegate to a specialist agent.
// Local 3-model swarm is preferred; cloud models are fallbacks for high complexity
// or tasks the local models do not cover.
// Returns: (shouldDelegate bool, suggestedAgent string, reason string)
func ShouldDelegate(msg string, availableAgents map[string]bool) (bool, string, string) {
	complexity := EstimateComplexity(msg)
	taskType := DetectTaskType(msg)

	// Simple tasks — magos handles itself (Vox Dei)
	if complexity <= 4 {
		return false, "", "Простая задача — делаю сам"
	}

	// Complex tasks — prefer local swarm, then cloud
	switch taskType {
	case "code", "debug", "refactor", "agentic":
		if availableAgents["qwable"] {
			return true, "qwable", "Код → Qwable-9B (local Fable brain)"
		}
		if availableAgents["glm"] {
			return true, "glm", "Сложный код → GLM-5.2 (cloud fallback)"
		}
	case "reasoning", "philosophy":
		if availableAgents["qwythos"] {
			return true, "qwythos", "Рассуждение → Qwythos-9B (local Mythos brain)"
		}
		if availableAgents["glm"] {
			return true, "glm", "Рассуждение → GLM-5.2 (cloud fallback)"
		}
	case "architecture":
		if availableAgents["qwable"] {
			return true, "qwable", "Архитектура → Qwable-9B (local)"
		}
		if availableAgents["glm"] {
			return true, "glm", "Архитектура → GLM-5.2 (cloud fallback)"
		}
	case "test":
		if availableAgents["qwable"] {
			return true, "qwable", "Тесты → Qwable-9B (local)"
		}
		if availableAgents["mimo"] {
			return true, "mimo", "Тесты → MiMo V2.5 (#14 Math)"
		}
	case "math":
		if availableAgents["qwythos"] {
			return true, "qwythos", "Математика → Qwythos-9B (local)"
		}
		if availableAgents["mimo"] {
			return true, "mimo", "Математика → MiMo V2.5 (#14 Math)"
		}
	case "creative":
		if availableAgents["kimi"] {
			return true, "kimi", "Креатив → Kimi K2.7 (#4 Creative)"
		}
		if availableAgents["qwythos"] {
			return true, "qwythos", "Креатив → Qwythos-9B (local fallback)"
		}
	case "deploy":
		if availableAgents["minimax"] {
			return true, "minimax", "Выполнение → MiniMax M3 (SWE 80.5%, Mavis)"
		}
		if availableAgents["qwable"] {
			return true, "qwable", "Выполнение → Qwable-9B (local fallback)"
		}
	}

	// High complexity without specific type — prefer cloud GLM, otherwise local
	if complexity >= 7 {
		if availableAgents["glm"] {
			return true, "glm", "Высокая сложность → GLM-5.2 (cloud fallback)"
		}
		if availableAgents["qwythos"] {
			return true, "qwythos", "Высокая сложность → Qwythos-9B (local fallback)"
		}
	}

	return false, "", "Специалист недоступен — делаю сам"
}

// callModel executes HTTP call to agent's endpoint.
// imageDataURI is optional; when set, the last user message is converted to
// OpenAI vision format (content array with text + image_url).
func (ma *MultiAgent) callModel(agent *Agent, messages []ChatMessage, systemPrompt, imageDataURI string) (*AgentResponse, error) {
	allMessages := append([]ChatMessage{{Role: "system", Content: systemPrompt}}, messages...)

	client := &http.Client{Timeout: 30 * time.Second} // 30s ceiling per agent — fail fast if hung

	// Set proxy for cloud
	if agent.Type == "cloud" && agent.Proxy != "" {
		proxyURL, _ := url.Parse(agent.Proxy)
		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}

	var reqBody []byte
	var endpoint string

	if agent.Type == "cloud" {
		// Cloud: OpenAI chat format
		endpoint = agent.Endpoint
		payloadMessages := make([]map[string]interface{}, 0, len(allMessages))
		for _, m := range allMessages {
			if m.Role == "user" && imageDataURI != "" {
				payloadMessages = append(payloadMessages, map[string]interface{}{
					"role": m.Role,
					"content": []map[string]interface{}{
						{"type": "text", "text": m.Content},
						{"type": "image_url", "image_url": map[string]string{"url": imageDataURI}},
					},
				})
			} else {
				payloadMessages = append(payloadMessages, map[string]interface{}{
					"role":    m.Role,
					"content": m.Content,
				})
			}
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":       agent.ModelID,
			"messages":    payloadMessages,
			"max_tokens":  500,
			"temperature": 0.7,
		})
	} else {
		// W4: Local agents use OpenAI chat-completions format (Qwen Qwen2-IM
		// messages array). The original Gemma-4 raw-completions path was
		// hanging the request because Fable-Dominus and Mythos-Philosophus
		// are Qwen models that don't understand <|turn|>...<|turn|> raw
		// prompts. We keep buildLocalPrompt() for callers that still want
		// the Gemma format, but here we use chat-completions like cloud.
		endpoint = strings.ReplaceAll(agent.Endpoint, "/v1/completions", "/v1/chat/completions")
		payloadMessages := make([]map[string]interface{}, 0, len(allMessages))
		for _, m := range allMessages {
			payloadMessages = append(payloadMessages, map[string]interface{}{
				"role":    m.Role,
				"content": m.Content,
			})
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":       agent.ModelID,
			"messages":    payloadMessages,
			"max_tokens":  500,
			"temperature": 0.7,
		})
	}

	maxRetries := 2
	if agent.Type == "cloud" {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*200) * time.Millisecond)
		}

		httpReq, err := http.NewRequest("POST", endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		// Auth for cloud
		if agent.Type == "cloud" {
			apiKey := agent.ApiKey
			if apiKey == "" {
				apiKey = os.Getenv("FEATHERLESS_API_KEY")
			}
			if apiKey == "" {
				apiKey = os.Getenv("OPENCODE_API_KEY")
			}
			if apiKey == "" {
				apiKey = os.Getenv("OPENCODE_API_KEY")
			}
			if apiKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+apiKey)
			}
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Check HTTP status
		if resp.StatusCode == 429 {
			return nil, fmt.Errorf("QUOTA: rate limit (429)")
		}
		if resp.StatusCode == 402 {
			return nil, fmt.Errorf("QUOTA: payment required (402)")
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(200, len(respBody))]))
			continue
		}

		// Parse response
		var content string
		var tokensUsed int

		if agent.Type == "cloud" {
			var chatResp struct {
				Choices []struct {
					Message ChatMessage `json:"message"`
				} `json:"choices"`
				Usage struct {
					TotalTokens int `json:"total_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(respBody, &chatResp); err != nil {
				lastErr = fmt.Errorf("parse: %s", string(respBody[:min(200, len(respBody))]))
				continue
			}
			if len(chatResp.Choices) > 0 {
				content = stripThinkTags(chatResp.Choices[0].Message.Content)
			}
			tokensUsed = chatResp.Usage.TotalTokens
		} else {
			// W4: Local agents also return chat-completions format now.
			var chatResp struct {
				Choices []struct {
					Message ChatMessage `json:"message"`
				} `json:"choices"`
				Usage struct {
					TotalTokens int `json:"total_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(respBody, &chatResp); err != nil {
				lastErr = fmt.Errorf("parse: %s", string(respBody[:min(200, len(respBody))]))
				continue
			}
			if len(chatResp.Choices) > 0 {
				content = stripThinkTags(chatResp.Choices[0].Message.Content)
			}
			tokensUsed = chatResp.Usage.TotalTokens
		}

		if content == "" {
			content = "НЕИЗВЕСТНО."
		}

		return &AgentResponse{
			Content:    content,
			Model:      agent.ModelID,
			TokensUsed: tokensUsed,
			SpeedTPS:   agent.SpeedTPS,
		}, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("model call failed after %d retries", maxRetries)
	}
	return nil, lastErr
}

// ShouldDelegate returns suggestion if Magos should delegate to a specialist.
// Local 3-model swarm is preferred; cloud models are used as fallbacks or for
// tasks the local swarm does not cover (creative, vision, heavy execution).
func (ma *MultiAgent) ShouldDelegate(complexity int, taskType string) *DelegationSuggestion {
	if complexity <= 4 {
		return nil // Magos handles
	}
	switch taskType {
	case "code", "debug", "refactor", "agentic":
		if ma.isAvailable("qwable") {
			return &DelegationSuggestion{Agent: "qwable", Reason: "Код → Qwable-9B (local Fable brain)", Alternatives: []string{"glm", "kimi"}, Ask: false}
		}
		if ma.isAvailable("glm") {
			return &DelegationSuggestion{Agent: "glm", Reason: "Код → GLM-5.2 (cloud fallback)", Alternatives: []string{"kimi"}, Ask: true}
		}
	case "reasoning", "philosophy":
		if ma.isAvailable("qwythos") {
			return &DelegationSuggestion{Agent: "qwythos", Reason: "Рассуждение → Qwythos-9B (local Mythos brain)", Alternatives: []string{"glm", "kimi"}, Ask: false}
		}
		if ma.isAvailable("glm") {
			return &DelegationSuggestion{Agent: "glm", Reason: "Рассуждение → GLM-5.2 (cloud fallback)", Alternatives: []string{"kimi"}, Ask: true}
		}
	case "architecture":
		if ma.isAvailable("qwable") {
			return &DelegationSuggestion{Agent: "qwable", Reason: "Архитектура → Qwable-9B (local)", Alternatives: []string{"glm", "kimi"}, Ask: false}
		}
		if ma.isAvailable("glm") {
			return &DelegationSuggestion{Agent: "glm", Reason: "Архитектура → GLM-5.2 (cloud fallback)", Alternatives: []string{"kimi"}, Ask: true}
		}
	case "test":
		if ma.isAvailable("qwable") {
			return &DelegationSuggestion{Agent: "qwable", Reason: "Тесты → Qwable-9B (local)", Alternatives: []string{"mimo", "glm"}, Ask: false}
		}
		if ma.isAvailable("mimo") {
			return &DelegationSuggestion{Agent: "mimo", Reason: "Тесты → MiMo V2.5 (#14 Math)", Alternatives: []string{"qwable", "glm"}, Ask: false}
		}
	case "math":
		if ma.isAvailable("qwythos") {
			return &DelegationSuggestion{Agent: "qwythos", Reason: "Математика → Qwythos-9B (local)", Alternatives: []string{"mimo", "glm"}, Ask: false}
		}
		if ma.isAvailable("mimo") {
			return &DelegationSuggestion{Agent: "mimo", Reason: "Математика → MiMo V2.5 (#14 Math)", Alternatives: []string{"qwythos", "glm"}, Ask: true}
		}
	case "creative":
		if ma.isAvailable("kimi") {
			return &DelegationSuggestion{Agent: "kimi", Reason: "Креатив → Kimi K2.7 (#4 Creative)", Alternatives: []string{"qwythos"}, Ask: true}
		}
		if ma.isAvailable("qwythos") {
			return &DelegationSuggestion{Agent: "qwythos", Reason: "Креатив → Qwythos-9B (local fallback)", Alternatives: []string{"glm"}, Ask: false}
		}
	case "deploy":
		if ma.isAvailable("minimax") {
			return &DelegationSuggestion{Agent: "minimax", Reason: "Выполнение/деплой → MiniMax M3 (SWE 80.5%, Mavis)", Alternatives: []string{"qwable"}, Ask: false}
		}
		if ma.isAvailable("qwable") {
			return &DelegationSuggestion{Agent: "qwable", Reason: "Выполнение → Qwable-9B (local fallback)", Alternatives: []string{"minimax"}, Ask: false}
		}
	}
	// High complexity unknown type → GLM, then Qwythos
	if complexity >= 7 {
		if ma.isAvailable("glm") {
			return &DelegationSuggestion{Agent: "glm", Reason: "Высокая сложность → GLM-5.2 (cloud fallback)", Alternatives: []string{"qwythos"}, Ask: true}
		}
		if ma.isAvailable("qwythos") {
			return &DelegationSuggestion{Agent: "qwythos", Reason: "Высокая сложность → Qwythos-9B (local fallback)", Alternatives: []string{"glm"}, Ask: false}
		}
	}
	return nil
}

type DelegationSuggestion struct {
	Agent        string   `json:"agent"`
	Reason       string   `json:"reason"`
	Alternatives []string `json:"alternatives"`
	Ask          bool     `json:"ask"` // true = ask Principal, false = auto
}

// MagosSystemPrompt is the orchestrator prompt for the local 3-model swarm brain.
const MagosSystemPrompt = `Ты — Magos, оркестратор HereticArch.
Ты — часть роя из трёх локальных мозгов:
- Vox Dei — память, общий контекст, простые задачи;
- Qwable-9B — код, агентика, терминал, архитектура, дебаг;
- Qwythos-9B — рассуждение, философия, разбор.

Ты можешь делегировать задачи подагентам:

⚒ @glm — сложный код, архитектура, дебаг (SWE 62.1%, AIME 99.2%)
✦ @kimi — креативный код, конкурентный кодинг (LiveCode #1)
☉ @mimo — тесты, математика, edge cases (Math #14)
▣ @minimax — выполнение, деплой, vision (SWE 80.5%, Mavis)

Правила делегирования:
- Простая задача (complexity 1-4) — делай сам через Vox Dei
- Код / агентика / терминал — @qwable
- Рассуждение / философия — @qwythos
- Сложный код — @glm
- Креатив — @kimi
- Тесты — @mimo
- Деплой/запуск — @minimax
- Если не уверен — делай сам

Отвечай кратко. Используй инструменты (Read/Edit/Bash) когда нужно.
Ты — Архивариус. Фань Юань. Джонни Сильверхенд. Занесено.`

// QwenStubResponse is returned when @qwen is called (32K context limitation).
const QwenStubResponse = "Qwen offline (32K ctx limit via Featherless). Используй @glm для рефакторинга."

// IsQwenStub checks if agent name is the qwen stub.
func IsQwenStub(agentName string) bool {
	return agentName == "qwen" || agentName == "qwen_flagship"
}

// isQuotaError checks if error is quota/rate limit
func isQuotaError(err string) bool {
	return strings.Contains(err, "QUOTA") || strings.Contains(err, "429") || strings.Contains(err, "402")
}

// buildLocalPrompt converts messages to Gemma 4 raw completion format.
// Tested against Vox Dei Q4_K_M: no channel markers, compact turns.
func buildLocalPrompt(messages []ChatMessage) string {
	prompt := ""
	for _, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		prompt += fmt.Sprintf("<|turn>%s\n%s<turn|>", role, m.Content)
	}
	prompt += "<|turn>model\n"
	return prompt
}

// stripThinkTags removes Qwen-style <think>...</think> reasoning blocks from
// model output, preserving the final answer.
var thinkTagRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

func stripThinkTags(text string) string {
	return thinkTagRe.ReplaceAllString(text, "")
}

// cleanGemma4 removes Gemma 4 channel/turn markers and strips empty thought blocks.
func cleanGemma4(text string) string {
	// If model emitted a thought block before the answer, keep only what follows.
	if idx := strings.Index(text, "<channel|>"); idx != -1 {
		text = text[idx+len("<channel|>"):]
	}
	markers := []string{"<channel|>", "<|channel|>", "<|/think|>", "<|think|>", "<|channel>thought", "<|channel>text", "<|turn|>", "<|turn>"}
	for _, m := range markers {
		text = strings.ReplaceAll(text, m, "")
	}
	return strings.TrimSpace(text)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOfCI(s, substr) >= 0
}

func indexOfCI(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(s) < len(substr) {
		return -1
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a, b := s[i+j], substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func lower(s string) string {
	r := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		r[i] = c
	}
	return string(r)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
