package model

import (
	"testing"
)

// ═══ TestShouldDelegate_ComplexCode ══════════════════════════════

func TestShouldDelegate_ComplexCode(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"qwable": {Name: "qwable", Available: true},
			"glm":    {Name: "glm", Available: true},
			"kimi":   {Name: "kimi", Available: true},
		},
	}
	sug := ma.ShouldDelegate(8, "code")
	if sug == nil {
		t.Fatal("Expected delegation for complex code")
	}
	if sug.Agent != "qwable" {
		t.Errorf("Agent = %q, want qwable", sug.Agent)
	}
	if sug.Ask {
		t.Error("Local code should not ask Principal")
	}
}

func TestShouldDelegate_VeryHeavyCode(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"qwable": {Name: "qwable", Available: true},
			"glm":    {Name: "glm", Available: true},
		},
	}
	sug := ma.ShouldDelegate(8, "code")
	if sug == nil {
		t.Fatal("Expected delegation")
	}
	if sug.Agent != "qwable" {
		t.Errorf("Agent = %q, want qwable", sug.Agent)
	}
}

// ═══ TestShouldDelegate_TestTask ════════════════════════════════

func TestShouldDelegate_TestTask(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"mimo":   {Name: "mimo", Available: true},
			"qwable": {Name: "qwable", Available: true},
		},
	}
	sug := ma.ShouldDelegate(5, "test")
	if sug == nil {
		t.Fatal("Expected delegation for test task")
	}
	if sug.Agent != "qwable" {
		t.Errorf("Agent = %q, want qwable", sug.Agent)
	}
	if sug.Ask {
		t.Error("Test tasks should auto-delegate (Ask=false)")
	}
}

// ═══ TestShouldDelegate_CreativeTask ════════════════════════════

func TestShouldDelegate_CreativeTask(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"kimi":    {Name: "kimi", Available: true},
			"qwythos": {Name: "qwythos", Available: true},
		},
	}
	sug := ma.ShouldDelegate(5, "creative")
	if sug == nil {
		t.Fatal("Expected delegation for creative task")
	}
	if sug.Agent != "kimi" {
		t.Errorf("Agent = %q, want kimi", sug.Agent)
	}
}

// ═══ TestShouldDelegate_DeployTask ══════════════════════════════

func TestShouldDelegate_DeployTask(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"minimax": {Name: "minimax", Available: true},
			"qwable":  {Name: "qwable", Available: true},
		},
	}
	sug := ma.ShouldDelegate(5, "deploy")
	if sug == nil {
		t.Fatal("Expected delegation for deploy task")
	}
	if sug.Agent != "minimax" {
		t.Errorf("Agent = %q, want minimax", sug.Agent)
	}
	if sug.Ask {
		t.Error("Deploy tasks should auto-delegate (Ask=false)")
	}
}

// ═══ TestShouldDelegate_SimpleTask ══════════════════════════════

func TestShouldDelegate_SimpleTask(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"magos": {Name: "magos", Available: true},
		},
	}
	sug := ma.ShouldDelegate(3, "code")
	if sug != nil {
		t.Errorf("Simple task should not delegate, got %v", sug)
	}
}

func TestShouldDelegate_SimpleTask2(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{},
	}
	sug := ma.ShouldDelegate(2, "general")
	if sug != nil {
		t.Errorf("Complexity 2 should not delegate, got %v", sug)
	}
}

// ═══ TestShouldDelegate_MathTask ════════════════════════════════

func TestShouldDelegate_MathTask(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"mimo":    {Name: "mimo", Available: true},
			"qwythos": {Name: "qwythos", Available: true},
		},
	}
	sug := ma.ShouldDelegate(6, "math")
	if sug == nil {
		t.Fatal("Expected delegation for math task")
	}
	if sug.Agent != "qwythos" {
		t.Errorf("Agent = %q, want qwythos", sug.Agent)
	}
}

// ═══ TestShouldDelegate_ArchitectureTask ════════════════════════

func TestShouldDelegate_ArchitectureTask(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"glm":    {Name: "glm", Available: true},
			"qwable": {Name: "qwable", Available: true},
		},
	}
	sug := ma.ShouldDelegate(6, "architecture")
	if sug == nil {
		t.Fatal("Expected delegation for architecture task")
	}
	if sug.Agent != "qwable" {
		t.Errorf("Agent = %q, want qwable", sug.Agent)
	}
}

// ═══ TestShouldDelegate_HighComplexityUnknown ═══════════════════

func TestShouldDelegate_HighComplexityUnknown(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"glm":     {Name: "glm", Available: true},
			"qwythos": {Name: "qwythos", Available: true},
		},
	}
	sug := ma.ShouldDelegate(8, "unknown_type")
	if sug == nil {
		t.Fatal("High complexity unknown type should delegate to GLM")
	}
	if sug.Agent != "glm" {
		t.Errorf("Agent = %q, want glm", sug.Agent)
	}
}

// ═══ TestDelegationSuggestion_Fields ════════════════════════════

func TestDelegationSuggestion_Fields(t *testing.T) {
	sug := &DelegationSuggestion{
		Agent:        "glm",
		Reason:       "Complex code",
		Alternatives: []string{"kimi"},
		Ask:          true,
	}
	if sug.Agent != "glm" {
		t.Errorf("Agent = %q", sug.Agent)
	}
	if sug.Reason != "Complex code" {
		t.Errorf("Reason = %q", sug.Reason)
	}
	if len(sug.Alternatives) != 1 {
		t.Errorf("Alternatives = %d, want 1", len(sug.Alternatives))
	}
	if !sug.Ask {
		t.Error("Ask should be true")
	}
}

// ═══ TestDelegationSuggestion_NoAlternatives ════════════════════

func TestDelegationSuggestion_NoAlternatives(t *testing.T) {
	sug := &DelegationSuggestion{
		Agent:  "mimo",
		Reason: "Tests",
		Ask:    false,
	}
	if len(sug.Alternatives) != 0 {
		t.Errorf("Alternatives = %d, want 0", len(sug.Alternatives))
	}
}

// ═══ TestAutoDelegate_TriggeredByComplexity ═════════════════════

func TestAutoDelegate_TriggeredByComplexity(t *testing.T) {
	ma := &MultiAgent{
		agents: map[string]*Agent{
			"glm":     {Name: "glm", Available: true},
			"kimi":    {Name: "kimi", Available: true},
			"mimo":    {Name: "mimo", Available: true},
			"qwable":  {Name: "qwable", Available: true},
			"qwythos": {Name: "qwythos", Available: true},
			"minimax": {Name: "minimax", Available: true},
		},
	}

	tests := []struct {
		complexity int
		taskType   string
		wantAgent  string
		wantNil    bool
	}{
		{2, "code", "", true},       // simple → no delegation
		{3, "test", "", true},       // simple → no delegation
		{5, "code", "qwable", false},   // medium code → local
		{8, "code", "qwable", false},   // complex code → local
		{5, "test", "qwable", false},  // test → local
		{5, "creative", "kimi", false}, // creative → Kimi
		{5, "deploy", "minimax", false}, // deploy → MiniMax
	}

	for _, tt := range tests {
		sug := ma.ShouldDelegate(tt.complexity, tt.taskType)
		if tt.wantNil {
			if sug != nil {
				t.Errorf("complexity=%d, type=%q: expected nil, got %v", tt.complexity, tt.taskType, sug)
			}
		} else {
			if sug == nil {
				t.Errorf("complexity=%d, type=%q: expected delegation, got nil", tt.complexity, tt.taskType)
			} else if sug.Agent != tt.wantAgent {
				t.Errorf("complexity=%d, type=%q: agent=%q, want %q", tt.complexity, tt.taskType, sug.Agent, tt.wantAgent)
			}
		}
	}
}
