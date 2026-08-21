// Package tui — Tests for HighlightCode + ExtractCodeBlocks + HighlightCodeBlocks.
package tui

import (
	"strings"
	"testing"
)

func TestHighlightCode_Empty(t *testing.T) {
	if got := HighlightCode("", "go"); got != "" {
		t.Errorf("HighlightCode(empty) = %q, want empty", got)
	}
}

func TestHighlightCode_UnknownLanguage(t *testing.T) {
	code := "hello world"
	if got := HighlightCode(code, "ruby"); got != code {
		t.Errorf("HighlightCode(ruby) = %q, want unchanged", got)
	}
}

func TestHighlightCode_GoKeywords(t *testing.T) {
	code := "func main() { return 0 }"
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiAmber) {
		t.Errorf("expected amber color for keywords, got: %q", out)
	}
	if !strings.Contains(out, "func") {
		t.Errorf("expected 'func' to be preserved, got: %q", out)
	}
}

func TestHighlightCode_GoStrings(t *testing.T) {
	code := `x := "hello"`
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiGreen) {
		t.Errorf("expected green color for strings, got: %q", out)
	}
}

func TestHighlightCode_GoRawStrings(t *testing.T) {
	code := "x := `raw string`"
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiGreen) {
		t.Errorf("expected green for raw string, got: %q", out)
	}
}

func TestHighlightCode_GoComments(t *testing.T) {
	code := "// this is a comment\nx := 1"
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiDim) {
		t.Errorf("expected dim color for comments, got: %q", out)
	}
}

func TestHighlightCode_GoBlockComments(t *testing.T) {
	code := "/* block comment */\nx := 1"
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiDim) {
		t.Errorf("expected dim for block comment, got: %q", out)
	}
}

func TestHighlightCode_GoNumbers(t *testing.T) {
	code := "x := 42"
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiCyan) {
		t.Errorf("expected cyan color for numbers, got: %q", out)
	}
}

func TestHighlightCode_GoFloatNumbers(t *testing.T) {
	code := "x := 3.14"
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiCyan) {
		t.Errorf("expected cyan for float, got: %q", out)
	}
}

func TestHighlightCode_GoFunctions(t *testing.T) {
	code := `fmt.Println("hi")`
	out := HighlightCode(code, "go")
	if !strings.Contains(out, ansiRust) {
		t.Errorf("expected rust color for function names, got: %q", out)
	}
}

func TestHighlightCode_PythonKeywords(t *testing.T) {
	code := "def hello(): return 1"
	out := HighlightCode(code, "python")
	if !strings.Contains(out, ansiAmber) {
		t.Errorf("expected amber for Python keywords, got: %q", out)
	}
}

func TestHighlightCode_PythonComments(t *testing.T) {
	code := "# python comment\nx = 1"
	out := HighlightCode(code, "python")
	if !strings.Contains(out, ansiDim) {
		t.Errorf("expected dim for Python comments, got: %q", out)
	}
}

func TestHighlightCode_PythonStrings(t *testing.T) {
	code := `x = 'hello'`
	out := HighlightCode(code, "python")
	if !strings.Contains(out, ansiGreen) {
		t.Errorf("expected green for single-quoted string, got: %q", out)
	}
}

func TestHighlightCode_BashKeywords(t *testing.T) {
	code := "if true; then echo hi; else echo bye; fi"
	out := HighlightCode(code, "bash")
	if !strings.Contains(out, ansiAmber) {
		t.Errorf("expected amber for bash keywords, got: %q", out)
	}
}

func TestHighlightCode_BashComments(t *testing.T) {
	code := "# bash comment\necho hi"
	out := HighlightCode(code, "bash")
	if !strings.Contains(out, ansiDim) {
		t.Errorf("expected dim for bash comments, got: %q", out)
	}
}

func TestHighlightCode_JSONStrings(t *testing.T) {
	code := `{"name": "test", "value": 42, "active": true}`
	out := HighlightCode(code, "json")
	if !strings.Contains(out, ansiGreen) {
		t.Errorf("expected green for JSON strings, got: %q", out)
	}
}

func TestHighlightCode_JSONNumbers(t *testing.T) {
	code := `{"value": 42}`
	out := HighlightCode(code, "json")
	if !strings.Contains(out, ansiCyan) {
		t.Errorf("expected cyan for JSON numbers, got: %q", out)
	}
}

func TestHighlightCode_JSONBooleans(t *testing.T) {
	code := `{"active": true, "deleted": false, "data": null}`
	out := HighlightCode(code, "json")
	if !strings.Contains(out, ansiAmber) {
		t.Errorf("expected amber for JSON booleans/null, got: %q", out)
	}
}

func TestHighlightCode_YAMLNumbers(t *testing.T) {
	code := "name: test\nvalue: 42"
	out := HighlightCode(code, "yaml")
	if !strings.Contains(out, ansiCyan) {
		t.Errorf("expected cyan for YAML numbers, got: %q", out)
	}
}

func TestHighlightCode_GoDoesntHighlightInsideStrings(t *testing.T) {
	code := `x := "func return if"`
	out := HighlightCode(code, "go")
	// The string content "func return if" must be preserved in the output
	// AND must be wrapped in GREEN (string color), not AMBER (keyword).
	idx := strings.Index(out, "func return if")
	if idx < 0 {
		t.Fatalf("expected string content preserved, got: %q", out)
	}
	// ansiGreen is 11 bytes (\x1b[38;5;82m) + the opening quote = 12 chars
	// before the string content starts. Look at the 16 chars before "func".
	if idx < 16 {
		t.Fatalf("output too short to contain escape codes: %q", out)
	}
	prefix := out[idx-16 : idx]
	if !strings.Contains(prefix, ansiGreen) {
		t.Errorf("string content should be prefixed with green, got prefix %q in %q", prefix, out)
	}
	if strings.Contains(prefix, ansiAmber) {
		t.Errorf("string content should not be amber, got prefix %q in %q", prefix, out)
	}
}

func TestHighlightCode_ResetIsApplied(t *testing.T) {
	code := `func main() { return 0 }`
	out := HighlightCode(code, "go")
	resetCount := strings.Count(out, ansiReset)
	amberCount := strings.Count(out, ansiAmber)
	if resetCount < amberCount {
		t.Errorf("reset count %d should be >= amber count %d", resetCount, amberCount)
	}
}

func TestExtractCodeBlocks_Go(t *testing.T) {
	text := "Some text\n```go\nfunc main() {}\n```\nMore text"
	blocks := ExtractCodeBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Language != "go" {
		t.Errorf("language = %q, want go", blocks[0].Language)
	}
	if !strings.Contains(blocks[0].Code, "func main()") {
		t.Errorf("code should contain 'func main()', got: %q", blocks[0].Code)
	}
}

func TestExtractCodeBlocks_Python(t *testing.T) {
	text := "```python\nprint('hi')\n```"
	blocks := ExtractCodeBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Language != "python" {
		t.Errorf("language = %q, want python", blocks[0].Language)
	}
}

func TestExtractCodeBlocks_Untagged(t *testing.T) {
	text := "```\nplain code\n```"
	blocks := ExtractCodeBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Language != "" {
		t.Errorf("untagged block should have empty language, got: %q", blocks[0].Language)
	}
}

func TestExtractCodeBlocks_NoBlocks(t *testing.T) {
	text := "Just plain text, no code blocks here."
	blocks := ExtractCodeBlocks(text)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestExtractCodeBlocks_MultipleBlocks(t *testing.T) {
	text := "```go\nx := 1\n```\nbetween\n```python\ny = 2\n```"
	blocks := ExtractCodeBlocks(text)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Language != "go" {
		t.Errorf("block 0 language = %q, want go", blocks[0].Language)
	}
	if blocks[1].Language != "python" {
		t.Errorf("block 1 language = %q, want python", blocks[1].Language)
	}
}

func TestHighlightCodeBlocks_FullPipeline(t *testing.T) {
	text := "Here is Go code:\n```go\nfunc main() { return 42 }\n```\nDone."
	out := HighlightCodeBlocks(text)
	if !strings.Contains(out, ansiAmber) {
		t.Errorf("expected amber for keyword 'func' in highlighted output, got: %q", out)
	}
}

func TestSupportedLanguages(t *testing.T) {
	langs := SupportedLanguages()
	if len(langs) < 5 {
		t.Errorf("expected at least 5 languages, got %d: %v", len(langs), langs)
	}
}

func TestColorName(t *testing.T) {
	for _, tt := range []string{"keyword", "string", "comment", "number", "function"} {
		if got := ColorName(tt); got == "" || got == "?" {
			t.Errorf("ColorName(%q) = %q, want non-empty", tt, got)
		}
	}
	if got := ColorName("unknown"); got != "?" {
		t.Errorf("ColorName(unknown) = %q, want ?", got)
	}
}

func TestHighlightCode_CaseInsensitiveLanguage(t *testing.T) {
	code := "x := 1"
	lower := HighlightCode(code, "go")
	upper := HighlightCode(code, "GO")
	mixed := HighlightCode(code, "Go")
	if lower != upper || lower != mixed {
		t.Errorf("language case should not matter\nlower=%q\nupper=%q\nmixed=%q",
			lower, upper, mixed)
	}
}