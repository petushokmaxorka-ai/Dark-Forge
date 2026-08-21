// Package config — HereticArch Forge configuration
// Loads forge.yaml, provides model routing
// «Veritas in Crypta.»
package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelDef defines a single model endpoint
type ModelDef struct {
	Endpoint   string `json:"endpoint" yaml:"endpoint"`
	ModelID    string `json:"model_id" yaml:"model_id"`
	Type       string `json:"type" yaml:"type"`         // "local" or "cloud"
	Proxy      string `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	ApiKey     string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	SpeedTPS   int    `json:"speed_tps" yaml:"speed_tps"`
	Context    int    `json:"context_tokens" yaml:"context_tokens"`
	Role       string `json:"role" yaml:"role"`
	Desc       string `json:"description" yaml:"description"`
}

// RoutingRule defines when to use which model
type RoutingRule struct {
	Match            string `json:"match" yaml:"match"`
	Route            string `json:"route" yaml:"route"`
	Fallback         string `json:"fallback" yaml:"fallback"`
	SecondaryFallback string `json:"secondary_fallback,omitempty" yaml:"secondary_fallback,omitempty"`
	Reason           string `json:"reason" json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ForgeConfig is the full configuration
type ForgeConfig struct {
	Models  map[string]ModelDef `json:"models" yaml:"models"`
	Routing struct {
		Rules             []RoutingRule `json:"rules" yaml:"rules"`
		CloudDownFallback string        `json:"cloud_all_down" yaml:"cloud_all_down"`
	} `json:"routing" yaml:"routing"`
	Server struct {
		Host string `json:"host" yaml:"host"`
		Port int    `json:"port" yaml:"port"`
		Repo string `json:"repo" yaml:"repo"`
	} `json:"server" yaml:"server"`
	Commands []CustomCommand `json:"commands,omitempty" yaml:"commands,omitempty"`
}

// DefaultConfig returns hardcoded portable defaults.
// Endpoints assume a local OpenAI-compatible server (llama.cpp / Ollama /
// llama-swap) and vendor-canonical cloud APIs without proxies; override
// everything via forge.yaml.
func DefaultConfig(modelURL string) *ForgeConfig {
	local := modelURL
	if local == "" {
		local = "http://127.0.0.1:11434"
	}

	cfg := &ForgeConfig{}
	cfg.Models = map[string]ModelDef{
		"magos": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Vox Dei",
			Type:     "local",
			SpeedTPS: 33,
			Context:  4096,
			Role:     "memory-general",
			Desc:     "Gemma4-12B Q4_K_M — memory, general (RAM offloaded)",
		},
		"skitarii": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Vox Minor",
			Type:     "local",
			SpeedTPS: 80,
			Context:  4096,
			Role:     "fast-explorer",
			Desc:     "1.5B Qwen Coder — fast recon",
		},
		"qwable": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Qwable-9B",
			Type:     "local",
			SpeedTPS: 40,
			Context:  76800,
			Role:     "coding-agentic",
			Desc:     "Qwen3.5-9B + Claude Fable5 Q4_K_M — coding, agentic, terminal (GPU 0)",
		},
		"qwythos": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Qwythos-9B",
			Type:     "local",
			SpeedTPS: 40,
			Context:  1048576,
			Role:     "reasoning-philosophy",
			Desc:     "Qwen3.5-9B + Claude Mythos Q4_K_M — reasoning, philosophy (GPU 1)",
		},
		"servitor": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Ferrum Flagellum",
			Type:     "local",
			SpeedTPS: 25,
			Context:  8192,
			Role:     "code-editor",
			Desc:     "8B Llama — diff apply + lint",
		},
		"oculus": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Oculus Maleficarum",
			Type:     "local",
			SpeedTPS: 40,
			Context:  4096,
			Role:     "vision-specialist",
			Desc:     "4B Qwen3-VL — local vision (image/video analysis)",
		},
		"fabricator": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Fabricator Codicis",
			Type:     "local",
			SpeedTPS: 20,
			Context:  8192,
			Role:     "heavy-code-specialist",
			Desc:     "14B Qwen3 — multi-file code, API, bugs, pytest",
		},
		"magos_dominus": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Magos Dominus",
			Type:     "local",
			SpeedTPS: 15,
			Context:  4096,
			Role:     "architecture-specialist",
			Desc:     "17B GLM-4.7-Flash — architecture, complex analysis",
		},
		"haereticus": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Haereticus Mechanicus",
			Type:     "local",
			SpeedTPS: 10,
			Context:  4096,
			Role:     "research-specialist",
			Desc:     "20B gpt-oss — unconventional, deep reasoning",
		},
		"archmagos": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Archmagos Diabolus",
			Type:     "local",
			SpeedTPS: 25,
			Context:  4096,
			Role:     "fast-heavyweight",
			Desc:     "35B MoE (3B active) — fast heavyweight research",
		},
		"deus_ex_machina": {
			Endpoint: local + "/v1/chat/completions",
			ModelID:  "Deus Ex Machina",
			Type:     "local",
			SpeedTPS: 5,
			Context:  4096,
			Role:     "philosophy-specialist",
			Desc:     "70B DeepSeek R1 — massive reasoning, math proofs, strategy",
		},
		"glm": {
			Endpoint: "https://api.z.ai/api/coding/paas/v4/chat/completions",
			ModelID:  "glm-5.2",
			Type:     "cloud",
			Proxy:    "",
			SpeedTPS: 120,
			Context:  1000000,
			Role:     "primary-coder",
			Desc:     "GLM-5.2 — coding, 1M ctx (needs HERETIC_GLM_API_KEY)",
		},
		"kimi": {
			Endpoint: "https://api.kimi.com/coding/v1/chat/completions",
			ModelID:  "kimi-for-coding",
			Type:     "cloud",
			Proxy:    "",
			SpeedTPS: 120,
			Context:  256000,
			Role:     "creative-specialist",
			Desc:     "Kimi K2.7 — creative + agentic (needs HERETIC_KIMI_API_KEY)",
		},
		"mimo": {
			Endpoint: "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions",
			ModelID:  "mimo-v2.5-pro",
			Type:     "cloud",
			Proxy:    "",
			SpeedTPS: 100,
			Context:  1000000,
			Role:     "test-math-specialist",
			Desc:     "MiMo V2.5 Pro — tests + math (needs HERETIC_MIMO_API_KEY)",
		},
		"qwen": {
			Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
			ModelID:  "qwen-max",
			Type:     "cloud",
			Proxy:    "",
			SpeedTPS: 110,
			Context:  1000000,
			Role:     "refactor-specialist",
			Desc:     "Qwen3.7 Max — multilingual stability (needs HERETIC_QWEN_API_KEY)",
		},
		"qwen_flagship": {
			Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
			ModelID:  "qwen3-coder-plus",
			Type:     "cloud",
			Proxy:    "",
			SpeedTPS: 80,
			Context:  131072,
			Role:     "heavy-coder-specialist",
			Desc:     "Qwen3-Coder flagship — heavy code MoE (needs HERETIC_QWEN_API_KEY)",
		},
		"minimax": {
			Endpoint: "https://api.minimaxi.chat/v1/chat/completions",
			ModelID:  "MiniMax-M3",
			Type:     "cloud",
			Proxy:    "",
			SpeedTPS: 90,
			Context:  1000000,
			Role:     "multimodal-specialist",
			Desc:     "MiniMax M3 — multimodal (needs HERETIC_MINIMAX_API_KEY)",
		},
	}

	// Routing rules: local 3-model swarm is primary; cloud models are fallbacks
	cfg.Routing.Rules = []RoutingRule{
		// Very heavy code still goes to cloud flagship, then local Qwable
		{Match: "complexity>=7 and task==code", Route: "qwen_flagship", Fallback: "glm", SecondaryFallback: "qwable", Reason: "Heavy code → Qwen3-Coder-480B, fallback to local Qwable"},
		// Code and terminal/agentic tasks → local Qwable (GPU 0)
		{Match: "complexity>=5 and task==code", Route: "qwable", Fallback: "glm", SecondaryFallback: "magos", Reason: "Code → Qwable (local Fable coding brain)"},
		{Match: "task==code", Route: "qwable", Fallback: "glm", SecondaryFallback: "magos", Reason: "Code → Qwable (local Fable coding brain)"},
		{Match: "task==debug", Route: "qwable", Fallback: "glm", SecondaryFallback: "magos", Reason: "Debug → Qwable (local)"},
		{Match: "task==refactor", Route: "qwable", Fallback: "glm", SecondaryFallback: "magos", Reason: "Refactor → Qwable (local)"},
		{Match: "task==agentic", Route: "qwable", Fallback: "kimi", SecondaryFallback: "magos", Reason: "Agentic/terminal → Qwable (local)"},
		// Reasoning and philosophy → local Qwythos (GPU 1)
		{Match: "task==reasoning", Route: "qwythos", Fallback: "glm", SecondaryFallback: "magos", Reason: "Reasoning → Qwythos (local Mythos brain)"},
		{Match: "task==philosophy", Route: "qwythos", Fallback: "deus_ex_machina", SecondaryFallback: "magos", Reason: "Philosophy → Qwythos (local Mythos brain)"},
		// Simple non-code tasks → Vox Dei (RAM)
		{Match: "complexity<=4", Route: "magos", Fallback: "magos", Reason: "Simple → local Vox Dei (free)"},
		// Remaining cloud-backed specializations
		{Match: "task==architecture", Route: "glm", Fallback: "kimi", SecondaryFallback: "magos", Reason: "Architecture → GLM (#2 WebDev)"},
		{Match: "task==test", Route: "mimo", Fallback: "glm", SecondaryFallback: "magos", Reason: "Tests → MiMo (#14 Math)"},
		{Match: "task==math", Route: "mimo", Fallback: "glm", SecondaryFallback: "magos", Reason: "Math → MiMo (#14 Math)"},
		{Match: "task==creative", Route: "kimi", Fallback: "mimo", SecondaryFallback: "glm", Reason: "Creative → Kimi (#4 Creative)"},
		{Match: "task==multimodal", Route: "minimax", Fallback: "glm", SecondaryFallback: "magos", Reason: "Multimodal → MiniMax (video+image)"},
		// Local specialists (free, no cloud needed)
		{Match: "task==vision", Route: "oculus", Fallback: "minimax", SecondaryFallback: "magos", Reason: "Vision → Oculus 4B VL (local, free)"},
		{Match: "task==heavy_code", Route: "fabricator", Fallback: "glm", SecondaryFallback: "magos", Reason: "Heavy code → Fabricator 14B (local, free)"},
		{Match: "task==architecture and cloud_down", Route: "magos_dominus", Fallback: "glm", SecondaryFallback: "magos", Reason: "Architecture (cloud down) → Magos Dominus 17B (local)"},
		{Match: "task==research and nocturna", Route: "archmagos", Fallback: "glm", SecondaryFallback: "magos", Reason: "Research (Nocturna) → Archmagos 35B MoE (local, free)"},
		{Match: "task==philosophy and nocturna", Route: "deus_ex_machina", Fallback: "glm", SecondaryFallback: "magos", Reason: "Philosophy (Nocturna) → Deus Ex Machina 70B (local, free)"},
	}
	cfg.Routing.CloudDownFallback = "magos"

	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 9091
	cfg.Server.Repo = "."

	return cfg
}

// Load reads forge.yaml if exists, then forge.json, otherwise returns defaults.
// Search order: $DARKFORGE_CONFIG, ~/.config/dark-forge/forge.yaml, then CWD.
func Load(modelURL string) *ForgeConfig {
	paths := []string{}
	if p := os.Getenv("DARKFORGE_CONFIG"); p != "" {
		paths = append(paths, p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "dark-forge", "forge.yaml"))
	}
	paths = append(paths, "forge.yaml", "config/forge.yaml", "../forge.yaml")
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			cfg := &ForgeConfig{}
			if err := yaml.Unmarshal(data, cfg); err == nil {
				log.Printf("⚙ Config loaded from %s", p)
				cfg.expandEnvVars()
				return cfg
			}
			log.Printf("⚠ Failed to parse %s: %v", p, err)
		}
	}

	// Fallback to JSON if no YAML found
	paths = []string{"forge.json", "heretic-forge/forge.json", "../forge.json"}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			cfg := &ForgeConfig{}
			if err := json.Unmarshal(data, cfg); err == nil {
				log.Printf("⚙ Config loaded from %s", p)
				cfg.expandEnvVars()
				return cfg
			}
			log.Printf("⚠ Failed to parse %s: %v", p, err)
		}
	}

	// Fall back to defaults
	log.Printf("⚙ Using default config (no forge.yaml or forge.json found)")
	return DefaultConfig(modelURL)
}

// expandEnvVars resolves ${VAR} or $VAR references in ApiKey and Proxy fields.
func (c *ForgeConfig) expandEnvVars() {
	for name, m := range c.Models {
		m.ApiKey = os.ExpandEnv(m.ApiKey)
		m.Proxy = os.ExpandEnv(m.Proxy)
		c.Models[name] = m
	}
}

// LoadWithCommands loads forge.yaml and merges commands.yaml custom commands.
func LoadWithCommands(modelURL string) *ForgeConfig {
	cfg := Load(modelURL)
	if cfg == nil {
		cfg = DefaultConfig(modelURL)
	}
	repoPath := cfg.Server.Repo
	if repoPath == "" || repoPath == "." {
		repoPath, _ = os.Getwd()
	}
	repoPath = os.ExpandEnv(repoPath)
	if strings.HasPrefix(repoPath, "~/") {
		home, _ := os.UserHomeDir()
		repoPath = filepath.Join(home, repoPath[2:])
	}
	commands, err := LoadCustomCommands(repoPath)
	if err != nil {
		log.Printf("⚠ Failed to load commands.yaml: %v", err)
	}
	for i := range commands {
		commands[i].ExpandCommand(repoPath)
	}
	cfg.Commands = commands
	return cfg
}

// ExpandRepo resolves the configured repo path, expanding ~ and env vars.
func (c *ForgeConfig) ExpandRepo() string {
	repoPath := c.Server.Repo
	if repoPath == "" || repoPath == "." {
		repoPath, _ = os.Getwd()
	}
	repoPath = os.ExpandEnv(repoPath)
	if strings.HasPrefix(repoPath, "~/") {
		home, _ := os.UserHomeDir()
		repoPath = filepath.Join(home, repoPath[2:])
	}
	return repoPath
}

// GetModel returns model definition by name
func (c *ForgeConfig) GetModel(name string) (ModelDef, bool) {
	m, ok := c.Models[name]
	return m, ok
}

// Route decides which model to use based on complexity and task type
func (c *ForgeConfig) Route(complexity int, taskType string) (string, string, string) {
	// Returns: (primary, fallback, secondary_fallback)

	for _, rule := range c.Routing.Rules {
		if matchRule(rule.Match, complexity, taskType) {
			return rule.Route, rule.Fallback, rule.SecondaryFallback
		}
	}

	// Default: magos (local)
	return "magos", "magos", ""
}

func matchRule(match string, complexity int, taskType string) bool {
	match = strings.ReplaceAll(match, " ", "")

	if strings.Contains(match, "complexity<=") {
		threshold := parseInt(strings.TrimPrefix(match, "complexity<="))
		return complexity <= threshold
	}
	if strings.Contains(match, "complexity>=") {
		threshold := parseInt(strings.TrimPrefix(match, "complexity>="))
		if !strings.Contains(match, "and") {
			return complexity >= threshold
		}
		// Check AND condition
		parts := strings.SplitN(match, "and", 2)
		cond1 := strings.Contains(parts[0], ">=") && complexity >= parseInt(strings.TrimPrefix(parts[0], "complexity>="))
		cond2 := strings.Contains(parts[1], "task==") && taskType == strings.TrimPrefix(parts[1], "task==")
		return cond1 && cond2
	}
	if strings.HasPrefix(match, "task==") {
		return taskType == strings.TrimPrefix(match, "task==")
	}
	return false
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}
