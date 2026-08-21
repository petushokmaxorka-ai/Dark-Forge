// Package theme — Dark Mechanicus tags and small helpers.
// «Veritas in Crypta. Ordo ab Chao.»
// Color palette and Bubble Tea styles live in colors.go.
package theme

import "fmt"

// Latin tags — short canonical phrases for log/status lines.
const (
	TagBoot     = "«Veritas in Crypta.»"
	TagCompact  = "«Ordo ab Chao.»"
	TagCreate   = "«Mechanicum Ipsum.»"
	TagTestPass = "«Heresy Purged.»"
	TagBash     = "«Banelight Activated.»"
	TagCommit   = "«Занесено.»"
	TagFadeAway = "«Never fade away.»"
)

// Version is the canonical Forge version string.
const Version = "1.0.0"

// AgentStatus formats an agent status message with Dark Mechanicus styling
// using lipgloss-aware ANSI colors.
func AgentStatus(agent, action string) string {
	glyph, color := agentBadge(agent)
	return color(glyph + " " + agent + " " + action)
}

// agentBadge picks the glyph + color function for an agent name.
func agentBadge(agent string) (string, func(string) string) {
	switch agent {
	case "magos":
		return GlyphBuild, ColorGoldANSI
	case "vox_dei":
		return GlyphThink, ColorGoldANSI
	case "glm":
		return GlyphThink, ColorRustANSI
	case "kimi":
		return GlyphScan, ColorRustANSI
	case "mimo":
		return GlyphScan, ColorRustANSI
	case "qwen":
		return GlyphApply, ColorRustANSI
	case "minimax":
		return GlyphBuild, ColorRustANSI
	default:
		return "?", ColorRustANSI
	}
}

// CommitMessage generates a git commit message in Dark Mechanicus style.
func CommitMessage(change string) string {
	return fmt.Sprintf("forge: %s\n\n%s %s", change, TagCommit, TagFadeAway)
}

// Raw ANSI helpers (kept for non-Bubble Tea consumers like the CLI flag).
func ColorGoldANSI(s string) string  { return "\033[38;5;214m" + s + "\033[0m" }
func ColorRustANSI(s string) string  { return "\033[38;5;130m" + s + "\033[0m" }
func ColorRedANSI(s string) string   { return "\033[31m" + s + "\033[0m" }
func ColorGreenANSI(s string) string { return "\033[32m" + s + "\033[0m" }
func ColorCyanANSI(s string) string  { return "\033[36m" + s + "\033[0m" }
func ColorDimANSI(s string) string   { return "\033[38;5;242m" + s + "\033[0m" }