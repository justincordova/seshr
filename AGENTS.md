# Seshr — Agent Context

AI agent session replay & conversation editor. Bubbletea TUI in Go.
Detects running Claude Code / OpenCode agents, shows live status, groups
messages into topics, supports step-by-step replay, and prunes irrelevant turns.

## Repo layout

```
seshr/
├── cmd/
│   └── seshr/
│       └── main.go              # CLI entry point (Cobra)
├── internal/
│   ├── session/                 # Shared types: Session, Turn, ToolCall, Role, SourceKind
│   ├── backend/                 # Backend abstraction (interfaces + registry)
│   │   ├── backend.go           # SessionStore, LiveDetector, SessionEditor interfaces
│   │   ├── registry.go          # SourceKind → backend mapping
│   │   ├── process.go           # Shared ProcessScanner (ps + cwd lookup)
│   │   ├── selection.go         # Backend selection for a scanned session
│   │   ├── claude/              # Claude Code backend (JSONL)
│   │   └── opencode/            # OpenCode backend (SQLite + per-message parts)
│   ├── config/                  # CLI flags, paths, user config
│   ├── editor/                  # Pruner, backup, lock, pairing logic
│   ├── logging/                 # slog setup, log file management
│   ├── tokenizer/               # Token estimation from content length
│   ├── topics/                  # Source-agnostic topic clustering + labeling
│   ├── tui/                     # Bubbletea TUI (all screens)
│   │   ├── app.go               # Root Bubbletea model
│   │   ├── sessions.go          # Session picker
│   │   ├── session_view.go      # Session detail / topic list
│   │   ├── replay.go            # Step-by-step replay mode
│   │   ├── live_index.go        # Live session detection + refresh
│   │   ├── topics.go            # Topic cluster rendering
│   │   ├── search.go            # Full-text search within a session
│   │   ├── settings.go          # Settings screen
│   │   ├── styles.go            # Lipgloss style definitions
│   │   ├── theme.go             # Theme support
│   │   └── clipboard/           # OS clipboard (darwin + linux)
│   └── version/                 # Version embedding from ldflags
├── testdata/                    # Fixtures: JSONL, SQLite DBs, malformed inputs
├── tests/                       # Integration tests (prune roundtrip, restore)
├── docs/
│   ├── SPEC.md                  # Authoritative product spec
│   ├── TESTING.md               # Testing conventions and patterns
│   ├── LOGGING.md               # Logging conventions (slog, file only)
│   ├── MANUAL_TESTING.md        # Per-phase manual verification checklist
│   ├── designs/                 # In-flight feature design docs (win over SPEC.md when they conflict)
│   └── plans/                   # Phase implementation plans (gitignored)
├── scripts/                     # Build/codegen helpers
├── .golangci.yml                # Linter config
├── justfile                     # Task runner commands
└── go.mod                       # Go 1.26, module github.com/justincordova/seshr
```

## Commands

```bash
just build              # go build -o ./seshr ./cmd/seshr
just test               # go test ./...
just lint               # golangci-lint run
just check              # build → test → lint (pre-commit gate)
just clean              # Remove binary
```

The pre-commit gate is `just check`. All must pass before committing.

## Tech stack

| Layer | Choice |
|---|---|
| Language | Go 1.26 |
| TUI framework | Bubbletea + Lipgloss + Bubbles |
| Markdown rendering | Glamour |
| CLI | Cobra |
| Test assertions | testify (assert/require only) |
| Lint | golangci-lint (errcheck, govet, staticcheck, unused, ineffassign, gocritic) |
| Lint/format | gofmt via golangci-lint formatters |
| Token estimation | Heuristic from content length (no API calls) |
| Clipboard | OS-native (darwin/linux build tags) |

## Key architectural patterns

### Backend abstraction

Each agent platform is a backend that implements `SessionStore`, `LiveDetector`,
and `SessionEditor` (defined in `internal/backend/backend.go`). The TUI never
imports a specific backend directly — it goes through the registry
(`internal/backend/registry.go`). Adding a new agent = adding a new sub-package
under `internal/backend/`.

### Two backends

- **Claude Code** (`internal/backend/claude/`): reads JSONL session files from
  `~/.claude/projects/`. Handles streaming parse, incremental store updates,
  sidecar cursor tracking, and compact boundary detection.
- **OpenCode** (`internal/backend/opencode/`): reads SQLite databases from
  `~/.local/share/opencode/`. Handles branching chains, per-message parts, and
  incremental scanning.

### Live detection

`internal/backend/process.go` runs `ps` to find running agent processes, then
resolves their CWD to match against known session stores. Platform-specific code
lives in `process_darwin.go` and `process_linux.go` (build-tagged).

### Topic clustering

`internal/topics/` is source-agnostic. It receives flat turn lists from any
backend, clusters them by time gaps + content signals, and assigns labels.
TUI renders the same topic UI regardless of source.

### Safe editing (pruner)

`internal/editor/` handles prune operations with backup files, file locks
(`gofrs/flock`), and source-pairing validation. Backups go alongside the
original with a `.bak` suffix. Integration tests in `tests/` verify
roundtrip integrity.

## Key interfaces (`internal/backend/backend.go`)

- `SessionStore` — Scan, Load, Delete sessions from a source
- `LiveDetector` — Detect running agent processes, return live session info
- `SessionEditor` — Prune turns, create backup, restore from backup

## Go conventions

- Go 1.26, target `go 1.26` in `go.mod`
- `internal/` for private packages, no `pkg/`
- Wrap errors: `fmt.Errorf("...: %w", err)`
- Pass `context.Context` first to I/O functions
- Keep interfaces small (1–3 methods), define where consumed
- Platform-specific code uses `//go:build` tags (darwin, linux)

## Non-Negotiables

- **No logging to stdout/stderr.** TUI owns the terminal. Use `slog` to
  `~/.seshr/debug.log` only.
- **Never log raw message content.** Sessions contain user data. Log metadata
  only (counts, IDs, durations).
- **Testify only.** Don't mix assertion styles. Use `assert`/`require`, not
  `t.Error` directly.
- **No third-party logging library.** stdlib `slog` is enough.
- **Edit existing files.** Don't create new docs/READMEs unless asked.
- **Match the spec.** If a change would deviate from `docs/SPEC.md`, update the
  spec in the same change or stop and ask.
- **No commented-out code.** Delete it.

## Scope discipline

- **Claude Code + OpenCode for v1.** Other agent backends are post-v1.
- **No refactoring unrelated code** in a feature change.
- **No premature abstractions.** Three similar lines beats a bad interface.

## Known gotchas

- **TUI tests focus on `Update` logic.** View rendering is validated manually
  (see `docs/MANUAL_TESTING.md`). Don't try to assert on rendered strings.
- **`t.TempDir()` over mocks.** Prefer real files in temp dirs over mocked
  filesystems. Test fixtures go in `testdata/`.
- **`gofrs/flock` for file locks.** The pruner needs exclusive access to session
  files. Don't use `os.Lock` — it's not cross-platform reliable.
- **Platform build tags.** Clipboard and process detection have darwin/linux
  variants. If you add a new OS-level function, add both build-tagged files.
- **SQLite in testdata.** OpenCode test DBs are committed as binary fixtures.
  Don't try to parse them as text.

## Further reading

- **`docs/SPEC.md`** — authoritative product spec (feature behavior, UX flows,
  edge cases). Source of truth for all behavior questions.
- **`docs/TESTING.md`** — test file conventions, AAA pattern, coverage targets,
  fixture usage.
- **`docs/LOGGING.md`** — structured logging patterns, standard keys, log
  levels, what not to log.
- **`docs/MANUAL_TESTING.md`** — per-phase manual verification checklist. TUI
  bugs don't show up in `go test`.
