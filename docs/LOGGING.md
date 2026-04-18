# Seshr Logging Guide

## Overview

Seshr uses stdlib `log/slog` for structured file-based logging. The TUI owns the terminal, so **all log output goes to a file — never stdout or stderr**. No third-party logging library is used.

## Default Log File

```
~/.seshr/debug.log
```

Auto-created on first run. Parent directory is created if missing.

## Log Levels

| Level     | Usage                                                                 |
| --------- | --------------------------------------------------------------------- |
| **debug** | Parser internals, clustering decisions, Bubbletea message tracing     |
| **info**  | User-relevant events (session parsed, prune applied, backup restored) |
| **warn**  | Recoverable issues (unknown JSONL record type skipped, malformed row) |
| **error** | Stopping failures (file read error, prune validation failed)          |

`error`-level events should **also** surface in the UI — the log is never the only place the user sees an error.

## CLI Flags

| Flag      | Action                   |
| --------- | ------------------------ |
| `--debug` | Set log level to `debug` |

Default is `info`.

## TUI Log Viewer

Press `L` in the TUI to open the built-in log viewer. Reads from the same file.

## Setup

Create the logger once at startup in `main.go` and install via `slog.SetDefault`. Packages call `slog.Info/Debug/Warn/Error` directly — no per-struct logger fields unless a package genuinely needs scoped attrs.

```go
logPath := filepath.Join(home, ".seshr", "debug.log")
logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
if err != nil {
    return fmt.Errorf("open log: %w", err)
}

level := slog.LevelInfo
if debug {
    level = slog.LevelDebug
}

handler := slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: level})
slog.SetDefault(slog.New(handler))
```

## Structured Fields

Use key/value pairs. Never `fmt.Sprintf` into the message.

```go
slog.Info("parsed session", "path", path, "turns", n, "duration_ms", ms)
slog.Warn("unknown record type", "type", rec.Type, "line", lineNum)
slog.Error("prune failed", "path", path, "err", err)
```

## Standard Keys

Use these consistently so `grep` works across the codebase:

| Key           | Description                              |
| ------------- | ---------------------------------------- |
| `path`        | File path being operated on              |
| `session_id`  | Claude Code session UUID                 |
| `turns`       | Number of turns                          |
| `topics`      | Number of topics                         |
| `tokens`      | Approximate token count                  |
| `duration_ms` | Operation duration in milliseconds       |
| `err`         | Error object (always named `err`, not `error`) |
| `line`        | Line number in a JSONL file              |

## What NOT to Log

- **Raw message content.** Sessions can contain sensitive data from the user's work. Log counts, IDs, sizes — not the text.
- **Secrets, API keys, or file contents.**
- **User-facing errors only to the log.** If the user needs to act, the UI must show it too.

## Example Output

```
2026/04/15 10:30:00 INFO parsed session path=/Users/j/.claude/projects/foo/abc.jsonl turns=34 duration_ms=42
2026/04/15 10:30:01 DEBU clustered topics topics=5 gap_threshold=3m
2026/04/15 10:30:15 WARN unknown record type type=custom_event line=127
2026/04/15 10:30:20 ERRO prune failed path=/Users/j/.claude/projects/foo/abc.jsonl err="unmatched tool_use"
```
