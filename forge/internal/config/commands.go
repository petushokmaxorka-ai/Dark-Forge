// Package config — Custom slash command loader.
// Loads commands.yaml from the Forge root and exposes user-defined commands.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CommandType describes how a command is executed.
type CommandType string

const (
	CommandBuiltin  CommandType = "builtin"
	CommandBash     CommandType = "bash"
	CommandPrompt   CommandType = "prompt"
	CommandExternal CommandType = "external"
)

// CustomCommand is a user-defined slash command.
type CustomCommand struct {
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	Type        CommandType `json:"type" yaml:"type"`
	Bash        string      `json:"bash,omitempty" yaml:"bash,omitempty"`
	Builtin     string      `json:"builtin,omitempty" yaml:"builtin,omitempty"`
	Prompt      string      `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	PromptFile  string      `json:"prompt_file,omitempty" yaml:"prompt_file,omitempty"`
	Interactive bool        `json:"interactive" yaml:"interactive"`
}

// CommandFile is the top-level YAML structure.
type CommandFile struct {
	Commands []CustomCommand `json:"commands" yaml:"commands"`
}

// LoadCustomCommands reads commands.yaml from repo root if present.
func LoadCustomCommands(repoPath string) ([]CustomCommand, error) {
	path := filepath.Join(repoPath, "commands.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read commands.yaml: %w", err)
	}
	var cf CommandFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse commands.yaml: %w", err)
	}
	for i := range cf.Commands {
		cf.Commands[i].Name = normalizeCommandName(cf.Commands[i].Name)
	}
	return cf.Commands, nil
}

// ExpandCommand substitutes ${REPO_PATH} and ${EDITOR} placeholders.
func (c *CustomCommand) ExpandCommand(repoPath string) {
	c.Bash = expandVars(c.Bash, repoPath)
	c.Prompt = expandVars(c.Prompt, repoPath)
	c.PromptFile = expandVars(c.PromptFile, repoPath)
}

func normalizeCommandName(name string) string {
	name = strings.TrimSpace(name)
	if name != "" && !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return name
}

func expandVars(s, repoPath string) string {
	s = strings.ReplaceAll(s, "${REPO_PATH}", repoPath)
	s = strings.ReplaceAll(s, "${EDITOR}", os.Getenv("EDITOR"))
	return s
}
