# Seshr Testing Guide

## Overview

1. **Testify framework** — `github.com/stretchr/testify` for all assertions. No mixing styles.
2. **AAA pattern** — Arrange–Act–Assert with section comments.
3. **Pre-commit gate** — `go build ./... && go test ./... && golangci-lint run` must pass.
4. **Tests alongside code** — `*_test.go` next to the source file.
5. **No mocks for things you can use directly** — prefer real files in `t.TempDir()` over mocked filesystems, real fixtures over synthetic data.

## Coverage Targets

| Package              | Target |
| -------------------- | ------ |
| `internal/parser`    | 90%    |
| `internal/topics`    | 85%    |
| `internal/editor`    | 90%    |
| `internal/tokenizer` | 85%    |
| `internal/config`    | 80%    |
| `internal/tui`       | 60%    |

TUI coverage is lower by design — view rendering is validated manually. Focus tests on `Update` logic and keymap handling.

## Project Layout

```
seshr/
├── main.go
├── internal/
│   ├── parser/
│   │   ├── claude.go           → claude_test.go
│   │   └── types.go
│   ├── topics/
│   │   └── cluster.go          → cluster_test.go
│   ├── editor/
│   │   └── pruner.go           → pruner_test.go
│   ├── tokenizer/
│   │   └── estimate.go         → estimate_test.go
│   ├── config/
│   │   └── config.go           → config_test.go
│   └── tui/
│       ├── app.go              → app_test.go
│       └── ...
├── testdata/
│   ├── simple.jsonl
│   ├── multi_topic.jsonl
│   └── chained.jsonl
└── tests/
    └── integration_test.go
```

## AAA Pattern

```go
func TestClaudeParser_SimpleSession_ReturnsAllTurns(t *testing.T) {
    // Arrange
    p := parser.NewClaude()

    // Act
    session, err := p.Parse("testdata/simple.jsonl")

    // Assert
    require.NoError(t, err)
    require.Len(t, session.Turns, 4)
    assert.Equal(t, "user", session.Turns[0].Role)
}
```

## Testify Usage

- `require` when a failure must stop the test (setup, prerequisites, "parse must succeed before we check turns").
- `assert` for independent checks where failing one shouldn't hide the others.

```go
require.NoError(t, err, "parse must succeed")
assert.Equal(t, expected, got.TokenCount)
assert.Len(t, got.Topics, 3)
```

## Test Naming

```
Test<Function>_<Scenario>_<ExpectedResult>
```

Examples:

- `TestClaudeParser_SimpleSession_ReturnsAllTurns`
- `TestCluster_ThreeMinuteGap_CreatesBoundary`
- `TestPruner_UnpairedToolUse_ExpandsSelection`
- `TestPruner_ValidInput_CreatesBackupFile`

## Table-Driven Tests

Use for parser variants and clustering heuristics. `name` is the subtest name:

```go
func TestCluster_GapDetection(t *testing.T) {
    tests := []struct {
        name        string
        gapSeconds  int
        wantNewTopic bool
    }{
        {"no gap", 0, false},
        {"small gap", 30, false},
        {"exactly threshold", 180, false},
        {"over threshold", 181, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := cluster.IsBoundary(tt.gapSeconds)
            assert.Equal(t, tt.wantNewTopic, got)
        })
    }
}
```

## TUI Model Tests

Test Bubbletea models by feeding messages into `Update` and asserting on state:

```go
func TestSessionPicker_DownKey_MovesSelection(t *testing.T) {
    // Arrange
    m := tui.NewSessionPicker(fixtureSessions())

    // Act
    next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

    // Assert
    sp := next.(tui.SessionPicker)
    assert.Equal(t, 1, sp.Cursor())
}
```

Do not assert on rendered strings except for narrow snapshot tests — `View()` changes too often.

## Fixtures

- `testdata/` holds real JSONL samples (scrubbed of any sensitive content).
- Fixtures are checked in, not generated at test time.
- Name fixtures by the scenario they cover: `simple.jsonl`, `multi_topic.jsonl`, `chained.jsonl`, `unpaired_tool.jsonl`.

## Test Isolation

- Use `t.TempDir()` for any filesystem work — auto-cleanup, no cross-test pollution.
- Mark helpers with `t.Helper()`.
- No shared package-level state between tests. No ordering dependencies.

## Running Tests

```bash
go test ./...                                    # all tests
go test ./internal/parser/... -v                 # specific package
go test -race ./...                              # race detection
go test ./... -coverprofile=cover.out            # coverage
go tool cover -func=cover.out                    # coverage summary
```

## Integration Tests

`tests/integration_test.go` covers end-to-end flows:

- Parse a session → cluster topics → prune a topic → re-parse and verify structure.
- Delete session → verify file and empty project dir are removed.
- Restore from `.bak` → verify original content is back.

## Pre-Commit Workflow

Every commit must pass, in order:

```bash
go build ./...
go test ./...
golangci-lint run
```

If any step fails, fix it — don't commit broken code. See CLAUDE.md.

## Checklist

- [ ] AAA pattern with section comments
- [ ] Testify `require`/`assert` used appropriately
- [ ] Happy path, error path, and edge cases covered
- [ ] `t.TempDir()` for filesystem isolation
- [ ] Helpers marked with `t.Helper()`
- [ ] Coverage meets package target
- [ ] `go build ./... && go test ./... && golangci-lint run` passes
