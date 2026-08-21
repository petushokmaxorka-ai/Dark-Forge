// Package cogwheel — Dark Mechanicus cog wheel for terminal UIs.
//
// The cog wheel is the central visual symbol of Dark Mechanicus
// (Warhammer 40k Adeptus Mechanicus).  This package produces the
// ASCII-art cog wheel for terminal renderers, with optional
// "spinning" animation that cycles through phases.
//
// Visual design (5 lines tall, ~30 cols wide):
//
//	   ╓───╖      ╓───╖      ╓───╖      ╓───╖
//	   ║ ◉ ║  →  ║ ◈ ║  →  ║ ◉ ║  →  ║ ◈ ║
//	   ╙───╜      ╙───╜      ╙───╜      ╙───╜
//
// The inner glyph cycles between ◉ (filled) and ◈ (dotted) every
// 200ms to suggest rotation.  Outer ring is the static cog teeth.
//
// A second tier ("wide" layout) shows spokes:
//
//	   ╓─╖ ╓─╖ ╓─╖ ╓─╖
//	   ║◉║║ ║║◉║║ ║
//	   ╙─╜ ╙─╜ ╙─╜ ╙─╜
//
// Color states:
//   - Idle     : gold (Cogitator Gold #C8A84B)
//   - Thinking : animated (cycles per 200ms, gold -> rust -> gold)
//   - Working  : cyan (Noosphere #00BFBF)
//   - Error    : red (Omnissiah #FF0000)
//   - Success  : green (#39FF14)
//
// This package is language-agnostic in spirit: the cogitator-browser
// (TypeScript/Electron) side should implement the same visual using
// CSS animations and the exact glyph set (◉ ◈ ╓ ╖ ╙ ╜).  See
// COG_WHEEL_DESIGN.md in the cogitator-browser repo for the
// web-side implementation.
package cogwheel

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
)

// State is the operational state of the cog wheel.  Affects color
// and (for Thinking) the inner-glyph animation.
type State int

const (
	StateIdle State = iota
	StateThinking
	StateWorking
	StateError
	StateSuccess
)

// Style selects the cog wheel rendering style.
type Style int

const (
	// StyleCompact is the default 3-line cog, suitable for inline
	// rendering in headers and status bars.
	StyleCompact Style = iota
	// StyleWide is the 5-line cog with spokes, suitable for the
	// welcome screen and large logo placements.
	StyleWide
)

// Options controls cog wheel rendering.
type Options struct {
	// State determines the color and (for StateThinking) animation.
	State State
	// Style selects compact or wide layout.
	Style Style
	// Now is the reference time for animation cycling.  If zero,
	// time.Now() is used.  Tests can pass a fixed time.
	Now time.Time
}

// DefaultOptions returns the default rendering options (compact,
// idle, current time).
func DefaultOptions() Options {
	return Options{State: StateIdle, Style: StyleCompact, Now: time.Now()}
}

// Frame is a single rendered cog wheel as a multi-line string.
type Frame struct {
	Lines []string
	Width int
	Height int
}

// String returns the cog wheel as a single string with newlines
// between lines.
func (f Frame) String() string {
	return strings.Join(f.Lines, "\n")
}

// compactCog returns the 3-line compact cog (idle state).
func compactCog(color lipgloss.Style) Frame {
	// 3-line layout: top bracket, middle cog, bottom bracket.
	// Inner glyph cycles between ◉ and ◈ (we'll fill in caller).
	return Frame{
		Lines: []string{
			color.Render("╓───╖"),
			color.Render("║ ◉ ║"),
			color.Render("╙───╜"),
		},
		Width:  5,
		Height: 3,
	}
}

// wideCog returns the 5-line wide cog with spokes.
func wideCog(color lipgloss.Style) Frame {
	// 5-line layout:
	//   ╓─╖ ╓─╖ ╓─╖ ╓─╖
	//   ║◉║║ ║║◉║║ ║
	//   ╙─╜ ╙─╜ ╙─╜ ╙─╜
	return Frame{
		Lines: []string{
			color.Render("╓─╖ ╓─╖ ╓─╖ ╓─╖"),
			color.Render("║◉║║ ║║◉║║ ║"),
			color.Render("╙─╜ ╙─╜ ╙─╜ ╙─╜"),
		},
		Width:  15,
		Height: 3,
	}
}

// Render returns a single Frame for the given options.
func Render(opts Options) Frame {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	// Choose color by state
	var color lipgloss.Style
	switch opts.State {
	case StateIdle, StateThinking:
		color = theme.GoldBoldStyle
	case StateWorking:
		color = lipgloss.NewStyle().Foreground(theme.ColorNoosphereCyan).Bold(true)
	case StateError:
		color = lipgloss.NewStyle().Foreground(theme.ColorOmnissiahRed).Bold(true)
	case StateSuccess:
		color = lipgloss.NewStyle().Foreground(theme.ColorGreen).Bold(true)
	default:
		color = theme.GoldBoldStyle
	}

	var frame Frame
	switch opts.Style {
	case StyleWide:
		frame = wideCog(color)
	default:
		frame = compactCog(color)
	}

	// For StateThinking, cycle the inner glyph every 200ms.
	if opts.State == StateThinking {
		idx := (now.UnixMilli() / 200) % 2 // 0 or 1
		glyph := "◉"
		if idx == 1 {
			glyph = "◈"
		}
		// Replace the middle line's center glyph
		frame = replaceCenterGlyph(frame, glyph)
	}

	return frame
}

// replaceCenterGlyph substitutes the center cog glyph in the middle
// row with the given rune.  Used to animate the "thinking" state.
func replaceCenterGlyph(f Frame, glyph string) Frame {
	if len(f.Lines) < 2 {
		return f
	}
	mid := f.Lines[1]
	plain := stripANSI(mid)
	if len(plain) < 3 {
		return f
	}
	center := len(plain) / 2
	newPlain := plain[:center] + glyph + plain[center+1:]
	lines := make([]string, len(f.Lines))
	copy(lines, f.Lines)
	lines[1] = theme.GoldBoldStyle.Render(newPlain)
	return Frame{Lines: lines, Width: f.Width, Height: f.Height}
}

// stripANSI removes ANSI escape sequences for plain-text indexing.
// Extracted helper for replaceCenterGlyph.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' || r == 'K' || r == 'H' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// RenderCentered returns the cog wheel centered within width w
// with the appropriate padding.  If the cog is wider than w,
// returns it unchanged.
func RenderCentered(w int, opts Options) string {
	frame := Render(opts)
	if w <= 0 || frame.Width >= w {
		return frame.String()
	}
	pad := (w - frame.Width) / 2
	indent := strings.Repeat(" ", pad)
	var lines []string
	for _, l := range frame.Lines {
		plain := stripANSI(l)
		plainWidth := len(plain)
		rightPad := w - pad - plainWidth
		if rightPad < 0 {
			rightPad = 0
		}
		lines = append(lines, indent+l+strings.Repeat(" ", rightPad))
	}
	return strings.Join(lines, "\n")
}

// AsciiDoc is a pure-ASCII (no ANSI) version of the cog for use in
// docs, comments, and TS-side references.  No animation.
func AsciiDoc() string {
	return strings.Join([]string{
		"╓───╖",
		"║ ◉ ║",
		"╙───╜",
	}, "\n")
}

// fmt is imported to avoid unused-import errors when the package
// grows in the future.
var _ = fmt.Sprintf
