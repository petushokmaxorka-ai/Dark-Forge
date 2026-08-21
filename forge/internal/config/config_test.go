package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ═══ TestDefaultConfig ═══════════════════════════════════════════

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("")

	if len(cfg.Models) == 0 {
		t.Error("DefaultConfig has no models")
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 9091 {
		t.Errorf("Port = %d, want 9091", cfg.Server.Port)
	}
	if len(cfg.Routing.Rules) == 0 {
		t.Error("No routing rules")
	}
}

func TestDefaultConfig_HasAllModels(t *testing.T) {
	cfg := DefaultConfig("")
	required := []string{"magos", "skitarii", "servitor", "glm", "kimi", "mimo"}
	for _, name := range required {
		if _, ok := cfg.Models[name]; !ok {
			t.Errorf("Missing model: %s", name)
		}
	}
}

func TestDefaultConfig_HasRoutingRules(t *testing.T) {
	cfg := DefaultConfig("")
	if len(cfg.Routing.Rules) < 5 {
		t.Errorf("Expected 5+ routing rules, got %d", len(cfg.Routing.Rules))
	}
	if cfg.Routing.CloudDownFallback == "" {
		t.Error("CloudDownFallback is empty")
	}
}

// ═══ TestLoad ═══════════════════════════════════════════════════

func TestLoad_MissingFile(t *testing.T) {
	cfg := Load("")
	// Should fall back to defaults
	if len(cfg.Models) == 0 {
		t.Error("Load with missing file should return defaults")
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal forge.json
	data := `{"models":{"test":{"endpoint":"http://localhost","model_id":"test","type":"local"}},"server":{"host":"127.0.0.1","port":9091}}`
	os.WriteFile(filepath.Join(dir, "forge.json"), []byte(data), 0644)

	// Change to that dir so Load finds it
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cfg := Load("")
	if _, ok := cfg.Models["test"]; !ok {
		t.Error("Load should parse forge.json")
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "forge.json"), []byte("{bad json"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cfg := Load("")
	// Should fall back to defaults
	if len(cfg.Models) == 0 {
		t.Error("Malformed JSON should fall back to defaults")
	}
}

// ═══ TestRoute ═══════════════════════════════════════════════════

func TestRoute_SimpleCode(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(3, "code")
	if primary != "qwable" {
		t.Errorf("Route(3, code) = %q, want qwable", primary)
	}
}

func TestRoute_ComplexCode(t *testing.T) {
	cfg := DefaultConfig("")
	primary, fallback, secondary := cfg.Route(8, "code")
	if primary != "qwen_flagship" {
		t.Errorf("Route(8, code) = %q, want qwen_flagship", primary)
	}
	if fallback != "glm" {
		t.Errorf("fallback = %q, want glm", fallback)
	}
	if secondary != "qwable" {
		t.Errorf("secondary = %q, want qwable", secondary)
	}
}

func TestRoute_Test(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "test")
	if primary != "mimo" {
		t.Errorf("Route(5, test) = %q, want mimo", primary)
	}
}

func TestRoute_Math(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(6, "math")
	if primary != "mimo" {
		t.Errorf("Route(6, math) = %q, want mimo", primary)
	}
}

func TestRoute_Creative(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "creative")
	if primary != "kimi" {
		t.Errorf("Route(5, creative) = %q, want kimi", primary)
	}
}

func TestRoute_Fallback(t *testing.T) {
	cfg := DefaultConfig("")
	_, fallback, secondary := cfg.Route(8, "code")
	if fallback != "glm" {
		t.Errorf("fallback = %q, want glm", fallback)
	}
	if secondary != "qwable" {
		t.Errorf("secondary = %q, want qwable", secondary)
	}
}

func TestRoute_Architecture(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "architecture")
	if primary != "glm" {
		t.Errorf("Route(5, architecture) = %q, want glm", primary)
	}
}

func TestRoute_Agentic(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "agentic")
	if primary != "qwable" {
		t.Errorf("Route(5, agentic) = %q, want qwable", primary)
	}
}

func TestRoute_Reasoning(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "reasoning")
	if primary != "qwythos" {
		t.Errorf("Route(5, reasoning) = %q, want qwythos", primary)
	}
}

func TestRoute_Philosophy(t *testing.T) {
	cfg := DefaultConfig("")
	primary, fallback, _ := cfg.Route(5, "philosophy")
	if primary != "qwythos" {
		t.Errorf("Route(5, philosophy) = %q, want qwythos", primary)
	}
	if fallback != "deus_ex_machina" {
		t.Errorf("fallback = %q, want deus_ex_machina", fallback)
	}
}

func TestRoute_Refactor(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "refactor")
	if primary != "qwable" {
		t.Errorf("Route(5, refactor) = %q, want qwable", primary)
	}
}

func TestRoute_ReasoningAndPhilosophyModelsExist(t *testing.T) {
	cfg := DefaultConfig("")
	for _, name := range []string{"qwable", "qwythos"} {
		if _, ok := cfg.Models[name]; !ok {
			t.Errorf("Missing routing target model: %s", name)
		}
	}
}

func TestRoute_DefaultFallback(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(3, "unknown_task")
	if primary != "magos" {
		t.Errorf("Route(3, unknown) = %q, want magos", primary)
	}
}

// ═══ TestGetModel ═══════════════════════════════════════════════

func TestGetModel(t *testing.T) {
	cfg := DefaultConfig("")

	m, ok := cfg.GetModel("magos")
	if !ok {
		t.Fatal("GetModel(magos) returned false")
	}
	if m.ModelID != "Vox Dei" {
		t.Errorf("ModelID = %q, want Vox Dei", m.ModelID)
	}
	if m.Type != "local" {
		t.Errorf("Type = %q, want local", m.Type)
	}

	_, ok = cfg.GetModel("nonexistent")
	if ok {
		t.Error("GetModel(nonexistent) returned true")
	}
}

func TestGetModel_NotFound(t *testing.T) {
	cfg := DefaultConfig("")
	_, ok := cfg.GetModel("nonexistent_model")
	if ok {
		t.Error("GetModel(nonexistent) should return false")
	}
}

func TestGetModel_AllModelsHaveEndpoint(t *testing.T) {
	cfg := DefaultConfig("")
	for name, m := range cfg.Models {
		if m.Endpoint == "" {
			t.Errorf("Model %q has empty endpoint", name)
		}
		if m.ModelID == "" {
			t.Errorf("Model %q has empty ModelID", name)
		}
	}
}

// ═══ TestMatchRule ═══════════════════════════════════════════════

func TestMatchRule_ComplexityLE(t *testing.T) {
	if !matchRule("complexity<=4", 3, "code") {
		t.Error("matchRule(complexity<=4, 3) = false, want true")
	}
	if matchRule("complexity<=4", 5, "code") {
		t.Error("matchRule(complexity<=4, 5) = true, want false")
	}
}

func TestMatchRule_ComplexityGE(t *testing.T) {
	if !matchRule("complexity>=7", 8, "code") {
		t.Error("matchRule(complexity>=7, 8) = false, want true")
	}
	if matchRule("complexity>=7", 5, "code") {
		t.Error("matchRule(complexity>=7, 5) = true, want false")
	}
}

func TestMatchRule_TaskEq(t *testing.T) {
	if !matchRule("task==test", 5, "test") {
		t.Error("matchRule(task==test, test) = false, want true")
	}
	if matchRule("task==test", 5, "code") {
		t.Error("matchRule(task==test, code) = true, want false")
	}
}

func TestMatchRule_ComplexityAndTask(t *testing.T) {
	if !matchRule("complexity>=5 and task==code", 8, "code") {
		t.Error("matchRule(complexity>=5 and task==code, 8, code) = false, want true")
	}
	if matchRule("complexity>=5 and task==code", 3, "code") {
		t.Error("matchRule(complexity>=5 and task==code, 3, code) = true, want false")
	}
	if matchRule("complexity>=5 and task==code", 8, "test") {
		t.Error("matchRule(complexity>=5 and task==code, 8, test) = true, want false")
	}
}

func TestMatchRule_NoMatch(t *testing.T) {
	if matchRule("invalid_rule", 5, "code") {
		t.Error("matchRule(invalid_rule) = true, want false")
	}
}

func TestMatchRule_EmptyMatch(t *testing.T) {
	if matchRule("", 5, "code") {
		t.Error("matchRule('') = true, want false")
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"4", 4},
		{"10", 10},
		{"0", 0},
		{"abc", 0},
		{"7abc", 7},
	}
	for _, tt := range tests {
		got := parseInt(tt.input)
		if got != tt.want {
			t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ═══ TestDefaultConfig_HasMagos (PRIORITY 2) ════════════════════

func TestDefaultConfig_HasMagos(t *testing.T) {
	cfg := DefaultConfig("")
	magos, ok := cfg.GetModel("magos")
	if !ok {
		t.Fatal("DefaultConfig missing magos")
	}
	if magos.ModelID != "Vox Dei" {
		t.Errorf("magos ModelID = %q, want Vox Dei", magos.ModelID)
	}
	if magos.Type != "local" {
		t.Errorf("magos Type = %q, want local", magos.Type)
	}
}

func TestDefaultConfig_LocalModelsUseCompletions(t *testing.T) {
	cfg := DefaultConfig("")
	for name, m := range cfg.Models {
		if m.Type == "local" {
			// Local models have /v1/chat/completions in config
			// but router.go converts to /v1/completions
			if !contains(m.Endpoint, "/v1/chat/completions") && !contains(m.Endpoint, "/v1/completions") {
				t.Errorf("Local model %q endpoint missing completions path: %q", name, m.Endpoint)
			}
		}
	}
}

func TestDefaultConfig_CloudModelsHaveEndpoint(t *testing.T) {
	cfg := DefaultConfig("")
	for name, m := range cfg.Models {
		if m.Type == "cloud" && m.Endpoint == "" {
			t.Errorf("Cloud model %q has no endpoint", name)
		}
	}
}

func TestDefaultConfig_ServerBindsLocalhost(t *testing.T) {
	cfg := DefaultConfig("")
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want 127.0.0.1", cfg.Server.Host)
	}
}

func TestDefaultConfig_DefaultPort(t *testing.T) {
	cfg := DefaultConfig("")
	if cfg.Server.Port != 9091 {
		t.Errorf("Server.Port = %d, want 9091", cfg.Server.Port)
	}
}

func TestLoad_YAMLConfig(t *testing.T) {
	// Load with no file → should use defaults
	cfg := Load("")
	if len(cfg.Models) == 0 {
		t.Error("Load should return defaults when no file found")
	}
}

func TestLoad_JSONFallback(t *testing.T) {
	dir := t.TempDir()
	// Create a forge.json
	data := `{"models":{"test":{"endpoint":"http://localhost","model_id":"test","type":"local"}},"server":{"host":"127.0.0.1","port":9091}}`
	os.WriteFile(filepath.Join(dir, "forge.json"), []byte(data), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cfg := Load("")
	if _, ok := cfg.Models["test"]; !ok {
		t.Error("Load should parse forge.json")
	}
}

func TestRoute_Debug(t *testing.T) {
	cfg := DefaultConfig("")
	primary, _, _ := cfg.Route(5, "debug")
	if primary != "qwable" {
		t.Errorf("Route(5, debug) = %q, want qwable", primary)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
