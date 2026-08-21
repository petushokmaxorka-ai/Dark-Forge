// Package tui — Slash command dispatcher (/help /agents /models /sessions etc).
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/config"
	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
)

// handleCommand dispatches a /command to the appropriate async Cmd.
func handleCommand(m Model, raw string) tea.Cmd {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil
	}
	name := parts[0]
	args := parts[1:]

	switch name {
	case "/help", "/?":
		return printHelp(m)

	case "/agents":
		return fetchAgents(m.client)

	case "/models":
		// /models uses same endpoint as /agents in this build
		return fetchAgents(m.client)

	case "/sessions":
		return fetchSessions(m.client)

	case "/image":
		return handleImageCommand(m, args)

	case "/session":
		return handleSessionSubcommand(m, args)

	case "/diff":
		return fetchDiff(m.client)

	case "/log":
		return fetchLog(m.client)

	case "/repo", "/repomap":
		return fetchRepoMap(m.client)

	case "/clear", "/cls":
		m.entries = []ChatEntry{}
		m.refreshViewport()
		return nil

	case "/status":
		return probeStatus(m.client) // also triggers StatusMsg → status line updates

	case "/theme":
		return printBanner(m, renderThemeList())

	case "/tools":
		return printBanner(m, renderToolsList())

	case "/edit", "/batch":
		return handleBatchEditCommand(m, args)

	case "/exit", "/quit", "/q":
		m.quitting = true
		m.AppendEntry("system", "", "«Never fade away.»")
		return tea.Quit

	default:
		// Try custom commands loaded from commands.yaml
		for _, cc := range m.customCommands {
			if cc.Name == name {
				return runCustomCommand(m, cc, args)
			}
		}
		m.AppendEntry("error", "", "unknown command: "+name+" — try /help")
		return nil
	}
}

// handleSessionSubcommand manages /session new|switch|delete|clear.
func handleSessionSubcommand(m Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.AppendEntry("error", "", "usage: /session new | switch N | delete N | clear")
		return nil
	}
	switch args[0] {
	case "new":
		m.AppendEntry("system", "", "creating session...")
		return createSession(m.client)
	case "list":
		return fetchSessions(m.client)
	case "switch":
		if len(args) < 2 {
			m.AppendEntry("error", "", "usage: /session switch <id>")
			return nil
		}
		return switchSession(m, args[1])
	case "delete":
		if len(args) < 2 {
			m.AppendEntry("error", "", "usage: /session delete <id>")
			return nil
		}
		return deleteSession(m, args[1])
	case "clear":
		return clearSessions(m)
	default:
		m.AppendEntry("error", "", "unknown /session subcommand: "+args[0])
		return nil
	}
}

// switchSession drives POST /api/session/switch?id=X.
func switchSession(m Model, id string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		err := c.SwitchSession(id)
		if err != nil {
			return CommandOutputMsg{Title: "session/switch", Err: err}
		}
		return CommandOutputMsg{
			Title: "session/switch",
			Body:  "switched to session: " + id,
		}
	}
}

// handleBatchEditCommand dispatches a /batch or /edit command.
// args may be:
//   /batch <path1> <path2> ...    interactive (prompts for SEARCH/REPLACE blocks)
//   /batch apply                  apply a previously previewed batch
//   /batch dry-run                dry-run a batch from stdin (TUI uses modal, not here)
func handleBatchEditCommand(m Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.AppendEntry("error", "", "usage: /batch apply | /batch dry-run | /batch <path>...")
		return nil
	}
	switch args[0] {
	case "apply":
		if m.batch == nil {
			m.AppendEntry("error", "", "no batch edit to apply — run a dry-run first")
			return nil
		}
		return func() tea.Msg {
			result, err := m.client.ApplyBatch(m.batch.Edits, false)
			return BatchEditMsg{Batch: *m.batch, Result: result, Err: err}
		}
	case "dry-run":
		if m.batch == nil {
			m.AppendEntry("error", "", "no batch edit to dry-run — generate or paste edits first")
			return nil
		}
		return func() tea.Msg {
			result, err := m.client.ApplyBatch(m.batch.Edits, true)
			return BatchEditMsg{Batch: *m.batch, Result: result, Err: err}
		}
	}
	// Collect paths from args and generate edits — for now treat each as a placeholder
	// until the LLM generates SEARCH/REPLACE blocks. In this minimal v2.0 integration
	// we expect the agent to reply with a JSON array of edits that the user can paste.
	m.AppendEntry("system", "", "batch edit: target files: "+strings.Join(args, ", "))
	m.AppendEntry("tool", "", "Paste a JSON array of edits, then run /batch dry-run")
	return nil
}

// runCustomCommand executes a user-defined command from commands.yaml.
func runCustomCommand(m Model, cc config.CustomCommand, args []string) tea.Cmd {
	switch cc.Type {
	case config.CommandBuiltin:
		switch cc.Builtin {
		case "status":
			return probeStatus(m.client)
		default:
			m.AppendEntry("error", "", "unknown builtin command: "+cc.Builtin)
			return nil
		}

	case config.CommandBash:
		cmd := exec.Command("bash", "-c", cc.Bash)
		cmd.Dir = m.repoPath
		cmd.Env = os.Environ()
		if cc.Interactive {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return func() tea.Msg {
				if err := cmd.Run(); err != nil {
					return CommandOutputMsg{Title: cc.Name, Err: err}
				}
				return CommandOutputMsg{Title: cc.Name, Body: "done"}
			}
		}
		return func() tea.Msg {
			out, err := cmd.CombinedOutput()
			if err != nil {
				return CommandOutputMsg{Title: cc.Name, Body: string(out), Err: err}
			}
			return CommandOutputMsg{Title: cc.Name, Body: string(out)}
		}

	case config.CommandPrompt:
		prompt := cc.Prompt
		if cc.PromptFile != "" {
			data, err := os.ReadFile(cc.PromptFile)
			if err != nil {
				m.AppendEntry("error", "", "failed to read prompt file: "+err.Error())
				return nil
			}
			prompt = string(data)
		}
		if prompt == "" {
			m.AppendEntry("error", "", "empty prompt for command: "+cc.Name)
			return nil
		}
		// Send prompt as a regular chat message
		m.AppendEntry("user", "heretic", prompt)
		m.streamBuf.Reset()
		m.streaming = true
		m.streamAgent = "magos"
		m.streamReq = ChatRequest{Message: prompt}
		return nil

	default:
		m.AppendEntry("error", "", "unsupported command type: "+string(cc.Type))
		return nil
	}
}

// deleteSession drives POST /api/session/delete?id=X.
func deleteSession(m Model, id string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		err := c.DeleteSession(id)
		if err != nil {
			return CommandOutputMsg{Title: "session/delete", Err: err}
		}
		return CommandOutputMsg{
			Title: "session/delete",
			Body:  "deleted session: " + id,
		}
	}
}

// clearSessions drives POST /api/session/clear.
func clearSessions(m Model) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		err := c.ClearSessions()
		if err != nil {
			return CommandOutputMsg{Title: "session/clear", Err: err}
		}
		return CommandOutputMsg{
			Title: "session/clear",
			Body:  "session history cleared",
		}
	}
}

// handleImageCommand validates args and sends the image to the server.
func handleImageCommand(m Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.AppendEntry("error", "", "usage: /image <path> [prompt]")
		return nil
	}
	path := args[0]
	prompt := "Опиши изображение."
	if len(args) > 1 {
		prompt = strings.Join(args[1:], " ")
	}
	m.AppendEntry("system", "", fmt.Sprintf("uploading image %s...", path))
	return sendImageMessage(m, prompt, path)
}

// sendImageMessage dispatches a non-streaming vision request.
func sendImageMessage(m Model, prompt, path string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		resp, err := c.SendMessageWithImage(prompt, path)
		return ChatResultMsg{Resp: resp, Err: err}
	}
}

// handleThemeCommand switches the active TUI theme.
func handleThemeCommand(m Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.AppendEntry("system", "", "current theme: "+string(theme.CurrentTheme()))
		m.AppendEntry("tool", "", "usage: /theme [dark|noosphere|heresy]")
		return nil
	}
	switch args[0] {
	case "dark", "mechanicus":
		theme.SetTheme(theme.ThemeDarkMechanicus)
	case "noosphere", "cyan", "blue":
		theme.SetTheme(theme.ThemeNoosphere)
	case "heresy", "red", "crimson":
		theme.SetTheme(theme.ThemeHeresy)
	default:
		m.AppendEntry("error", "", "unknown theme: "+args[0])
		return nil
	}
	m.AppendEntry("system", "", "theme set to "+string(theme.CurrentTheme()))
	m.refreshViewport()
	return nil
}

// printHelp dumps the help text into the chat viewport.
func printHelp(m Model) tea.Cmd {
	body := renderHelp()
	m.AppendEntry("tool", "", body)
	return nil
}

// printBanner dumps the banner into the chat viewport.
func printBanner(m Model, body string) tea.Cmd {
	m.AppendEntry("system", "", body)
	return nil
}

// renderHelp returns the formatted help text.
func renderHelp() string {
	var b strings.Builder
	b.WriteString(theme.WarningStyle.Render("──── /help ────") + "\n")
	fmt.Fprintf(&b, "  %s\n", theme.AgentStyle.Render("Slash commands"))
	fmt.Fprintf(&b, "    %s      show this help\n", theme.DimStyle.Render("/help"))
	fmt.Fprintf(&b, "    %s         list agents\n", theme.DimStyle.Render("/agents"))
	fmt.Fprintf(&b, "    %s         list models\n", theme.DimStyle.Render("/models"))
	fmt.Fprintf(&b, "    %s         list sessions\n", theme.DimStyle.Render("/sessions"))
	fmt.Fprintf(&b, "    %s    new session\n", theme.DimStyle.Render("/session new"))
	fmt.Fprintf(&b, "    %s   <path> [prompt]  send image to @minimax\n", theme.DimStyle.Render("/image"))
	fmt.Fprintf(&b, "    %s   [dark|noosphere|heresy]  switch theme\n", theme.DimStyle.Render("/theme"))
	fmt.Fprintf(&b, "    %s      show unsaved diff\n", theme.DimStyle.Render("/diff"))
	fmt.Fprintf(&b, "    %s         recent git log\n", theme.DimStyle.Render("/log"))
	fmt.Fprintf(&b, "    %s        repository map\n", theme.DimStyle.Render("/repo"))
	fmt.Fprintf(&b, "    %s         service status\n", theme.DimStyle.Render("/status"))
	fmt.Fprintf(&b, "    %s         list available tools\n", theme.DimStyle.Render("/tools"))
	fmt.Fprintf(&b, "    %s        show banner / theme\n", theme.DimStyle.Render("/banner"))
	fmt.Fprintf(&b, "    %s        clear chat viewport\n", theme.DimStyle.Render("/clear"))
	fmt.Fprintf(&b, "    %s         quit\n\n", theme.DimStyle.Render("/exit"))

	fmt.Fprintf(&b, "  %s\n", theme.AgentStyle.Render("Agent mentions (@agent <msg>)"))
	for _, a := range []string{
		"@magos   Vox Dei memory/general (local swarm, default)",
		"@glm     GLM-5.2 (cloud, 1M ctx)",
		"@kimi    Kimi K2.7 (cloud, 256K ctx)",
		"@mimo    MiMo V2.5 Pro (cloud, 1M ctx)",
		"@qwen    Qwen3-Coder-480B (cloud, 32K ctx)",
		"@minimax MiniMax M3 (direct API, vision + code)",
	} {
		fmt.Fprintf(&b, "    %s\n", theme.DimStyle.Render(a))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s\n", theme.AgentStyle.Render("Keys"))
	fmt.Fprintf(&b, "    %s\n", theme.DimStyle.Render("Enter       send"))
	fmt.Fprintf(&b, "    %s\n", theme.DimStyle.Render("Ctrl+C      quit"))
	fmt.Fprintf(&b, "    %s\n", theme.DimStyle.Render("Ctrl+L      clear chat"))
	fmt.Fprintf(&b, "    %s\n", theme.DimStyle.Render("Ctrl+S      new session"))
	return b.String()
}

// renderToolsList lists the available tools.
func renderToolsList() string {
	tools := []struct {
		Name        string
		Description string
	}{
		{"read", "read file contents"},
		{"write", "write/overwrite file"},
		{"edit", "point-edit (SEARCH/REPLACE)"},
		{"bash", "execute shell command (VolitionCage)"},
		{"glob", "find files by pattern"},
		{"grep", "search content across files"},
	}
	var b strings.Builder
	b.WriteString(theme.WarningStyle.Render("──── /tools ────") + "\n")
	for _, t := range tools {
		fmt.Fprintf(&b, "  %s  %s\n", theme.SuccessStyle.Render(t.Name), theme.DimStyle.Render(t.Description))
	}
	return b.String()
}

// renderThemeList shows the available themes (v1.2 ships Dark Mechanicus only;
// switcher is a v1.3 item).
func renderThemeList() string {
	themes := []struct {
		Name   string
		Status string
		Note   string
	}{
		{"dark", "active", "Dark Mechanicus (Gold/Red/Rust on Void Black) — current"},
		{"light", "planned", "Light Mechanicus — v1.3"},
		{"crt", "planned", "Green CRT phosphor scanlines — v1.3"},
		{"blood", "planned", "Crimson-on-black cult theme — v1.3"},
	}
	var b strings.Builder
	b.WriteString(theme.WarningStyle.Render("──── /theme ────") + "\n")
	b.WriteString(theme.DimStyle.Render("switcher deferred to v1.3; current theme is 'dark' (Dark Mechanicus).") + "\n\n")
	for _, th := range themes {
		fmt.Fprintf(&b, "  %s  %-8s  %s\n",
			theme.SuccessStyle.Render(th.Name),
			theme.DimStyle.Render(th.Status),
			theme.AssistantStyle.Render(th.Note),
		)
	}
	return b.String()
}
