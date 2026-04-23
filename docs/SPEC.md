# Seshr — Product Specification

**AI Agent Session Cockpit & Conversation Editor**
A Bubbletea TUI for understanding, managing, and live-monitoring AI agent conversations

v1.0.0-draft · April 2026

---

## 1. Overview

Seshr is a terminal-based tool built in Go with Bubbletea and Lipgloss that lets developers observe, inspect, and edit AI agent conversation sessions for Claude Code and OpenCode. It detects running agents, shows their live status and current action in a unified picker, and auto-refreshes session views while the agent is working. It groups messages into topics automatically, supports a step-by-step Replay Mode for understanding what happened, and supports pruning irrelevant turns (and tool calls) from session stores.

v1 covers two agent platforms as first-class backends: **Claude Code** (JSONL files) and **OpenCode** (SQLite + per-message parts). Backend abstraction is designed so additional agents (LangChain, Cursor) can be added post-v1 without touching TUI code.

### 1.1 Problem Statement

AI agent sessions accumulate irrelevant context over time. Off-topic tangents, failed tool calls, and exploratory dead ends all consume tokens and degrade agent performance. Developers running agents across multiple projects lose track of which sessions are live, what they're doing, and when context is filling up. There is currently no tool that unifies session observation (what's running now) with session management (what to prune before resuming) in one place. Existing tools either focus on cross-session memory (claude-mem), live output streaming (claude-esp), or process-level monitoring (abtop) — none provide an interactive editor paired with live cockpit visibility.

### 1.2 Target User

Developers who use Claude Code, OpenCode, or both daily and run long or multi-agent workflows where context management matters. They are comfortable in the terminal, likely use tmux, and want a tool that sits in an adjacent pane showing what their agents are doing while they manage older sessions.

### 1.3 Core Value Proposition

- **Unified cockpit:** see all your Claude Code and OpenCode sessions in one picker, with live status (working / waiting), current tool call, and context % at a glance
- **Live refresh:** open a session while the agent is running; new turns, new tool calls, and new topics appear automatically as the agent works
- **Topic visualization:** groups session history into auto-detected topics with token counts — understand what's in a session before you prune
- **Replay Mode:** step-by-step walkthrough of agent behavior, tool calls, and decisions
- **Safe pruning:** remove irrelevant topics to clean session stores before resuming; per-source pairing and backup rules prevent corruption
- **Multi-agent parity:** Claude Code and OpenCode both fully supported — scan, view, live-detect, prune, restore — under one UX
- **Privacy-first, local-only:** no telemetry, no network calls, session content never leaves disk

---

## 2. Architecture

Seshr separates responsibilities into four layers: **session** (shared types), **backend** (per-source Scan/Load/Detect/Edit implementations), **topics** (source-agnostic clustering), and **tui** (Bubbletea rendering + state management). Each backend implements the same interfaces so TUI code is agent-agnostic. Adding a new agent = adding a new backend.

### 2.1 Project Structure

```
seshr/
├── cmd/
│   └── seshr/
│       └── main.go            # CLI entry point (Cobra)
├── internal/
│   ├── session/                    # Shared types (renamed from parser/)
│   │   └── types.go                # Session, Turn, ToolCall, ToolResult,
│   │                                 Role, CompactBoundary, SourceKind
│   ├── backend/                    # Backend abstraction
│   │   ├── backend.go              # SessionStore, LiveDetector, SessionEditor
│   │   │                             interfaces; SessionMeta, LiveSession,
│   │   │                             Cursor types
│   │   ├── registry.go             # SourceKind → backend mapping
│   │   ├── process.go              # Shared ProcessScanner (ps + cwd lookup)
│   │   ├── process_linux.go        # //go:build linux
│   │   ├── process_darwin.go       # //go:build darwin
│   │   ├── claude/
│   │   │   ├── store.go            # SessionStore implementation
│   │   │   ├── jsonl.go            # JSONL decoder (was internal/session/claude.go)
│   │   │   ├── scan.go             # Directory scan
│   │   │   ├── record.go           # JSONL record decoding
│   │   │   ├── sidecar.go          # ~/.claude/sessions/*.json parse
│   │   │   ├── detect.go           # LiveDetector: sidecar + cwd fallback + CPU
│   │   │   ├── detect_linux.go     # //go:build linux — CLAUDE_CONFIG_DIR via /proc
│   │   │   ├── detect_darwin.go    # //go:build darwin — env only
│   │   │   └── cursor.go           # Byte-offset + file-identity cursor
│   │   └── opencode/
│   │       ├── store.go            # SessionStore via modernc.org/sqlite
│   │       ├── db.go               # Read + write connection management
│   │       ├── decode.go           # Walks parent_id; translates parts → Turn
│   │       ├── detect.go           # LiveDetector: cwd inference, ambiguity
│   │       ├── detect_linux.go     # //go:build linux
│   │       ├── detect_darwin.go    # //go:build darwin
│   │       └── cursor.go           # (time_created, id) cursor
│   ├── topics/
│   │   ├── cluster.go              # Topic clustering
│   │   ├── cluster_append.go       # Incremental cluster for live updates
│   │   ├── signals.go              # Clustering signals (time gap, file shift)
│   │   ├── label.go                # Topic label generation
│   │   ├── stopwords.go            # Stopword list
│   │   └── fileset.go              # File set extraction from tool calls
│   ├── editor/
│   │   ├── editor.go               # SessionEditor interface
│   │   ├── selection.go            # Generic selection type
│   │   ├── claude/
│   │   │   ├── pruner.go           # JSONL rewriter
│   │   │   ├── pairing.go          # Claude tool_use ↔ tool_result pairing
│   │   │   ├── backup.go           # .bak sibling file
│   │   │   └── lock.go             # flock advisory lock
│   │   └── opencode/
│   │       ├── pruner.go           # SQL DELETE for partial prune
│   │       ├── deleter.go          # Whole-session delete (picker `d` action)
│   │       ├── backup.go           # JSON export to ~/.seshr/backups/opencode/
│   │       ├── lockfile.go         # Per-session backup-dir flock for retention
│   │       └── retention.go        # Keep last 5 backups per session
│   ├── tokenizer/
│   │   └── estimate.go             # Token estimation
│   ├── config/
│   │   └── config.go               # ~/.seshr/config.json
│   ├── logging/
│   │   └── logging.go              # slog → ~/.seshr/debug.log
│   ├── version/
│   │   └── version.go              # const Version
│   └── tui/
│       ├── app.go                  # Root model, screen routing, ticker owner
│       ├── sessions.go             # Picker: live pulse, source badge, status
│       ├── picker_groups.go        # Project grouping, sort order, stats
│       ├── landing.go              # NEW: per-session summary screen
│       ├── session_view.go         # NEW: *SessionView per-session state
│       ├── live_ticker.go          # NEW: fast (2s) + slow (10s) tick routing
│       ├── resume_overlay.go       # NEW: `c` resume-command overlay
│       ├── topics.go               # Topic overview; consumes *SessionView
│       ├── replay.go               # Replay mode; consumes *SessionView
│       ├── replay_autoplay.go      # Auto-play state machine
│       ├── replay_render.go        # Turn rendering
│       ├── confirm.go              # Confirmation dialogs
│       ├── help.go                 # ? overlay
│       ├── search.go               # / search bar
│       ├── settings.go             # , settings popup
│       ├── logviewer.go            # L log viewer
│       ├── load.go                 # Async load Cmds
│       ├── theme.go                # Color schemes
│       ├── keys.go                 # Keybindings
│       ├── styles.go               # Lipgloss styles
│       └── chrome.go               # Shared layout primitives
├── testdata/
│   ├── simple.jsonl                # Claude simple fixture
│   ├── multi_topic.jsonl           # Claude multi-topic
│   ├── embedded_tool_results.jsonl # Claude embedded tool results
│   ├── compact_boundary.jsonl      # Claude /compact fixture
│   ├── prune_basic.jsonl           # Claude prune pairing
│   ├── replay_basic.jsonl          # Claude replay fixture
│   ├── malformed.jsonl             # Parser resilience
│   ├── claude_live_sidecar.json    # Claude ~/.claude/sessions/*.json sample
│   ├── opencode_simple.db          # OC linear session SQLite fixture
│   ├── opencode_branching.db       # OC session with parent_id branching
│   ├── opencode_with_tools.db      # OC tool parts (all 4 statuses)
│   ├── opencode_compaction.db      # OC compaction part fixture
│   ├── ps_output.txt               # Mocked ps output
│   └── lsof_cwd_output.txt         # Mocked lsof cwd output
├── go.mod
└── go.sum
```

### 2.2 Data Flow

**Cold open (ended session):**
```
pick session → backend.SessionStore.Scan() → SessionMeta list
            → backend.SessionStore.Load()   → *session.Session + Cursor
            → topics.Cluster()              → []Topic
            → tui renders Landing Page      → t/r/c shortcuts
            → Topic Overview / Replay / Resume overlay
```

**Live open (running agent):**
```
process scan → backend.ProcessScanner.Scan()  → ProcessSnapshot (ps+lsof)
            → backend.LiveDetector.DetectLive() → []LiveSession
fast tick  → backend.SessionStore.LoadIncremental(cursor) → new Turns
            → topics.ClusterAppend()          → updated Topics
            → re-render picker row + open view
```

**Prune:**
```
selection → backend.SessionEditor.Prune(selection)
         (Claude: JSONL rewrite + .bak; OC: SQL DELETE in tx + JSON backup)
         → *.bak / ~/.seshr/backups/opencode/<id>/<ts>.json
```

### 2.3 Backend Abstraction

Three interfaces, one per concern:

- `SessionStore` — reads session metadata and turns (Scan, Load, LoadIncremental, LoadRange)
- `LiveDetector` — detects running agent processes and maps them to sessions
- `SessionEditor` — pruning, deletion, and restore for a given source

Claude and OpenCode each implement all three. The TUI only ever sees these interfaces; adding a new agent means writing three implementations and registering them in `backend.Registry`.

`Cursor` is a typed struct (`{Kind, Data []byte}`) carrying opaque source-specific state. The `Kind` field tags which backend owns it, preventing accidental cross-source misuse.

Full interface signatures are in §6.1 Backend Interfaces.

### 2.4 Supporting Types

A few TUI-level types coordinate live-session state across screens.

```go
// internal/backend/process.go

type ProcessSnapshot struct {
    At       time.Time
    ByPID    map[int]ProcInfo
    Children map[int][]int  // ppid → []pid
}

type ProcInfo struct {
    PID     int
    PPID    int
    Command string
    CPU     float64
    RSSKB   int64
    CWD     string // populated lazily, only for agent processes
}

type ProcessScanner struct { /* internals */ }

// Scan runs ps + lsof (or /proc) once and returns a snapshot.
// Called by the slow ticker in the TUI app; shared across detectors.
func (p *ProcessScanner) Scan(ctx context.Context) (ProcessSnapshot, error)
```

```go
// internal/tui/session_view.go

// SessionView holds per-session live state coordinated across screens
// (Landing, Topics, Replay). One source of truth per open session.
type SessionView struct {
    Meta     backend.SessionMeta
    Session  *session.Session
    Topics   []topics.Topic
    Cursor   backend.Cursor
    Live     *backend.LiveSession // nil when not live
    LastTick time.Time
    LastErr  error

    // Bounded-memory window (see §7.5).
    TurnsLoadedFrom int // index of first turn in memory
    TurnsLoadedTo   int // index of last turn in memory (exclusive)
    TotalTurns      int // total turns the session has on disk/DB
}
```

---

## 3. Screens & User Flow

### 3.1 Session Picker

The entry point. On launch, Seshr scans both sources:
- Claude Code: `~/.claude/projects/*/*.jsonl` (and `CLAUDE_CONFIG_DIR` if set)
- OpenCode: `~/.local/share/opencode/opencode.db` (SQLite read-only)

Sessions from both sources are unified into one picker, **grouped by project**. A source badge (`claude` / `opencode`) appears on each row; live sessions float to the top of their group with a pulsing status dot.

```
┌─ ◆ Seshr v1.0 ──────────────────────────────────────────────────────────────┐
│  SESSIONS 12 · LIVE 3 · PROJECTS 7 · TOKENS 381M · SIZE 53 MiB · LATEST now │
│                                                                              │
│  ▌ JUSTIN                                     ▾ 1 session  15.7M tok       │
│  ▌   ● 146a51d6-ade8-…   claude     15.7M  working · Edit auth.go           │
│                                                                              │
│  ▌ BOOT                                       ▾ 1 session  65.8M tok       │
│  ▌   ▸ bb859dee-0744-…   claude     65.8M  ended 2 days ago                 │
│                                                                              │
│  ▌ DARTLY                                     ▾ 2 sessions  25.7M tok      │
│  ▌   ● ses_3df67faf…     opencode    8.2M  waiting · ctx 87% ⚠              │
│  ▌   ▸ 323f0680-89be-…   claude     11.2M  ended 6 days ago                 │
│                                                                              │
│  ↑↓/jk nav · enter open · l live-only · d delete · / search · q quit        │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Row Markers and Columns

| Marker | Meaning |
| --- | --- |
| `▸` | Ended session |
| `●` | Live session (color-coded): green = Working, yellow = Waiting |
| `◌` | Ambiguous-live (multiple OpenCode candidates in same cwd) |

Source badge (`claude` / `opencode`): fixed-width dim column, word-form for scanability.

Right-most column for live rows: `status · current-task-or-context`
- Working: `working · <tool> <arg>` — truncated at 30 chars
- Waiting: `waiting`
- Waiting + context ≥ 80%: `waiting · ctx N% ⚠`
- Working + context ≥ 80%: `working · <task> · ctx N% ⚠`
- Ambiguous: `? live`

Ended rows show the relative timestamp as before.

#### Sort order within each project group

1. Live sessions first: Working → Waiting → Ambiguous
2. Within each status class: most-recent activity first
3. Ended sessions after all live, by `UpdatedAt` descending

#### Stats strip

Top-of-picker aggregate line shows `SESSIONS N · LIVE M · PROJECTS · TOKENS · SIZE · LATEST`. The `LIVE M` field is hidden when zero.

#### Live-detection-unavailable banner

If the shared ProcessScanner fails for 3 consecutive slow ticks (~30s) — typically because `ps` or `lsof` is restricted — a dim line appears below the stats strip:

```
  live detection off · press ? for details
```

Seshr degrades gracefully: all sessions show as ended. Use `--no-live` to opt out explicitly (see §10).

#### Session Picker Keybindings

| Key            | Action                      | Notes                                     |
| -------------- | --------------------------- | ----------------------------------------- |
| `↑/↓` or `j/k` | Navigate session list       | Vim-style                                 |
| `enter`        | Open session                | Live → Landing Page (§3.2); Ended → Topic Overview directly |
| `r`            | Open directly in Replay Mode| Skips landing page + topics               |
| `l`            | Toggle live-only view       | Filters picker to only live sessions      |
| `d`            | Delete session              | Confirmation dialog; ended sessions only  |
| `R`            | Restore from backup         | Only if a backup exists (§4.5)            |
| `/`            | Fuzzy search/filter sessions| Matches project name + session ID         |

Global overlays: `,` settings, `L` log viewer, `?` help, `q` quit.

#### Session Deletion

When `d` is pressed on an ended session, a confirmation dialog warns that deletion cannot be undone.

- **Claude Code:** deletes the `.jsonl` from `~/.claude/projects/<project>/`. Removes `.bak` and `.lock` siblings. Empty project dir is cleaned up.
- **OpenCode:** deletes the session and all its messages/parts in a SQL transaction with `_foreign_keys=on`. Writes a backup before the transaction (same JSON format as prune backup; see §6.3 Pruning).

Live sessions cannot be deleted. The delete confirmation refuses with a message directing the user to close the running agent first.

### 3.2 Session Landing Page

A per-session summary screen, shown when the user presses `enter` on a picker row **for a live session**. Ended sessions skip this screen and go directly to Topic Overview, where the `c` resume keybinding is also available. This keeps ended-session navigation snappy (the most common flow) while preserving a place for live-only telemetry.

> **Future:** see [Session Cockpit](designs/session-cockpit.md) for the planned redesign of this screen into a richer live-telemetry dashboard inspired by [abtop](https://github.com/graykode/abtop). Ships after OpenCode parity (Phases 8–11).

Same layout for live and ended sessions; different data fills it. (Note: the ended-session variant is documented for completeness — when implemented, it would only be reached programmatically, e.g. via the `i` info overlay from a live session that has since ended.)

**Live session:**

```
┌─ ◆ Seshr · Session ─────────────────────────────────────────────────────────┐
│  bb859dee-0744-… · boot · claude · WORKING ●                                 │
│  838 turns · 65.7M tok · 4 compactions · context 85% ⚠                       │
│                                                                              │
│  First prompt:   "help me set up the trading bot"                            │
│  Current action: Edit src/bot/strategy.go · 4s ago                           │
│  Files in play:  strategy.go, config.go, main.go, strategy_test.go, …  (+62) │
│                                                                              │
│  Tokens                                                                      │
│  ████████████████████████████████████████████████ ~65.7M total               │
│  ▌ ~32.9K user  ▌ ~65.0M AI  ▌ ~680K tool results                            │
│                                                                              │
│  t topics  ·  r replay  ·  c resume  ·  esc back                             │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Ended session:**

```
┌─ ◆ Seshr · Session ─────────────────────────────────────────────────────────┐
│  bb859dee-0744-… · boot · claude · ended 2 days ago                          │
│  838 turns · 65.7M tok · 4 compactions                                       │
│                                                                              │
│  First prompt:   "help me set up the trading bot"                            │
│  Last action:    Bash · ended 2 days ago                                     │
│  Files touched:  strategy.go, config.go, main.go, strategy_test.go, …  (+62) │
│                                                                              │
│  Tokens                                                                      │
│  ████████████████████████████████████████████████ ~65.7M total               │
│  ▌ ~32.9K user  ▌ ~65.0M AI  ▌ ~680K tool results                            │
│                                                                              │
│  t topics  ·  r replay  ·  c resume  ·  esc back                             │
└──────────────────────────────────────────────────────────────────────────────┘
```

For OpenCode sessions, total cost is shown alongside tokens (Claude JSONL doesn't record per-message cost):

```
│  838 turns · 65.7M tok · $12.43 · 4 compactions                              │
```

Live-only indicators: pulsing `●` in the header, context-% warning when ≥ 80%, "current action" (vs. "last action") labels.

**Field definitions:**
- *First prompt:* the first user turn's content, truncated to one line.
- *Current action* (live) / *Last action* (ended): the most-recent assistant turn's last `tool_use` / OpenCode tool part, with its first arg (e.g. `Edit src/foo.go`). Truncated at 60 chars on the landing page (vs 30 on the picker).
- *Files in play* / *Files touched*: union of `Topic.FileSet` across all topics in the session, sorted by mention frequency, top 5 shown with `(+N)` overflow indicator. Empty for sessions without tool calls that reference files.
- *Tokens bar:* same proportional split shown in the existing stats panel — user, AI, tool results.

#### Landing Page Keybindings

| Key | Action |
| --- | --- |
| `t` | Topic Overview |
| `r` | Replay Mode |
| `c` | Resume overlay (see §3.3) |
| `i` | Info overlay — full session metadata (first prompt, version, agent/model, etc.) |
| `ctrl+l` | Jump to picker in live-only mode |
| `esc` | Back to picker |
| `/` | Search (delegates to topic overview) |
| `?` | Help |

No `p` (prune) on this screen — pruning requires a selection, which lives in Topic Overview.

### 3.3 Resume Overlay

Pressing `c` on the landing page opens a centered overlay with the command to resume this session in its source agent:

```
┌──────────────────────────────────────────┐
│  Resume this session                     │
│                                          │
│  claude --resume bb859dee-0744-44f1-…    │
│                                          │
│  enter  copy to clipboard                │
│  esc    close                            │
└──────────────────────────────────────────┘
```

Per-source resume commands (verified against installed CLIs):

- **Claude Code:** `claude --resume <session-id>` or `claude -r <session-id>`.
- **OpenCode:** `opencode -s <session-id>`. OpenCode's flag is `-s` / `--session`, NOT `--resume`. Also supports `--fork` for branching.

Clipboard copy uses `pbcopy` / `xclip` / `wl-copy` via platform-gated helpers. After a successful copy, the footer flashes `✓ copied — paste in your terminal` for 2 seconds then reverts. If the clipboard tool is missing, the footer shows `copy unavailable · select and copy manually`.

**Tmux integration (stretch for v1):** if `$TMUX` is set, a second action `s spawn in new tmux window` is added — runs the resume command in a new tmux window without seshr exiting. Hidden when not in tmux.

### 3.4 UX Principles

This section consolidates interaction rules that apply across screens.

#### First-launch welcome

On first launch (detected by absence of `~/.seshr/config.json`), a single-line banner appears above the picker stats strip:

```
  Welcome to seshr. Select a session to open, or press ? for help.
```

Dismisses on any keypress. Never shown after the first launch.

#### Live pulse animation

Live pulse dots animate at 1Hz between `●` and `◉` (same color, subtle shift). Conveys "connection is live" even when no data changes. Stops when a live session becomes Done.

#### Prune confirmation shows every topic

The confirmation dialog lists every selected topic with its token count before deletion, so the user can verify the selection before committing. Same layout across sources; per-source warnings adjust the footer line only.

#### Landing page priority

- **Live sessions:** pulsing status, current action, context warning (if ≥ 80%) are the visual focus. First prompt becomes a secondary detail accessed via `i`.
- **Ended sessions:** first prompt and last action are primary.
- Token bar is always secondary on the landing page (one line). The full segmented bar remains in the stats panel under Topic Overview.

#### Picker scroll preservation

Scroll position and row selection are preserved when navigating back from the landing page via `esc`. Reset only on `q` / app restart or when entering search/filter mode.

#### Live badge on non-picker screens

When the user is on the landing page, topic overview, or replay mode, a compact `· N live` indicator appears at the right edge of the global footer if any live sessions exist. `ctrl+l` jumps back to the picker in live-only mode. Keeps users aware of other agents without cluttering the current view.

#### Landing page action emphasis

The recommended next action (`t`, `r`, or none) is rendered in accent color based on context:

- Ended, never opened → `t topics` emphasized.
- Ended, previously opened (with a recorded action) → `r replay`; footnote shows the last action (e.g. `last time: pruned 2 topics`).
- Live → no emphasis; the page is meant for observation.

#### Live-detection banner (shown selectively)

The "live detection paused" banner appears only when live detection previously worked on this machine and subsequently failed. First-launch on sandboxes / containers silently operates in ended-only mode — no alarming banner. `--no-live` also suppresses the banner. The config tracks last-seen-working state.

### 3.5 Topic Overview (Shared View)

The core screen for inspecting and editing sessions. Displays the parsed session as a list of auto-detected topics, each showing a label, token count, turn range, tool call count, and duration. Selection and pruning are done inline — there is no separate Edit Mode screen.

```
┌─ ◆ Seshr ───────────────────────────────────────────────────────────────────┐
│  TURNS 34 │ TOKENS ~47,231 │ TOPICS 5 │ DURATION 2 hours                    │
│                                                                              │
│  ▌  1. Project setup & Express init                    ~12,400               │
│       turns 1–5 · 8 tool calls · 12 min · 1 week ago                        │
│                                                                              │
│  ▌  2. Authentication with JWT                        ~8,200                 │
│       turns 6–11 · 4 tool calls · 9 min · 6 days ago                        │
│                                                                              │
│  ▌  3. Where to buy a house                           ~2,100                 │
│       turns 12–13 · 0 tool calls · 2 min · 5 days ago                       │
│                                                                              │
│  ↑↓/jk nav · enter expand · f fold · space select · a toggle · p prune      │
│  r replay · tab stats · / search · esc back                                  │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Topic Overview Keybindings

| Key            | Action                      | Notes                                    |
| -------------- | --------------------------- | ---------------------------------------- |
| `↑/↓` or `j/k` | Navigate topics             |                                          |
| `enter` or `→` or `l` | Expand/collapse topic | Shows individual turns within            |
| `f`            | Fold all / unfold all       | Collapses all if any expanded; vice versa |
| `space`        | Toggle topic selection      | Selects all turns in the topic           |
| `a`            | Toggle select all           | Select all if any unselected; deselect all if all selected |
| `p`            | Prune selected topics       | Shows confirmation with token savings    |
| `r`            | Enter Replay Mode           | Starts from selected topic               |
| `c`            | Resume overlay              | Opens the resume command for this session |
| `/`            | Fuzzy search within session | Searches topic labels + turn content     |
| `tab`          | Toggle stats panel          | Right-side aggregate stats               |
| `esc`          | Back to Session Picker      |                                          |

#### Compact Boundary Dividers

When a session contains one or more `/compact` calls, a divider is inserted in the topic list between the last pre-compact topic and the first post-compact topic:

```
  1. Project setup & Express init         ░
     turns 1–5 · 8 tool calls · 12 min

  2. Authentication with JWT              ░
     turns 6–11 · 4 tool calls · 9 min

  ── compacted ─ manual · 141,000 tok · 2m 22s ──────────────

  3. Where to buy a house
     turns 12–13 · 0 tool calls · 2 min
```

Pre-compact topics (those whose turns all precede the earliest boundary) are rendered dimmed with an `░` right-margin indicator. They remain selectable and expandable. The compact divider is styled with the theme accent color. Multiple `/compact` calls each produce their own divider.

#### Stats Panel

When toggled on, the right side shows: total token count, breakdown by role (user turns/tokens, assistant turns/tokens), tool call and tool result counts, number of topics detected, total session duration, and number of unique files touched. If the session has compact boundaries, a `Compactions: N (last: trigger, tok)` line is also shown.

### 3.6 Replay Mode

Split-pane view. Left sidebar shows the topic list with the current position highlighted. Main pane shows the full content of the current turn.

```
┌─ Replay ────────────────────────────────────────────────────────────────────┐
│  Topics         │  Turn 7/34 · ~890 tok                                    │
│─────────────────┼──────────────────────────────────────────────────────────│
│                 │                                                          │
│  1. Setup       │  ● ASSISTANT              +3m 22s                        │
│ ▸ 2. Auth  ◂    │                                                          │
│  3. House       │  I'll add JWT authentication to the                      │
│  4. Rate lim    │  Express app. First, let me install                      │
│  5. Errors      │  the dependency:                                         │
│                 │                                                          │
│                 │  ┌─ Tool: Bash ──────────────────┐                       │
│                 │  │ npm install jsonwebtoken       │                       │
│                 │  └────────────────────────────────┘                       │
│                 │                                                          │
│─────────────────┴──────────────────────────────────────────────────────────│
│  ←→/hl turns · space auto · [/] topics · tab sidebar · t think · c compact │
│  / search · esc back                                                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Turn Display

Each turn in replay shows:

- **Role badge:** Colored label — User (green), Assistant (blue), Tool Use (yellow), Tool Result (dim), Agent (purple)
- **Timestamp delta:** Time elapsed since previous turn
- **Token count:** Approximate tokens for this turn
- **Full message content:** Rendered with glamour for markdown formatting including code blocks
- **Tool calls:** Tool name in a bordered box, input parameters as formatted JSON
- **Tool results:** Truncated to 20 lines by default. Press `enter` on a tool result to expand it in a full-screen viewport. Press `esc` to return.
- **Thinking blocks:** Collapsed by default, toggled with `t`. Rendered in dim text.

#### Slim Mode

Press `c` to toggle slim mode (previously called "compact mode"), which hides non-Agent tool calls and tool results. This lets you focus on the conversation flow without tool noise. An indicator badge (`slim`) appears in the header when active.

#### Pre-compact Badges and Continuation Summary

- **Pre-compact turns:** Each turn whose index falls before the earliest compact boundary shows a dim `pre-compact` label in the turn header.
- **Continuation summary:** The user message that begins with "This session is being continued from a previous conversation…" is rendered collapsed by default with a `continuation summary` badge instead of the normal USER badge. Press `enter` to expand the full summary in a viewport.

#### Sidebar Compact Dividers

The topic sidebar in wide mode inserts an accent-colored horizontal rule between the last pre-compact topic and the first post-compact topic, matching the Topic Overview divider style.

#### Sidebar Focus

Press `tab` to toggle focus between the main content and the topic sidebar. When the sidebar is focused, `↑/↓` navigates the topic list and `enter` jumps to the first turn of that topic.

#### Replay Keybindings

| Key        | Action                 | Notes                                            |
| ---------- | ---------------------- | ------------------------------------------------ |
| `→` or `l` | Next turn              |                                                  |
| `←` or `h` | Previous turn          |                                                  |
| `space`    | Toggle auto-play       | Steps at configurable speed                      |
| `+` / `-`  | Adjust auto-play speed | Only during auto-play                            |
| `]`        | Jump to next topic     |                                                  |
| `[`        | Jump to previous topic |                                                  |
| `t`        | Toggle thinking blocks | Show/hide extended thinking                      |
| `c`        | Toggle slim mode       | Hide non-Agent tool calls/results                |
| `tab`      | Toggle sidebar focus   | Navigate topic list in sidebar                   |
| `enter`    | Expand tool/summary    | Full-screen viewport for tool results or continuation summary |
| `/`        | Search within session  | Shows results panel, jumps on enter              |
| `esc`      | Back to Topic Overview | Or close expanded tool result / search           |

### 3.7 Inline Editing & Pruning

Selection and pruning happen directly on the Topic Overview — there is no separate Edit Mode screen. Users select entire topics to prune from the same screen used for browsing.

```
┌─ ◆ Seshr ───────────────────────────────────────────────────────────────────┐
│  TURNS 34 │ TOKENS ~47,231 │ TOPICS 5 │ DURATION 2 hours                    │
│                                                                              │
│  [ ] 1. Project setup & Express init                    ~12,400               │
│       turns 1–5 · 8 tool calls · 12 min · 1 week ago                        │
│                                                                              │
│  [ ] 2. Authentication with JWT                        ~8,200                 │
│       turns 6–11 · 4 tool calls · 9 min · 6 days ago                        │
│                                                                              │
│  [x] 3. Where to buy a house                           ~2,100                 │
│       turns 12–13 · 0 tool calls · 2 min · 5 days ago                       │
│                                                                              │
│  1 topic selected · ~2,100 tokens freed                                      │
│  ↑↓ nav · space select · a toggle · p prune · esc clear                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

Selection is at the topic level. Individual turn selection within a topic is not supported in v1.

#### Context-Aware Footer

The selection detail panel shows two lines:

1. Count and token breakdown. If the selection includes both pre-compact and active topics: `3 topics selected · ~30,200 tokens freed (~20,600 pre-compact, ~9,600 active)`.
2. Safety indicator:
   - `✓ Safe to prune — not in active context` (all pre-compact)
   - `⚠ Warning: these turns are in the active context` (all active)
   - `⚠ Includes active context turns — requires /clear before resume` (mixed)

#### Prune Confirmation

When `p` is pressed, a context-aware confirmation dialog appears:

**All pre-compact selection:**
```
  Prune 2 pre-compact topics?
  Turns removed: 11 (~20,600 tokens)
  ✓ These are not in the active context and can be safely removed.
  A .bak backup will be created automatically.
```

**Includes active context:**
```
  Prune 3 topics?
  Turns removed: 15 (~30,200 tokens)
  ⚠ ~9,600 of these tokens are in the active context window.
  Type /clear in Claude Code before resuming this session.
  A .bak backup will be created automatically.
```

#### Pruning a Live Session

Live-session pruning policy differs by source:

- **Claude (live):** blocked. JSONL append from a running Claude Code process can race a rewrite and corrupt the file. The dialog refuses with a clear message: "Cannot prune a live Claude Code session. Close Claude Code first."
- **OpenCode (live):** allowed with a warning. SQLite WAL handles concurrent reads/writes safely, but the dialog still alerts the user that very-recent turns may race the prune and that running/pending tool parts are skipped.

#### Concurrent Access (per source)

- **Claude:** the pruner takes an advisory file lock (`flock`) on the target `.jsonl` for the rewrite. Reads do not require a lock. If the lock is held, prune is cancelled with "Session is locked by another process."
- **OpenCode:** the pruner uses a separate writable SQLite connection (DSN `_foreign_keys=on&_busy_timeout=500`) and runs the DELETE in a transaction. If the lock is held by OpenCode beyond 500ms, the operation aborts cleanly and the user can retry.

#### Safe Message Pairing (per source)

Pairing rules differ because the storage models differ:

- **Claude:** `tool_use` blocks live in assistant turns; `tool_result` blocks live in subsequent user turns or separate `tool_result` records, linked by `tool_use_id`. The pruner expands a selection to include the matching half and shows this in the confirmation. User and assistant turns are always deleted as pairs. System messages and compact summaries (`isCompactSummary: true`) are never selectable.
- **OpenCode:** tool calls are atomic — a single `part` row of `type: "tool"` contains both the call (`state.input`) and result (`state.output`). No cross-message pairing logic is needed. However, parts with `state.status` of `running` or `pending` are excluded from prune selections (the agent owns them). System parts (`step-start`, `step-finish`) are never selectable.

#### Backups

- **Claude:** `.bak` sibling file next to the original `.jsonl`. Most-recent backup overwrites prior on each prune.
- **OpenCode:** JSON file at `~/.seshr/backups/opencode/<session-id>/<YYYYMMDD-HHMMSS>.json`. Retention: keep the last 5 backups per session ID; older are deleted on each new prune.

---

## 4. Global Keybindings & Overlays

These keybindings are available on every screen, handled by the root app model.

### 4.1 Help Overlay (`?`)

Pressing `?` on any screen displays a centered overlay showing all keybindings for the current view. Dismisses on any keypress. Built on `bubbles/key` binding definitions, rendered with custom formatting.

```
┌──────────────────────────────────────┐
│         Keyboard Shortcuts           │
│──────────────────────────────────────│
│                                      │
│  Navigation                          │
│  j/k            Move up/down         │
│  enter/→        Open / Expand        │
│  esc            Go back              │
│                                      │
│  Actions                             │
│  t              Topic overview       │
│  r              Replay mode          │
│  c              Resume command       │
│  p              Prune (in topics)    │
│  d              Delete session       │
│  i              Info overlay         │
│  l              Toggle live-only     │
│                                      │
│  Global                              │
│  /              Search               │
│  ctrl+l         Jump to live picker  │
│  ,              Settings             │
│  L              Log viewer           │
│  ?              This help            │
│  q              Quit                 │
│                                      │
│        Press any key to close        │
└──────────────────────────────────────┘
```

### 4.2 Fuzzy Search (`/`)

Pressing `/` on any list screen opens an inline filter bar at the top of the list. As the user types, items are fuzzy-matched in real time and the list filters to show only matching items. Press `esc` to clear the filter and restore the full list.

Uses `github.com/sahilm/fuzzy` for matching. Search targets vary by screen:

- **Session Picker:** Matches against project name and session ID
- **Topic Overview:** Matches against topic labels and turn content
- **Replay Mode:** Matches against turn content, tool call input, and tool result content. Shows a results panel with excerpts; `enter` jumps to the selected turn.

### 4.3 Settings (`,`)

Opens a centered popup showing current configuration values. Editable inline with `enter` or `space` to cycle values. Saves to `~/.seshr/config.json`.

Settings for v1:

| Setting         | Default           | Description                                 |
| --------------- | ----------------- | ------------------------------------------- |
| `theme`         | `catppuccin-mocha`| Color scheme (catppuccin-mocha, nord, dracula) |
| `gap_threshold` | `180`             | Time gap in seconds for topic boundary detection |

**Schema evolution rule:** Unknown fields in the config file are ignored with a `warn` log entry. Missing fields are filled with defaults on load and written back on next save. There is no explicit migration step; adding a field is always backwards-compatible.

### 4.4 Log Viewer (`L`)

Opens a full-screen viewport showing the last 1000 lines of `~/.seshr/debug.log`. Scrollable with `j/k` and `g/G` (top/bottom). Press `esc` or `q` to close. Useful for debugging parser issues or seeing why a session failed to load.

### 4.5 Backup Restore

Every prune operation writes a backup before deleting anything. Restore mechanics differ by source:

- **Claude:** `.bak` sibling next to the original `.jsonl` (e.g. `session.jsonl.bak`). Restore copies the `.bak` over the original. The most-recent backup overwrites prior on each prune.
- **OpenCode:** JSON file at `~/.seshr/backups/opencode/<session-id>/<YYYYMMDD-HHMMSS>.json`. Restore re-INSERTs the rows in a transaction (idempotent: re-inserting present rows is a no-op). Last 5 backups per session are retained; older are pruned on each new backup.

In the Session Picker, sessions with a backup show a small `↶` indicator. Pressing `R` (shift-r) on such a session opens a confirmation dialog and, on confirm, restores from the most-recent backup. Restore is blocked on live sessions for the same reasons pruning is blocked (Claude) or warned (OpenCode).

---

## 5. Topic Clustering Algorithm

Topic clustering is the core intelligence of the tool. It takes a flat list of turns and groups them into logical conversation topics using heuristics (no LLM calls, fully offline).

### 5.1 Clustering Signals

Each signal produces a score between 0 and its weight. The total score is compared against a boundary threshold (default 0.5).

**Compact boundaries (hard split):** If a compact boundary (`/compact` call) falls between two consecutive turns, a topic split is forced unconditionally. This overrides all other signals — no topic may span a compact boundary.

**Time gaps (weight 0.45):** If more than the configured gap threshold elapses between consecutive turns, the time-gap signal fires. This is the strongest signal.

**File context shifts (weight 0.25):** If the set of files referenced in tool calls changes significantly between turns (Jaccard similarity below 0.3 between consecutive file sets), this suggests a topic change.

**Explicit markers (weight 0.15):** User messages containing phrases like "let's move on", "new topic", "switching to", "actually, can you", "switching gears", "change of topic", "different question", "next topic", or "unrelated but" are treated as strong topic boundary signals.

**Keyword divergence (weight 0.15):** Extract keywords from each turn using frequency analysis. If keyword overlap with the previous turn drops below 20%, this contributes to the boundary score. Only used to confirm boundaries suggested by other signals.

### 5.2 Topic Labels

Generated by extracting the top 3 meaningful keywords from the turns in each topic. The first user message content is used as a fallback label if keyword extraction produces nothing useful. Final fallback is "Topic N".

---

## 6. Backend Specification

A backend is a per-source implementation of three interfaces — `SessionStore`, `LiveDetector`, `SessionEditor` — plus a typed `Cursor`. All backends produce/consume the same `session.Session`, `session.Turn`, `session.ToolCall`, `session.ToolResult` shapes; downstream code (`topics`, `tui`) is source-agnostic.

### 6.1 Backend Interfaces

```go
type SessionStore interface {
    Kind() session.SourceKind
    Scan(ctx context.Context) ([]SessionMeta, error)
    Load(ctx context.Context, id string) (*session.Session, Cursor, error)
    LoadIncremental(ctx context.Context, id string, cur Cursor) ([]session.Turn, Cursor, error)
    LoadRange(ctx context.Context, id string, fromIdx, toIdx int) ([]session.Turn, error)
    Close() error
}

type LiveDetector interface {
    Kind() session.SourceKind
    DetectLive(ctx context.Context, procs ProcessSnapshot) ([]LiveSession, error)
}

type SessionEditor interface {
    Kind() session.SourceKind
    Prune(ctx context.Context, id string, sel Selection) error
    Delete(ctx context.Context, id string) error       // whole-session delete (picker `d`)
    RestoreBackup(ctx context.Context, id string) error
    HasBackup(id string) bool
}

type Cursor struct {
    Kind session.SourceKind  // tags which backend owns this cursor
    Data []byte               // source-specific JSON state
}

type SessionMeta struct {
    ID         string
    Kind       session.SourceKind
    Project    string
    Directory  string
    Title      string
    TokenCount int       // 0 if not yet computed; Claude estimates, OC exact
    CostUSD    float64   // OpenCode only; 0 for Claude
    CreatedAt  time.Time
    UpdatedAt  time.Time
    SizeBytes  int64
    HasBackup  bool
}

type LiveSession struct {
    SessionID     string
    Kind          session.SourceKind
    PID           int
    Status        Status            // Working | Waiting
    CurrentTask   string            // ≤ 30 chars on picker, ≤ 60 on landing
    LastActivity  time.Time
    ContextTokens int
    ContextWindow int               // 200_000 or 1_000_000
    Ambiguous     bool              // true for OC cwd with >1 candidate session
}

type Status int
const (
    StatusWaiting Status = iota
    StatusWorking
)
```

The TUI is given `SessionStore` / `LiveDetector` / `SessionEditor` only. It never sees source-specific types. `Cursor` is opaque to the TUI; only the owning backend can interpret `Data`.

### 6.2 Claude Code Backend

#### File Format

Each line in a Claude Code JSONL session file is a JSON object with a `type` field:

| Type          | Description                | Key Fields                                                              |
| ------------- | -------------------------- | ----------------------------------------------------------------------- |
| `user`        | User message               | `message.role`, `message.content`, `timestamp`                          |
| `assistant`   | Claude response            | `message.content` (array of text/tool_use/thinking blocks), `timestamp` |
| `tool_result` | Result of a tool call      | `message.content`, `tool_use_id`                                        |
| `system`      | System/compaction messages | `message.content`, `isCompactSummary`, `subtype`, `compactMetadata`     |
| `summary`     | Session summary            | Summary text, generated asynchronously                                  |

Unknown types (`file-history-snapshot`, `progress`, `hook`, etc.) are logged at warn and skipped.

#### Compact Boundary Records

`system` records with `subtype: "compact_boundary"` mark where a `/compact` call occurred. Parsed into `Session.CompactBoundaries []CompactBoundary` (ordered by position).

User turns whose content starts with `"This session is being continued"` are marked with `Turn.IsCompactContinuation = true` (continuation summaries injected after compaction).

#### Embedded Tool Results

Tool results may appear as top-level records or embedded within assistant content blocks (`type: "tool_result"`). The parser extracts embedded ones and attaches them to the owning assistant turn.

#### Live Detection (two-layer)

1. **Sidecar (primary):** parse `~/.claude/sessions/*.json` (and `CLAUDE_CONFIG_DIR`/`sessions/*.json` on Linux). Each file has `{pid, sessionId, cwd, startedAt}`. Cross-reference with the ProcessSnapshot to filter PIDs running `claude`.
2. **CWD fallback:** for any `claude` process not matched via sidecar, find the most-recently-modified transcript under `~/.claude/projects/<encoded-cwd>/` (modified within last 5 min). Marked live with reduced confidence.

Status derivation: transcript mtime < 30s → Working; else CPU > 1% on the claude process or > 5% on any descendant → Working; else Waiting; PID gone → ended.

CurrentTask: extracted from the latest assistant turn's last `tool_use` block. Truncated at 30 chars; falls back to tool name; falls back to status alone.

#### Incremental Read

Cursor: `{ByteOffset int64, FileIdentity}` where `FileIdentity` is `(inode, mtime_ns)` on Linux and `(mtime_ns, size_bytes)` on macOS (macOS stat doesn't expose inode reliably). Seeks to offset, parses tail. On rotation/truncation (identity mismatch), falls back to full Load.

#### Pruning

Existing JSONL rewriter under `internal/editor/claude/`. Strict pairing rules (see §3.7 Safe Message Pairing). Advisory `flock` for the duration of the rewrite. `.bak` sibling.

### 6.3 OpenCode Backend

#### Storage

OpenCode stores sessions in a SQLite database at `~/.local/share/opencode/opencode.db`. Two main tables:

- `session` — metadata (id, project_id, directory, title, time_created, time_updated, time_archived, time_compacting)
- `message` — envelope rows (id, session_id, time_created, data) keyed to a session, with a JSON `data` payload (role, parentID, modelID, providerID, tokens, cost)
- `part` — content blocks (id, message_id, session_id, time_created, data) where `data.type` is one of `text`, `reasoning`, `tool`, `patch`, `file`, `compaction`, `step-start`, `step-finish`, `agent`, `subtask`

The `project` table provides optional name + worktree path for human display.

#### Connection Strategy

- **Read pool:** read-only DSN (`file:...?mode=ro&_busy_timeout=500`) opened at Store construction. `SetMaxOpenConns(2)` to allow concurrent Scan refresh + Load without serializing every query. WAL mode on the DB makes concurrent reads safe.
- **Write connection:** read-write DSN (`file:...?mode=rw&_foreign_keys=on&_busy_timeout=500`) opened lazily on first Prune. `SetMaxOpenConns(1)` — pruning is serial.
- Both pools use `modernc.org/sqlite` (pure Go, CGO-free, cross-platform).

#### Scanning

```sql
SELECT s.id, s.project_id, s.directory, s.title, s.time_created, s.time_updated,
       p.name AS project_name, p.worktree AS project_worktree
FROM session s
LEFT JOIN project p ON s.project_id = p.id
WHERE s.time_archived IS NULL
ORDER BY s.time_updated DESC;
```

Token and cost totals are aggregated via SQL:
```sql
SELECT session_id,
       SUM(CAST(json_extract(data, '$.tokens.total') AS INTEGER)) AS tokens,
       SUM(CAST(json_extract(data, '$.cost') AS REAL)) AS cost
FROM message
WHERE json_extract(data, '$.role') = 'assistant'
GROUP BY session_id;
```

Project name: `project.name` if set, else last component of `session.directory`.

`session.directory` is the cwd captured when the session was created; `project.worktree` is the canonical project path. They normally match. If they diverge (project moved on disk), prefer `project.worktree` for grouping.

#### Branching and Decode

OpenCode supports message branching via `data.parentID` — when a user edits or regenerates, alternate branches accumulate in the DB (94% of sessions in real-world data). Decode walks the **current branch only** (from the most-recent leaf back to the root), ignoring dormant branches.

Part decode rules:

| OC Part Type | Translates to |
| --- | --- |
| `text` | appended to `Turn.Content` for the owning message |
| `reasoning` | appended to `Turn.Thinking` |
| `tool` (status `completed` / `error`) | emits both `ToolCall` and `ToolResult` on the owning assistant Turn — atomic, not paired across messages |
| `tool` (status `running` / `pending`) | emits `ToolCall` only; result not yet present; excluded from prune selections |
| `patch` | treated like `text` for display |
| `file` | emits a `ToolResult` with content = file path + contents |
| `compaction` | emits a `session.CompactBoundary` at the part's position |
| `step-start` / `step-finish` | ignored (internal framing) |
| `agent` / `subtask` | ignored in v1 (multi-agent subsessions deferred) |

Message `role`: `"user"` → `RoleUser`, `"assistant"` → `RoleAssistant`. Others logged at warn, skipped.

#### Live Detection

For each `opencode` process in the ProcessSnapshot, look up its CWD (via `lsof -p PID -d cwd` on macOS, `/proc/<PID>/cwd` on Linux). Find sessions in the DB matching that CWD with `time_updated` within the last 5 min:

- 0 candidates → process not in a session, skip
- 1 candidate → mark live
- 2+ candidates → mark all candidates live with `Ambiguous=true`; UI renders these as `◌ ? live`

Status derivation mirrors Claude (DB `time_updated` < 30s OR process/descendant CPU above thresholds).

CurrentTask: most-recent `part` of `type=tool` for that session within the last 60s. Cached per session, only re-queried when `session.time_updated` advances.

#### Incremental Read

Cursor: `{LastTimeCreated int64, LastMessageID string}`.

```sql
SELECT id, message_id, time_created, data
FROM part
WHERE session_id = ?
  AND (time_created > ? OR (time_created = ? AND id > ?))
ORDER BY time_created, id
LIMIT 1000;
```

Owning messages are fetched and decoded with branch-walking. If the new branch leaf is not a descendant of the prior cursor, the backend triggers a full Load to recover correctness.

#### Pruning

```
1. Resolve selection to message IDs (and optional individual part IDs in v1.1).
2. Filter out tool parts whose state.status is `running` or `pending`.
3. Export to-be-deleted rows to ~/.seshr/backups/opencode/<id>/<ts>.json
4. PRAGMA foreign_keys = ON;          -- set on writable connection
   BEGIN IMMEDIATE;                    -- reserves write lock; readers continue
   DELETE FROM part WHERE id IN (...); -- belt + suspenders (cascade would handle this)
   DELETE FROM message WHERE id IN (...);
   COMMIT;
5. Backup retention: keep last 5 per session ID; older deleted (under per-session lockfile).
```

Live OpenCode pruning is allowed with a warning dialog (see §3.7).

#### Restore

Read the most-recent backup file for the session ID. Re-INSERT messages and parts in a single transaction with `INSERT OR IGNORE` (idempotent).

#### Whole-Session Delete

Distinct from prune. The picker `d` action removes an entire session. Live sessions are blocked (same policy as prune).

```
1. Write a full-session backup to
   ~/.seshr/backups/opencode/<id>/<ts>-delete.json
   (same format as prune backup; includes ALL messages and parts)
2. PRAGMA foreign_keys = ON;
   BEGIN IMMEDIATE;
   DELETE FROM session WHERE id = ?;  -- cascade drops messages and parts
   COMMIT;
3. Apply backup retention (last 5 per session).
```

Restore from a `*-delete.json` backup re-INSERTs the session row first, then messages, then parts. Idempotent like prune restore.

### 6.4 Adding a Future Backend

A new backend implements `SessionStore`, `LiveDetector`, `SessionEditor`, plus a `Cursor` shape, then registers itself in `internal/backend/registry.go` against a new `session.SourceKind`. The TUI does not change. Topic clustering does not change. Editor pairing rules remain per-source (Claude has them; OpenCode does not).

---

## 7. Live Sessions

Live-session detection and refresh is what makes seshr a cockpit rather than a post-hoc viewer. The same screens render live and ended sessions; what changes is the data refresh cadence and the indicators shown.

### 7.1 Process Scanning

A shared `backend.ProcessScanner` runs once per slow tick (every 10s) and produces a `ProcessSnapshot{ByPID, Children, At}`. Per slow tick it does:

1. **Process list:** `ps -ww -eo pid,ppid,rss,%cpu,command` — one subprocess invocation, returns the full table.
2. **CWD lookup (only for agent processes):** matches argv tokens against `claude` / `opencode`, then for each match runs `lsof -p PID -d cwd` (macOS) or reads `/proc/<PID>/cwd` (Linux). Typically 1–5 lookups per tick.

Each `LiveDetector` consumes the same snapshot and filters for its agent. Total cost per slow tick on a busy machine: ~30ms.

If `ProcessScanner.Scan` fails 3 consecutive ticks (~30s), seshr surfaces a `live detection off` banner in the picker header and degrades to ended-only display. The `--no-live` CLI flag (see §10) disables scanning explicitly.

### 7.2 Tickers

Two tickers in the TUI app:

| Ticker | Cadence | Active when | Purpose |
| --- | --- | --- | --- |
| Slow | 10s | always | Refresh ProcessSnapshot, run all LiveDetectors, reconcile live set, apply hysteresis |
| Fast | 2s | any live session known | Pull `LoadIncremental` on every live session, refresh CurrentTask, append to viewed session if open |

**Cold-start latency:** on launch, no live sessions are known yet → fast tick is suspended → first slow tick fires after 10s and discovers any live sessions. Picker initially shows everything as ended; live pulses light up on the first slow-tick result. To shorten this, the slow tick fires once immediately on launch (not waiting the full 10s for the first cycle).

Both tickers suspend while help / settings / log viewer overlays are open to avoid rerender storms under modals.

### 7.3 Status Hysteresis

To prevent flicker at tool-call boundaries (when CPU briefly idles between calls):

- **Upgrade** Waiting → Working: instant, on first signal
- **Downgrade** Working → Waiting: only after 2 consecutive slow ticks (~20s) without any "working" signal

### 7.4 Live View Refresh

When the user opens a live session, the landing page subscribes to fast-tick refresh for that session ID. Each tick:

1. `backend.SessionStore.LoadIncremental(ctx, id, cursor)` fetches new turns since the last cursor
2. New turns appended to `view.Session.Turns`; `view.Cursor` updated
3. `topics.ClusterAppend(view.Session, view.Topics, newTurns)` extends the topic list — the last topic grows or a new topic opens at a boundary; historical topics never re-cluster
4. UI rerenders

If the session ends mid-view (process gone), `view.Live` becomes nil; the screen continues to function as an ended-session view.

### 7.5 Bounded Memory Window

Long-lived live sessions accumulate turns indefinitely. Policy:

- Keep last 500 turns in `view.Session.Turns` at all times.
- When `LoadIncremental` would push past 500, oldest turns are evicted. `view.TurnsLoadedFrom` advances.
- If the user scrolls/jumps to an evicted range in replay, `backend.SessionStore.LoadRange(ctx, id, fromIdx, toIdx)` loads the requested window on demand.
- Topics are never evicted — only per-turn content. Topic-level navigation is unaffected because topics hold turn indices, not turn pointers.

Default 500 (≈ 40MB at the largest real-world session size) is conservative; configurable via `config.max_turns_in_memory` if needed.

### 7.6 Pruning Live Sessions

See §3.7 for full policy. Summary:
- Claude live: blocked (JSONL append + rewrite race risk)
- OpenCode live: allowed with warning (SQLite WAL handles concurrency safely)

### 7.7 Shutdown

On `ctrl+c` / `tea.Quit`:
1. App cancels the global `context.Context`
2. In-flight backend operations receive `ctx.Done()` and return
3. All `Store.Close()` called in parallel with a 500ms timeout
4. Log file flushed and closed
5. Process exits

No goroutine may outlive the app context.

---

## 8. Token Estimation

Seshr estimates token counts for display purposes using a character-based heuristic: divide rune count by 3.5 for English text. This gives an approximation within 10-15% of actual Claude tokenization. All token counts are prefixed with `~` in the UI to indicate they are approximate.

**Per source:**
- **Claude:** if `message.usage` fields (`input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`) are present, the parser prefers those over the heuristic.
- **OpenCode:** every assistant message records exact `tokens.total` (and a breakdown of input/output/reasoning/cache). Aggregated via SQL during Scan; no heuristic needed.

OpenCode also records exact `cost` per message; cumulative cost is shown on the OpenCode landing page (Claude JSONL doesn't expose this).

---

## 9. Color Scheme

### 9.1 Default Theme: Catppuccin Mocha

Seshr uses Catppuccin Mocha as the default color scheme. Three themes are available: `catppuccin-mocha` (default), `nord`, and `dracula`. All colors are defined using `lipgloss.AdaptiveColor` so they degrade gracefully on light terminal backgrounds.

The `Theme` struct holds: Background, Foreground, Accent, Muted, Error, plus role badge colors (UserColor, AssistantColor, ToolUseColor, ToolResultColor, AgentColor), palette entries for project gutters (ProjectPalette), and overlay/surface colors for UI elements.

### 9.2 Theme Switching

Themes are selectable via the settings popup (`,`) or `--theme` CLI flag. The active theme is stored in `~/.seshr/config.json`.

### 9.3 Style Constants

Base styles are defined in `internal/tui/styles.go` using the Catppuccin Mocha palette. Key styles:

- **Box borders:** Rounded border style (`lipgloss.RoundedBorder()`) using Surface1 color
- **Role badges:** Colored foreground with bold text
- **Selected row:** Project-colored gutter with bold/bright text
- **Footer:** Dim text for descriptions, accent-colored keys
- **Compact divider:** Accent (`Theme.Accent`) colored horizontal rule, rendered between pre-compact and post-compact topics in the Topic Overview and Replay sidebar
- **Pre-compact indicator:** `░` marker and dimmed text (`colSurface1` / `dimStyle`) on pre-compact topic cards and turn headers

---

## 10. Responsive Layout

The TUI must adapt to different terminal sizes. Use `tea.WindowSizeMsg` to detect the terminal dimensions and adjust layout accordingly.

**Minimum size:** 60 columns × 15 rows. Below this, show a "terminal too small" message.

**Replay split pane:** The topic sidebar takes 20% of width (minimum 16 columns, maximum 24 columns). The main pane takes the remainder. If the terminal is narrower than 80 columns, hide the sidebar and show topics as a header bar instead.

**Long content:** Use `bubbles/viewport` for scrollable content in the replay main pane and log viewer. Word wrapping based on available width.

**Loading states:** Large sessions take time to parse. Show a `bubbles/spinner` with "Parsing session..." while loading.

---

## 11. Error UX Standard

Errors are surfaced inline within the current screen:

- **Delete errors:** Shown as a red error line below the session list in the picker. Auto-clears.
- **Prune errors:** Shown in the confirmation dialog or as inline status text.
- **Session load errors:** Full-screen error state with the error message and an `esc` to go back prompt.
- **Live detection failures:** Persistent dim banner in the picker header (`live detection off · press ? for details`) after 3 consecutive failed slow ticks. See §3.1 and §7.1.
- **Live refresh failures (per session):** Logged at warn; the next slow tick triggers a full Load to recover. UI shows the last successfully-loaded turns; no error overlay during transient failures.
- **Log correlation:** Every displayed error writes a matching `error`-level slog entry with the same message and an `err` field.

---

## 12. Privacy & Telemetry

Seshr collects **no telemetry**. No analytics, no crash reporting, no network calls, no update pings. The only network-capable dependency is `glamour` (for rendering images in markdown, which is disabled). If any future feature would phone home, it is opt-in and documented in this section before landing.

Session content never leaves disk. All processing is local. The log file at `~/.seshr/debug.log` contains metadata only — see LOGGING.md for the "no raw content" rule.

---

## 13. CLI Specification

Seshr uses Cobra for CLI argument parsing.

| Command / Flag    | Description                                         | Default            |
| ----------------- | --------------------------------------------------- | ------------------ |
| `seshr`           | Launch TUI with session picker                      | Scans default dirs |
| `--dir <path>`    | Scan a custom directory for Claude sessions         | Auto-detected      |
| `--theme <name>`  | Color theme                                         | `catppuccin-mocha` |
| `--debug`         | Enable debug logging                                | `false`            |
| `--no-live`       | Disable live detection; all sessions show as ended  | `false`            |
| `--version`       | Print version and exit                              |                    |

`--no-live` is for restricted environments where `ps` / `lsof` aren't available (sandboxes, locked-down hosts) or when the user simply doesn't want background process scanning.

---

## 14. Tech Stack & Go Best Practices

### 14.1 Go Version

Seshr targets **Go 1.26** (latest stable, released February 2026). The `go.mod` file specifies `go 1.26`.

### 14.2 Dependencies

| Package                           | Purpose                                                             |
| --------------------------------- | ------------------------------------------------------------------- |
| `charmbracelet/bubbletea`         | TUI framework, application model and event loop                     |
| `charmbracelet/lipgloss`          | Terminal styling, colors, borders, layout                           |
| `charmbracelet/bubbles`           | Pre-built components: viewport, textinput, spinner, key             |
| `charmbracelet/glamour`           | Markdown rendering for assistant responses in replay view           |
| `github.com/spf13/cobra`          | CLI argument parsing and subcommands                                |
| `github.com/stretchr/testify`     | Testing: `assert`, `require` packages — standard everywhere         |
| `log/slog` (stdlib)               | Structured logging to file (TUI owns stdout)                        |
| `github.com/sahilm/fuzzy`         | Fuzzy string matching for `/` search                                |
| `github.com/dustin/go-humanize`   | Human-friendly formatting: "2h ago", "47k", "1.2 MB"                |
| `github.com/gofrs/flock`          | Advisory file locking during Claude prune                           |
| `modernc.org/sqlite`              | Pure-Go SQLite driver for OpenCode database access (no CGO)         |

`modernc.org/sqlite` adds ~12MB to the binary (15MB → ~27MB total). Accepted as the cost of cross-platform Go binaries without a C toolchain dependency. CGO-based alternatives (`mattn/go-sqlite3`) would be smaller but break goreleaser's static cross-compilation.

**Explicitly not used:** no third-party logging library (stdlib `log/slog` is sufficient), no YAML (config is JSON). Clipboard for the resume overlay shells out to `pbcopy` / `xclip` / `wl-copy` rather than pulling a clipboard library.

### 14.3 Logging

**Library choice:** Use stdlib `log/slog` only. No third-party logging library (zap, zerolog, logrus). slog covers structured logging, levels, and handlers; adding another library would be dead weight for a tool this size.

**Conventions:**

- **Destination:** Always `~/.seshr/debug.log`. Never stdout/stderr — the TUI owns the terminal.
- **Levels:** `info` by default, `debug` when `--debug` is passed. `warn` for recoverable parser issues (unknown JSONL types, malformed records skipped). `error` for failures the user should see (file read errors, prune validation failures) — these also surface in the UI, never only in the log.
- **Structured fields:** Use key/value pairs, not formatted strings. Prefer `slog.Info("parsed session", "path", p, "turns", n)` over `slog.Info(fmt.Sprintf(...))`.
- **Standard keys:** `path` (file path), `session_id`, `turns`, `topics`, `duration_ms`, `err`. Keep keys consistent across the codebase so log grep works.
- **No secrets or full message content:** log metadata (turn counts, IDs, sizes), not the raw conversation. Session content can include sensitive data from the user's work.

### 14.4 Testing

**Framework:** `github.com/stretchr/testify` is the project's testing library. All tests use it — do not mix in other assertion libraries or write raw `if got != want { t.Fatalf(...) }` style checks.

**Conventions:**

- `testify/require` for assertions that must pass before continuing (fail-fast: parse succeeded, file exists, no error returned).
- `testify/assert` for non-critical checks where the test should keep running to surface multiple failures.
- Table-driven tests for parser and clustering logic — each case gets a `name` field used as the subtest name.
- Test files sit next to the code they test (`claude_test.go` beside `claude.go`).
- `testdata/` holds sample JSONL fixtures. Fixtures are checked in, not generated.
- Run `just test` before any commit (see CLAUDE.md pre-commit gate).

**Test fixtures added for v1:**

- `testdata/opencode_simple.db` — linear OpenCode session (2 sessions, ~20 messages)
- `testdata/opencode_branching.db` — session with `parent_id` branching
- `testdata/opencode_with_tools.db` — tool parts in all 4 statuses (completed, error, running, pending)
- `testdata/opencode_compaction.db` — session with `compaction` part
- `testdata/claude_live_sidecar.json` — sample `~/.claude/sessions/<uuid>.json`
- `testdata/ps_output.txt` — mocked `ps -ww -eo pid,ppid,rss,%cpu,command` output
- `testdata/lsof_cwd_output.txt` — mocked `lsof -p PID -d cwd` output

**Per-package test coverage expectations for v1:**

| Package | Key test areas |
|---|---|
| `backend/claude/` | Sidecar decode round-trip; `LoadIncremental` appends correctly; reset on rotation; `LoadRange` returns correct slice; `DetectLive` layers 1 & 2; platform build tags compile on darwin + linux |
| `backend/opencode/` | Scan across linear/branching/compaction fixtures; `Load` walks current branch; `LoadIncremental` handles branch-change edge case; `LoadRange` window; part-type decode rules; `DetectLive` ambiguity (≥2 candidates → `Ambiguous=true`); status derivation thresholds |
| `backend/process.go` | `ps` output parsing; children map; CWD lookup gated by platform |
| `editor/claude/` | Prune round-trip on paired tool_use/tool_result; pairing under orphan selection; backup/restore round-trip |
| `editor/opencode/` | Prune round-trip with no FK orphans; skip running/pending tool parts; backup format + restore idempotence; retention (6th prune deletes oldest backup); `foreign_keys=ON` verified |
| `tui/` | Picker row golden tests (ended/working/waiting/ambiguous); cross-source mixed rendering; Landing page rendering (live + ended, with/without cost); resume overlay command format per source; clipboard helper invocation + "copy unavailable"; `SessionView` bounded-memory eviction + `LoadRange` trigger; hysteresis (2-tick downgrade, instant upgrade); live-detection-paused banner appears after 3 failures, clears on recovery |

**Integration tests** (`tests/` package):
- Unified picker: Claude + OpenCode fixtures both appear
- Live → ended transition: simulate process exit; verify pulse → arrow
- Prune blocked on Claude live
- Prune allowed on OpenCode live

**Coverage targets:**
- New `backend/` and `editor/opencode/` packages: ≥ 80%
- Modified `tui/landing.go`, `session_view.go`: ≥ 70%
- No regression on existing package coverage

### 14.5 Go Best Practices

- **Project layout:** Use `internal/` for all private packages. No `pkg/` directory.
- **Error handling:** Wrap errors with `fmt.Errorf` and `%w`. Define sentinel errors at package level. Never panic in library code.
- **Interfaces:** Define where consumed, not where implemented. Keep small (1-3 methods).
- **Context:** Pass `context.Context` as first param for I/O functions. Use for graceful TUI shutdown.
- **Naming:** MixedCaps, acronyms all caps (ID, URL). Short lowercase package names.
- **Concurrency:** Use Bubbletea `Cmd` for async operations (parsing, file I/O). The TUI event loop is single-threaded.
- **Linting:** Run `golangci-lint` with gocritic, errcheck, govet enabled.
- **Platform gates:** Use `//go:build linux` / `//go:build darwin` for OS-specific code (process scanning, cwd lookup). Never use `//go:build !windows` style negation; be explicit about supported platforms.
- **Build:** Use `goreleaser` for cross-platform builds: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64. Windows targets are disabled in v1.

---

## 15. Implementation Order

Each phase builds clean (`just check` passes), delivers incremental value, and is a separate commit (or a small sequence of related commits). Phases have stable boundaries so `just check` + manual-testing checklists apply cleanly per phase.

1. **Rename `internal/session/` → `internal/session/`.** Mechanical. All imports updated. No behavior change.
2. **Introduce `internal/backend/` package** with `SessionStore`, `LiveDetector`, `SessionEditor` interfaces. Claude implementations are thin wrappers over existing code. TUI still calls them directly (interfaces not yet consumed).
3. **Shared `ProcessScanner`** with platform gates (`process_linux.go` + `process_darwin.go`).
4. **Claude `LiveDetector`** (layer 1 sidecar + layer 2 cwd fallback) + `--no-live` CLI flag. Picker rows start showing live pulse + source badge.
5. **Claude `LoadIncremental` and `LoadRange`** + `SessionView` with bounded-memory window.
6. **Fast/slow tickers** with hysteresis. Live-detection-paused banner.
7. **Landing page.** Intermediate screen; `c` resume overlay with platform-gated clipboard helper.
8. **OpenCode `SessionStore`** (Scan with backup discovery + token aggregate, Load with parent_id branch walk, decode).
9. **OpenCode `LiveDetector`** (cwd inference, ambiguity handling, `◌ ? live` rendering).
10. **OpenCode `LoadIncremental` and `LoadRange`.**
11. **OpenCode `SessionEditor`** (prune, whole-session delete, backup, retention with per-session lockfile, restore).
12. **Polish + docs.** MANUAL_TESTING.md additions; README.md update; OpenCode resume command verification.

Phases 1–7 deliver full Claude live experience. Phases 8–11 deliver full OpenCode parity. Phase 12 closes v1.

---

## 16. Non-Findings (Considered and Declined)

Intentionally not addressed in v1. Recorded here so future-reviewers know the decision was deliberate rather than an oversight.

- **ProcessScanner caching / diffing.** At ≤ 5 agent processes and ~20ms per `ps` + `lsof` invocation, caching saves nothing meaningful. Reconsider if benchmarks show scan > 100ms.
- **CPU-independent live status.** The 3-signal model (transcript mtime < 30s, process CPU > 1%, descendant CPU > 5%) is empirically proven via abtop on thousands of users. IO-bound agents show CPU on the parent during network calls; truly idle agents are correctly classified Waiting.
- **Cursor persistence across restarts.** Cold-start full reload is fast (~300ms on the largest real session). Persisting cursors adds versioning + staleness handling without proportionate benefit.
- **Per-source configuration toggles.** Flat config stays flat in v1. Schema is forward-compatible (unknown keys ignored) so per-source settings can be added later without a migration.
- **Model / provider display.** OpenCode exposes `modelID`/`providerID`; Claude exposes `version`. Surfacing these in the UI competes for screen space with higher-priority information. Accessible via log and `i` info overlay (landing page) if needed.

---

## 17. Future Features (Post-Launch)

These are explicitly out of scope for v1 but are natural extensions:

- **Suggestions engine:** proactive nudges on the landing page ("prune this cold topic", "consider /compact at 85%", "duplicate Bash calls detected").
- **Autopsy view:** retrospective analysis of failed tool calls, loops, files touched unnecessarily.
- **Sparklines and token charts:** visual time series of token growth, turn rate, context evolution.
- **All-live dashboard:** abtop-style grid showing every live session at once (currently surfaced as picker rows + `l` filter).
- **Per-source settings:** enable/disable each source independently; custom paths for non-standard installs.
- **Additional backends:** LangChain traces, Cursor conversation logs, generic JSONL agent logs.
- **OpenCode multi-agent subsessions:** decode `agent` and `subtask` part types as nested sub-sessions in the topic view.
- **OpenCode alternate-branch display:** surface dormant branches (regenerated responses) in the replay view.
- **Claude continuation-chain reconstruction:** stitch multi-file Claude sessions linked by compaction continuation summaries.
- **Claude subagent transcripts:** sessions spawned by the Task tool live at `~/.claude/projects/<p>/<session>/subagents/*.jsonl`. Currently invisible to seshr; future work would surface them nested under their parent session.
- **Individual turn / part selection for pruning:** more granular than topic-level (currently topic-level only).
- **Cursor persistence across restarts:** save where the user left off in a live session.
- **Notifications:** OS notification when a Waiting OpenCode/Claude session needs the user's input.
- **tmux pane-jumping:** `enter` on a live row jumps to its tmux pane (abtop-style).
- **Session comparison / diff:** side-by-side compare of two sessions.
- **Export:** generate clean markdown or HTML reports for documentation or sharing.
- **Word wrap toggle in replay:** toggle between wrapped and horizontal-scroll display.
- **Windows support:** currently disabled in goreleaser; bubbletea + sqlite both work on Windows but live detection (ps/lsof) does not.

---

## 18. Risks & Mitigations

**High-priority risks** (data loss, broken sessions, blocking failures): Claude JSONL format changes, OpenCode schema changes, invalid JSONL after pruning, OpenCode FK orphans, accidental deletion, OpenCode branch detection picks wrong leaf.

**Medium-priority** (UX degradation, perf): SQLite write lock, sidecar absent, ps/lsof unavailable, ambiguous OpenCode cwd, JSONL rotation mid-session, memory growth, scan slow at scale.

**Low-priority** (cosmetic, recoverable): backup disk bloat, status flicker, Charm import migration.

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Claude JSONL format changes | Parser breaks, sessions fail to load | Pin to known types; ignore unknown with warn; monitor Claude Code changelogs |
| OpenCode SQLite schema changes | Scan/decode breaks | Fixture-based schema test as canary; pin tested OpenCode version in README |
| Large session files (100k+ lines) | Slow parsing, high memory | Stream-parse JSONL; spinner during parse; bounded-memory window in live view (§7.5) with on-demand `LoadRange` |
| Accidental deletion of important sessions | Data loss | Confirmation dialog; backup before any write (`.bak` for Claude, JSON for OpenCode) |
| Invalid JSONL after Claude pruning | Session cannot be resumed | Strict pairing rules; validate output before writing; always keep `.bak` |
| OpenCode FK orphans after pruning | Inconsistent DB | `_foreign_keys=on` in writable connection; explicit `DELETE FROM part` first as belt-and-suspenders |
| SQLite write lock contention during prune | Prune fails | 500ms `_busy_timeout`; user-visible retry message; abort cleanly |
| Claude sidecar files absent or removed | Live detection degrades | Layer-2 cwd-fallback detection (§6.2); still works without sidecars |
| `ps` / `lsof` unavailable | No live detection | Persistent banner in picker; `--no-live` flag; UX still usable for ended sessions |
| Ambiguous OpenCode cwd (multiple agents same dir) | Wrong "live" marker | "? live" hollow-dot rendering; don't pick one |
| Claude JSONL rotation mid-session | Incremental parse misses turns | File identity check every tick; reset cursor on mismatch |
| OpenCode branch detection picks wrong leaf | Wrong conversation displayed | Walk from most-recent leaf by `MAX(time_created)`; regression test on branching fixture |
| Long-lived live session memory growth | RAM pressure | 500-turn window + on-demand `LoadRange` |
| OpenCode scan slow at scale (1000+ sessions) | Laggy launch | Benchmark first; cache aggregates in `~/.seshr/opencode_meta.db` if needed |
| Backup file disk bloat | Disk pressure | Retention: keep last 5 per session ID for OpenCode; Claude keeps 1 `.bak` |
| Topic clustering produces poor groupings | Confusing UI | Configurable `gap_threshold`; manual boundary insertion in future version |
| Pulse / status flicker | Jarring UX | Downgrade hysteresis: Working → Waiting requires 2 consecutive slow ticks |
| Charm library import path migration | Build failures | Pin exact versions in go.mod; document import paths used |
| Goroutine leaks on shutdown | Zombie processes | Every backend exposes `Close()`; app shutdown waits bounded; context cancellation everywhere |
| OpenCode binary version drift (resume command changes) | Resume overlay shows wrong command | Verify `--resume` flag at implementation time; document tested OC version; fall back to "show ID only" if flag absent |
