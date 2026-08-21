// Package tui — Code syntax highlighting via ANSI escape codes.
// «Veritas in Crypta. Ordo ab Chao.»
// Regex-based — no tree-sitter dependency (deliberate choice for v2.0).
// Supports: go, python, bash, json, yaml.
//
// Color mapping (Dark Mechanicus palette):
//   keyword  → bold + amber   (#FFB000)
//   string   → green          (#39FF14)
//   comment  → dim gray       (#666666)
//   number   → cyan           (#00FFFF)
//   function → rust           (#CD7F32)
package tui

import (
	"regexp"
	"strings"
)

// ANSI escape codes (256-color palette).
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"

	ansiAmber = "\x1b[38;5;214m" // keywords: #FFB000
	ansiGreen = "\x1b[38;5;82m"  // strings:  #39FF14
	ansiCyan  = "\x1b[38;5;51m"  // numbers:  #00FFFF
	ansiRust  = "\x1b[38;5;130m" // functions: #CD7F32
	ansiDim   = "\x1b[38;5;242m" // comments: #666666
)

// langSpec holds a combined regex with named groups for the 5 token kinds
// we highlight. A single regex per language means a single pass over the
// code, avoiding the issue where a regex applied later sees ANSI escape
// codes inserted by an earlier one.
//
// Group names: "comment", "string", "keyword", "number", "function".
type langSpec struct {
	re *regexp.Regexp
}

// Specs is the registry of supported languages.
var langSpecs = map[string]langSpec{
	"go": {re: regexp.MustCompile(
		`(?P<comment>//[^\n]*|/\*[\s\S]*?\*/)` +
			`|(?P<string>"(?:\\.|[^"\\])*"|` + "`" + `(?:\\.|[^` + "`" + `\\])*` + "`" + `)` +
			`|(?P<keyword>\b(?:break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b)` +
			`|(?P<number>\b\d+(?:\.\d+)?\b)` +
			`|(?P<function>\b[A-Z][a-zA-Z0-9_]*)`,
	)},
	"python": {re: regexp.MustCompile(
		`(?P<comment>#[^\n]*)` +
			`|(?P<string>"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')` +
			`|(?P<keyword>\b(?:and|as|assert|async|await|break|class|continue|def|del|elif|else|except|finally|for|from|global|if|import|in|is|lambda|nonlocal|not|or|pass|raise|return|try|while|with|yield)\b)` +
			`|(?P<number>\b\d+(?:\.\d+)?\b)` +
			`|(?P<function>\b[a-zA-Z_]\w*)`,
	)},
	"bash": {re: regexp.MustCompile(
		`(?P<comment>#[^\n]*)` +
			`|(?P<string>"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')` +
			`|(?P<keyword>\b(?:if|then|else|elif|fi|case|esac|for|while|do|done|function|return|exit|echo|cd|ls|export|source|set|unset)\b)` +
			`|(?P<number>\b\d+\b)` +
			`|(?P<function>\b[a-zA-Z_][a-zA-Z0-9_]*)`,
	)},
	"json": {re: regexp.MustCompile(
		`(?P<string>"(?:\\.|[^"\\])*")` +
			`|(?P<keyword>\b(?:true|false|null)\b)` +
			`|(?P<number>-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b)`,
	)},
	"yaml": {re: regexp.MustCompile(
		`(?P<comment>#[^\n]*)` +
			`|(?P<string>"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')` +
			`|(?P<keyword>\b(?:true|false|yes|no|null|~)\b)` +
			`|(?P<number>\b\d+(?:\.\d+)?\b)`,
	)},
}

func SupportedLanguages() []string {
	out := make([]string, 0, len(langSpecs))
	for k := range langSpecs {
		out = append(out, k)
	}
	return out
}

// HighlightCode applies ANSI syntax highlighting to a code block.
// Uses a single regex with named groups so all token kinds are matched
// in one pass — this prevents later patterns from seeing ANSI codes
// inserted by earlier ones (which would corrupt the output).
func HighlightCode(code, language string) string {
	if code == "" {
		return ""
	}
	lang := strings.ToLower(language)
	spec, ok := langSpecs[lang]
	if !ok {
		return code
	}

	var sb strings.Builder
	matches := spec.re.FindAllStringSubmatchIndex(code, -1)
	// Build a list of group names in declaration order so we can map a
	// non-empty submatch span back to its token kind.
	groupNames := spec.re.SubexpNames()[1:] // skip index 0 (whole match)
	pos := 0
	for _, m := range matches {
		if m[0] > pos {
			sb.WriteString(code[pos:m[0]])
		}
		match := code[m[0]:m[1]]
		// Determine which named group has a non-empty match.
		var color string
		for i, name := range groupNames {
			if name == "" {
				continue // unnamed group
			}
			start, end := m[2*(i+1)], m[2*(i+1)+1]
			if start < 0 || end < 0 || start >= end {
				continue
			}
			if start == m[0] && end == m[1] {
				// This group's span matches the whole match.
				color = colorFor(name)
				break
			}
		}
		if color != "" {
			sb.WriteString(color)
			sb.WriteString(match)
			sb.WriteString(ansiReset)
		} else {
			sb.WriteString(match)
		}
		pos = m[1]
	}
	if pos < len(code) {
		sb.WriteString(code[pos:])
	}
	return sb.String()
}

// colorFor maps a regex group name to an ANSI color sequence.
func colorFor(name string) string {
	switch name {
	case "comment":
		return ansiDim
	case "string":
		return ansiGreen
	case "keyword":
		return ansiBold + ansiAmber
	case "number":
		return ansiCyan
	case "function":
		return ansiRust
	default:
		return ""
	}
}

// CodeBlock is one fenced code block extracted from chat content.
type CodeBlock struct {
	Language string
	Code     string
}

func ExtractCodeBlocks(text string) []CodeBlock {
	if text == "" {
		return nil
	}
	re := regexp.MustCompile("(?s)\x60\x60\x60([a-zA-Z0-9_+-]*)\\n(.*?)\x60\x60\x60")
	matches := re.FindAllStringSubmatch(text, -1)
	out := make([]CodeBlock, 0, len(matches))
	for _, m := range matches {
		out = append(out, CodeBlock{
			Language: strings.ToLower(m[1]),
			Code:     m[2],
		})
	}
	return out
}

// HighlightCodeBlocks returns the original text with all fenced code
// blocks replaced by their ANSI-highlighted versions.
func HighlightCodeBlocks(text string) string {
	if text == "" {
		return text
	}
	re := regexp.MustCompile("(?s)\x60\x60\x60([a-zA-Z0-9_+-]*)\\n(.*?)\x60\x60\x60")
	return re.ReplaceAllStringFunc(text, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		lang := strings.ToLower(sub[1])
		if lang == "" {
			return match
		}
		highlighted := HighlightCode(sub[2], lang)
		return "```" + lang + "\n" + highlighted + "\n```"
	})
}

// ColorName returns the friendly name of a color for a token type.
func ColorName(tokenType string) string {
	switch tokenType {
	case "keyword":
		return "amber (Gold #FFB000)"
	case "string":
		return "green (Mech Spirit #39FF14)"
	case "comment":
		return "dim gray (Bulkhead #666666)"
	case "number":
		return "cyan (Noosphere #00FFFF)"
	case "function":
		return "rust (Iron #CD7F32)"
	default:
		return "?"
	}
}