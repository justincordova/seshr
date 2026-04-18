# Seshr

AI agent session replay & conversation editor. Bubbletea TUI in Go.

## Key Docs

- **[docs/SPEC.md](docs/SPEC.md)** — product spec, source of truth
- **[docs/LOGGING.md](docs/LOGGING.md)** — how to log (stdlib `slog`, file only)
- **[docs/TESTING.md](docs/TESTING.md)** — testify, AAA pattern, coverage targets
- **[docs/MANUAL_TESTING.md](docs/MANUAL_TESTING.md)** — checklist per phase; TUI bugs don't show up in `go test`
- **docs/plans/** — phase implementation plans (gitignored)

## Pre-Commit Gate

Every commit must pass, in order:

```bash
go build ./...
go test ./...
golangci-lint run
```

Fix failures before committing. Do not skip.

## Non-Negotiables

- **No logging to stdout/stderr.** TUI owns the terminal. Use `slog` to `~/.seshr/debug.log` only.
- **Never log raw message content.** Sessions contain user data. Log metadata only.
- **Testify only.** Don't mix assertion styles.
- **No third-party logging library.** stdlib `slog` is enough.
- **Edit existing files.** Don't create new docs/READMEs unless asked.
- **Match the spec.** If a change would deviate from `docs/SPEC.md`, update the spec in the same change or stop and ask.

## Go Conventions

- Go 1.26, target `go 1.26` in `go.mod`
- `internal/` for private packages, no `pkg/`
- Wrap errors: `fmt.Errorf("...: %w", err)`
- Pass `context.Context` first to I/O functions
- Keep interfaces small (1–3 methods), define where consumed

## Scope Discipline

- **Claude Code only for v1.** OpenCode and other parsers are post-v1.
- **No refactoring unrelated code** in a feature change.
- **No premature abstractions.** Three similar lines beats a bad interface.
- **No commented-out code.** Delete it.
