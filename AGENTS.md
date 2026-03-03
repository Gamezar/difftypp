# AGENTS.md — diffty

## Project Overview

diffty is a web-based Git diff visualization and code review tracking tool written in **Go 1.22**.
It uses only the Go standard library (zero external dependencies) and embeds templates/static
assets into the binary via `//go:embed`. Storage is JSON file-based under `$HOME/.diffty/`.

## Build / Test / Lint Commands

```bash
# Build
go build -o diffty ./cmd/diffty

# Run all tests
go test ./...

# Run all tests with verbose output
go test -v ./...

# Run a single test by name (regex match on test function name)
go test -v -run TestFunctionName ./internal/package/

# Examples:
go test -v -run TestGetBranches ./internal/git/
go test -v -run TestSaveAndLoadReviewState ./internal/storage/
go test -v -run TestHandleIndex ./internal/server/

# Run tests for a single package
go test -v ./internal/git/
go test -v ./internal/server/
go test -v ./internal/storage/

# Run tests with race detection
go test -race ./...

# Format code (always run before committing)
gofmt -w .
goimports -w .

# Vet (static analysis)
go vet ./...
```

There is no Makefile, no CI pipeline, and no linter config. Use `gofmt` and `go vet` as baseline checks.

## Denied Bash Commands

Piped usage of denied commands (`cat`, `head`, `tail`, `grep`, `rg`) is also blocked — OpenCode checks each pipeline segment independently, so `go test ./... | tail -5` is denied even though `go test*` is allowed.

To get specific sections of long command output without wasting context, redirect to a file and use `Read` with `offset`/`limit`:

```bash
# Step 1: redirect output to a temp file (matches "go test*" — allowed)
go test -v ./... > test_logs/test-output.log 2>&1

# Step 2: read just the last N lines with the Read tool
# Read test_logs/test-output.log with offset near end, limit 10
```

## Project Structure

```
cmd/diffty/main.go          — CLI entry point; parses flags, initializes storage + server
internal/
  git/git.go                — Git CLI wrapper (exec.Command); branches, diffs, file lists
  models/models.go          — Data types: FileReview, ReviewState, DiffFile, DiffHunk, BranchCompare
  server/server.go          — HTTP server, routing (Go 1.22 ServeMux), handlers, embedded templates
  server/templates/          — HTML templates (layout, index, compare, diff, error)
  server/static/css/         — Tailwind CSS (CDN) + custom CSS
  storage/storage.go        — Storage interface + JSONStorage implementation (JSON file persistence)
```

All application code lives under `internal/` — nothing is importable externally.

## Code Style Guidelines

### Imports

- Use `goimports`-style grouping: standard library first, blank line, then internal packages.
- All imports are absolute (`github.com/darccio/diffty/internal/...`). No relative imports.
- Alphabetical ordering within each group.

```go
import (
    "fmt"
    "net/http"
    "strings"

    "github.com/darccio/diffty/internal/git"
    "github.com/darccio/diffty/internal/models"
)
```

### Naming Conventions

| Element              | Convention         | Example                              |
|----------------------|--------------------|--------------------------------------|
| Exported types       | PascalCase         | `Server`, `Repository`, `DiffFile`   |
| Unexported fields    | camelCase          | `baseStoragePath`, `reposPath`       |
| Functions (exported) | PascalCase         | `NewRepository`, `IsValidRepo`       |
| Functions (private)  | camelCase          | `handleIndex`, `renderError`         |
| Variables            | camelCase          | `repoPath`, `fileStatusMap`          |
| Constants (exported) | PascalCase         | `StateApproved`, `StateRejected`     |
| Files                | lowercase/snake    | `server.go`, `server_test.go`        |
| Directories          | lowercase single   | `git/`, `server/`, `storage/`        |
| Method receivers     | Single letter      | `s` for Server, `r` for Repository   |
| Constructors         | `New<Type>`        | `NewJSONStorage()`, `NewRepository()` |

### Types and Interfaces

- Interfaces are small and focused, defined in the package that owns them.
- Structs use JSON struct tags for all serialized fields: `` `json:"field_name"` ``.
- Template data uses `map[string]interface{}` (not typed structs).
- No generics are used in this codebase.
- No custom error types — use `fmt.Errorf` with `%w` wrapping exclusively.

### Error Handling

- Always return `(value, error)` pairs. Check errors immediately after calls.
- Wrap errors with context using `fmt.Errorf("meaningful message: %w", err)`.
- Use `log.Fatalf` only for unrecoverable startup errors in `main()`.
- In HTTP handlers, render user-facing error pages via `renderError()` — never panic.
- Validation errors use plain `fmt.Errorf("description")` without wrapping.

```go
branches, err := repo.GetBranches()
if err != nil {
    s.renderError(w, "Error", fmt.Sprintf("Failed to list branches: %v", err), http.StatusInternalServerError)
    return
}
```

### Comments

- Every exported symbol gets a doc comment starting with the symbol name.
- Multi-line doc comments for functions with non-obvious parameter semantics.
- Inline comments for clarifying non-obvious logic, not for restating code.
- `//go:embed` directives for embedded assets.

```go
// GetDiff returns the diff between two branches
// targetBranch is the base branch (what we're merging INTO, e.g. main)
// sourceBranch is the feature branch (what we're merging FROM, e.g. feature-branch)
func (r *Repository) GetDiff(sourceBranch, targetBranch string) (string, error) {
```

### Function Style

- Use `func` declarations for all named functions (no arrow functions in Go).
- Anonymous closures only for template function maps, sort comparators, and test-swappable globals.
- Method receivers are single-letter, consistent per type (`s *Server`, `r *Repository`).

### File Organization (within a .go file)

1. `package` declaration
2. `import` block
3. Package-level variables / embed directives
4. Type definitions (interface first, then concrete struct)
5. Constructor (`New...`)
6. Exported methods
7. Unexported methods / helpers

### Testing Conventions

- Test files: `*_test.go` alongside source files.
- Use `t.Run("subtest name", ...)` for grouping related test cases.
- Use `t.Helper()` in all test setup/helper functions.
- Use `t.Fatalf` for fatal setup failures, `t.Errorf` for assertion failures.
- Use `t.Cleanup()` for teardown (restoring swapped globals, removing temp dirs).
- Use `t.Skip()` when external tools (like `git`) are unavailable.
- Mock dependencies via interfaces (e.g., `MockStorage` implements `Storage`).
- Use `httptest.NewRequest` + `httptest.NewRecorder` for HTTP handler tests.
- Use `fstest.MapFS` for mocking embedded filesystem templates.
- Swappable package-level `var` for injecting test behavior (e.g., `getTemplateDir`).

### HTTP Routing

- Uses Go 1.22 `ServeMux` with method-based routing: `"GET /compare"`, `"POST /api/review-state"`.
- Handler methods are unexported and follow the pattern `handle<Action>`.
- The `Router()` method returns the configured `*http.ServeMux`.

### Runtime Requirements

- Git 2.30+ must be installed and available in `$PATH`.
- The application stores data at `$HOME/.diffty/`.

## Code Style Rules

### Code Formatting (TypeScript/JavaScript)

- No semicolons (enforced)
- Single quotes (enforced)
- No unnecessary curly braces (enforced)
- 2-space indentation
- Import order: external → internal → types

### Use Context7 MCP for Loading Documentation

Context7 MCP is available to fetch up-to-date documentation with code examples.

**Recommended library IDs**:

- `/golang/go` - Go standard library documentation (net/http, html/template, encoding/json, os/exec, testing, embed, io/fs, etc.)

### Use Serena MCP for Semantic Code Analysis instead of regular code search and editing

Serena MCP is available for advanced code retrieval and editing capabilities.

**When to use Serena:**
- Symbol-based code navigation (find definitions, references, implementations)
- Precise code manipulation in structured codebases
- Prefer symbol-based operations over file-based grep/sed when available

**Key tools:**
- `find_symbol` - Find symbol by name across the codebase
- `find_referencing_symbols` - Find all symbols that reference a given symbol
- `get_symbols_overview` - Get overview of top-level symbols in a file
- `read_file` - Read file content within the project directory

**Usage notes:**
- Memory files can be manually reviewed/edited in `.serena/memories/`

## Use Codemap CLI for Codebase Navigation

Codemap CLI is available for intelligent codebase visualization and navigation.

**Required Usage** - You MUST use `codemap --diff` to research changes different from default branch, and `git diff` + `git status` to research current working state.

### UNSAFE Modes (NEVER use from within OpenCode)

The following modes are interactive/persistent and will capture the terminal, blocking OpenCode's TUI:

- `--watch` — Starts a foreground daemon that holds stdout. Will freeze the OpenCode interface.
- `--skyline` — Renders animated ANSI art directly to the terminal. Corrupts OpenCode's TUI.
- `--skyline --animate` — Same as above, with animation loop.

Only use these modes in a **separate terminal window outside of OpenCode**.

### Safe Modes (use these from within OpenCode)

```bash
codemap .                    # Project tree
codemap --only go .          # Just Go files
codemap --exclude .css,.html . # Hide non-Go assets
codemap --depth 2 .          # Limit depth
codemap --diff               # What changed vs main
codemap --deps .             # Dependency flow
codemap --json .             # JSON output (safe for piping)
codemap --importers <file>   # Impact analysis
```

### Options

| Flag | Description | Safe in OpenCode? |
|------|-------------|-------------------|
| `--depth, -d <n>` | Limit tree depth (0 = unlimited) | Yes |
| `--only <exts>` | Only show files with these extensions | Yes |
| `--exclude <patterns>` | Exclude files matching patterns | Yes |
| `--diff` | Show files changed vs main branch | Yes |
| `--ref <branch>` | Branch to compare against (with --diff) | Yes |
| `--deps` | Dependency flow mode | Yes |
| `--importers <file>` | Check who imports a file | Yes |
| `--json` | Output JSON | Yes |
| `--skyline` | City skyline visualization | **NO** |
| `--watch` | Live file watcher daemon | **NO** |

**Smart pattern matching** - no quotes needed:
- `.png` - any `.png` file
- `Fonts` - any `/Fonts/` directory
- `*Test*` - glob pattern

### Diff Mode

See what you're working on:

```bash
codemap --diff
codemap --diff --ref develop
```
