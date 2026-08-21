// Package main — Heretic Forge TUI (Bubble Tea terminal).
// «Per Ignis, per Ferrum, per Codicem.»
// Dark Mechanicus coding terminal — local replacement for OpenCode.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/config"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/tui"
)

func main() {
	forgeURL := flag.String("url", "http://127.0.0.1:9091", "Forge server URL")
	repoPath := flag.String("repo", ".", "Repository path (informational)")
	flag.Parse()

	// Print banner once at startup (before alt screen)
	fmt.Println(theme.Banner())
	fmt.Println()

	// Load custom commands from commands.yaml in the repo root.
	if *repoPath == "." {
		if cwd, err := os.Getwd(); err == nil {
			*repoPath = cwd
		}
	} else {
		*repoPath, _ = filepath.Abs(*repoPath)
	}
	commands, err := config.LoadCustomCommands(*repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "forge-tui: failed to load commands.yaml: %v\n", err)
	}
	for i := range commands {
		commands[i].ExpandCommand(*repoPath)
	}

	model := tui.Initial(*forgeURL, commands, *repoPath)
	model.AppendEntry("system", "", fmt.Sprintf("repo=%s | url=%s", *repoPath, *forgeURL))

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "forge-tui: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(theme.DimStyle.Render("\n⚒ Занесено. Never fade away."))
}
