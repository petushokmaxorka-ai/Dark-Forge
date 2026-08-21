// Package theme — Dark Mechanicus lipgloss palette for Bubble Tea TUI.
// «Veritas in Crypta. Ordo ab Chao.»
package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// ThemeName identifies one of the built-in Forge color themes.
type ThemeName string

const (
	ThemeDarkMechanicus ThemeName = "dark"
	ThemeNoosphere      ThemeName = "noosphere"
	ThemeHeresy         ThemeName = "heresy"
)

// themeStorage caches the active theme and persists it to disk.
type themeStorage struct {
	mu       sync.RWMutex
	current  ThemeName
	initOnce sync.Once
}

var ts themeStorage

func themeConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "hereticarch", "theme.txt")
}

func loadThemeFromDisk() ThemeName {
	p := themeConfigPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return ThemeDarkMechanicus
	}
	name := strings.TrimSpace(string(data))
	switch ThemeName(name) {
	case ThemeNoosphere, ThemeHeresy:
		return ThemeName(name)
	default:
		return ThemeDarkMechanicus
	}
}

func saveThemeToDisk(name ThemeName) {
	p := themeConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(string(name)), 0644)
}

func ensureThemeLoaded() {
	ts.initOnce.Do(func() {
		ts.current = loadThemeFromDisk()
	})
}

// SetTheme switches the active color theme and persists it.
func SetTheme(name ThemeName) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ensureThemeLoaded()
	switch name {
	case ThemeDarkMechanicus, ThemeNoosphere, ThemeHeresy:
		ts.current = name
		saveThemeToDisk(name)
		applyTheme(name)
	default:
		fmt.Fprintf(os.Stderr, "unknown theme %q, keeping %s\n", name, ts.current)
	}
}

// CurrentTheme returns the active theme name.
func CurrentTheme() ThemeName {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	ensureThemeLoaded()
	return ts.current
}

// Palette holds all colors for a single theme.
type Palette struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Success   lipgloss.Color
	Error     lipgloss.Color
	Warning   lipgloss.Color
	Dim       lipgloss.Color
	Bg        lipgloss.Color
	BgLight   lipgloss.Color
	Text      lipgloss.Color
}

var palettes = map[ThemeName]Palette{
	ThemeDarkMechanicus: {
		Primary:   lipgloss.Color("#FFB000"), // Gold
		Secondary: lipgloss.Color("#FF0000"), // Blood Red
		Accent:    lipgloss.Color("#00FFFF"), // Cyan
		Success:   lipgloss.Color("#39FF14"), // Green
		Error:     lipgloss.Color("#FF0000"),
		Warning:   lipgloss.Color("#CD7F32"), // Rust
		Dim:       lipgloss.Color("#666666"),
		Bg:        lipgloss.Color("#0A0A0A"),
		BgLight:   lipgloss.Color("#141414"),
		Text:      lipgloss.Color("#E0E0E0"),
	},
	ThemeNoosphere: {
		Primary:   lipgloss.Color("#00FFFF"), // Cyan
		Secondary: lipgloss.Color("#1E90FF"), // Blue
		Accent:    lipgloss.Color("#FFFFFF"), // White
		Success:   lipgloss.Color("#00FA9A"), // Spring Green
		Error:     lipgloss.Color("#FF6B6B"), // Soft Red
		Warning:   lipgloss.Color("#87CEEB"), // Sky Blue
		Dim:       lipgloss.Color("#778899"), // Light Slate
		Bg:        lipgloss.Color("#05121A"), // Deep navy
		BgLight:   lipgloss.Color("#0B1E2A"),
		Text:      lipgloss.Color("#F0F8FF"), // Alice
	},
	ThemeHeresy: {
		Primary:   lipgloss.Color("#DC143C"), // Crimson
		Secondary: lipgloss.Color("#8B0000"), // Dark Red
		Accent:    lipgloss.Color("#000000"), // Black
		Success:   lipgloss.Color("#FF4500"), // Orange Red
		Error:     lipgloss.Color("#FF0000"),
		Warning:   lipgloss.Color("#B22222"), // Firebrick
		Dim:       lipgloss.Color("#555555"),
		Bg:        lipgloss.Color("#050000"),
		BgLight:   lipgloss.Color("#110000"),
		Text:      lipgloss.Color("#FFE4E1"), // Misty Rose
	},
}

// Current palette accessors
var (
	ColorGold    lipgloss.Color
	ColorRed     lipgloss.Color
	ColorRust    lipgloss.Color
	ColorGreen   lipgloss.Color
	ColorCyan    lipgloss.Color
	ColorDim     lipgloss.Color
	ColorBg      lipgloss.Color
	ColorBgLight lipgloss.Color
)

func applyTheme(name ThemeName) {
	p := palettes[name]
	ColorGold = p.Primary
	ColorRed = p.Secondary
	ColorRust = p.Warning
	ColorGreen = p.Success
	ColorCyan = p.Accent
	ColorDim = p.Dim
	ColorBg = p.Bg
	ColorBgLight = p.BgLight
	ColorBg = p.Bg
	ColorBgLight = p.BgLight

	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Primary).
		Background(p.Bg).
		Padding(0, 2)

	UserStyle = lipgloss.NewStyle().
		Foreground(p.Accent).
		Bold(true)

	AgentStyle = lipgloss.NewStyle().
		Foreground(p.Primary)

	AssistantStyle = lipgloss.NewStyle().
		Foreground(p.Text)

	ErrorStyle = lipgloss.NewStyle().
		Foreground(p.Error).
		Bold(true)

	WarningStyle = lipgloss.NewStyle().
		Foreground(p.Warning)

	SuccessStyle = lipgloss.NewStyle().
		Foreground(p.Success)

	DimStyle = lipgloss.NewStyle().
		Foreground(p.Dim)

	BoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Secondary).
		Padding(1, 2)

	HeaderStyle = lipgloss.NewStyle().
		Foreground(p.Primary).
		Background(p.Bg).
		Bold(true).
		Padding(0, 1)

	StatusStyle = lipgloss.NewStyle().
		Foreground(p.Dim).
		Background(p.BgLight).
		Padding(0, 1)

	InputPromptStyle = lipgloss.NewStyle().
		Foreground(p.Primary).
		Bold(true)

	InputStyle = lipgloss.NewStyle().
		Foreground(p.Accent)

	ToolCallStyle = lipgloss.NewStyle().
		Foreground(p.Warning).
		Italic(true)

	BannerStyle = lipgloss.NewStyle().
		Foreground(p.Primary).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(p.Secondary).
		Padding(1, 4).
		Align(lipgloss.Center)
}

// Style bundles for the TUI
var (
	TitleStyle       lipgloss.Style
	UserStyle        lipgloss.Style
	AgentStyle       lipgloss.Style
	AssistantStyle   lipgloss.Style
	ErrorStyle       lipgloss.Style
	WarningStyle     lipgloss.Style
	SuccessStyle     lipgloss.Style
	DimStyle         lipgloss.Style
	BoxStyle         lipgloss.Style
	HeaderStyle      lipgloss.Style
	StatusStyle      lipgloss.Style
	InputPromptStyle lipgloss.Style
	InputStyle       lipgloss.Style
	ToolCallStyle    lipgloss.Style
	BannerStyle      lipgloss.Style
)

// Glyphs (ASCII Dark Mechanicus, not emojis)
const (
	GlyphBuild    = "⚒"
	GlyphThink    = "⚙"
	GlyphScan     = "☉"
	GlyphApply    = "➜"
	GlyphOK       = "✓"
	GlyphBlock    = "✗"
	GlyphRollback = "☠"
)

// Banner returns the startup banner string for the TUI.
func Banner() string {
	return BannerStyle.Render(
		"\n" + GlyphBuild + "  DARK FORGE v1.1\n\n" +
			"Dark Mechanicus Coding Terminal\n" +
			"«Per Ignis, per Ferrum, per Codicem.»\n\n" +
			"6 agents | 15 models | 3-model local swarm | OpenCode replica\n\n" +
			"/help for commands | @agent to delegate | Ctrl+C exit",
	)
}

// AgentGlyph returns the display glyph + name for an agent.
func AgentGlyph(name string) string {
	switch name {
	case "magos":
		return GlyphBuild + " Magos"
	case "glm":
		return GlyphThink + " GLM-5.2"
	case "kimi":
		return GlyphScan + " Kimi"
	case "mimo":
		return GlyphScan + " MiMo"
	case "qwen":
		return GlyphApply + " Qwen"
	case "minimax":
		return GlyphBuild + " MiniMax"
	case "vox_dei":
		return GlyphThink + " Vox Dei (memory)"
	case "qwable":
		return GlyphBuild + " Qwable"
	case "qwythos":
		return GlyphScan + " Qwythos"
	default:
		return "?" + " " + name
	}
}

// ── Brand color aliases (locked to cogitator-browser theme.css) ──
const (
	// ColorOmnissiahRed is the danger / focus accent (#FF0000).
	ColorOmnissiahRed = lipgloss.Color("#FF0000")
	// ColorCogitatorGold is the primary brand accent (#C8A84B).
	ColorCogitatorGold = lipgloss.Color("#C8A84B")
	// ColorNoosphereCyan is the user-prompt / online status accent (#00BFBF).
	ColorNoosphereCyan = lipgloss.Color("#00BFBF")
	// ColorSacredWhite is the primary text color (#E8E8E8).
	ColorSacredWhite = lipgloss.Color("#E8E8E8")
	// ColorParchment is the default body text color (#D4C5A0).
	ColorParchment = lipgloss.Color("#D4C5A0")
	// ColorTextMuted is the tertiary / hints color (#6A6A6A).
	ColorTextMuted = lipgloss.Color("#6A6A6A")
	// ColorVoidBlack is the screen background (#0A0A0A).
	ColorVoidBlack = lipgloss.Color("#0A0A0A")
	// ColorIronDark is the panel / input background (#1E1E1E).
	ColorIronDark = lipgloss.Color("#1E1E1E")
	// ColorIronGray is the default border color (#2A2A2A).
	ColorIronGray = lipgloss.Color("#2A2A2A")
	// ColorSteelGray is the scrollbar / divider color (#3A3A3A).
	ColorSteelGray = lipgloss.Color("#3A3A3A")
)

// Extended style bundles for the TUI welcome screen.  All colors are
// pinned to the cogitator-browser palette — do not change them without
// syncing with cogitator-browser/src/renderer/styles/theme.css.
var (
	// GoldStyle is the basic Gold (no bold/italic).  Alias of GoldBoldStyle
	// for compatibility with existing code that expects theme.GoldStyle.
	GoldStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorCogitatorGold)
	// GoldBoldStyle is the FORGE logo: Gold, bold, no italic.
	GoldBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorCogitatorGold)
	// GoldDimStyle is the FORGE subtitle and "MMXXVI" footer: Gold-dim
	// (pale yellow), italic, no bold.
	GoldDimStyle = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#8B7355"))
	// SteelGrayStyle is the decorative ═══ divider and status hints.
	SteelGrayStyle = lipgloss.NewStyle().Foreground(ColorSteelGray)
	// TextMutedStyle is the bottom footer "v1.1.0 · MMXXVI" — very muted.
	TextMutedStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
)

// CenterText centers a single line of text within width w.  If text is
// wider than w, returns it unchanged.  Handles unicode width correctly.
func CenterText(w int, text string) string {
	if w <= 0 {
		return text
	}
	tw := lipgloss.Width(text)
	if tw >= w {
		return text
	}
	pad := (w - tw) / 2
	return strings.Repeat(" ", pad) + text
}

func init() {
	SetTheme(loadThemeFromDisk())
}
