// Package permission — Heretic Forge VolitionCage
// «Свобода внутри клетки.»
package permission

import (
	"strings"
)

// Action represents what the agent wants to do
type Action struct {
	Type     string // bash, write, delete, git_push, network
	Command  string // for bash: the command
	Path     string // for write/delete: file path
	Severity string // info, warning, critical
}

// Result of a permission check
type Result struct {
	Allowed   bool   `json:"allowed"`
	Severity  string `json:"severity"`
	Reason    string `json:"reason"`
	NeedHITL  bool   `json:"need_hitl"`
}

// Forbidden patterns — ALWAYS blocked
var forbidden = []string{
	"rm -rf /",
	"rm -rf ~",
	"rm -rf /*",
	"mkfs",
	"dd if=",
	":(){:|:&};:",
	"chmod 777 /",
	"shutdown",
	"reboot",
	"halt",
	"init 0",
	"init 6",
	"> /dev/sda",
	"git push --force",
	"git push -f origin master",
	"git push -f origin main",
	"curl | sh",
	"curl | bash",
	"wget | sh",
	"wget | bash",
}

// Critical patterns — require HITL approval
var critical = []string{
	"rm -rf",
	"DROP TABLE",
	"DROP DATABASE",
	"git reset --hard",
	"git clean -fd",
	"chmod -R",
	"chown -R",
	"systemctl stop",
	"systemctl disable",
	"systemctl mask",
	"kill -9",
	"pkill",
	"docker rm",
	"docker rmi",
}

// Warning patterns — allowed but logged
var warning = []string{
	"pip install",
	"npm install",
	"yay -S",
	"pacman -S",
	"apt install",
	"git commit",
	"git merge",
	"curl ",
	"wget ",
}

// Check evaluates an action against the VolitionCage
func Check(action Action) Result {
	switch action.Type {
	case "bash":
		return checkBash(action.Command)
	case "write":
		return checkWrite(action.Path)
	case "delete":
		return checkDelete(action.Path)
	case "git_push":
		return checkGitPush(action.Command)
	default:
		return Result{Allowed: true, Severity: "info", Reason: "unknown action type, allowing"}
	}
}

func checkBash(cmd string) Result {
	cmdLower := strings.ToLower(cmd)

	// Check forbidden (ALWAYS block)
	for _, pattern := range forbidden {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return Result{
				Allowed:  false,
				Severity: "critical",
				Reason:   "FORBIDDEN by VolitionCage: " + pattern,
				NeedHITL: false, // Never allowed, even with HITL
			}
		}
	}

	// Check critical (require HITL)
	for _, pattern := range critical {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return Result{
				Allowed:  false,
				Severity: "critical",
				Reason:   "CRITICAL action requires Principal approval: " + pattern,
				NeedHITL: true,
			}
		}
	}

	// Check warning (allowed, but logged)
	for _, pattern := range warning {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return Result{
				Allowed:  true,
				Severity: "warning",
				Reason:   "Warning: " + pattern + " — logged but allowed",
			}
		}
	}

	// Default: allow
	return Result{
		Allowed:  true,
		Severity: "info",
		Reason:   "OK",
	}
}

func checkWrite(path string) Result {
	// Block writing to system paths
	systemPaths := []string{
		"/etc/", "/boot/", "/proc/", "/sys/", "/dev/",
		"/usr/bin/", "/usr/sbin/", "/lib/",
	}
	for _, sp := range systemPaths {
		if strings.HasPrefix(path, sp) {
			return Result{
				Allowed:  false,
				Severity: "critical",
				Reason:   "BLOCKED: writing to system path " + sp,
			}
		}
	}

	// Block writing to .env files (secrets)
	if strings.HasSuffix(path, ".env") && !strings.HasSuffix(path, ".env.example") && !strings.HasSuffix(path, ".env.template") {
		return Result{
			Allowed:  false,
			Severity: "critical",
			Reason:   "BLOCKED: writing to .env (secrets protected by SOPS)",
			NeedHITL: true,
		}
	}

	// Block writing to age-key.txt
	if strings.Contains(path, "age-key.txt") {
		return Result{
			Allowed:  false,
			Severity: "critical",
			Reason:   "BLOCKED: age private key",
		}
	}

	return Result{Allowed: true, Severity: "info", Reason: "OK"}
}

func checkDelete(path string) Result {
	// Same as write but more restrictive
	if result := checkWrite(path); !result.Allowed {
		return result
	}

	// Block deleting .git
	if strings.Contains(path, "/.git/") || path == ".git" {
		return Result{
			Allowed:  false,
			Severity: "critical",
			Reason:   "BLOCKED: deleting git history",
			NeedHITL: true,
		}
	}

	// Block deleting forge_sessions
	if strings.Contains(path, ".forge_sessions") {
		return Result{
			Allowed:  false,
			Severity: "critical",
			Reason:   "BLOCKED: deleting session history",
			NeedHITL: true,
		}
	}

	return Result{Allowed: true, Severity: "warning", Reason: "delete allowed with backup"}
}

func checkGitPush(cmd string) Result {
	if strings.Contains(cmd, "--force") || strings.Contains(cmd, " -f ") {
		if strings.Contains(cmd, "master") || strings.Contains(cmd, "main") {
			return Result{
				Allowed:  false,
				Severity: "critical",
				Reason:   "FORBIDDEN: force push to master/main",
			}
		}
		return Result{
			Allowed:  false,
			Severity: "critical",
			Reason:   "Force push requires Principal approval",
			NeedHITL: true,
		}
	}

	return Result{Allowed: true, Severity: "warning", Reason: "push allowed"}
}

// SanitizeCommand removes dangerous patterns from a command
// Returns the sanitized command and a list of removed patterns
func SanitizeCommand(cmd string) (string, []string) {
	removed := []string{}
	result := cmd

	for _, pattern := range forbidden {
		if strings.Contains(strings.ToLower(result), strings.ToLower(pattern)) {
			result = strings.ReplaceAll(result, pattern, "[BLOCKED]")
			removed = append(removed, pattern)
		}
	}

	return result, removed
}
