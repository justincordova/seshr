# Seshr — Product Specification

**AI Agent Session Replay & Conversation Editor**
A Bubbletea TUI for understanding and managing AI agent conversations

v0.1.0 · April 2026

---

## 1. Overview

Seshr is a terminal-based tool built in Go with Bubbletea and Lipgloss that lets developers replay, inspect, and edit AI agent conversation sessions. In v1 it reads conversation logs from Claude Code (JSONL), groups messages into topics automatically, and provides two core modes: a step-by-step Replay Mode for understanding what happened, and an Edit Mode for pruning irrelevant turns from session files. The parser layer is designed to be extensible — additional formats (OpenCode, LangChain, Cursor) are planned post-v1.

### 1.1 Problem Statement

AI agent sessions accumulate irrelevant context over time. Off-topic tangents, failed tool calls, and exploratory dead ends all consume tokens and degrade agent performance. There is currently no tool that lets developers visualize their session history by topic and selectively remove unwanted turns. Existing tools either focus on cross-session memory (claude-mem) or streaming live output (claude-esp) but none provide an interactive editor for session files.

### 1.2 Target User

Developers who use Claude Code, OpenCode, or other LLM-based coding agents daily and run long sessions where context management matters. They are comfortable in the terminal, likely use tmux, and want a tool that fits into their existing workflow as a second pane.

### 1.3 Core Value Proposition

- See what's in your agent session at a glance, grouped by topic with token counts
- Replay sessions step-by-step to understand agent behavior and decisions
- Prune irrelevant conversation topics to clean session files before resuming
- Manage sessions by deleting old or unnecessary session files from disk
- Extensible parser layer — Claude Code in v1, additional agent platforms post-v1

---

## 2. Architecture

Seshr follows a simple three-layer architecture: a parser layer that reads and understands conversation files, a topic clustering engine that groups turns into logical topics, and a TUI layer built with Bubbletea that renders the interface and handles user interaction.

### 2.1 Project Structure

```
seshr/
├── cmd/
│   └── seshr/
│       └── main.go            # CLI entry point (Cobra)
├── internal/
│   ├── parser/
│   │   ├── parser.go          # SessionParser interface (Parse)
│   │   ├── claude.go          # Claude Code JSONL parser
│   │   ├── scan.go            # Directory scanning and session discovery
│   │   ├── record.go          # JSONL record types and decoding
│   │   └── types.go           # Shared types: Turn, ToolCall, Session
│   ├── topics/
│   │   ├── cluster.go         # Topic clustering algorithm
│   │   ├── signals.go         # Clustering signals (time gap, file shift, etc.)
│   │   ├── label.go           # Topic label generation
│   │   ├── stopwords.go       # Stopword list for keyword extraction
│   │   └── fileset.go         # File set extraction from tool calls
│   ├── editor/
│   │   ├── pruner.go          # JSONL rewriting with safe message pairing
│   │   ├── pairing.go         # Turn pair enforcement rules
│   │   ├── backup.go          # .bak file creation and restore
│   │   ├── lock.go            # Advisory file locking (flock)
│   │   └── load.go            # Session loading helper
│   ├── tokenizer/
│   │   └── estimate.go        # Token count estimation
│   ├── config/
│   │   └── config.go          # Settings management (~/.seshr/config.json)
│   ├── logging/
│   │   └── logging.go         # slog setup → ~/.seshr/debug.log
│   ├── version/
│   │   └── version.go         # const Version, injected via ldflags on release
│   └── tui/
│       ├── app.go             # Root Bubbletea model, screen routing, global overlays
│       ├── sessions.go        # Session picker view (grouped by project)
│       ├── picker_groups.go   # Project grouping, row flattening, summary stats
│       ├── topics.go          # Topic overview view (shared foundation)
│       ├── replay.go          # Replay mode view
│       ├── replay_autoplay.go # Auto-play state machine
│       ├── replay_render.go   # Replay turn rendering (markdown, tool blocks, search)
│       ├── editor.go          # Editor mode view
│       ├── editor_render.go   # Editor turn rendering
│       ├── confirm.go         # Confirmation dialog component (delete, prune, restore)
│       ├── help.go            # Help overlay component (? key)
│       ├── search.go          # Fuzzy search bar component (/ key)
│       ├── settings.go        # Settings popup (, key)
│       ├── logviewer.go       # Log viewer (L key)
│       ├── theme.go           # Color scheme definitions (Catppuccin, Nord, Dracula)
│       ├── keys.go            # Keybinding definitions per screen
│       ├── styles.go          # Lipgloss style constants
│       └── chrome.go          # Shared layout primitives: header/footer bars, pill, panel, kbd, hRule
├── testdata/
│   ├── simple.jsonl           # Simple Claude Code session fixture
│   ├── multi_topic.jsonl      # Multi-topic session with tool calls
│   └── embedded_tool_results.jsonl  # Session with embedded tool results
├── go.mod
└── go.sum
```

### 2.2 Data Flow

```
Session file(s) on disk
       │
       ▼
  parser.Scan()            →  []SessionMeta (discovery)
  parser.Parse()           →  *Session ([]Turn, tool calls, metadata)
       │
       ▼
  topics.Cluster()         →  []Topic (grouped turns with labels)
       │
       ▼
  tui renders Topic Overview
       │
   ┌───┴───┐
   ▼       ▼
 Replay   Editor
 Mode     Mode
            │
            ▼
  pruner.Prune()           →  Rewritten file (selected turns removed)
```

---

## 3. Screens & User Flow

### 3.1 Session Picker

The entry point of the application. On launch, Seshr scans the default directory for Claude Code (`~/.claude/projects/`) sessions. Users can also pass a custom path via `--dir` flag.

Sessions are **grouped by project** into collapsible groups with colored gutters. The header shows a logo. A stats strip shows aggregate SESSIONS · PROJECTS · TOKENS · SIZE · LATEST. Each session row shows a truncated session ID, token count (compact format like `15.7M`), and relative timestamp.

```
┌─ ◆ Seshr v0.1 ──────────────────────────────────────────────────────────────┐
│  SESSIONS 10 · PROJECTS 7 · TOKENS 381,449,721 · SIZE 53 MiB · LATEST 3s ago│
│                                                                              │
│  ▌ JUSTIN                                     ▾ 1 session  15.7M tok       │
│  ▌   ▸ 146a51d6-ade8-42df-…                          15.7M  2 minutes ago  │
│                                                                              │
│  ▌ BOOT                                       ▾ 1 session  65.8M tok       │
│  ▌   ▸ bb859dee-0744-44f1-…                          65.8M  2 minutes ago  │
│                                                                              │
│  ▌ DARTLY                                     ▾ 2 sessions  25.7M tok      │
│  ▌   ▸ d29d5ec6-e19d-4362-…                          14.4M  2 minutes ago  │
│  ▌   ▸ 323f0680-89be-497f-…                          11.2M  2 minutes ago  │
│                                                                              │
│  ↑↓/jk nav · enter open · d delete · / search · q quit                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Session Picker Keybindings

| Key            | Action                        | Notes                                  |
| -------------- | ----------------------------- | -------------------------------------- |
| `↑/↓` or `j/k` | Navigate session list         | Vim-style navigation                   |
| `enter`        | Open session → Topic Overview | Parses and clusters into topics        |
| `r`            | Open directly in Replay Mode  | Skips topic overview                   |
| `e`            | Open directly in Edit Mode    | Skips topic overview                   |
| `d`            | Delete session                | Confirmation dialog before deleting    |
| `R`            | Restore from `.bak`           | Only if a `.bak` sibling exists (§4.5) |
| `/`            | Fuzzy search/filter sessions  | Matches project name + session ID      |

Global overlays (handled by root app model): `,` settings, `L` log viewer, `?` help, `q` quit.

#### Session Deletion

When a user presses `d`, a confirmation dialog appears with the session display name and project path, warning that this cannot be undone. On confirmation:

- **Claude Code:** Deletes the `.jsonl` file from `~/.claude/projects/<project-dir>/`. Also removes any `.bak` and `.lock` siblings. If the project directory is now empty, the empty directory is also cleaned up.

### 3.2 Topic Overview (Shared View)

The core screen and shared foundation for both Replay and Edit modes. Displays the parsed session as a list of auto-detected topics, each showing a label, token count, turn range, tool call count, and duration.

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
│  ↑↓/jk nav · enter expand · r replay · e edit · tab stats · / search · esc back │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Topic Overview Keybindings

| Key            | Action                      | Notes                         |
| -------------- | --------------------------- | ----------------------------- |
| `↑/↓` or `j/k` | Navigate topics             |                               |
| `enter` or `→` or `l` | Expand/collapse topic | Shows individual turns within |
| `r`            | Enter Replay Mode           | Starts from selected topic    |
| `e`            | Enter Edit Mode             | Enables selection checkboxes  |
| `/`            | Fuzzy search within session | Searches topic labels + turn content |
| `tab`          | Toggle stats panel          | Right-side aggregate stats    |
| `esc`          | Back to Session Picker      |                               |

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

### 3.3 Replay Mode

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

### 3.4 Edit Mode

Adds selection controls to the Topic Overview. Users select entire topics to prune. Footer shows a running count of selected items and estimated tokens freed.

```
┌─ ◆ Seshr ─── EDIT MODE ─────────────────────────────────────────────────────┐
│  Select topics or turns to prune                                             │
│                                                                              │
│  [ ] 1. Project setup & Express init          ~12.4k                        │
│  [ ] 2. Authentication with JWT                ~8.2k                        │
│  [x] 3. Where to buy a house                   ~2.1k                        │
│  [ ] 4. Rate limiting implementation           ~9.8k                        │
│  [ ] 5. Error handling & validation           ~14.7k                        │
│                                                                              │
│  space select · a all · A none · enter expand · p prune · esc cancel        │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Edit Mode Keybindings

| Key     | Action                | Notes                                        |
| ------- | --------------------- | -------------------------------------------- |
| `space` | Toggle selection      | Selecting a topic selects all its turns      |
| `a`     | Select all            |                                              |
| `A`     | Deselect all          |                                              |
| `p`     | Prune selected        | Shows confirmation with token savings        |
| `enter` | Expand/collapse topic | View individual turns within the topic       |
| `esc`   | Cancel and return     | Discards selections                          |

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

#### Concurrent Access

Only one `seshr` process should write to a given session file at a time. The pruner takes an advisory file lock (`flock`) on the target `.jsonl` for the duration of the rewrite. If the lock is held, the prune confirmation shows "Session is locked by another process" and the operation is cancelled. Reads (parsing, replay) do not require a lock.

#### Safe Message Pairing

The pruner enforces strict pairing rules to prevent invalid session files:

- User and assistant turns are always deleted as pairs. Cannot delete one without the other.
- `tool_use` and `tool_result` blocks must be deleted together. A `tool_use` without its matching `tool_result` (or vice versa) breaks the session on resume.
- If a user selects only one half of a pair, the pruner automatically includes the other half and shows this in the confirmation.
- System messages and compact summaries (`isCompactSummary: true`) are never selectable.

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
│  r              Replay mode          │
│  e              Edit mode            │
│  d              Delete session       │
│                                      │
│  Global                              │
│  /              Search               │
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

Every prune operation writes a `.bak` sibling file next to the original (e.g. `session.jsonl.bak`). If a pruned session fails to resume in Claude Code, the user can restore from backup:

- In the Session Picker, a session with a `.bak` sibling shows a small `↶` indicator.
- Pressing `R` (shift-r) on such a session opens a confirmation dialog: "Restore from backup? This will overwrite the current session file."
- On confirm, the `.bak` is copied over the original. The backup is preserved until the next prune overwrites it.

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

## 6. Parser Specification

### 6.1 Parser Interface

```go
type SessionParser interface {
    Parse(ctx context.Context, path string) (*Session, error)
}
```

The Claude parser also provides `Detect(path) bool` and `Source() string` as methods on the concrete type, but these are not part of the interface.

### 6.2 Claude Code JSONL Format

Each line in a Claude Code JSONL session file is a JSON object with a `type` field:

| Type          | Description                | Key Fields                                                              |
| ------------- | -------------------------- | ----------------------------------------------------------------------- |
| `user`        | User message               | `message.role`, `message.content`, `timestamp`                          |
| `assistant`   | Claude response            | `message.content` (array of text/tool_use/thinking blocks), `timestamp` |
| `tool_result` | Result of a tool call      | `message.content`, `tool_use_id`                                        |
| `system`      | System/compaction messages | `message.content`, `isCompactSummary`, `subtype`, `compactMetadata`     |
| `summary`     | Session summary            | Summary text, generated asynchronously                                  |

The parser ignores unknown types (e.g. `file-history-snapshot`, `progress`, `hook`) and logs a warning via slog.

#### Compact Boundary Records

`system` records with `subtype: "compact_boundary"` mark where a `/compact` call occurred. The parser extracts these into `Session.CompactBoundaries []CompactBoundary` (ordered by position). Each boundary carries:

- `TurnIndex int` — index of the first turn after this boundary
- `Trigger string` — `"manual"` or `"auto"`
- `PreTokens int` — token count before compaction
- `DurationMs int` — compaction duration in milliseconds

User turns whose content starts with `"This session is being continued"` are marked with `Turn.IsCompactContinuation = true`. These are the continuation summaries Claude Code injects after compaction.

#### Embedded Tool Results

Tool results can appear either as top-level records or embedded within assistant message content blocks (with `type: "tool_result"`). The parser extracts embedded tool results and attaches them to the parent assistant turn.

### 6.3 Session Scanning

`parser.Scan()` discovers session files by walking the Claude Code projects directory. For each `.jsonl` file, it reads metadata (session ID, project, timestamps, token count, file size) without fully parsing the file. Sessions are grouped by project directory name.

### 6.4 Adding Future Parsers

The parser interface is designed so additional formats can be added without modifying existing code. To add a new parser: implement the `SessionParser` interface, add detection logic, and register it in the parser registry. The registry tries each parser's `Detect` method in order until one matches.

---

## 7. Token Estimation

Seshr estimates token counts for display purposes using a character-based heuristic: divide rune count by 3.5 for English text. This gives an approximation within 10-15% of actual Claude tokenization. All token counts are prefixed with `~` in the UI to indicate they are approximate.

If the JSONL records contain actual token usage data (some Claude Code versions include `usage` fields in assistant messages with `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, and `cache_read_input_tokens`), the parser prefers those values over the heuristic.

---

## 8. Color Scheme

### 8.1 Default Theme: Catppuccin Mocha

Seshr uses Catppuccin Mocha as the default color scheme. Three themes are available: `catppuccin-mocha` (default), `nord`, and `dracula`. All colors are defined using `lipgloss.AdaptiveColor` so they degrade gracefully on light terminal backgrounds.

The `Theme` struct holds: Background, Foreground, Accent, Muted, Error, plus role badge colors (UserColor, AssistantColor, ToolUseColor, ToolResultColor, AgentColor), palette entries for project gutters (ProjectPalette), and overlay/surface colors for UI elements.

### 8.2 Theme Switching

Themes are selectable via the settings popup (`,`) or `--theme` CLI flag. The active theme is stored in `~/.seshr/config.json`.

### 8.3 Style Constants

Base styles are defined in `internal/tui/styles.go` using the Catppuccin Mocha palette. Key styles:

- **Box borders:** Rounded border style (`lipgloss.RoundedBorder()`) using Surface1 color
- **Role badges:** Colored foreground with bold text
- **Selected row:** Project-colored gutter with bold/bright text
- **Footer:** Dim text for descriptions, accent-colored keys
- **Compact divider:** Accent (`Theme.Accent`) colored horizontal rule, rendered between pre-compact and post-compact topics in the Topic Overview and Replay sidebar
- **Pre-compact indicator:** `░` marker and dimmed text (`colSurface1` / `dimStyle`) on pre-compact topic cards and turn headers

---

## 9. Responsive Layout

The TUI must adapt to different terminal sizes. Use `tea.WindowSizeMsg` to detect the terminal dimensions and adjust layout accordingly.

**Minimum size:** 60 columns × 15 rows. Below this, show a "terminal too small" message.

**Replay split pane:** The topic sidebar takes 20% of width (minimum 16 columns, maximum 24 columns). The main pane takes the remainder. If the terminal is narrower than 80 columns, hide the sidebar and show topics as a header bar instead.

**Long content:** Use `bubbles/viewport` for scrollable content in the replay main pane and log viewer. Word wrapping based on available width.

**Loading states:** Large sessions take time to parse. Show a `bubbles/spinner` with "Parsing session..." while loading.

---

## 9.5 Error UX Standard

Errors are surfaced inline within the current screen:

- **Delete errors:** Shown as a red error line below the session list in the picker. Auto-clears.
- **Prune errors:** Shown in the confirmation dialog or as inline status text in the editor.
- **Session load errors:** Full-screen error state with the error message and an `esc` to go back prompt.
- **Log correlation:** Every displayed error writes a matching `error`-level slog entry with the same message and an `err` field.

---

## 9.6 Privacy & Telemetry

Seshr collects **no telemetry**. No analytics, no crash reporting, no network calls, no update pings. The only network-capable dependency is `glamour` (for rendering images in markdown, which is disabled). If any future feature would phone home, it is opt-in and documented in this section before landing.

Session content never leaves disk. All processing is local. The log file at `~/.seshr/debug.log` contains metadata only — see LOGGING.md for the "no raw content" rule.

---

## 10. CLI Specification

Seshr uses Cobra for CLI argument parsing.

| Command / Flag          | Description                    | Default                |
| ----------------------- | ------------------------------ | ---------------------- |
| `seshr`                | Launch TUI with session picker | Scans default dirs     |
| `--dir <path>`          | Scan a custom directory        | Auto-detected          |
| `--theme <name>`        | Color theme                    | `catppuccin-mocha`     |
| `--debug`               | Enable debug logging           | `false`                |
| `--version`             | Print version and exit         |                        |

---

## 11. Tech Stack & Go Best Practices

### 11.1 Go Version

Seshr targets **Go 1.26** (latest stable, released February 2026). The `go.mod` file should specify `go 1.26`.

### 11.2 Dependencies

| Package                           | Purpose                                                             |
| --------------------------------- | ------------------------------------------------------------------- |
| `charmbracelet/bubbletea`         | TUI framework, application model and event loop                     |
| `charmbracelet/lipgloss`          | Terminal styling, colors, borders, layout                           |
| `charmbracelet/bubbles`           | Pre-built components: viewport, textinput, spinner, key             |
| `charmbracelet/glamour`           | Markdown rendering for Claude's responses in replay view            |
| `github.com/spf13/cobra`          | CLI argument parsing and subcommands                                |
| `github.com/stretchr/testify`     | Testing: `assert`, `require` packages — standard everywhere         |
| `log/slog` (stdlib)               | Structured logging to file (TUI owns stdout)                        |
| `github.com/sahilm/fuzzy`         | Fuzzy string matching for `/` search                                |
| `github.com/dustin/go-humanize`   | Human-friendly formatting: "2h ago", "47k", "1.2 MB"                |
| `github.com/gofrs/flock`          | Advisory file locking during prune                                  |

**Explicitly not used:** no third-party logging library (stdlib `log/slog` is sufficient), no YAML (config is JSON), no pty/clipboard/diff libraries (not needed for v1 scope).

### 11.3 Logging

**Library choice:** Use stdlib `log/slog` only. No third-party logging library (zap, zerolog, logrus). slog covers structured logging, levels, and handlers; adding another library would be dead weight for a tool this size.

**Conventions:**

- **Destination:** Always `~/.seshr/debug.log`. Never stdout/stderr — the TUI owns the terminal.
- **Levels:** `info` by default, `debug` when `--debug` is passed. `warn` for recoverable parser issues (unknown JSONL types, malformed records skipped). `error` for failures the user should see (file read errors, prune validation failures) — these also surface in the UI, never only in the log.
- **Structured fields:** Use key/value pairs, not formatted strings. Prefer `slog.Info("parsed session", "path", p, "turns", n)` over `slog.Info(fmt.Sprintf(...))`.
- **Standard keys:** `path` (file path), `session_id`, `turns`, `topics`, `duration_ms`, `err`. Keep keys consistent across the codebase so log grep works.
- **No secrets or full message content:** log metadata (turn counts, IDs, sizes), not the raw conversation. Session content can include sensitive data from the user's work.

### 11.4 Testing

**Framework:** `github.com/stretchr/testify` is the project's testing library. All tests use it — do not mix in other assertion libraries or write raw `if got != want { t.Fatalf(...) }` style checks.

**Conventions:**

- `testify/require` for assertions that must pass before continuing (fail-fast: parse succeeded, file exists, no error returned).
- `testify/assert` for non-critical checks where the test should keep running to surface multiple failures.
- Table-driven tests for parser and clustering logic — each case gets a `name` field used as the subtest name.
- Test files sit next to the code they test (`claude_test.go` beside `claude.go`).
- `testdata/` holds sample JSONL fixtures. Fixtures are checked in, not generated.
- Run tests with `go test ./...` before any commit (see CLAUDE.md pre-commit gate).

### 11.5 Go Best Practices

- **Project layout:** Use `internal/` for all private packages. No `pkg/` directory.
- **Error handling:** Wrap errors with `fmt.Errorf` and `%w`. Define sentinel errors at package level. Never panic in library code.
- **Interfaces:** Define where consumed, not where implemented. Keep small (1-3 methods).
- **Context:** Pass `context.Context` as first param for I/O functions. Use for graceful TUI shutdown.
- **Naming:** MixedCaps, acronyms all caps (ID, URL). Short lowercase package names.
- **Concurrency:** Use Bubbletea `Cmd` for async operations (parsing, file I/O). The TUI event loop is single-threaded.
- **Linting:** Run `golangci-lint` with gocritic, errcheck, govet enabled.
- **Build:** Use `goreleaser` for cross-platform builds: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64.

---

## 12. Future Features (Post-Launch)

These are explicitly out of scope for v1 but are natural extensions:

- **OpenCode parser:** SQLite + file-based + CLI export fallback. Deferred from v1 to keep scope focused on Claude Code.
- **Live session watching:** Use fsnotify to watch an active session and update the TUI in real time, similar to claude-esp. Turns Seshr from post-hoc analysis into a live companion.
- **Additional parsers:** LangChain traces, Cursor conversation logs, generic JSONL agent logs.
- **Session comparison:** Side-by-side diff of two sessions to understand how different approaches played out.
- **Export:** Generate clean markdown or HTML reports from sessions for documentation or sharing.
- **Session continuation chains:** Reconstruct multi-file sessions from Claude Code's compaction continuation chains.
- **Individual turn selection in editor:** Allow selecting specific turns within a topic for more granular pruning.
- **Word wrap toggle in replay:** Toggle between wrapped and horizontal-scroll display of long lines.

---

## 13. Risks & Mitigations

| Risk                                        | Impact                               | Mitigation                                                                                                              |
| ------------------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| JSONL format changes in Claude Code updates | Parser breaks, sessions fail to load | Pin parser to known format types. Ignore unknown types gracefully with warning logs. Monitor Claude Code changelogs.    |
| Large session files (100k+ lines)           | Slow parsing, high memory usage      | Stream-parse JSONL instead of loading entire file. Show spinner during parse. Paginate displayed turns.                 |
| Accidental deletion of important sessions   | Data loss                            | Require confirmation dialog. Create `.bak` backup before any write.                                                      |
| Invalid JSONL after pruning breaks resume   | Session cannot be resumed            | Enforce strict message pairing rules. Validate output structure before writing. Always keep `.bak`.                     |
| Topic clustering produces poor groupings    | Confusing UI                         | Make clustering configurable (`gap_threshold`). Allow manual topic boundary insertion in future version.                |
| Charm library import path migration         | Build failures                       | Pin exact versions in go.mod. Document which import paths are used.                                                     |
