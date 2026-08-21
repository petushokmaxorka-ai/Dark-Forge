// Package tui — Bubble Tea view (rendering).
// Layout: OpenCode-style with Dark Mechanicus palette (locked to
// cogitator-browser/src/renderer/styles/theme.css).
//
// Top bar:   title  ·  commands  (Copy / Paste / Find /  X)
// Body:      chat viewport (user + assistant messages)
// Input:     single-line prompt area with mode + model + variant chips
// Footer:    status bar  (mode, agent, model, session, cost, tokens)
//
// Visual elements (faithful to OpenCode):
//   · Title in Cogitator Gold (#C8A84B) bold
//   · Commands in muted with gold keys
//   · User message    → Noosphere Cyan (#00BFBF) bold, right-aligned
//   · Assistant msg  → Cogitator Gold (#C8A84B) left, ×Magos + Italic body
//   · Tool calls      → Omnissiah Red (#FF0000) with ✓ / ✗
//   · Plan mode       → boxed with [a]pprove [s]tep [r]eject
//   · Status bar      → Iron Dark bg, Steel Gray fg, gold values
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/cogwheel"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/edit"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
)

// FORGE_YEAR is the Roman-numeral year for the footer (Mechanicus touch).
const FORGE_YEAR = "MMXXVI"

// FORGE_FOOTER is the sacred blessing at the bottom of the welcome screen
// and (optionally) the chat mode status bar.
const FORGE_FOOTER = "Blessed by the Omnissiah · Sanctified for the Machine Cult · Powered by Forge"

// FORGE_VERSION is the build/version label (locked to v1.1.0 per spec).
const FORGE_VERSION = "v1.1.0"

// View renders the full TUI.
func (m Model) View() string {
	if m.quitting {
		return theme.DimStyle.Render("⚒ Занесено. Never fade away.") + "\n"
	}
	if !m.ready {
		return m.renderWelcome()
	}

	w := m.width
	if w < 80 {
		w = 80
	}
	h := m.height
	if h < 20 {
		h = 20
	}

	// Command palette overlay (Ctrl+P) — covers most of the screen
	if m.paletteMode {
		header := m.renderTopBar(w)
		chat := m.renderChatBlock(w, h-6)
		planPanel := ""
		if m.planMode && m.plan != nil {
			planPanel = m.renderPlan() + "\n"
		}
		footer := m.renderStatusBar(w)
		palette := m.renderCommandPalette()
		sections := []string{header, chat, planPanel, palette, footer}
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	// Vertical budget: top(1) + chat(h-4) + input(2) + footer(1) = h
	chatH := h - 6
	if chatH < 5 {
		chatH = 5
	}

	header := m.renderTopBar(w)
	chat := m.renderChatBlock(w, chatH)

	// Plan mode overlay (above input) when plan is pending approval
	planPanel := ""
	if m.planMode && m.plan != nil {
		planPanel = m.renderPlan() + "\n"
	}

	input := m.renderInput(w)
	footer := m.renderStatusBar(w)

	sections := []string{header, chat}
	if planPanel != "" {
		sections = append(sections, planPanel)
	}
	sections = append(sections, input, footer)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderTopBar produces the OpenCode-style header with title left and
// commands on the right.  Real OpenCode has  Copy / Paste / Find / X.
func (m Model) renderTopBar(w int) string {
	ver := m.version.Version
	if ver == "" {
		ver = "?"
	}
	left := theme.TitleStyle.Render(fmt.Sprintf("⚒ HereticForge %s", ver))

	// Right side: commands with gold keys + dimmed labels
	commands := []string{
		"Copy",
		"Paste",
		"Find",
		"X",
	}
	cmdParts := make([]string, 0, len(commands))
	for i, c := range commands {
		key := ""
		switch i {
		case 0:
			key = "⎡"
		case len(commands) - 1:
			key = "⎤"
		default:
			key = "│"
		}
		_ = key
		cmdParts = append(cmdParts, theme.AgentStyle.Render(c))
	}
	right := strings.Join(cmdParts, " "+theme.DimStyle.Render("·")+" ")

	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	row := left + repeat(" ", gap) + "  " + right

	// Bottom rule: thin rust line across full width (cogitator-gold dim)
	rule := lipgloss.NewStyle().Foreground(theme.ColorRust).Render(strings.Repeat("─", w))
	return row + "\n" + rule
}

// renderWelcome is the Noosphere-styled centered welcome screen
// (OpenCode-style with Cogitator Dark Mechanicus palette).
// Faithful to forge-tui-design-spec.md: cog wheel, [FORGE] logo,
// "Воззвать к Forge..." input, status hints, footer, MMXXVI.
func (m Model) renderWelcome() string {
	w := m.width
	if w < 80 {
		w = 80
	}
	h := m.height
	if h < 24 {
		h = 24
	}

	var lines []string

	// Vertical centering
	for i := 0; i < (h-16)/2; i++ {
		lines = append(lines, "")
	}

	// Cog wheel (5-line wide, centered)
	cog := cogwheel.RenderCentered(w, cogwheel.Options{
		State: cogwheel.StateIdle,
		Style: cogwheel.StyleWide,
		Now:   time.Now(),
	})
	lines = append(lines, cog)
	lines = append(lines, "")

	// [ FORGE ] — gold with text-shadow glow
	logo := renderForgeLogo()
	lines = append(lines, theme.CenterText(w, logo))
	lines = append(lines, "")

	// Subtitle in gold-dim
	subtitle := theme.GoldDimStyle.Render("Magos Dominus · Z.AI Mechanicus Coding Plan")
	lines = append(lines, theme.CenterText(w, subtitle))
	lines = append(lines, "")

	// Decorative divider
	divider := theme.SteelGrayStyle.Render(strings.Repeat("═", 28))
	lines = append(lines, theme.CenterText(w, divider))
	lines = append(lines, "")

	// Input box (opencode-style: omnissiah-red border)
	lines = append(lines, theme.CenterText(w, m.renderWelcomeInput(w)))
	lines = append(lines, "")

	// Status hints with muted gold keys
	lines = append(lines, theme.CenterText(w, renderWelcomeHints()))
	lines = append(lines, "")

	// Rivet + Litany tip
	lines = append(lines, theme.CenterText(w, renderWelcomeTip()))
	lines = append(lines, "")

	// Pad middle to push footer to bottom
	used := len(lines) + 4
	if used < h-2 {
		for i := 0; i < h-used; i++ {
			lines = append(lines, "")
		}
	}

	// Footer: sacred blessing + version
	lines = append(lines, theme.CenterText(w, theme.DimStyle.Render(FORGE_FOOTER)))
	verLine := fmt.Sprintf("%s · %s", FORGE_VERSION, FORGE_YEAR)
	lines = append(lines, theme.CenterText(w, theme.TextMutedStyle.Render(verLine)))

	return strings.Join(lines, "\n")
}

// renderForgeLogo returns the [ FORGE ] text with text-shadow glow.
func renderForgeLogo() string {
	glow := theme.GoldDimStyle.Render(strings.Repeat(" ", 11))
	left := theme.GoldStyle.Render("[")
	right := theme.GoldStyle.Render("]")
	letter := theme.GoldBoldStyle.Render("FORGE")
	return glow + left + " " + letter + " " + right
}

// renderWelcomeInput is the welcome-screen input box.
func (m Model) renderWelcomeInput(w int) string {
	placeholder := "Воззвать к Forge..."
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	box := lipgloss.NewStyle().
		Width(inner).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorOmnissiahRed).
		Padding(0, 1)
	return box.Render(theme.GoldDimStyle.Italic(true).Render(placeholder))
}

// renderWelcomeHints is the muted status hints row.
func renderWelcomeHints() string {
	hints := []string{"tab agents", "ctrl+p cogitations", "/help"}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = theme.AgentStyle.Render(h)
	}
	return theme.DimStyle.Render(strings.Join(parts, " "+theme.DimStyle.Render("·")+" "))
}

// renderWelcomeTip is the rivet + Litany tip line.
func renderWelcomeTip() string {
	rivet := theme.SteelGrayStyle.Render("◉")
	label := theme.GoldDimStyle.Italic(true).Render("Litany:")
	text := theme.DimStyle.Render(` --format sigil для machine-readable communion`)
	return rivet + " " + label + text
}

// renderChatBlock renders the chat viewport with OpenCode-style
// gold left-rule, per-message styling.
func (m Model) renderChatBlock(w, h int) string {
	box := lipgloss.NewStyle().
		Width(w - 2).
		Height(h).
		Border(lipgloss.Border{Left: "┃"}, false, false, false, false).
		BorderForeground(theme.ColorGold).
		Padding(0, 1)
	return box.Render(m.renderChatLines(w - 4))
}

// renderChatLines formats viewport content with OpenCode-style
// per-message styling.  Faithful mapping:
//   "user"     → Noosphere Cyan bold, right-aligned, ▶ prefix
//   "assistant"→ Cogitator Gold "× Magos" + Italic persona voice
//   "tool"     → Omnissiah Red with ✓ / ✗
//   "error"    → Omnissiah Red ✗
//   "system"   → text-muted italic, • prefix
func (m Model) renderChatLines(width int) string {
	if len(m.entries) == 0 {
		return theme.DimStyle.Render("⚒ Forge ready. Напиши @magos или /help…")
	}
	var b strings.Builder
	for _, e := range m.entries {
		switch e.Role {
		case "user":
			line := theme.UserStyle.Render("▶ " + e.Content)
			pad := width - lipgloss.Width(line)
			if pad < 0 {
				pad = 0
			}
			b.WriteString(strings.Repeat(" ", pad) + line + "\n")
		case "assistant":
			glyph := theme.AgentGlyph(e.Agent)
			if glyph == "" {
				glyph = "×"
			}
			header := theme.AgentStyle.Render(fmt.Sprintf("%s %s", glyph, e.Agent))
			b.WriteString(header + "\n")
			for _, line := range strings.Split(e.Content, "\n") {
				// Persona voice in italic
				b.WriteString("  " + theme.AssistantStyle.Italic(true).Render(line) + "\n")
			}
			b.WriteString("\n")
		case "tool":
			b.WriteString(theme.DimStyle.Render("  ⚙ "+e.Content) + "\n")
		case "error":
			b.WriteString(theme.ErrorStyle.Render("  ✗ "+e.Content) + "\n")
		case "system":
			b.WriteString(theme.DimStyle.Italic(true).Render("  • "+e.Content) + "\n")
		default:
			b.WriteString(theme.AssistantStyle.Render(e.Content) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderInput produces the input box.  OpenCode-style: gold prompt +
// placeholder when empty, gold rule above, no animations in input.
func (m Model) renderInput(width int) string {
	prompt := theme.InputPromptStyle.Render("⚒ >")
	placeholder := "Воззвать к Forge…"
	value := m.input.Value()

	box := lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.Border{Top: "─"}, false, false, false, false).
		BorderForeground(theme.ColorRust).
		Padding(0, 0)

	content := value
	if content == "" {
		content = theme.GoldDimStyle.Italic(true).Render(placeholder)
	} else {
		content = theme.InputStyle.Render(content) + theme.InputPromptStyle.Render("│")
	}
	return box.Render(prompt + " " + content)
}

// renderStatusBar produces the bottom status bar.  OpenCode-style:
// left  : state · agent · model · tokens
// right : session · cost · server URL
//
// Token format: "Input 1234 (45%) · Output 567 · $0.0123"  (per opencode spec)
func (m Model) renderStatusBar(w int) string {
	// Left side: state · agent · model · token usage
	stateStr := "● " + m.activeAgent() + " ready"
	if m.pending {
		stateStr = "◐ " + m.activeAgent() + " thinking…"
	} else if m.streaming {
		stateStr = "◐ " + m.activeAgent() + " streaming…"
	}
	state := theme.SuccessStyle.Render(stateStr)

	model := m.status.Model
	if model == "" {
		model = "?"
	}

	tokens := ""
	if m.usage != nil {
		tokens = "  " + theme.DimStyle.Render(m.usage.FormatTokens())
		if pct := m.usage.ContextPct(); pct != "" {
			tokens += " " + theme.DimStyle.Render("(" + pct + ")")
		}
		if cost := m.usage.FormatCost(); cost != "" {
			tokens += "  " + theme.GoldDimStyle.Render(cost)
		}
	}

	left := fmt.Sprintf("%s · %s%s", state, theme.AgentStyle.Render(model), tokens)

	// Right side: session · cost · server URL
	right := ""
	if m.sessionID != "" {
		right = "session: " + theme.AgentStyle.Render(m.sessionID)
	}
	right += "  " + theme.DimStyle.Render(m.apiURL)

	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return theme.StatusStyle.Render(left + repeat(" ", gap) + "  " + right)
}

// renderCommandPalette renders the Ctrl+P command palette overlay.
// OpenCode-style: centered list with name, description, shortcut.
func (m Model) renderCommandPalette() string {
	w := m.width
	if w < 60 {
		w = 60
	}

	entries := m.filteredPalette()
	if len(entries) == 0 {
		return theme.CenterText(w, theme.DimStyle.Italic(true).Render("no commands match"))
	}

	// Build lines
	var lines []string
	lines = append(lines, theme.AgentStyle.Render("⎡  COMMANDS  ⎤"))
	lines = append(lines, "")

	// Show up to 8 entries
	max := 8
	if len(entries) < max {
		max = len(entries)
	}
	for i := 0; i < max; i++ {
		e := entries[i]
		name := theme.GoldBoldStyle.Render(e.Name)
		desc := theme.DimStyle.Render(" — " + e.Description)
		short := theme.DimStyle.Render(" [" + e.Shortcut + "]")
		lines = append(lines, name+desc+short)
	}
	if len(entries) > max {
		lines = append(lines, theme.DimStyle.Italic(true).Render(
			fmt.Sprintf("  ... +%d more", len(entries)-max)))
	}
	lines = append(lines, "")

	// Input row
	prompt := theme.AgentStyle.Render("❯ ")
	input := m.paletteInput.View()
	lines = append(lines, prompt+input)
	lines = append(lines, "")
	lines = append(lines, theme.DimStyle.Render(
		"  type to filter · Enter to run · Esc to close"))

	return theme.CenterText(w, strings.Join(lines, "\n"))
}

// renderPlan produces the plan approval/rejection UI.
func (m Model) renderPlan() string {
	if m.plan == nil || len(m.plan.Steps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(theme.AgentStyle.Render("⚒ MAGOS предлагает план:"))
	b.WriteString("\n\n")
	if m.plan.Summary != "" {
		b.WriteString(theme.DimStyle.Render(m.plan.Summary))
		b.WriteString("\n\n")
	}
	for i, step := range m.plan.Steps {
		var riskIcon string
		switch step.Risk {
		case "safe":
			riskIcon = theme.SuccessStyle.Render("[safe]")
		case "moderate":
			riskIcon = theme.WarningStyle.Render("[mod]")
		case "dangerous":
			riskIcon = theme.ErrorStyle.Render("[DANGER]")
		default:
			riskIcon = theme.DimStyle.Render("[?]")
		}
		fmt.Fprintf(&b, "  %d. %s %-6s %s\n",
			i+1, riskIcon, step.Action, theme.DimStyle.Render(step.Target))
		if step.Description != "" {
			b.WriteString("     " + theme.DimStyle.Render(step.Description) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(theme.DimStyle.Render("  [a]pprove all  [s]tep  [r]eject"))
	return theme.BoxStyle.Render(b.String())
}

// renderBatchPreview formats a BatchResult for the chat viewport.
func renderBatchPreview(result edit.BatchResult) string {
	var b strings.Builder
	if result.DryRun {
		b.WriteString(theme.WarningStyle.Render("DRY RUN — no files modified") + "\n\n")
	} else {
		b.WriteString(theme.SuccessStyle.Render("BATCH EDIT") + "\n\n")
	}
	b.WriteString(fmt.Sprintf("  applied: %d  failed: %d\n",
		len(result.Applied), len(result.Failed)))
	for _, fr := range result.Applied {
		fmt.Fprintf(&b, "    %s  %s\n", theme.SuccessStyle.Render("✓"), fr.Path)
	}
	for _, fr := range result.Failed {
		fmt.Fprintf(&b, "    %s  %s\n  %s\n",
			theme.ErrorStyle.Render("✗"), fr.Path, theme.DimStyle.Render(fr.Error))
	}
	return strings.TrimRight(b.String(), "\n")
}

// repeat returns s repeated n times.
func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}
