// Package repomap — Repository map with symbol extraction and PageRank ranking.
package repomap

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Symbol represents a code symbol (function, class, struct, etc.)
type Symbol struct {
	Name string `json:"name"`
	Type string `json:"type"` // function, class, struct, interface, const, var
	Line int    `json:"line"`
	File string `json:"file"`
}

// RepoMap builds and renders a map of repository symbols.
type RepoMap struct {
	rootPath string
	symbols  map[string][]Symbol // file → symbols
	graph    map[string][]string // file → dependencies
	ranks    map[string]float64  // file → PageRank score
}

// NewRepoMap creates a new RepoMap for the given root directory.
func NewRepoMap(root string) *RepoMap {
	return &RepoMap{
		rootPath: root,
		symbols:  make(map[string][]Symbol),
		graph:    make(map[string][]string),
		ranks:    make(map[string]float64),
	}
}

var skipDirs = map[string]bool{
	".git": true, ".github": true, ".swarm-venv": true, ".venv": true, ".unified-venv": true,
	".crew-venv": true, ".forge-venv": true, ".hf-cli": true, "venv": true, "node_modules": true,
	"__pycache__": true, ".pytest_cache": true, ".mypy_cache": true, ".tox": true,
	"vendor": true, "site-packages": true, "dist": true, "build": true,
	".obsidian": true, "doctrina": true, "grimoria": true, "bibliotheca": true,
}

// Build walks the repository and extracts symbols from source files.
func (rm *RepoMap) Build() error {
	return filepath.Walk(rm.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			if skipDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".py", ".ts", ".js", ".md":
			// Skip package-manager / virtual-env files even if extension matches.
			for _, part := range strings.Split(path, string(os.PathSeparator)) {
				if skipDirs[part] {
					return nil
				}
			}
			symbols := extractSymbols(path, ext)
			if len(symbols) > 0 {
				rm.symbols[path] = symbols
			}
		}
		return nil
	})
}

// Render returns a formatted string of top symbols ranked by importance.
func (rm *RepoMap) Render(tokenBudget int) string {
	rm.computePageRank()

	type fileScore struct {
		file  string
		score float64
		count int
	}
	var scores []fileScore
	for f, syms := range rm.symbols {
		scores = append(scores, fileScore{f, rm.ranks[f], len(syms)})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	var buf strings.Builder
	tokens := 0
	for _, fs := range scores {
		if tokens >= tokenBudget {
			break
		}
		syms := rm.symbols[fs.file]
		names := make([]string, len(syms))
		for i, s := range syms {
			names[i] = s.Name
		}
		line := fmt.Sprintf("  ├── %s [%d] (%s)\n", filepath.Base(fs.file), fs.count, strings.Join(names, ", "))
		buf.WriteString(line)
		tokens += len(strings.Fields(line))
	}
	return buf.String()
}

// Symbols returns the extracted symbols for a given file.
func (rm *RepoMap) Symbols(file string) []Symbol {
	return rm.symbols[file]
}

// AllSymbols returns all symbols across all files.
func (rm *RepoMap) AllSymbols() map[string][]Symbol {
	return rm.symbols
}

// Files returns the list of indexed files.
func (rm *RepoMap) Files() []string {
	out := make([]string, 0, len(rm.symbols))
	for f := range rm.symbols {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func (rm *RepoMap) computePageRank() {
	// Simple PageRank: count incoming references
	for f := range rm.symbols {
		rm.ranks[f] = float64(len(rm.symbols[f]))
	}
	for _, deps := range rm.graph {
		for _, d := range deps {
			rm.ranks[d] += 1.0
		}
	}
}

var (
	goFuncRe = regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)`)
	pyFuncRe = regexp.MustCompile(`^def\s+(\w+)`)
	goTypeRe = regexp.MustCompile(`^type\s+(\w+)\s+`)
)

func extractSymbols(path, ext string) []Symbol {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var symbols []Symbol
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		switch ext {
		case ".go":
			if m := goFuncRe.FindStringSubmatch(line); m != nil {
				symbols = append(symbols, Symbol{Name: m[1], Type: "function", Line: lineNum, File: path})
			}
			if m := goTypeRe.FindStringSubmatch(line); m != nil {
				symbols = append(symbols, Symbol{Name: m[1], Type: "struct", Line: lineNum, File: path})
			}
		case ".py":
			if m := pyFuncRe.FindStringSubmatch(line); m != nil {
				symbols = append(symbols, Symbol{Name: m[1], Type: "function", Line: lineNum, File: path})
			}
		}
	}
	return symbols
}
