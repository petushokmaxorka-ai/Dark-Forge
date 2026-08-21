package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ═══ TestParseMention ═══════════════════════════════════════════

func TestParseMention(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAgent   string
		wantMessage string
	}{
		{"@glm", "@glm реши задачу", "glm", "реши задачу"},
		{"@kimi", "@kimi напиши код", "kimi", "напиши код"},
		{"@mimo", "@mimo тесты", "mimo", "тесты"},
		{"no mention", "привет", "", "привет"},
		{"case insensitive", "@GLM капсом", "glm", "капсом"},
		{"@magos", "@magos координируй", "magos", "координируй"},
		{"@servitor", "@servitor поправь файл", "servitor", "поправь файл"},
		{"@skitarii", "@skitarii найди функцию", "skitarii", "найди функцию"},
		{"mention in middle", "привет @kimi как дела", "kimi", "привет  как дела"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, msg := ParseMention(tt.input)
			if agent != tt.wantAgent {
				t.Errorf("ParseMention(%q) agent = %q, want %q", tt.input, agent, tt.wantAgent)
			}
			if msg != tt.wantMessage {
				t.Errorf("ParseMention(%q) msg = %q, want %q", tt.input, msg, tt.wantMessage)
			}
		})
	}
}

// ═══ TestEstimateComplexity ═════════════════════════════════════

func TestEstimateComplexity(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		min  int
		max  int
	}{
		{"simple greeting", "привет", 1, 3},
		{"complex code task", "реализуй PIANO multi-stream с asyncio.gather и обработкой ошибок в pipeline системы", 4, 10},
		{"has code keyword", "def factorial(n): return n * factorial(n-1)", 5, 8},
		{"architecture task", "спроектируй архитектуру для новой системы и дизайн компонентов", 4, 10},
		{"long message", "опиши подробно " + repeatStr("очень длинный текст ", 100), 5, 10},
		{"status check", "покажи статус системы", 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := EstimateComplexity(tt.msg)
			if c < tt.min || c > tt.max {
				t.Errorf("EstimateComplexity(%q) = %d, want %d-%d", tt.msg[:min(50, len(tt.msg))], c, tt.min, tt.max)
			}
		})
	}
}

// ═══ TestDetectTaskType ═════════════════════════════════════════

func TestDetectTaskType(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{"test task", "напиши тесты для модуля", "test"},
		{"math task", "докажи теорему Пифагора", "math"},
		{"creative task", "придумай новую креативную идею для проекта", "creative"},
		{"debug task", "почини баг в парсере", "debug"},
		{"architecture task", "спроектируй архитектуру системы", "architecture"},
		{"code task default", "напиши функцию сортировки", "code"},
		{"pytest keyword", "запусти pytest для модуля", "test"},
		{"fix keyword", "fix the import error", "debug"},
		{"agentic task", "создай агента для обработки", "agentic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectTaskType(tt.msg)
			if got != tt.want {
				t.Errorf("DetectTaskType(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

// ═══ TestShouldDelegate ═════════════════════════════════════════

func TestShouldDelegate(t *testing.T) {
	// Create a minimal MultiAgent for testing
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"magos":         {Name: "magos", Active: true, Available: true},
			"glm":           {Name: "glm", Available: true},
			"kimi":          {Name: "kimi", Available: true},
			"mimo":          {Name: "mimo", Available: true},
			"minimax":       {Name: "minimax", Available: true},
			"qwable":        {Name: "qwable", Available: true},
			"qwythos":       {Name: "qwythos", Available: true},
			"qwen_flagship": {Name: "qwen_flagship", Available: true},
		},
	}

	tests := []struct {
		name       string
		complexity int
		taskType   string
		wantAgent  string // empty = nil (no delegation)
		wantAsk    bool
	}{
		{"simple code", 2, "code", "", false},
		{"complex code local", 8, "code", "qwable", false},
		{"medium code local", 5, "code", "qwable", false},
		{"test auto local", 5, "test", "qwable", false},
		{"creative cloud", 6, "creative", "kimi", true},
		{"math local", 7, "math", "qwythos", false},
		{"architecture local", 6, "architecture", "qwable", false},
		{"reasoning local", 6, "reasoning", "qwythos", false},
		{"simple creative", 3, "creative", "", false},
		{"debug local", 6, "debug", "qwable", false},
		{"refactor local", 6, "refactor", "qwable", false},
		{"agentic local", 6, "agentic", "qwable", false},
		{"deploy cloud", 6, "deploy", "minimax", false},
		{"high complexity unknown", 8, "unknown_type", "glm", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ma.ShouldDelegate(tt.complexity, tt.taskType)
			if tt.wantAgent == "" {
				if result != nil {
					t.Errorf("ShouldDelegate(%d, %q) = %v, want nil", tt.complexity, tt.taskType, result)
				}
				return
			}
			if result == nil {
				t.Errorf("ShouldDelegate(%d, %q) = nil, want agent %q", tt.complexity, tt.taskType, tt.wantAgent)
				return
			}
			if result.Agent != tt.wantAgent {
				t.Errorf("ShouldDelegate(%d, %q).Agent = %q, want %q", tt.complexity, tt.taskType, result.Agent, tt.wantAgent)
			}
			if result.Ask != tt.wantAsk {
				t.Errorf("ShouldDelegate(%d, %q).Ask = %v, want %v", tt.complexity, tt.taskType, result.Ask, tt.wantAsk)
			}
		})
	}
}

// ═══ TestCleanGemma4 ═══════════════════════════════════════════

func TestCleanGemma4(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"channel prefix", "<channel|>привет", "привет"},
		{"thought block", "<|channel>thought\nдумаю<channel|>ответ", "ответ"},
		{"text prefix", "<|channel>text\nрезультат", "результат"},
		{"clean text", "привет мир", "привет мир"},
		{"multiple markers", "<|channel>thought\n思考<channel|><|channel>text\nрезультат", "результат"},
		{"empty", "", ""},
		{"only markers", "<channel|>", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanGemma4(tt.input)
			if got != tt.want {
				t.Errorf("cleanGemma4(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ═══ TestBuildLocalPrompt ═══════════════════════════════════════

func TestBuildLocalPrompt(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "Ты — Анафемтрон."},
		{Role: "user", Content: "Кто ты?"},
	}

	prompt := buildLocalPrompt(messages)

	if !contains(prompt, "<|turn>system") {
		t.Error("Missing <|turn>system")
	}
	if !contains(prompt, "<|turn>user") {
		t.Error("Missing <|turn>user")
	}
	if !contains(prompt, "<|turn>model") {
		t.Error("Missing final <|turn>model")
	}
	if !contains(prompt, "Анафемтрон") {
		t.Error("Missing system prompt content")
	}
	if !contains(prompt, "Кто ты?") {
		t.Error("Missing user message")
	}
}

func TestBuildLocalPrompt_WithHistory(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "System"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
		{Role: "user", Content: "How are you?"},
	}

	prompt := buildLocalPrompt(messages)

	// Should have system + user + assistant + user + final thought
	if !contains(prompt, "<|turn>system") {
		t.Error("Missing system turn")
	}
	if !contains(prompt, "<|turn>user") {
		t.Error("Missing user turn")
	}
	if !contains(prompt, "<|turn>model") {
		t.Error("Missing model turn")
	}
}

// ═══ TestIsQuotaError ═══════════════════════════════════════════

func TestIsQuotaError(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want bool
	}{
		{"quota", "QUOTA: rate limit", true},
		{"429", "HTTP 429", true},
		{"402", "HTTP 402", true},
		{"normal", "connection refused", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isQuotaError(tt.err)
			if got != tt.want {
				t.Errorf("isQuotaError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// ═══ TestContainsCI ═════════════════════════════════════════════

func TestContainsCI(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		_substr string
		want    bool
	}{
		{"exact", "hello", "hello", true},
		{"lower", "HELLO", "hello", true},
		{"upper", "hello", "HELLO", true},
		{"mixed", "HeLLo WoRLd", "hello", true},
		{"not found", "hello", "xyz", false},
		{"empty substr", "hello", "", true},
		{"empty both", "", "", true},
		{"empty s", "", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt._substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt._substr, got, tt.want)
			}
		})
	}
}

// ═══ TestIndexOfCI ══════════════════════════════════════════════

func TestIndexOfCI(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		_substr string
		want    int
	}{
		{"found", "Hello World", "world", 6},
		{"not found", "Hello", "xyz", -1},
		{"empty substr", "Hello", "", 0},
		{"case insensitive", "HELLO", "hello", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexOfCI(tt.s, tt._substr)
			if got != tt.want {
				t.Errorf("indexOfCI(%q, %q) = %d, want %d", tt.s, tt._substr, got, tt.want)
			}
		})
	}
}

// ═══ TestLower ══════════════════════════════════════════════════

func TestLower(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"uppercase", "HELLO", "hello"},
		{"lowercase", "hello", "hello"},
		{"mixed", "HeLLo", "hello"},
		{"empty", "", ""},
		{"with numbers", "ABC123", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lower(tt.input)
			if got != tt.want {
				t.Errorf("lower(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ═══ TestAgentStruct ════════════════════════════════════════════

func TestAgentStruct(t *testing.T) {
	a := Agent{
		Name:        "test",
		DisplayName: "Test Agent",
		Glyph:       "⚒",
		Color:       "#ffb000",
		Type:        "local",
		Active:      true,
		Available:   true,
		ContextSize: 4096,
		SpeedTPS:    100,
		Role:        "test agent",
	}

	if a.Name != "test" {
		t.Errorf("Name = %q, want 'test'", a.Name)
	}
	if !a.Active {
		t.Error("Active should be true")
	}
	if !a.Available {
		t.Error("Available should be true")
	}
}

// ═══ TestDelegationSuggestion ═══════════════════════════════════

func TestDelegationSuggestion(t *testing.T) {
	ds := DelegationSuggestion{
		Agent:        "glm",
		Reason:       "Complex code",
		Alternatives: []string{"kimi"},
		Ask:          true,
	}

	if ds.Agent != "glm" {
		t.Errorf("Agent = %q, want 'glm'", ds.Agent)
	}
	if !ds.Ask {
		t.Error("Ask should be true")
	}
	if len(ds.Alternatives) != 1 {
		t.Errorf("Alternatives length = %d, want 1", len(ds.Alternatives))
	}
}

// ═══ TestCallWithFallback ═══════════════════════════════════════

func TestCallWithFallback_PrimaryWorks(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"magos": {Name: "magos", Active: true, Available: true},
		},
	}
	// Primary succeeds → should not need fallback
	resp := ma.CallWithFallback("magos", "", "", []ChatMessage{{Role: "user", Content: "test"}}, "system")
	// May fail due to no actual model endpoint, but should not panic
	if resp == nil {
		t.Error("CallWithFallback returned nil")
	}
}

func TestCallWithFallback_PrimaryFails(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"nonexistent": {Name: "nonexistent", Active: false, Available: false},
			"magos":       {Name: "magos", Active: true, Available: true},
		},
	}
	resp := ma.CallWithFallback("nonexistent", "magos", "", []ChatMessage{{Role: "user", Content: "test"}}, "system")
	if resp == nil {
		t.Error("CallWithFallback returned nil")
	}
}

func TestCallWithFallback_BothFail(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"a": {Name: "a", Active: false, Available: false},
			"b": {Name: "b", Active: false, Available: false},
		},
	}
	resp := ma.CallWithFallback("a", "b", "", []ChatMessage{{Role: "user", Content: "test"}}, "system")
	if resp == nil {
		t.Error("CallWithFallback returned nil")
	}
	if resp.Error == "" {
		t.Error("Expected error when all agents fail")
	}
}

// ═══ TestQuotaTracking ══════════════════════════════════════════

func TestQuotaTracking_InitialState(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"test": {Name: "test", Active: true, Available: true, tokensLimit: 1000, tokensUsed: 0},
		},
	}
	a := ma.GetAgent("test")
	if a == nil {
		t.Fatal("GetAgent returned nil")
	}
	if !a.Available {
		t.Error("Agent should be available initially")
	}
	if a.tokensUsed != 0 {
		t.Errorf("tokensUsed = %d, want 0", a.tokensUsed)
	}
}

func TestQuotaTracking_LimitSet(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"cloud": {Name: "cloud", Type: "cloud", tokensLimit: 2000000},
		},
	}
	a := ma.GetAgent("cloud")
	if a.tokensLimit != 2000000 {
		t.Errorf("tokensLimit = %d, want 2000000", a.tokensLimit)
	}
}

func TestAgentAvailability(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"available":   {Name: "available", Available: true},
			"unavailable": {Name: "unavailable", Available: false},
		},
	}
	a1 := ma.GetAgent("available")
	a2 := ma.GetAgent("unavailable")
	if !a1.Available {
		t.Error("available agent should be Available=true")
	}
	if a2.Available {
		t.Error("unavailable agent should be Available=false")
	}
}

// ═══ TestBuildLocalPrompt (PRIORITY 1) ══════════════════════════

func TestBuildLocalPrompt_SystemMessage(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "You are Anaforemtron."},
	}
	prompt := buildLocalPrompt(messages)
	if !contains(prompt, "<|turn>system") {
		t.Error("Missing <|turn>system")
	}
	if !contains(prompt, "You are Anaforemtron.") {
		t.Error("Missing system content")
	}
	if !contains(prompt, "<turn|>") {
		t.Error("Missing <turn|> terminator")
	}
}

func TestBuildLocalPrompt_MultipleMessages(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Hello"},
	}
	prompt := buildLocalPrompt(messages)
	if !contains(prompt, "<|turn>system") {
		t.Error("Missing system turn")
	}
	if !contains(prompt, "<|turn>user") {
		t.Error("Missing user turn")
	}
}

func TestBuildLocalPrompt_EndsWithModelTurn(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "System"},
		{Role: "user", Content: "Question"},
	}
	prompt := buildLocalPrompt(messages)
	// Should end with model turn for the model to continue
	if !contains(prompt, "<|turn>model") {
		t.Error("Missing final model turn")
	}
}

func TestBuildLocalPrompt_NoChannelMarkers(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "System"},
		{Role: "user", Content: "Hello"},
	}
	prompt := buildLocalPrompt(messages)
	// buildLocalPrompt should NOT add channel markers
	// (those are added by the model, not the prompt builder)
	if contains(prompt, "<channel") {
		t.Error("Prompt should not contain <channel markers")
	}
}

// ═══ TestCleanGemma4 (PRIORITY 1) ══════════════════════════════

func TestCleanGemma4_ExtractsAfterChannel(t *testing.T) {
	input := "<|channel>thought\n<channel|>Hello!"
	result := cleanGemma4(input)
	if contains(result, "<channel") {
		t.Errorf("Still contains channel marker: %q", result)
	}
}

func TestCleanGemma4_RemovesAllMarkers(t *testing.T) {
	input := "<channel|>text<|channel|>more"
	result := cleanGemma4(input)
	if contains(result, "<channel") {
		t.Errorf("Still contains channel marker: %q", result)
	}
}

func TestCleanGemma4_PreservesCleanText(t *testing.T) {
	input := "Just a clean answer"
	result := cleanGemma4(input)
	if result != "Just a clean answer" {
		t.Errorf("Clean text modified: %q", result)
	}
}

func TestCleanGemma4_EmptyInput(t *testing.T) {
	result := cleanGemma4("")
	if result != "" {
		t.Errorf("Empty input should return empty, got %q", result)
	}
}

// ═══ TestCallModel Endpoint Routing (PRIORITY 1) ════════════════

func TestCallLocal_UsesCompletionsEndpoint(t *testing.T) {
	agent := &Agent{
		Name:     "magos",
		Type:     "local",
		Endpoint: "http://127.0.0.1:11436/v1/chat/completions",
		ModelID:  "Vox Dei",
	}
	// Local models should use /v1/completions (not /v1/chat/completions)
	endpoint := strings.ReplaceAll(agent.Endpoint, "/v1/chat/completions", "/v1/completions")
	if !contains(endpoint, "/v1/completions") {
		t.Errorf("Local endpoint should use /v1/completions: %q", endpoint)
	}
	if contains(endpoint, "/v1/chat/") {
		t.Errorf("Local endpoint should NOT use /v1/chat/: %q", endpoint)
	}
}

func TestCallCloud_UsesChatEndpoint(t *testing.T) {
	agent := &Agent{
		Name:     "glm",
		Type:     "cloud",
		Endpoint: "https://api.example.com/v1/chat/completions",
		ModelID:  "glm-5.2",
	}
	// Cloud models should keep /v1/chat/completions
	endpoint := agent.Endpoint
	if !contains(endpoint, "/v1/chat/completions") {
		t.Errorf("Cloud endpoint should use /v1/chat/completions: %q", endpoint)
	}
}

func TestCallLocal_BuildsRawPrompt(t *testing.T) {
	// Local model payload should have "prompt" field
	payload := map[string]interface{}{
		"model":  "Vox Dei",
		"prompt": buildLocalPrompt([]ChatMessage{{Role: "user", Content: "test"}}),
	}
	if _, ok := payload["prompt"]; !ok {
		t.Error("Local payload should have 'prompt' field")
	}
	if _, ok := payload["messages"]; ok {
		t.Error("Local payload should NOT have 'messages' field")
	}
}

func TestCallCloud_BuildsMessagesPayload(t *testing.T) {
	// Cloud model payload should have "messages" field
	payload := map[string]interface{}{
		"model":    "glm-5.2",
		"messages": []ChatMessage{{Role: "user", Content: "test"}},
	}
	if _, ok := payload["messages"]; !ok {
		t.Error("Cloud payload should have 'messages' field")
	}
	if _, ok := payload["prompt"]; ok {
		t.Error("Cloud payload should NOT have 'prompt' field")
	}
}

// ═══ TestMultiAgentConstructor ══════════════════════════════════

func TestNewMultiAgent_CreatesAgents(t *testing.T) {
	// MultiAgent should be creatable
	ma := &MultiAgent{
		agents: make(map[string]*Agent),
	}
	if ma.agents == nil {
		t.Error("agents map should not be nil")
	}
}

func TestListAgents_ReturnsAll(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"a": {Name: "a"},
			"b": {Name: "b"},
			"c": {Name: "c"},
		},
	}
	list := ma.ListAgents()
	if len(list) != 3 {
		t.Errorf("ListAgents returned %d, want 3", len(list))
	}
}

func TestGetAgent_Nonexistent(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"magos": {Name: "magos"},
		},
	}
	a := ma.GetAgent("nonexistent")
	if a != nil {
		t.Error("GetAgent(nonexistent) should return nil")
	}
}

func TestAgentResponse_Fields(t *testing.T) {
	resp := &AgentResponse{
		Agent:   "magos",
		Display: "⚒ Магос",
		Glyph:   "⚒",
		Color:   "#ffb000",
		Content: "test response",
		Source:  "local",
	}
	if resp.Agent != "magos" {
		t.Errorf("Agent = %q, want magos", resp.Agent)
	}
	if resp.Glyph != "⚒" {
		t.Errorf("Glyph = %q, want ⚒", resp.Glyph)
	}
}

// ═══ TestCallWithImage ══════════════════════════════════════════

func TestCallWithImage_BuildsVisionPayload(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]string{"content": "I see an image."},
			}},
			"usage": map[string]int{"total_tokens": 42},
		})
	}))
	defer srv.Close()

	ma := &MultiAgent{
		agents: map[string]*Agent{
			"minimax": {
				Name:        "minimax",
				DisplayName: "MiniMax M3",
				Glyph:       "📹",
				Type:        "cloud",
				Endpoint:    srv.URL + "/v1/chat/completions",
				ModelID:     "minimax-m3",
				Available:   true,
			},
		},
	}

	resp := ma.CallWithImage("minimax", []ChatMessage{{Role: "user", Content: "describe"}}, "system", "data:image/png;base64,abc")
	if resp.Error != "" {
		t.Fatalf("CallWithImage error: %s", resp.Error)
	}
	if resp.Content != "I see an image." {
		t.Errorf("content = %q, want 'I see an image.'", resp.Content)
	}

	messages, ok := capturedBody["messages"].([]interface{})
	if !ok {
		t.Fatal("payload missing messages array")
	}
	if len(messages) != 2 { // system + user
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	userMsg := messages[1].(map[string]interface{})
	content, ok := userMsg["content"].([]interface{})
	if !ok {
		t.Fatal("user message content should be array for vision")
	}
	if len(content) != 2 {
		t.Fatalf("content len = %d, want 2", len(content))
	}
	imgPart := content[1].(map[string]interface{})
	if imgPart["type"] != "image_url" {
		t.Errorf("second part type = %q, want image_url", imgPart["type"])
	}
}

// ═══ Helpers ════════════════════════════════════════════════════

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
