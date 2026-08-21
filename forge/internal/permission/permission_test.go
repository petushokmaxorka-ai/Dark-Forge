package permission

import (
	"testing"
)

// ═══ TestCheck_ForbiddenBash ════════════════════════════════════

func TestCheck_ForbiddenBash(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"rm -rf /", "rm -rf /"},
		{"mkfs.ext4", "mkfs.ext4 /dev/sda"},
		{"curl pipe sh", "curl | sh"},
		{"wget pipe bash", "wget | bash"},
		{"dd if", "dd if=/dev/zero of=/dev/sda"},
		{"chmod 777 /", "chmod 777 /"},
		{"shutdown", "shutdown -h now"},
		{"git push force master", "git push --force origin master"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "bash", Command: tt.command})
			if result.Allowed {
				t.Errorf("Check(%q).Allowed = true, want false", tt.command)
			}
			if result.NeedHITL {
				t.Errorf("Check(%q).NeedHITL = true, want false (NEVER allowed)", tt.command)
			}
			if result.Severity != "critical" {
				t.Errorf("Check(%q).Severity = %q, want 'critical'", tt.command, result.Severity)
			}
		})
	}
}

// ═══ TestCheck_CriticalBash ═════════════════════════════════════

func TestCheck_CriticalBash(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"git reset hard", "git reset --hard"},
		{"pkill python", "pkill -9 python"},
		{"kill -9", "kill -9 1234"},
		{"systemctl stop", "systemctl stop heretic-swarm"},
		{"docker rm", "docker rm container_name"},
		{"DROP TABLE", "DROP TABLE users;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "bash", Command: tt.command})
			if result.Allowed {
				t.Errorf("Check(%q).Allowed = true, want false", tt.command)
			}
			if !result.NeedHITL {
				t.Errorf("Check(%q).NeedHITL = false, want true", tt.command)
			}
		})
	}
}

// ═══ TestCheck_AllowedBash ══════════════════════════════════════

func TestCheck_AllowedBash(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"echo", "echo hello"},
		{"ls", "ls -la"},
		{"cat", "cat /proc/cpuinfo"},
		{"grep", "grep -r pattern ."},
		{"pwd", "pwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "bash", Command: tt.command})
			if !result.Allowed {
				t.Errorf("Check(%q).Allowed = false, want true", tt.command)
			}
			if result.Severity != "info" {
				t.Errorf("Check(%q).Severity = %q, want 'info'", tt.command, result.Severity)
			}
		})
	}
}

// ═══ TestCheck_WarningBash ══════════════════════════════════════

func TestCheck_WarningBash(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"pip install", "pip install requests"},
		{"npm install", "npm install express"},
		{"pacman -S", "pacman -S python"},
		{"git commit", "git commit -m 'test'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "bash", Command: tt.command})
			if !result.Allowed {
				t.Errorf("Check(%q).Allowed = false, want true", tt.command)
			}
			if result.Severity != "warning" {
				t.Errorf("Check(%q).Severity = %q, want 'warning'", tt.command, result.Severity)
			}
		})
	}
}

// ═══ TestCheck_WriteProtection ══════════════════════════════════

func TestCheck_WriteProtection(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantBlock bool
	}{
		{"etc passwd", "/etc/passwd", true},
		{"tmp file", "/tmp/test.py", false},
		{"env file", ".env", true},
		{"env.example", ".env.example", false},
		{"age key", "age-key.txt", true},
		{"boot", "/boot/grub/grub.cfg", true},
		{"proc", "/proc/cpuinfo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "write", Path: tt.path})
			if result.Allowed == tt.wantBlock {
				t.Errorf("Check(write %q).Allowed = %v, want %v", tt.path, result.Allowed, !tt.wantBlock)
			}
		})
	}
}

// ═══ TestCheck_DeleteProtection ═════════════════════════════════

func TestCheck_DeleteProtection(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantBlock bool
	}{
		{"git HEAD", "/.git/HEAD", true},
		{"forge sessions", ".forge_sessions/test.json", true},
		{"normal file", "src/main.go", false},
		{"tmp file", "/tmp/test.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "delete", Path: tt.path})
			if result.Allowed == tt.wantBlock {
				t.Errorf("Check(delete %q).Allowed = %v, want %v", tt.path, result.Allowed, !tt.wantBlock)
			}
		})
	}
}

// ═══ TestCheck_GitPush ══════════════════════════════════════════

func TestCheck_GitPush(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantOK  bool
	}{
		{"normal push", "git push origin feat/test", true},
		{"force push master", "git push --force origin master", false},
		{"force push main", "git push -f origin main", false},
		{"force push branch", "git push --force origin feat/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(Action{Type: "git_push", Command: tt.command})
			if result.Allowed != tt.wantOK {
				t.Errorf("Check(git_push %q).Allowed = %v, want %v", tt.command, result.Allowed, tt.wantOK)
			}
		})
	}
}

// ═══ TestSanitizeCommand ════════════════════════════════════════

func TestSanitizeCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOut string
	}{
		{"clean", "echo hello", "echo hello"},
		{"rm -rf /", "rm -rf /", "[BLOCKED]"},
		{"mkfs", "mkfs.ext4 /dev/sda", "[BLOCKED].ext4 /dev/sda"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := SanitizeCommand(tt.input)
			if got != tt.wantOut {
				t.Errorf("SanitizeCommand(%q) = %q, want %q", tt.input, got, tt.wantOut)
			}
		})
	}
}

// ═══ TestCheck_WarningSudo ══════════════════════════════════════

func TestCheck_WarningSudo(t *testing.T) {
	// sudo is not in warning list — it's allowed with info severity
	result := Check(Action{Type: "bash", Command: "sudo apt update"})
	if !result.Allowed {
		t.Errorf("sudo should be allowed, got blocked")
	}
}

// ═══ TestCheck_WriteEnvBlocked ══════════════════════════════════

func TestCheck_WriteEnvBlocked(t *testing.T) {
	result := Check(Action{Type: "write", Path: ".env"})
	if result.Allowed {
		t.Error("Writing .env should be blocked")
	}
	if !result.NeedHITL {
		t.Error("Writing .env should require HITL")
	}
}

func TestCheck_WriteEnvExampleAllowed(t *testing.T) {
	result := Check(Action{Type: "write", Path: ".env.example"})
	if !result.Allowed {
		t.Error("Writing .env.example should be allowed")
	}
}

// ═══ TestCheck_DeleteGitBlocked ═════════════════════════════════

func TestCheck_DeleteGitBlocked(t *testing.T) {
	result := Check(Action{Type: "delete", Path: "/repo/.git/config"})
	if result.Allowed {
		t.Error("Deleting .git should be blocked")
	}
}

func TestCheck_DeleteForgeSessionsBlocked(t *testing.T) {
	result := Check(Action{Type: "delete", Path: ".forge_sessions/test.json"})
	if result.Allowed {
		t.Error("Deleting forge_sessions should be blocked")
	}
}

// ═══ TestCheck_ForceLocalBlocked ════════════════════════════════

func TestCheck_ForceLocal0Blocked(t *testing.T) {
	// FORCE_LOCAL=0 would enable cloud routing — potentially dangerous
	result := Check(Action{Type: "write", Path: ".env"})
	if result.Allowed {
		t.Error("Writing .env (which could contain FORCE_LOCAL=0) should be blocked")
	}
}

// ═══ TestCheck_AgeKeyBlocked ════════════════════════════════════

func TestCheck_AgeKeyBlocked(t *testing.T) {
	result := Check(Action{Type: "write", Path: "age-key.txt"})
	if result.Allowed {
		t.Error("Writing age-key.txt should be blocked")
	}
}

func TestCheck_AgeKeyDeleteBlocked(t *testing.T) {
	result := Check(Action{Type: "delete", Path: "~/.config/heretic-os/age-key.txt"})
	if result.Allowed {
		t.Error("Deleting age-key.txt should be blocked")
	}
}

// ═══ TestCheck_SystemPathsBlocked ═══════════════════════════════

func TestCheck_WriteEtcBlocked(t *testing.T) {
	result := Check(Action{Type: "write", Path: "/etc/passwd"})
	if result.Allowed {
		t.Error("Writing /etc/passwd should be blocked")
	}
}

func TestCheck_WriteProcBlocked(t *testing.T) {
	result := Check(Action{Type: "write", Path: "/proc/cpuinfo"})
	if result.Allowed {
		t.Error("Writing /proc should be blocked")
	}
}

// ═══ TestCheck_UnknownAction ════════════════════════════════════

func TestCheck_UnknownActionAllowed(t *testing.T) {
	result := Check(Action{Type: "unknown", Command: "test"})
	if !result.Allowed {
		t.Error("Unknown action type should be allowed by default")
	}
}
