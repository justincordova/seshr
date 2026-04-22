# Live Sessions and Multi-Agent Support — Design

Status: design draft (revised after data verification)
Date: 2026-04-20
Scope: v1 feature set expansion

## Summary

Seshr today is a post-hoc Claude Code session editor. This design adds two
capabilities that transform it into a live cockpit for AI agent sessions:

1. **Live session support** — detect running Claude Code and OpenCode agents,
   show live status inline on the picker, auto-refresh session views while
   the agent is working.
2. **Multi-agent support** — unify Claude Code and OpenCode under one picker
   and landing page. Both sources are first-class in v1 (scan, view, prune,
   restore).

Existing functionality (topic overview, replay, prune, backup/restore) is
preserved and extended. No UI fork between live and ended sessions — same
screens, different data.

This design has been verified against real OpenCode data (1290 sessions,
39k messages on the author's machine). Assumptions that did not survive
contact with data have been corrected.

---

## 1. Architecture

### 1.1 Package Layout

```
internal/
├── session/                    ← RENAMED from parser. Shared types only.
│   └── types.go                ← Session, Turn, ToolCall, ToolResult, Role,
│                                 CompactBoundary, SourceKind
├── backend/                    ← NEW. Backend abstraction (was 'source').
│   ├── backend.go              ← SessionStore + LiveDetector interfaces,
│   │                             SessionMeta, LiveSession, Cursor types
│   ├── registry.go             ← Registers Claude and OpenCode backends
│   ├── process.go              ← Shared ProcessScanner (ps once per slow tick)
│   ├── process_linux.go        ← //go:build linux — /proc/<pid>/cwd,environ
│   ├── process_darwin.go       ← //go:build darwin — lsof -p PID -d cwd
│   ├── claude/
│   │   ├── store.go            ← SessionStore impl
│   │   ├── jsonl.go            ← JSONL decoder (was internal/parser/claude.go)
│   │   ├── scan.go             ← directory scan (was internal/parser/scan.go)
│   │   ├── record.go           ← JSONL record types (was internal/parser/record.go)
│   │   ├── sidecar.go          ← parse ~/.claude/sessions/*.json
│   │   ├── detect.go           ← LiveDetector (sidecar + cwd fallback + CPU)
│   │   └── cursor.go           ← byte offset + file identity
│   └── opencode/
│       ├── store.go            ← SessionStore impl via SQLite
│       ├── db.go               ← read and write connection management
│       ├── decode.go           ← walks parent_id chain; translates parts
│       │                         into session.Turn / ToolCall / ToolResult
│       ├── detect.go           ← LiveDetector (cwd inference)
│       └── cursor.go           ← (time_created, id) cursor
├── topics/                     ← UNCHANGED. Works on session.Session.
├── tokenizer/                  ← UNCHANGED.
├── editor/                     ← RESTRUCTURED per source.
│   ├── editor.go               ← SessionEditor interface
│   ├── selection.go            ← generic selection type
│   ├── claude/
│   │   ├── pruner.go           ← JSONL rewriter (existing logic)
│   │   ├── pairing.go          ← MOVED from editor/. Claude-only.
│   │   ├── backup.go           ← .bak sibling file
│   │   └── lock.go             ← flock on .jsonl
│   └── opencode/
│       ├── pruner.go           ← SQL DELETE for partial prune (selected msgs)
│       ├── deleter.go          ← Whole-session delete (for picker `d` action);
│       │                         deletes session row + cascades to messages/parts
│       ├── backup.go           ← JSON export to ~/.seshr/backups/opencode/
│       ├── lockfile.go         ← Per-session backup-dir lockfile for retention
│       └── retention.go        ← keep last 5 backups per session
├── config/                     ← UNCHANGED.
├── logging/                    ← UNCHANGED.
├── version/                    ← UNCHANGED.
└── tui/
    ├── app.go                  ← owns tickers + ProcessScanner
    ├── sessions.go             ← picker w/ live pulse, source badge, status
    ├── landing.go              ← NEW. Per-session summary screen.
    ├── session_view.go         ← NEW. *SessionView per-session state holder.
    ├── live_ticker.go          ← NEW. Fast + slow tick routing.
    ├── resume_overlay.go       ← NEW. `c` on landing = show resume command.
    ├── topics.go               ← modified to consume *SessionView
    ├── replay.go               ← modified to consume *SessionView
    └── (other existing files unchanged)

testdata/
├── (all existing Claude fixtures)
├── opencode_simple.db          ← NEW. Small fixture, 2 linear sessions
├── opencode_branching.db       ← NEW. Fixture with parent_id branching
├── opencode_with_tools.db      ← NEW. Fixture with tool parts (completed,
│                                     error, running statuses)
├── opencode_compaction.db      ← NEW. Fixture with compaction parts
├── claude_live_sidecar.json    ← NEW. Sample sidecar JSON
├── ps_output.txt               ← NEW. Mocked ps output
└── lsof_cwd_output.txt         ← NEW. Mocked lsof cwd output
```

**Rationale for renaming `parser` → `session` (package) and `source` →
`backend` (package):**

- `parser` implied behavior; after the refactor it only holds shared types.
  `session` reflects the actual content.
- `source` was overloaded. `session.SourceKind` is a discriminator tag;
  `backend.SessionStore` is an implementation of that tag. Renaming one
  (the package) to `backend` disambiguates. `session.SourceKind` is the
  discriminator with values `SourceClaude` and `SourceOpenCode`.

### 1.2 Core Types

```go
// internal/session/types.go — shared types only

type SourceKind string

const (
    SourceClaude   SourceKind = "Claude Code"
    SourceOpenCode SourceKind = "OpenCode"
)

// Session, Turn, ToolCall, ToolResult, Role, CompactBoundary
// continue as today. No structural changes.

// internal/backend/backend.go

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
    RestoreBackup(ctx context.Context, id string) error
    HasBackup(id string) bool
}

type SessionMeta struct {
    ID         string
    Kind       session.SourceKind
    Project    string
    Directory  string
    Title      string
    TokenCount int         // 0 if not yet computed; Claude estimates, OC sums
    CostUSD    float64     // OpenCode only; 0 for Claude (no cost data)
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
    CurrentTask   string            // ≤ 30 chars, may be empty
    LastActivity  time.Time
    ContextTokens int
    ContextWindow int               // 200_000 or 1_000_000
    Ambiguous     bool              // true for OC cwd-matched with >1 candidate
}

type Status int
const (
    StatusWaiting Status = iota
    StatusWorking
)

// Cursor is an opaque bookmark for incremental reads, typed by source.
type Cursor struct {
    Kind session.SourceKind
    Data []byte // source-specific, JSON-encoded
}
```

**Why split Store / Detector / Editor:** three orthogonal failure modes.
Live detection can fail (no ps/lsof) without Store breaking. Editor
(writes) can be blocked (live session, lock) without affecting reads.
Each platform-gated file can be isolated.

**Why `Cursor` is a typed struct:** the `Kind` field tags which backend
owns it, preventing accidental cross-source use. `Data` is JSON-encoded
state internal to each backend (byte-offset for Claude, `(time_created,
id)` for OpenCode).

### 1.3 Shared Process Scanner

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

type ProcessScanner struct { /* ... */ }

// Scan runs ps + (lazily) lsof/proc CWD lookups and returns a snapshot.
// Called by the slow ticker in the TUI app.
func (p *ProcessScanner) Scan(ctx context.Context) (ProcessSnapshot, error)
```

Running `ps` once per slow tick regardless of source count avoids
per-detector duplication. Each `LiveDetector.DetectLive` receives the
snapshot and filters. CWD is populated only for processes matching
`claude` or `opencode` in the first two argv tokens.

### 1.4 Session View

```go
// internal/tui/session_view.go

type SessionView struct {
    Meta    backend.SessionMeta
    Session *session.Session
    Topics  []topics.Topic
    Cursor  backend.Cursor
    Live    *backend.LiveSession // nil when not live
    LastTick time.Time
    LastErr  error

    // Bounded-memory window (see §2.6)
    TurnsLoadedFrom int  // index of first turn in memory
    TurnsLoadedTo   int  // index of last turn in memory (exclusive)
    TotalTurns      int  // total turns the session has on disk/DB
}
```

Landing, Topics, Replay all hold `*SessionView`, not duplicated state.

---

## 2. Data Flow

### 2.1 Cold Open

```
user presses enter on a picker row
      │
      ▼
app dispatches openSessionCmd(meta SessionMeta)
      │
      ▼
store := registry.Store(meta.Kind)
session, cursor, err := store.Load(ctx, meta.ID)
      │
      ▼
topics := topics.Cluster(session)
      │
      ▼
view := &SessionView{Meta, Session, Topics, Cursor, TurnsLoadedFrom: 0,
                     TurnsLoadedTo: len(session.Turns),
                     TotalTurns: len(session.Turns)}
      │
      ▼
if live := liveIndex.Lookup(meta.ID); live != nil:
    view.Live = live
    liveTicker.ensureFastTick()
      │
      ▼
push LandingModel(view)
```

### 2.2 Fast Refresh Tick (2s)

Active whenever any live session is known to exist (not only when viewing
one). This keeps picker `CurrentTask` responsive.

```
tea.Tick fires → liveRefreshMsg
      │
      ▼
for each live session in liveIndex:
    store := registry.Store(live.Kind)
    newTurns, newCursor, err := store.LoadIncremental(ctx, id, view.Cursor)
    if err: log warn; continue
    if len(newTurns) == 0: update live.LastActivity from mtime/DB; continue

    view := openSessionViews[id] // nil if not currently open
    if view != nil:
        view.Session.Turns = append(view.Session.Turns, newTurns...)
        view.TurnsLoadedTo += len(newTurns)
        view.TotalTurns += len(newTurns)
        view.Cursor = newCursor
        view.Topics = topics.ClusterAppend(view.Session, view.Topics, newTurns)

    // Update the shared live cache so picker reflects new CurrentTask
    liveIndex.Update(id, computeLiveFromTail(newTurns))
      │
      ▼
trigger rerender
```

If no live sessions exist, the fast tick suspends itself until the slow
tick discovers new ones.

### 2.3 Slow Refresh Tick (10s)

```
tea.Tick fires → liveSlowRefreshMsg
      │
      ▼
snapshot, err := processScanner.Scan(ctx)
if err: liveIndex.MarkDetectionFailed(); show banner
      │
      ▼
for each LiveDetector in registry:
    lives, err := detector.DetectLive(ctx, snapshot)
    // err is logged; other detectors continue

merged := merge(lives...)
transitions := liveIndex.Reconcile(merged)
      │
      ▼
for each new/ended transition:
    if viewing an affected session, invalidate view.Live accordingly
    mark picker row for rerender
      │
      ▼
if len(merged) > 0: liveTicker.ensureFastTick()
if len(merged) == 0: liveTicker.suspendFastTick()
```

### 2.4 Status Hysteresis

Applied inside `liveIndex.Reconcile`:

- Upgrade `Waiting → Working`: instant, on first signal.
- Downgrade `Working → Waiting`: only after 2 consecutive slow ticks
  without any "working" signal (~20s).

Prevents flicker during tool-call boundaries when CPU briefly idles.

### 2.5 Incremental Cluster Update

`topics.ClusterAppend(sess, existing, newTurns)` contract:

- Re-evaluates boundaries only between `existing[-1].lastTurn` and
  `newTurns[0]`, and within `newTurns`.
- Never re-opens earlier topics.
- If a backend-provided `CompactBoundary` falls within `newTurns`, a new
  topic is opened at that boundary.

Invariant: `ClusterAppend` called N times with single-turn slices produces
the same result as `Cluster` called once on the concatenated session, for
sessions without late-arriving compact boundaries. Enforced by a
regression test.

### 2.6 Bounded-Memory Window

Long-lived live sessions grow unboundedly. Policy:

- Keep last 500 turns in `view.Session.Turns` at all times.
- When `LoadIncremental` would push past 500, the oldest turns are evicted
  from memory. `TurnsLoadedFrom` advances.
- If the user scrolls/jumps to an evicted range in replay:
  - `backend.SessionStore.LoadRange(ctx, id, fromIdx, toIdx)` loads the
    requested window.
  - Loaded window replaces the in-memory slice; `TurnsLoadedFrom/To`
    updated.
- Topic-level navigation is unaffected because topics hold turn indices,
  not turn pointers.
- Topics themselves are never evicted — only the per-turn content.

Default 500 is conservative (≈ 40MB at boot-session scale). Configurable
via `config.max_turns_in_memory` as a hidden v1 setting if needed.

### 2.7 Error Handling

| Failure | Behavior |
|---|---|
| Store.Scan fails for one source | Log warn. Empty for that source. Other sources still shown. |
| Store.Load fails | Full-screen error state with `esc to back`. Log error. |
| Store.LoadIncremental fails | Log warn. Next tick does full Load for that session to recover. |
| Store.LoadRange fails | Show "failed to load range" in replay; user can navigate elsewhere. |
| LiveDetector fails | Log warn. Sessions from that source show as ended. No UI break. |
| ProcessScanner fails | "live detection off" banner in picker header. Retry each slow tick. |
| SQLite locked (OC) | Retry once with 100ms backoff. Skip this tick if still locked. |
| OC prune transaction conflicts with OpenCode writing | 500ms busy_timeout; show "session busy, try again" to user. |
| Context canceled | Clean shutdown path; see §7.6. |

---

## 3. UI

### 3.1 Session Picker

**Ended session row:**
```
▌   ▸ bb859dee-0744-…      claude     15.7M    2 minutes ago
```

**Live working (Claude):**
```
▌   ● bb859dee-0744-…      claude     15.7M    working · Edit strategy.go
```

**Live waiting (OpenCode):**
```
▌   ● ses_3df67faf…        opencode    8.2M     waiting
```

**Live waiting near context limit:**
```
▌   ● ses_3df67faf…        opencode    8.2M     waiting · ctx 87% ⚠
```

**Ambiguous OC (multiple candidates in same cwd):**
```
▌   ◌ ses_3df67faf…        opencode    8.2M     ? live
▌   ◌ ses_a1b2c3d4…        opencode    7.9M     ? live
```

Rules:

- Arrow `▸` is the ended marker. Dot `●` is live (color-coded:
  green = Working, yellow = Waiting). Hollow dot `◌` is ambiguous-live.
- Source badge (`claude` / `opencode`) in a fixed-width column, dim.
  Word-form (not abbreviations) for scanability. No color competition
  with the project gutter.
- Right-most column for live rows: `status · current-task-or-context`.
  - Working: `working · <current-task>` truncated at 30 chars
  - Waiting + context < 80%: `waiting`
  - Waiting + context ≥ 80%: `waiting · ctx N% ⚠`
  - Working + context ≥ 80%: `working · <task> · ctx N% ⚠`
- Current-task fallback chain: full string (e.g. "Edit strategy.go") →
  tool name only (e.g. "Edit") → status alone.
- Ambiguous rows: no confident pulse; `? live` indicator; tooltip
  "multiple agents in this directory — live detection ambiguous."

**Sort order:**

Project groups are sorted by:
1. Groups containing any live session first (project currently in use
   appears at top of the picker).
2. Within that: most-recent `UpdatedAt` across the group's sessions.
3. Fully-ended groups after, by most-recent `UpdatedAt` descending.

Within each project group:
1. Live sessions first: Working → Waiting → Ambiguous.
2. Within each status class: most-recent activity first (`LastActivity`
   or `UpdatedAt`).
3. Ended sessions after all live, by `UpdatedAt` descending.

**Stats strip top of picker:**
```
SESSIONS 12 · LIVE 3 · PROJECTS 7 · TOKENS 381M · SIZE 53 MiB · LATEST now
```

`LIVE N` hidden when zero. `TOKENS` counts are exact for OpenCode, estimates
for Claude — no UI distinction (both are already prefixed with `~` in the
detail views).

**Live-detection-unavailable banner:**
Shown only when live detection *previously* worked on this machine (config
remembers last-seen-working), then started failing. The config flag
`seshr.live_detection_last_ok` is written on first successful scan.

After 3 consecutive failed slow ticks (≈30s):
```
  live detection paused · press ? for details
```
`?` overlay explains the specific failure (e.g. "ps command not found",
"permission denied on /proc"). Banner disappears once scanning recovers.

On first launch with no prior success recorded (sandbox, container,
restricted env), no banner — seshr silently operates in ended-only
mode. If the user explicitly runs `seshr --no-live`, no banner either.

**Keybindings (new/changed):**
- `l` (lowercase): toggle live-only view (filters picker).
- `L` (uppercase): log viewer (unchanged).
- `enter`: open landing page (was: open topic overview).

**OpenCode scale handling:**
- `Scan()` is indexed SQL — 1290 sessions returns in tens of ms.
- Picker renders up to 50 sessions per project group by default.
- Groups with > 50 sessions show `show all (N)` on expand.

**Responsive row layout:**

| Terminal width | Row format |
|---|---|
| ≥ 100 cols | Full row: `marker id badge tokens status·task` |
| 80–99 cols | Drop badge column; source encoded in gutter color (claude = theme Blue, opencode = theme Peach) |
| < 80 cols | Additionally truncate CurrentTask to 20 chars |

The source-in-gutter-color at narrow widths reuses the per-project gutter
slot with a source-blended color (alpha-blend the project color with the
source's signature hue). Users keep both dimensions of information even
when a dedicated badge column doesn't fit.

**Empty state** (both sources return zero sessions):
```
  No sessions found.

  Seshr looks for sessions in:
    ~/.claude/projects/              (Claude Code)
    ~/.local/share/opencode/         (OpenCode)

  Install one of these tools and run it once, then relaunch seshr.

    q  quit
```

### 3.2 Session Landing Page

New intermediate screen. Enter on picker → landing. `t` on landing →
topic overview.

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

For OpenCode sessions, the `~65.7M tok` line also shows total cost:
```
│  838 turns · 65.7M tok · $12.43 · 4 compactions                              │
```
(Cost omitted for Claude since Claude Code JSONL doesn't record per-message cost.)

**Keybindings (landing page):**

| Key | Action |
|---|---|
| `t` | Topic Overview |
| `r` | Replay Mode |
| `c` | Resume overlay (see §3.3) |
| `i` | Info overlay: full session metadata (first prompt, version, model usage, etc.) |
| `ctrl+l` | Jump to picker in live-only mode |
| `esc` | Back to picker |
| `/` | Search (delegates to topic overview) |
| `?` | Help |

### 3.3 Resume Overlay

Pressing `c` on the landing page opens a centered overlay:

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

- **Claude Code:** `claude --resume <session-id>` (equivalent: `claude -r <session-id>`).
  Claude uses UUID-format session IDs.
- **OpenCode:** `opencode -s <session-id>` (flag is `-s`/`--session`, NOT `--resume`).
  Also supports `--fork` to branch from the session.

Both CLIs must be on PATH for the command to execute. The overlay shows
the command regardless and attempts copy-to-clipboard.

Clipboard copy shells out via a small platform-gated helper:
`pbcopy` on macOS, `xclip -selection clipboard` (with `wl-copy` fallback
for Wayland) on Linux. No new Go dependency.

**Post-copy feedback:** after a successful copy, the overlay's
`enter copy to clipboard` footer briefly flashes `✓ copied — paste in
your terminal` for 2 seconds, then reverts. If copy fails (tool missing
or error), the footer shows `copy unavailable · select and copy manually`
persistently.

**Tmux integration (stretch for v1, otherwise v1.1):** if `$TMUX` is set,
the overlay adds a second action `s  spawn in new tmux window` that runs
the resume command in a new tmux window without seshr exiting. For
non-tmux users this action is hidden.

### 3.4 Pruning a Live Session

**Claude live session:** blocked. Dialog:
```
  Cannot prune a live Claude Code session.

  This session is being written to by PID 12345.
  Pruning JSONL while Claude Code appends to it can corrupt the file.

  Close Claude Code first, then try again.

    [ OK ]
```

**OpenCode live session:** allowed with warning. Dialog:
```
  ⚠ This session is live.

  OpenCode (PID 12345) may write new messages during pruning.
  SQLite handles this safely, but:
    - Very-recent turns may race the prune and not be removed.
    - If the prune times out waiting for a write lock, it aborts
      cleanly and you can retry.

  A backup is written to ~/.seshr/backups/opencode/ before deletion.

    enter  prune anyway    esc  cancel
```

**Running tool parts (OpenCode only):** pruning logic excludes any
`part.data.type == "tool" AND state.status in ("running", "pending")`.
These belong to the agent and may be mutated while we hold the selection.
A pre-check warns if the user's selection included such parts and shows:
`N tool calls skipped (still running)`.

### 3.5 UX Principles

This section codifies the interaction rules that cut across screens.

#### 3.5.1 First-launch welcome

On first launch (detected by absence of `~/.seshr/config.json`), a
dismissable banner appears above the picker stats strip:

```
  Welcome to seshr. Select a session to open, or press ? for help.
```

Dismisses on any keypress. Never shown after the first launch.

#### 3.5.2 Live pulse animation

Live pulse dots in picker and landing page animate at 1Hz between
`●` and `◉` (subtle size change, same color). Conveys "connection is
live" even when no data has changed. Stops immediately when a live
session becomes Done.

#### 3.5.3 Prune confirmation shows what will be deleted

Pre-deletion dialog lists every selected topic so the user can verify
before committing. Example:

```
  Prune 3 topics?

  1. Authentication with JWT              ~8,200
  2. Where to buy a house                 ~2,100
  3. Database migration tangent           ~9,800
                                      ~20,100 tokens freed

  A backup will be saved to ~/.seshr/backups/ before deletion.

  enter  prune   esc  cancel
```

The live-session warning (Claude blocked / OpenCode allowed) replaces
the "prune" / "cancel" footer line with the appropriate per-source
copy — the body (topic list + token total) stays identical across
sources and states.

#### 3.5.4 Landing page: live sessions emphasize NOW, ended emphasize HISTORY

For live sessions, visual priority in the header is:
1. Pulsing status indicator (`●` + text)
2. Current action (what the agent is doing right now)
3. Context % warning if ≥ 80%

For ended sessions, visual priority is:
1. "First prompt" (what the user asked for)
2. "Last action" (where the session ended)

On live sessions, "First prompt" becomes a secondary detail accessed
by pressing `i` (info overlay). This preserves screen real-estate for
real-time information without losing the field.

#### 3.5.5 Token bar is secondary on the landing page

Collapsed to a single line on the landing page:
```
Tokens: ████ ~65.7M · ~32.9K user · ~65M AI · ~680K tool
```

The full three-line breakdown with segmented bar remains in the
existing stats panel (`tab` from Topic Overview).

#### 3.5.6 Picker scroll position preserved across navigation

When the user opens a session via `enter` and returns via `esc`, the
picker's scroll position and row selection are preserved. Position is
reset only on `q` / app restart or after the user explicitly enters
search/filter mode.

#### 3.5.7 Live badge on non-picker screens

When the user is on any screen other than the picker, a compact
indicator appears at the right edge of the global footer:

```
                                                              · 3 live
```

Visible only when live session count > 0. Clicking is a no-op in a TUI;
pressing `ctrl+l` jumps back to the picker filtered to live-only
(`l` mode active). Avoids losing awareness of other live sessions
while viewing a specific one.

#### 3.5.8 Landing page action emphasis

The recommended next action is rendered in the accent color; others
remain dim. Rules:

| State | Emphasized |
|---|---|
| Ended session, never opened before | `t topics` |
| Ended session, opened before | `r replay` (with small footnote `last time: pruned 2 topics` if applicable) |
| Live session | none emphasized (user is meant to observe) |

---

## 4. OpenCode Backend Details

### 4.1 Data Access

- Library: `modernc.org/sqlite` (pure Go, no CGO, cross-platform).
- Two connections per Store:
  1. **Read connection:** DSN `file:...?mode=ro&_busy_timeout=500`.
     Shared by Scan/Load/LoadIncremental/LoadRange.
  2. **Write connection:** lazily opened on first Prune. DSN
     `file:...?_foreign_keys=on&_busy_timeout=500`. Held until Close().
- `SetMaxOpenConns(1)` on each connection, `SetConnMaxLifetime(0)`.
- DB file never rotates during seshr's lifetime — held open for process life.

### 4.2 Scan

```sql
-- One query for session metadata:
SELECT s.id, s.project_id, s.directory, s.title, s.time_created, s.time_updated,
       p.name AS project_name, p.worktree AS project_worktree
FROM session s
LEFT JOIN project p ON s.project_id = p.id
WHERE s.time_archived IS NULL
ORDER BY s.time_updated DESC;

-- One query for aggregate token/cost totals:
SELECT session_id,
       SUM(CAST(json_extract(data, '$.tokens.total') AS INTEGER)) AS tokens,
       SUM(CAST(json_extract(data, '$.cost') AS REAL)) AS cost
FROM message
WHERE json_extract(data, '$.role') = 'assistant'
GROUP BY session_id;
```

**Benchmark-first approach:** measure scan time on 1290-session fixture.
If < 500ms: ship as-is. If ≥ 500ms: add a cache in
`~/.seshr/opencode_meta.db` (session_id → tokens, cost, last computed
time_updated). Cache populated lazily after each Load; Scan reads cache
and only re-aggregates sessions with stale `time_updated`.

Project name: prefer `p.name` when set, else last component of
`s.directory`.

**Backup discovery:** during Scan, the OC store also stats
`~/.seshr/backups/opencode/<session-id>/` for each session ID. If the
directory exists with at least one `*.json` backup, `SessionMeta.HasBackup`
is set to true. Cheap (one stat per session). Same mechanism the Claude
store uses for `*.jsonl.bak` siblings.

### 4.3 Load

Walks the current branch of the session.

```go
func (s *Store) Load(ctx, id) (*session.Session, Cursor, error) {
    // 1. Fetch all messages for session, keyed by id.
    msgs := queryAllMessages(ctx, id)  // SELECT * FROM message WHERE session_id=?

    // 2. Find leaf: the most-recent message with no children.
    leaf := findLeaf(msgs)

    // 3. Walk parent_id chain from leaf to root.
    chain := walkParents(msgs, leaf.id)

    // 4. For each message in chain, load its parts.
    parts := queryPartsForMessages(ctx, id, chain.ids)
        // SELECT * FROM part WHERE session_id=? AND message_id IN (...)
        // ORDER BY time_created, id

    // 5. Decode chain + parts into []session.Turn.
    turns := decodeChain(chain, parts)

    // 6. Extract compact boundaries from parts with type="compaction".
    boundaries := extractCompactions(parts)

    return &session.Session{
        ID: id, Source: session.SourceOpenCode,
        Turns: turns, CompactBoundaries: boundaries,
        ...
    }, firstCursor(chain), nil
}
```

**Branching handling:** 94% of sessions have branches. We walk the
"current" branch — the chain from the most-recent leaf back to root.
Alternate branches exist in the DB but are not surfaced in v1.

**Decode rules** (`decode.go`):

| OC Part Type | Translates to |
|---|---|
| `text` | appended to `Turn.Content` for the owning message |
| `reasoning` | appended to `Turn.Thinking` |
| `tool` (status=completed/error) | emits both `ToolCall` and `ToolResult` on the owning assistant Turn; atomic, not paired across messages |
| `tool` (status=running/pending) | emits `ToolCall` only; no result yet; excluded from prune selections |
| `patch` | treated like `text` for display (patch data shown verbatim) |
| `file` | emits a `ToolResult` with content = file path + contents |
| `compaction` | emits a `session.CompactBoundary` at the part's position |
| `step-start` / `step-finish` | ignored (internal framing) |
| `agent` | TODO: multi-agent subsessions, deferred to v1.1 |
| `subtask` | TODO: multi-agent subsessions, deferred to v1.1 |

Message `role` → `session.Role`:
- `"user"` → `RoleUser`
- `"assistant"` → `RoleAssistant`
- others (if seen) logged at warn, skipped.

### 4.4 LoadIncremental

Cursor: `{LastTimeCreated int64, LastMessageID string}`.

```sql
SELECT id, message_id, time_created, data
FROM part
WHERE session_id = ?
  AND (time_created > ? OR (time_created = ? AND id > ?))
ORDER BY time_created, id
LIMIT 1000;
```

Then fetches the owning messages for the returned parts, walks `parent_id`
from any new messages back to previously-seen ones, and decodes. The
1000-row cap bounds per-tick work.

**Edge case:** if new messages branch off an earlier message (user
regenerated), the new branch may become the current leaf. Incremental
load detects this by checking whether the new leaf's ancestry includes
the prior cursor's message. If not, a full Load is triggered instead.
Rare (< 1% of ticks in practice) but handled.

### 4.5 LoadRange

```go
func (s *Store) LoadRange(ctx, id, fromIdx, toIdx int) ([]session.Turn, error)
```

Used by the TUI memory window (§2.6). Re-queries messages at specific
indices within the current branch. Costs one query to get the branch chain
(cached per session, invalidated on tick) plus one query for the parts of
the range.

### 4.6 DetectLive

```
1. Filter ProcessSnapshot.ByPID for processes matching `opencode` in
   argv[0..1]. De-dupe by parent: when both `node /opt/...opencode` and
   the native `opencode` binary appear (parent + child), prefer the
   native child (it's the actual agent process, smaller PID surface).
2. Read the child's CWD from the snapshot. Parent and child share cwd
   on macOS/Linux, but the child is canonical.
3. For each PID's CWD:
     candidates := SELECT ... FROM session
                   WHERE directory = ?
                     AND time_archived IS NULL
                     AND time_updated > (now - 5 min)
                   ORDER BY time_updated DESC
4. If 0 candidates: the process may be in a session-less state (just
   launched, no first prompt yet); skip.
5. If 1 candidate: mark live with Ambiguous=false.
6. If > 1 candidates: mark ALL candidates live with Ambiguous=true and
   record `PID` on each as the same OpenCode process. Picker renders
   them as `◌ ? live`.
```

**Status derivation per live session:**

- DB `time_updated` within last 30s → `Working`
- Else `opencode` process CPU > 1% OR any descendant CPU > 5% → `Working`
- Else → `Waiting`
- PID gone → session dropped from live set (next Reconcile marks ended)

**CurrentTask derivation (OpenCode):**
Query for the most-recent `part` with `type=tool` and
`state.status in (running, pending, completed)` within the last 60s for
that session. Use the tool name + truncated arg. Cached per session; only
re-queried when `session.time_updated` changes (cheap index lookup).

**Cache staleness window:** `session.time_updated` is compared across
slow ticks (every 10s), so a live CurrentTask can be up to ~10s stale
relative to actual OpenCode writes. The fast tick (2s) still calls
`LoadIncremental`, which detects new `part` rows even when
`time_updated` hasn't propagated yet — so the fast path keeps the
display fresh; the 10s ceiling is only for sessions the user isn't
currently viewing.

### 4.7 Prune

Claude's `editor/pairing.go` does NOT apply to OpenCode — tool calls and
results are atomic in one `part` row, not split across messages.

Pruning a topic on OpenCode:

```go
func (e *Editor) Prune(ctx, id, sel Selection) error {
    // 1. Resolve selection to a set of message IDs and optional part IDs.
    //    - Topic-level selection: all messages in the topic.
    //    - Individual tool part selection (v1.1): single part ID.
    // 2. Filter out tool parts whose status is running/pending.
    // 3. Fetch rows that will be deleted; write backup JSON.
    // 4. PRAGMA foreign_keys = ON; (set on the writable connection)
    //    BEGIN IMMEDIATE;  -- reserves write lock without blocking readers
    //    DELETE FROM part WHERE id IN (...);
    //    DELETE FROM message WHERE id IN (...);
    //    COMMIT;
    // 5. Update SessionMeta (token count recalc deferred to next Scan tick).
}
```

**Why explicit DELETE FROM part first:** belt-and-suspenders. FK cascade
should handle it given `foreign_keys=ON`, but deleting parts first makes
the operation robust to future schema changes that might drop cascade.

**Backup format** (`~/.seshr/backups/opencode/<session-id>/<YYYYMMDD-HHMMSS>.json`):
```json
{
  "version": 1,
  "source": "opencode",
  "session_id": "ses_3df67faf...",
  "pruned_at": "2026-04-20T12:34:56Z",
  "messages": [
    {"id": "msg_...", "session_id": "...", "time_created": ..., "data": "{...}"}
  ],
  "parts": [
    {"id": "prt_...", "message_id": "...", "session_id": "...",
     "time_created": ..., "data": "{...}"}
  ]
}
```

Retention: **keep last 5 backups per session ID.** On each prune, after
writing the new backup, delete older files in
`~/.seshr/backups/opencode/<session-id>/` beyond the 5 most recent.

**Concurrent prune protection:** before writing the backup and running
retention, the OpenCode editor takes a per-session advisory lockfile at
`~/.seshr/backups/opencode/<session-id>/.lock` using `flock`. If the
lock is held (another seshr process mid-prune on the same session), the
second caller fails with "another seshr is pruning this session." This
prevents backup-retention races when two seshr processes run in
parallel. Same `gofrs/flock` dependency Claude already uses.

**Restore:**
```
1. Find most-recent backup for this session ID.
2. BEGIN;
3. INSERT OR IGNORE messages, then parts (FKs now intact).
4. COMMIT.
```

Idempotent: re-inserting already-present rows is a no-op.

#### Whole-Session Delete (picker `d` action)

Distinct from prune (which removes selected messages). Delete removes
the entire session.

```
1. Block if session is live (same policy as prune).
2. Write a full-session backup to ~/.seshr/backups/opencode/<id>/<ts>-delete.json
   (same format as prune backup, includes ALL messages and parts).
3. PRAGMA foreign_keys = ON; BEGIN IMMEDIATE;
   DELETE FROM session WHERE id = ?;  -- cascade drops messages and parts
   COMMIT;
4. Apply backup retention.
```

Restore from a `*-delete.json` backup re-INSERTs the session row first,
then messages and parts. Idempotent like prune restore.

### 4.8 Scan/Load Testability

Fixture DBs are checked into `testdata/` and regenerated by a small
script (`scripts/generate_opencode_fixtures.sh`) documented in
`docs/TESTING.md`. Fixtures cover: linear session, branching session,
session with tool parts (all 4 statuses), session with compaction part.

---

## 5. Claude Backend Details

Existing Claude parser moves intact from `internal/parser/` to
`internal/backend/claude/`. No behavior changes for reads. New additions
below.

### 5.1 LoadIncremental

Cursor: `{ByteOffset int64, FileIdentity fileIdentity}` where
`fileIdentity` is:
- Linux: `(inode, mtime_ns)`
- Darwin: `(mtime_ns, size_bytes)` (no inode on macOS stat reliably)

```go
func (s *Store) LoadIncremental(ctx, id, cur Cursor) ([]session.Turn, Cursor, error) {
    path := s.findTranscript(id)
    identity := fileIdentityOf(path)
    if identity != cur.FileIdentity:
        // Rotated/truncated — fall back to full Load.
        sess, newCur, err := s.Load(ctx, id)
        return sess.Turns, newCur, err
    fh := openForReadAt(path, cur.ByteOffset)
    newTurns := parseFrom(fh)
    return newTurns, Cursor{ByteOffset: fh.Tell(), FileIdentity: identity}, nil
}
```

### 5.2 LoadRange

Claude JSONL is line-oriented. `LoadRange(from, to)` scans the file line
by line until `from`, parses through `to`. No random access; cost is
O(lines). Acceptable since this is triggered by explicit user navigation,
not per-tick.

For very large sessions a sidecar index (`~/.seshr/index/<id>.idx` mapping
turn-idx → byte-offset) could speed this up. Deferred to v1.1 unless
benchmarks show it's needed.

### 5.3 DetectLive

Two-layer:

```
Layer 1 (primary, when sidecar files present):
  1. Glob ~/.claude/sessions/*.json (and CLAUDE_CONFIG_DIR if set on Linux).
  2. For each, decode {pid, sessionId, cwd, startedAt}.
  3. Filter to entries whose PID is in ProcessSnapshot with argv containing
     `claude` (not `--print`).

Layer 2 (fallback, when sidecar empty or missing):
  4. For any `claude` process in ProcessSnapshot with no sidecar match:
     - Read cwd from snapshot.
     - Encode cwd to Claude Code's project-dir naming convention:
       replace `/`, `_`, and `.` with `-`. Example:
       `/Users/foo/bar.v2` → `-Users-foo-bar-v2`.
     - Find the most-recently-modified transcript in
       `~/.claude/projects/<encoded-cwd>/*.jsonl`.
     - If modified within last 5 min, mark it live (best-effort, no
       sidecar confidence).
```

Layer 2 handles: older Claude versions that don't write sidecars, sidecar
file deletion, non-standard Claude installs.

**Status derivation (same 3-signal model as OpenCode):**
- Transcript mtime < 30s → `Working`
- Else claude CPU > 1% OR descendant CPU > 5% → `Working`
- Else → `Waiting`
- PID gone → dropped.

**CurrentTask derivation (Claude):** extract from the most-recent
assistant turn's last `tool_use` block. Same truncation and fallback
rules (full arg → tool name → empty).

### 5.4 Prune

Unchanged. Existing logic moves to `editor/claude/` with `pairing.go`
alongside (Claude-specific). `.bak` sibling file. Flock. No retention
change (Claude keeps only the most-recent `.bak`, matching today's
behavior).

---

## 6. Platform Gates

| Capability | macOS | Linux | Windows |
|---|---|---|---|
| Process scan (`ps`) | ✓ | ✓ | ✗ |
| cwd of process | `lsof -p PID -d cwd` | `/proc/<PID>/cwd` | ✗ |
| `CLAUDE_CONFIG_DIR` lookup | seshr env only | `/proc/<PID>/environ` | ✗ |
| SQLite (OpenCode) | ✓ | ✓ | (deferred) |
| JSONL (Claude) | ✓ | ✓ | (deferred) |
| Clipboard for resume overlay | `pbcopy` | `xclip` / `wl-copy` | (deferred) |

Build tags used:
- `backend/process_linux.go` / `process_darwin.go`
- `backend/claude/detect_linux.go` / `detect_darwin.go`
- `backend/opencode/detect_linux.go` / `detect_darwin.go`

Windows: disabled in goreleaser for v1. The TUI compiles on Windows
(bubbletea works) but live detection returns empty and SQLite works —
so a Windows build could read ended sessions only. Not a v1 target.

---

## 7. TUI Behavior

### 7.1 Tickers

- **Slow (10s):** always active. Refreshes ProcessSnapshot + runs
  LiveDetectors. Drives status up/down transitions. Applies hysteresis.
- **Fast (2s):** active whenever the shared `liveIndex` has any entries.
  Refreshes `CurrentTask`, tails new turns for the active view. Suspended
  automatically when no live sessions exist.

Both tickers suspend while help / settings / log viewer overlays are
open to avoid rerender storms under modals.

### 7.2 Search on Live Sessions

If `/` search is active when new turns arrive, match IDs (not indices)
preserve the selected result position. No cursor jump.

Search operates on the post-decode `Session.Turn` shape — `Content`,
`Thinking`, `ToolCalls[].Input`, `ToolResults[].Content`. Both Claude
and OpenCode produce the same Turn structure after decode, so search
works identically across sources without per-source code paths. Picker
search (across sessions) matches project name + session ID + title;
both sources expose all three via SessionMeta.

### 7.3 Replay Autoplay on Live Sessions

Autoplay pauses at the end of known turns. New turns arriving via live
refresh do NOT auto-resume playback; user must press space again.

### 7.4 Sidebar Focus Preservation

When sidebar-focused topics refresh under live updates, focus stays on
the same topic by ID, not index.

### 7.5 Shutdown

On `ctrl+c` / `tea.Quit`:
1. TUI emits `tea.Quit`.
2. App cancels the app-wide `context.Context`.
3. In-flight source operations receive `ctx.Done()` and return.
4. All `Store.Close()` called in parallel, 500ms timeout.
5. Log file flushed and closed.
6. Process exits.

No goroutine may outlive the app context.

### 7.6 Rerender Cost

Expected rerender cadence during a live view: every 2s minimum. Profiling
task: verify no layout-recalculation spike > 16ms per tick on 120x40.
Captured as a manual-testing checklist item.

---

## 8. CLI

| Flag | Default | Description |
|---|---|---|
| `--dir <path>` | auto-detected | Scan a custom directory for Claude sessions |
| `--theme <name>` | `catppuccin-mocha` | Color theme |
| `--debug` | false | Enable debug logging |
| `--no-live` | false | Disable live detection; all sessions show as ended |
| `--version` | — | Print version and exit |

`--no-live` is for users in restricted environments (sandboxes without
`ps`/`lsof`) or who don't want process scans.

---

## 9. Config

Unchanged for v1: flat `~/.seshr/config.json` with `theme` and
`gap_threshold`. Per-source settings deferred; schema is
forward-compatible (unknown keys ignored). Documented explicitly so we
don't paint into a corner.

---

## 10. Testing

### 10.1 Unit Tests

**`backend/claude/`:**
- Sidecar decode round-trip
- LoadIncremental appends correctly, resets on rotation
- LoadRange returns correct slice
- DetectLive layer 1 (sidecar + PID match)
- DetectLive layer 2 (sidecar missing, cwd fallback)
- Platform build tags compile on both darwin and linux

**`backend/opencode/`:**
- Scan across linear + branching + compaction fixtures
- Load walks current branch correctly on branching fixture
- LoadIncremental handles branch-change edge case
- LoadRange returns correct window
- Decode: each part type translates correctly
- DetectLive ambiguity handling (multiple candidates → Ambiguous=true)
- Status derivation (working / waiting thresholds)

**`backend/process.go`:**
- ps output parsing
- Children map construction
- CWD lookup gated by platform

**`editor/claude/`:**
- Prune round-trip on paired tool_use/tool_result
- Pairing correctness under selection with orphans
- Backup/restore round-trip

**`editor/opencode/`:**
- Prune round-trip: parts and messages removed, no orphans
- Skip running/pending tool parts correctly
- Backup format correctness + restore idempotence
- Retention: 6th prune deletes oldest backup
- FK-on verification

**`tui/`:**
- Picker row rendering (golden): ended, working, waiting, ambiguous
- Picker cross-source rendering: mixed Claude + OpenCode rows sort and render correctly within one project group
- Landing page rendering: live + ended, with/without cost
- Resume overlay command formatting per source (Claude / OpenCode)
- Resume overlay clipboard helper: `pbcopy` / `xclip` invocation; "copy unavailable" hint when missing
- SessionView bounded-memory eviction on 1000-turn append; LoadRange triggers correctly when scrolling into evicted range
- Hysteresis: downgrade requires 2 consecutive ticks; upgrade is instant
- Live-detection-off banner: appears after 3 consecutive scan failures, disappears on recovery

### 10.2 Integration Tests

- Unified picker: Claude + OpenCode fixtures both appear
- Live → ended transition: simulate process exit, verify pulse → arrow
- Prune blocked on Claude live
- Prune allowed on OpenCode live (SQLite-only fixture can't simulate
  concurrent writer, so this is a logic test — confirms the dialog path
  and the DELETE succeeds)

### 10.3 Manual Testing Additions

Added to `docs/MANUAL_TESTING.md`:
- Live pulse shows up for running agents
- CurrentTask updates within 2s of new tool call
- Status transitions with hysteresis (no flicker)
- Landing page populates for both sources
- Resume overlay copies command
- OpenCode prune on live session works end-to-end
- 1000+ turn OpenCode session loads without hang
- Memory window eviction: scroll back in replay triggers LoadRange
- Clean shutdown: no zombie processes after ctrl+c

### 10.4 Coverage Targets

- New `backend/` and `editor/opencode/`: ≥ 80%
- Modified `tui/landing.go`, `session_view.go`: ≥ 70%
- No regression on existing package coverage.

---

## 11. Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| OpenCode schema changes | Scan/decode breaks | Fixture-based schema test as canary; pin tested OC version in README |
| SQLite write lock contention during prune | Prune fails / times out | 500ms busy_timeout, user-visible retry message |
| Claude sidecar files absent | Live detection degrades | Layer-2 cwd-fallback detection (§5.3) |
| `ps` / `lsof` unavailable | No live detection | Persistent banner in picker; `--no-live` flag; UX still usable |
| Ambiguous OpenCode cwd (2+ agents) | Wrong "live" marker | "? live" hollow-dot rows; don't pick one |
| Claude JSONL rotation mid-session | Incremental misses turns | File identity check every tick; reset on mismatch |
| Branching in OpenCode, wrong current-leaf detection | Wrong chain displayed | Walk from most-recent leaf by `MAX(time_created)` within the session; regression test with fixture |
| Long-lived live session RAM growth | Memory pressure | 500-turn window + LoadRange on-demand |
| OpenCode scan too slow at scale | Laggy launch | Benchmark first; cache in `opencode_meta.db` if needed |
| Backup file bloat | Disk pressure | Retention: last 5 per session |
| Refactor breaks existing Claude tests | Regression | Tests move alongside code; `just check` gate every phase |
| Binary size growth | 15MB → 25-30MB | Accepted; documented in README |
| Context cancellation missed | Goroutine leak | Every backend exposes Close(); app shutdown waits bounded |
| Pulse flicker at tool-call boundaries | Jarring UX | Downgrade hysteresis (2 slow ticks) |

---

## 12. Non-Findings (Considered and Declined)

Explicitly not addressing these, for the record:

- **ProcessScanner caching / diffing:** At ≤ 5 agent processes and ~20ms
  per `ps` + `lsof` invocation, caching saves nothing meaningful.
  Reconsider if benchmarks show scan > 100ms.
- **CPU-independent live status:** The 3-signal model (transcript mtime,
  process CPU, descendant CPU) is empirically proven via abtop on
  thousands of users. IO-bound agents show CPU on the parent process
  during network calls; truly idle agents are correctly classified
  Waiting.
- **Cursor persistence across restarts:** Cold-start full reload is
  fast (~300ms on the author's largest session). Persisting cursors
  adds versioning and staleness complexity without proportionate benefit.

---

## 13. Out of Scope for v1

- Suggestions engine (proactive nudges)
- Sparklines and token charts
- Autopsy view
- Dashboard-of-all-live-sessions screen (abtop-style grid)
- Per-source settings / enable-disable toggles
- tmux pane-jumping integration
- Notifications when Waiting sessions need input
- Session comparison / diff
- Export to markdown / HTML
- Claude continuation-chain reconstruction
- **Claude subagent transcripts** (`~/.claude/projects/<p>/<session>/subagents/*.jsonl` produced by the Task tool — currently shown as separate sessions, not nested under their parent)
- Individual turn selection for pruning (topic-level only)
- OpenCode `agent` / `subtask` parts (multi-agent subsessions)
- OpenCode alternate-branch display
- Windows support
- Model/provider display
- Sparse-file index for Claude LoadRange

---

## 14. Implementation Order

Each phase builds clean (`just check` passes), delivers incremental value,
and is a separate commit.

1. **Rename `internal/parser/` → `internal/session/`.** Mechanical. All
   imports updated. No behavior change.
2. **Introduce `internal/backend/` package** with `SessionStore`,
   `LiveDetector`, `SessionEditor` interfaces. Claude implementations
   are thin wrappers over existing code. TUI still calls them directly.
3. **Shared `ProcessScanner`** with platform gates.
4. **Claude `LiveDetector`** (layer 1 + layer 2) + `--no-live` CLI
   flag (allows opt-out from the moment live detection ships). Picker
   rows start showing live pulse + source badge.
5. **Claude `LoadIncremental` and `LoadRange`** + `SessionView` memory
   window wiring.
6. **Fast/slow tickers** with hysteresis. Live-detection-off banner.
7. **Landing page.** Intermediate screen; `c` resume overlay with
   platform-gated clipboard helper.
8. **OpenCode `SessionStore`** (Scan with backup discovery + token
   aggregate, Load with branching, decode).
9. **OpenCode `LiveDetector`** (cwd inference, ambiguity handling).
10. **OpenCode `LoadIncremental` and `LoadRange`.**
11. **OpenCode `SessionEditor`** (prune, delete, backup, retention,
    restore). Per-session backup-dir lockfile to serialize retention.
12. **Polish + docs:** SPEC.md final pass, MANUAL_TESTING.md additions,
    README.md update, OpenCode resume command verification.

Phases 1-7 deliver full Claude experience.
Phases 8-11 deliver full OpenCode experience.
Phase 12 closes v1.

---

## 15. Open Questions

Resolved at implementation time:

- Source badge color: pick during phase 4 with manual testing; must not
  clash with project gutter.
- Picker row width budget at 80 cols: if badge + status column doesn't
  fit, hide badge at narrow widths.
- OpenCode resume command: verify against `opencode --help` at implementation
  time. If resume requires different flags or isn't supported, adjust
  overlay text.
