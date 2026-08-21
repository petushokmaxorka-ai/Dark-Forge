// Package theme — tests for Dark Mechanicus palette.
package theme

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// ═══ Banner ═════════════════════════════════════════════════════

func TestBanner_Renders(t *testing.T) {
	b := Banner()
	if b == "" {
		t.Fatal("Banner() returned empty string")
	}
	if len(b) < 50 {
		t.Errorf("Banner() too short: %d bytes", len(b))
	}
}

func TestBanner_ContainsForgeName(t *testing.T) {
	b := Banner()
	if !contains(b, "DARK FORGE") {
		t.Error("Banner missing 'DARK FORGE'")
	}
}

func TestBanner_ContainsLatinTag(t *testing.T) {
	b := Banner()
	if !contains(b, "Per Ignis") {
		t.Error("Banner missing Latin tag")
	}
}

// ═══ AgentGlyph ═════════════════════════════════════════════════

func TestAgentGlyph_AllKnown(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"magos", "Magos"},
		{"glm", "GLM"},
		{"kimi", "Kimi"},
		{"mimo", "MiMo"},
		{"qwen", "Qwen"},
		{"minimax", "MiniMax"},
		{"vox_dei", "Vox Dei"},
	}
	for _, c := range cases {
		got := AgentGlyph(c.name)
		if got == "" {
			t.Errorf("AgentGlyph(%q) empty", c.name)
		}
	}
}

func TestAgentGlyph_Unknown(t *testing.T) {
	got := AgentGlyph("unknown_agent")
	if !contains(got, "?") {
		t.Errorf("Unknown agent should get '?', got %q", got)
	}
}

func TestAgentGlyph_ContainsGlyph(t *testing.T) {
	cases := map[string]string{
		"magos":   GlyphBuild,
		"glm":     GlyphThink,
		"kimi":    GlyphScan,
		"mimo":    GlyphScan,
		"qwen":    GlyphApply,
		"minimax": GlyphBuild,
		"vox_dei": GlyphThink,
	}
	for agent, wantGlyph := range cases {
		got := AgentGlyph(agent)
		if !contains(got, wantGlyph) {
			t.Errorf("AgentGlyph(%q) = %q, missing glyph %q", agent, got, wantGlyph)
		}
	}
}

// ═══ AgentStatus ════════════════════════════════════════════════

func TestAgentStatus_Renders(t *testing.T) {
	status := AgentStatus("magos", "thinking...")
	if status == "" {
		t.Error("AgentStatus returned empty")
	}
	if !contains(status, "magos") {
		t.Error("AgentStatus missing agent name")
	}
}

func TestAgentStatus_ContainsAction(t *testing.T) {
	status := AgentStatus("glm", "coding")
	if !contains(status, "coding") {
		t.Errorf("AgentStatus missing action: %q", status)
	}
}

// ═══ CommitMessage ══════════════════════════════════════════════

func TestCommitMessage_Format(t *testing.T) {
	msg := CommitMessage("add foo")
	if msg == "" {
		t.Fatal("CommitMessage returned empty")
	}
	for _, want := range []string{"forge:", "Занесено.", "Never fade away"} {
		if !contains(msg, want) {
			t.Errorf("CommitMessage missing %q: %q", want, msg)
		}
	}
}

func TestCommitMessage_ContainsChange(t *testing.T) {
	msg := CommitMessage("fix bug #42")
	if !contains(msg, "fix bug #42") {
		t.Errorf("CommitMessage missing change text: %q", msg)
	}
}

// ═══ Latin Tags ═════════════════════════════════════════════════

func TestLatinTags_AllDefined(t *testing.T) {
	tags := map[string]string{
		"TagBoot":     TagBoot,
		"TagCompact":  TagCompact,
		"TagCreate":   TagCreate,
		"TagTestPass": TagTestPass,
		"TagBash":     TagBash,
		"TagCommit":   TagCommit,
		"TagFadeAway": TagFadeAway,
	}
	for name, tag := range tags {
		if tag == "" {
			t.Errorf("%s is empty", name)
		}
		if len(tag) < 5 {
			t.Errorf("%s too short: %q", name, tag)
		}
	}
}

func TestTagTestPass_ContainsHeresy(t *testing.T) {
	if !contains(TagTestPass, "Heresy") {
		t.Errorf("TagTestPass missing 'Heresy': %q", TagTestPass)
	}
}

func TestTagCommit_ContainsZaneseno(t *testing.T) {
	if !contains(TagCommit, "Занесено") {
		t.Errorf("TagCommit missing 'Занесено': %q", TagCommit)
	}
}

// ═══ Glyphs ═════════════════════════════════════════════════════

func TestGlyphs_AllDefined(t *testing.T) {
	glyphs := map[string]string{
		"GlyphBuild":    GlyphBuild,
		"GlyphThink":    GlyphThink,
		"GlyphScan":     GlyphScan,
		"GlyphApply":    GlyphApply,
		"GlyphOK":       GlyphOK,
		"GlyphBlock":    GlyphBlock,
		"GlyphRollback": GlyphRollback,
	}
	for name, g := range glyphs {
		if g == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestGlyphs_AreUnicode(t *testing.T) {
	glyphs := []string{
		GlyphBuild, GlyphThink, GlyphScan, GlyphApply,
		GlyphOK, GlyphBlock, GlyphRollback,
	}
	for _, g := range glyphs {
		// All glyphs should be non-empty Unicode
		if len(g) == 0 {
			t.Error("Empty glyph found")
		}
	}
}

// ═══ Colors (ANSI) ═════════════════════════════════════════════

func TestColorGoldANSI_ContainsANSI(t *testing.T) {
	result := ColorGoldANSI("test")
	if !contains(result, "\033[") {
		t.Error("ColorGoldANSI missing ANSI escape")
	}
	if !contains(result, "test") {
		t.Error("ColorGoldANSI missing input text")
	}
}

func TestColorRustANSI_ContainsANSI(t *testing.T) {
	result := ColorRustANSI("test")
	if !contains(result, "\033[") {
		t.Error("ColorRustANSI missing ANSI escape")
	}
}

func TestColorRedANSI_ContainsANSI(t *testing.T) {
	result := ColorRedANSI("error")
	if !contains(result, "\033[") {
		t.Error("ColorRedANSI missing ANSI escape")
	}
}

func TestColorGreenANSI_ContainsANSI(t *testing.T) {
	result := ColorGreenANSI("ok")
	if !contains(result, "\033[") {
		t.Error("ColorGreenANSI missing ANSI escape")
	}
}

// ═══ Version ════════════════════════════════════════════════════

func TestVersion_NotEmpty(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty")
	}
}

func TestVersion_Format(t *testing.T) {
	// Should be semver-like
	parts := strings.Split(Version, ".")
	if len(parts) < 2 {
		t.Errorf("Version %q not semver-like", Version)
	}
}

// ═══ SetTheme (placeholder — theme switching not yet implemented) ═

func TestSetTheme_DarkMechanicus(t *testing.T) {
	// DarkMechanicus is the default — verify colors are set
	if ColorGold == lipgloss.Color("") {
		t.Error("ColorGold is empty")
	}
	if ColorRed == lipgloss.Color("") {
		t.Error("ColorRed is empty")
	}
}

func TestSetTheme_ColorsAreDistinct(t *testing.T) {
	SetTheme(ThemeDarkMechanicus)
	dmGold := ColorGold

	SetTheme(ThemeNoosphere)
	if ColorGold == dmGold {
		t.Error("Noosphere primary should differ from DarkMechanicus gold")
	}

	SetTheme(ThemeHeresy)
	if ColorRed == ColorGold {
		t.Error("Heresy primary and secondary should differ")
	}
}

func TestSetTheme_BgColors(t *testing.T) {
	for _, name := range []ThemeName{ThemeDarkMechanicus, ThemeNoosphere, ThemeHeresy} {
		SetTheme(name)
		if ColorBg == ColorBgLight {
			t.Errorf("%s: Bg and BgLight should differ", name)
		}
	}
}

func TestSetTheme_PersistsToDisk(t *testing.T) {
	SetTheme(ThemeNoosphere)
	if CurrentTheme() != ThemeNoosphere {
		t.Errorf("current theme = %q, want noosphere", CurrentTheme())
	}
	data, err := os.ReadFile(themeConfigPath())
	if err != nil {
		t.Fatalf("theme file not written: %v", err)
	}
	if strings.TrimSpace(string(data)) != "noosphere" {
		t.Errorf("theme file content = %q, want noosphere", string(data))
	}
	// restore default
	SetTheme(ThemeDarkMechanicus)
}

// ═══ Helper ═════════════════════════════════════════════════════

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
