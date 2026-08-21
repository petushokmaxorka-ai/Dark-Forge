# LSP Integration Design — Forge v2.0

## Overview

Integrate Language Server Protocol (LSP) into Heretic Forge to enable:
- Go-to-definition
- Hover information
- Real-time diagnostics
- Symbol search
- Code completion context

## Architecture

```
User types: "fix the error in main.go"
  ↓
Forge agent calls LSP tool
  ↓
LSP wrapper spawns gopls (or pylsp, rust-analyzer)
  ↓
JSON-RPC over stdin/stdout
  ↓
Diagnostics returned to agent
  ↓
Agent sees: "main.go:15: undefined: fmt"
  ↓
Agent fixes the error
```

## Language Servers

| Language | Binary | Package |
|----------|--------|---------|
| Go | `gopls` | `golang.org/x/tools/gopls` |
| Python | `pylsp` | `python-lsp-server` |
| Rust | `rust-analyzer` | `rust-analyzer` |
| TypeScript | `typescript-language-server` | npm |

## Protocol

LSP uses JSON-RPC 2.0 over stdio:

```json
// Request
{"jsonrpc":"2.0","id":1,"method":"textDocument/didOpen","params":{...}}

// Response
{"jsonrpc":"2.0","id":1,"result":{...}}

// Notification (no id)
{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{...}}
```

## Implementation Plan

### Phase 1: LSP Manager (internal/lsp/)

```go
type LSPManager struct {
    servers map[string]*LSPServer  // language → server
}

type LSPServer struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout bufio.Scanner
    nextID int
}

// Methods:
func (m *LSPManager) Start(language string) error
func (m *LSPManager) Stop(language string) error
func (s *LSPServer) DidOpen(path string, content string) error
func (s *LSPServer) DidChange(path string, content string) error
func (s *LSPServer) Hover(path string, line, col int) (*HoverResult, error)
func (s *LSPServer) Definition(path string, line, col int) (*Location, error)
func (s *LSPServer) Diagnostics(path string) ([]Diagnostic, error)
func (s *LSPServer) Completion(path string, line, col int) ([]CompletionItem, error)
```

### Phase 2: MCP Tool Registration

Register LSP as MCP tools:

```
lsp.hover        — hover info at position
lsp.goto         — go to definition
lsp.diagnostics  — get all diagnostics for a file
lsp.complete     — code completion
lsp.symbols      — workspace symbol search
```

### Phase 3: Agent Integration

Update Magos system prompt:
```
When investigating code errors:
1. Call lsp.diagnostics to see errors
2. Call lsp.hover for type info
3. Call lsp.goto to find definitions
4. Fix the error
5. Call lsp.diagnostics again to verify
```

### Phase 4: TUI Integration

- Status bar shows diagnostic count: `⚠ 3 errors`
- On save: automatically run `lsp.diagnostics`
- `/diagnostics` command shows all errors

## Dependencies

- `golang.org/x/tools/gopls` (Go)
- No Go library dependency needed — use `exec.Cmd` to spawn LSP servers
- JSON-RPC: implement minimal client (no external dep needed)

## Key Decisions

1. **spawn per-language** — one gopls for .go, one pylsp for .py
2. **lazy start** — only start when agent needs LSP for that language
3. **context injection** — agent gets diagnostics as part of system prompt
4. **no UI rendering** — LSP data feeds the agent, not displayed as IDE overlays

## Complexity

- ~800 lines of Go code
- ~2 days implementation
- Testing: mock LSP server for unit tests

## Risks

1. **gopls startup time** (~2-3s) — mitigate with lazy start + caching
2. **Memory** — each LSP server uses 100-500MB RAM
3. **Concurrency** — JSON-RPC requires request ID tracking
