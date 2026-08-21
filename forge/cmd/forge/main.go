// Package main — Dark Forge entry point
// «Veritas in Crypta. Per Ignis, per Ferrum, per Codicem.»
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/server"
)

const (
	BANNER = `
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   ⚒  DARK FORGE                                               ║
║                                                               ║
║   AI Coding Forge — Council of Models                         ║
║   «Per Ignis, per Ferrum, per Codicem.»                       ║
║                                                               ║
║   Local + Cloud. Any OpenAI-compatible endpoint.              ║
║   Your keys stay yours.                                       ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
`
	VERSION = "1.0.0-darkforge"
)

func main() {
	port := flag.Int("port", 9091, "Forge web server port")
	host := flag.String("host", "127.0.0.1", "Bind address")
	modelURL := flag.String("model", "http://127.0.0.1:11434", "OpenAI-compatible model server URL (llama.cpp / Ollama / llama-swap)")
	repoPath := flag.String("repo", ".", "Repository path")
	communeURL := flag.String("commune", "", "Optional commune/persona service URL")
	configPath := flag.String("config", "", "Path to forge.yaml (default: $DARKFORGE_CONFIG, ~/.config/dark-forge/forge.yaml, ./forge.yaml)")
	flag.Parse()

	if *configPath != "" {
		os.Setenv("DARKFORGE_CONFIG", *configPath)
	}

	fmt.Print(BANNER)
	log.Printf("⚒ Dark Forge v%s starting...", VERSION)
	log.Printf("⚙ Model server: %s", *modelURL)
	log.Printf("☉ Repo: %s", *repoPath)
	log.Printf("➜ Web: http://%s:%d", *host, *port)

	srv := server.New(server.Config{
		Host:       *host,
		Port:       *port,
		ModelURL:   *modelURL,
		RepoPath:   *repoPath,
		CommuneURL: *communeURL,
	})

	if err := srv.Run(); err != nil {
		log.Fatalf("☠ Forge failed: %v", err)
	}
}
