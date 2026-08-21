// Package cogwheel — tests for the cog wheel rendering.
package cogwheel

import (
	"strings"
	"testing"
	"time"
)

func TestRender_Idle(t *testing.T) {
	f := Render(Options{State: StateIdle, Style: StyleCompact})
	if f.Height != 3 {
		t.Errorf("expected 3 lines, got %d", f.Height)
	}
	if f.Width != 5 {
		t.Errorf("expected width 5, got %d", f.Width)
	}
	s := f.String()
	if !strings.Contains(s, "◉") {
		t.Errorf("idle cog should contain filled glyph ◉, got %q", s)
	}
}

func TestRender_Wide(t *testing.T) {
	f := Render(Options{State: StateIdle, Style: StyleWide})
	if f.Width != 15 {
		t.Errorf("expected wide cog width 15, got %d", f.Width)
	}
}

func TestRender_ThinkingCyclesGlyph(t *testing.T) {
	now := time.Unix(0, 0) // 0ms
	f1 := Render(Options{State: StateThinking, Style: StyleCompact, Now: now})
	// 200ms later should show the other glyph
	now2 := time.UnixMilli(200)
	f2 := Render(Options{State: StateThinking, Style: StyleCompact, Now: now2})

	s1 := stripANSI(f1.String())
	s2 := stripANSI(f2.String())

	if !strings.Contains(s1, "◉") {
		t.Errorf("first frame should contain ◉, got %q", s1)
	}
	if !strings.Contains(s2, "◈") {
		t.Errorf("second frame should contain ◈, got %q", s2)
	}
}

func TestRenderCentered(t *testing.T) {
	out := RenderCentered(80, Options{State: StateIdle, Style: StyleCompact})
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	// Each line should start with some padding (spaces) to center the 5-col cog.
	for i, l := range lines {
		plain := stripANSI(l)
		leftPad := len(plain) - len(strings.TrimLeft(plain, " "))
		rightPad := len(plain) - len(strings.TrimRight(plain, " "))
		_ = i
		if leftPad == 0 && rightPad == 0 {
			t.Errorf("line %d has no centering padding: %q", i, plain)
		}
	}
}

func TestAsciiDoc(t *testing.T) {
	doc := AsciiDoc()
	if !strings.Contains(doc, "◉") {
		t.Errorf("AsciiDoc should contain ◉, got %q", doc)
	}
}

func TestStripANSI(t *testing.T) {
	s := "\x1b[31mred\x1b[0m"
	if got := stripANSI(s); got != "red" {
		t.Errorf("stripANSI failed: got %q", got)
	}
}

func TestFrame_String(t *testing.T) {
	f := Frame{Lines: []string{"a", "b", "c"}, Width: 1, Height: 3}
	if s := f.String(); s != "a\nb\nc" {
		t.Errorf("Frame.String failed: got %q", s)
	}
}

func TestRender_StateColor_AllStates(t *testing.T) {
	// Render each state and verify it produces output without panic.
	states := []State{StateIdle, StateThinking, StateWorking, StateError, StateSuccess}
	for _, s := range states {
		f := Render(Options{State: s, Style: StyleCompact})
		if f.Height != 3 {
			t.Errorf("state %d: expected 3 lines, got %d", int(s), f.Height)
		}
	}
}
