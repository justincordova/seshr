# AgentLens — Product Specification

**AI Agent Session Replay & Conversation Editor**
A Bubbletea TUI for understanding and managing AI agent conversations

v0.1.0 · April 2026

---

## 1. Overview

AgentLens is a terminal-based tool built in Go with Bubbletea and Lipgloss that lets developers replay, inspect, and edit AI agent conversation sessions. In v1 it reads conversation logs from Claude Code (JSONL), groups messages into topics automatically, and provides two core modes: a step-by-step Replay Mode for understanding what happened, and an Edit Mode for pruning irrelevant turns from session files. The parser layer is designed to be extensible — additional formats (OpenCode, LangChain, Cursor) are planned post-v1.

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

AgentLens follows a simple three-layer architecture: a parser layer that reads and understands conversation files, a topic clustering engine that groups turns into logical topics, and a TUI layer built with Bubbletea that renders the interface and handles user interaction.

### 2.1 Project Structure

```
agentlens/
├── cmd/
│   └── agentlens/
│       └── main.go            # CLI entry point (Cobra)
├── internal/
│   ├── parser/
│   │   ├── parser.go          # SessionParser interface
│   │   ├── claude.go          # Claude Code JSONL parser
│   │   └── types.go           # Shared types: Turn, ToolCall, Session
│   ├── topics/
│   │   └── cluster.go         # Topic clustering algorithm
│   ├── editor/
│   │   └── pruner.go          # JSONL rewriting with safe message pairing
│   ├── tokenizer/
│   │   └── estimate.go        # Token count estimation
│   ├── config/
│   │   └── config.go          # Settings management (~/.agentlens/config.json)
│   ├── version/
│   │   └── version.go         # const Version, injected via ldflags on release
│   └── tui/
│       ├── app.go             # Root Bubbletea model, screen routing
│       ├── sessions.go        # Session picker view
│       ├── topics.go          # Topic overview view (shared foundation)
│       ├── replay.go          # Replay mode view
│       ├── editor.go          # Editor mode view
│       ├── help.go            # Help overlay component (? key)
│       ├── search.go          # Fuzzy search bar component (/ key)
│       ├── settings.go        # Settings popup (, key)
│       ├── logviewer.go       # Log viewer (L key)
│       ├── theme.go           # Color scheme definitions
│       ├── keys.go            # Keybinding definitions per screen
│       └── styles.go          # Lipgloss style constants
├── testdata/
│   ├── simple.jsonl           # Simple Claude Code session fixture
│   ├── multi_topic.jsonl      # Multi-topic session with tool calls
│   └── chained.jsonl          # Session continuation chain
├── go.mod
└── go.sum
```

### 2.2 Data Flow

```
Session file(s) on disk
       │
       ▼
  parser.Parse()          →  []Turn (ordered list of messages)
       │
       ▼
  topics.Cluster()        →  []Topic (grouped turns with labels)
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
  pruner.Prune()          →  Rewritten file (selected turns removed)
```

---

## 3. Screens & User Flow

### 3.1 Session Picker

The entry point of the application. On launch, AgentLens scans the default directory for Claude Code (`~/.claude/projects/`) sessions. Users can also pass a custom path via CLI flag.

**Displayed per session:** project name (derived from directory), source badge (Claude Code), total token count (approximate), turn count, topic count, last modified timestamp.

```
┌─ AgentLens ──────────────────────────────────────────────┐
│  Sessions (4 found)                                      │
│──────────────────────────────────────────────────────────│
│                                                          │
│  ▸ REST API project             ~47k tok      2h ago     │
│    12 topics · 34 turns · Claude Code                    │
│                                                          │
│    Auth refactor                ~23k tok      1d ago     │
│    6 topics · 18 turns · Claude Code                     │
│                                                          │
│    Bug hunt #442                ~91k tok      3d ago     │
│    15 topics · 67 turns · Claude Code                    │
│                                                          │
│    Feature planning             ~12k tok      5d ago     │
│    3 topics · 11 turns · Claude Code                     │
│                                                          │
│──────────────────────────────────────────────────────────│
│  j/k Navigate  enter Open  r Replay  e Edit  d Delete    │
│  / Search  , Settings  L Logs  ? Help  q Quit            │
└──────────────────────────────────────────────────────────┘
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
| `/`            | Fuzzy search/filter sessions  | Inline filter bar, fuzzy matching      |
| `,`            | Open settings                 | Popup with current config              |
| `L`            | Open log viewer               | Shows ~/.agentlens/debug.log           |
| `?`            | Show help overlay             | Context-sensitive keybinding reference |
| `q`            | Quit application              |                                        |

#### Session Deletion

When a user presses `d`, a confirmation dialog appears with the session summary and a warning that this is permanent. On confirmation:

- **Claude Code:** Deletes the `.jsonl` file from `~/.claude/projects/<project-dir>/`. If the project directory is now empty, the empty directory is also cleaned up.

### 3.2 Topic Overview (Shared View)

The core screen and shared foundation for both Replay and Edit modes. Displays the parsed session as a list of auto-detected topics, each showing a label, token count, turn count, tool call count, and duration. Topics are collapsible — expanding reveals individual turns with role badges and message previews.

```
┌─ REST API project ───────────────────────────────────────┐
│  34 turns · ~47,231 tokens · 2 hours                     │
│──────────────────────────────────────────────────────────│
│                                                          │
│  1. Project setup & Express init         ██░░   ~12.4k   │
│     turns 1-5 · 8 tool calls · 12 min                    │
│                                                          │
│  2. Authentication with JWT              █░░░    ~8.2k   │
│     turns 6-11 · 4 tool calls · 9 min                    │
│                                                          │
│  3. Where to buy a house                 ▏░░░    ~2.1k   │
│     turns 12-13 · 0 tool calls · 2 min                   │
│                                                          │
│  4. Rate limiting implementation         ██░░    ~9.8k   │
│     turns 14-22 · 11 tool calls · 15 min                 │
│                                                          │
│  5. Error handling & validation          ███░   ~14.7k   │
│     turns 23-34 · 9 tool calls · 18 min                  │
│                                                          │
│──────────────────────────────────────────────────────────│
│  j/k Navigate  enter/→ Expand  r Replay  e Edit          │
│  / Search  tab Stats  ? Help  esc Back                    │
└──────────────────────────────────────────────────────────┘
```

#### Topic Overview Keybindings

| Key            | Action                      | Notes                         |
| -------------- | --------------------------- | ----------------------------- |
| `↑/↓` or `j/k` | Navigate topics             |                               |
| `enter` or `→` | Expand/collapse topic       | Shows individual turns within |
| `r`            | Enter Replay Mode           | Starts from selected topic    |
| `e`            | Enter Edit Mode             | Enables selection checkboxes  |
| `/`            | Fuzzy search within session | Searches turn content         |
| `tab`          | Toggle stats panel          | Right-side aggregate stats    |
| `?`            | Show help overlay           |                               |
| `esc` or `q`   | Back to Session Picker      |                               |

#### Stats Panel

When toggled on, the right side shows: total token count and percentage of context window (200k / 1M), breakdown by message type (user, assistant, tool_use, tool_result, thinking), number of topics detected, total session duration, and number of unique files touched.

### 3.3 Replay Mode

Split-pane view. Left sidebar shows the topic list with the current position highlighted. Main pane shows the full content of the current turn.

```
┌─ Replay ─────────────────────────────────────────────────┐
│  Topics         │  Turn 7/34 · ~890 tok                  │
│─────────────────┼────────────────────────────────────────│
│                 │                                        │
│  1. Setup       │  ● ASSISTANT              +3m 22s      │
│ ▸ 2. Auth  ◂    │                                        │
│  3. House       │  I'll add JWT authentication to the    │
│  4. Rate lim    │  Express app. First, let me install    │
│  5. Errors      │  the dependency:                       │
│                 │                                        │
│                 │  ┌─ Tool: Bash ──────────────────┐     │
│                 │  │ npm install jsonwebtoken       │     │
│                 │  └────────────────────────────────┘     │
│                 │                                        │
│                 │  Then I'll create the auth             │
│                 │  middleware in `src/middleware/`...     │
│                 │                                        │
│─────────────────┴────────────────────────────────────────│
│  ←/h Prev  →/l Next  space Auto-play  1-9 Speed          │
│  ]/n Next topic  [/p Prev topic  t Thinking  ? Help       │
│  w Wrap  / Search  esc Back                               │
└──────────────────────────────────────────────────────────┘
```

#### Turn Display

Each turn in replay shows:

- **Role badge:** Colored label — User (green), Assistant (blue), Tool Use (yellow), Tool Result (dim)
- **Timestamp delta:** Time elapsed since previous turn
- **Token count:** Approximate tokens for this turn
- **Full message content:** Rendered with glamour for markdown formatting and chroma for syntax-highlighted code blocks
- **Tool calls:** Tool name in a bordered box, input parameters as formatted JSON
- **Tool results:** Truncated to 20 lines by default. Press `enter` on a tool result to expand it in a full-screen viewport. Press `esc` to return.
- **Thinking blocks:** Collapsed by default, toggled with `t`. Rendered in dim text.

#### Replay Keybindings

| Key        | Action                 | Notes                                  |
| ---------- | ---------------------- | -------------------------------------- |
| `→` or `l` | Next turn              |                                        |
| `←` or `h` | Previous turn          |                                        |
| `space`    | Toggle auto-play       | Steps at configurable speed            |
| `1-9`      | Set auto-play speed    | 1 = slow (2s), 9 = fast (0.1s)         |
| `]` or `n` | Jump to next topic     |                                        |
| `[` or `p` | Jump to previous topic |                                        |
| `t`        | Toggle thinking blocks | Show/hide extended thinking            |
| `w`        | Toggle word wrap       | Wrap vs horizontal scroll              |
| `enter`    | Expand tool result     | Full-screen viewport for large results |
| `/`        | Search within session  | Fuzzy search, jumps to matching turn   |
| `?`        | Show help overlay      |                                        |
| `esc`      | Back to Topic Overview | Or close expanded tool result          |

### 3.4 Edit Mode

Adds selection controls to the Topic Overview. Users can select entire topics or individual turns within expanded topics. Footer shows a running count of selected items and estimated tokens freed.

```
┌─ REST API project ─── EDIT MODE ─────────────────────────┐
│  Select topics or turns to prune                         │
│──────────────────────────────────────────────────────────│
│                                                          │
│  [ ] 1. Project setup & Express init          ~12.4k     │
│  [ ] 2. Authentication with JWT                ~8.2k     │
│  [x] 3. Where to buy a house                   ~2.1k     │
│  [ ] 4. Rate limiting implementation           ~9.8k     │
│  [ ] 5. Error handling & validation           ~14.7k     │
│                                                          │
│──────────────────────────────────────────────────────────│
│  1 topic selected · ~2,100 tokens freed                  │
│──────────────────────────────────────────────────────────│
│  space Select  a All  A None  p Prune  enter Expand      │
│  / Search  ? Help  esc Cancel                             │
└──────────────────────────────────────────────────────────┘
```

#### Edit Mode Keybindings

| Key     | Action                | Notes                                        |
| ------- | --------------------- | -------------------------------------------- |
| `space` | Toggle selection      | Selecting a topic selects all its turns      |
| `a`     | Select all            |                                              |
| `A`     | Deselect all          |                                              |
| `p`     | Prune selected        | Shows confirmation with token savings        |
| `enter` | Expand/collapse topic | View individual turns for granular selection |
| `/`     | Search                | Filter topics/turns                          |
| `?`     | Show help overlay     |                                              |
| `esc`   | Cancel and return     | Discards selections                          |

#### Prune Confirmation

When `p` is pressed, a confirmation dialog shows: number of turns/topics selected, estimated tokens freed, and a reminder that this rewrites the file and that the user should type `/clear` in Claude Code then resume the session for changes to take effect. The dialog also warns this cannot be undone, though the tool creates a `.bak` backup automatically.

#### Concurrent Access

Only one `agentlens` process should write to a given session file at a time. The pruner takes an advisory file lock (`flock`) on the target `.jsonl` for the duration of the rewrite. If the lock is held, the prune confirmation shows "Session is locked by another process" and the operation is cancelled. Reads (parsing, replay) do not require a lock.

#### Safe Message Pairing

The pruner enforces strict pairing rules to prevent invalid session files:

- User and assistant turns are always deleted as pairs. Cannot delete one without the other.
- `tool_use` and `tool_result` blocks must be deleted together. A `tool_use` without its matching `tool_result` (or vice versa) breaks the session on resume.
- If a user selects only one half of a pair, the pruner automatically includes the other half and shows this in the confirmation.
- System messages and compact summaries (`isCompactSummary: true`) are never selectable.

---

## 4. Global Keybindings & Overlays

These keybindings are available on every screen.

### 4.1 Help Overlay (`?`)

Pressing `?` on any screen displays a centered overlay showing all keybindings for the current view. Follows the pattern used by GitHub's dashboard and lazygit. Dismisses on any keypress. Implemented as a reusable Bubbletea component using `bubbles/help` and `bubbles/key`.

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

Each screen renders its own help content dynamically via a `KeyMap` interface. The help component reads the active screen's `KeyMap` and renders it.

### 4.2 Fuzzy Search (`/`)

Pressing `/` on any list screen opens an inline filter bar at the top of the list. As the user types, items are fuzzy-matched in real time and the list filters to show only matching items. Press `esc` to clear the filter and restore the full list. Press `enter` to select the highlighted result and close the filter.

Uses `github.com/sahilm/fuzzy` for matching. Search targets vary by screen:

- **Session Picker:** Matches against project name and first user message
- **Topic Overview:** Matches against topic labels and turn content
- **Replay Mode:** Matches against turn content, jumps to the matching turn

### 4.3 Settings (`,`)

Opens a centered popup showing current configuration values. Editable inline with `enter` to confirm changes. Saves to `~/.agentlens/config.json`.

Settings for v1:

| Setting                  | Default         | Description                                 |
| ------------------------ | --------------- | ------------------------------------------- |
| `theme`                  | `catppuccin`    | Color scheme                                |
| `gap_threshold`          | `3m`            | Time gap for topic boundary detection       |
| `session_dirs`           | (auto-detected) | Additional directories to scan for sessions |
| `default_context_window` | `200000`        | Context window size for percentage display  |

**Schema evolution rule:** Unknown fields in the config file are ignored with a `warn` log entry. Missing fields are filled with defaults on load and written back on next save. There is no explicit migration step; adding a field is always backwards-compatible. Removing a field requires bumping a `schema_version` integer (absent → 1) and documenting the change in the release notes.

### 4.4 Log Viewer (`L`)

Opens a full-screen viewport showing the tail of `~/.agentlens/debug.log`. Scrollable with `j/k` and `g/G` (top/bottom). Press `esc` to close. Useful for debugging parser issues or seeing why a session failed to load.

### 4.5 Backup Restore

Every prune operation writes a `.bak` sibling file next to the original (e.g. `session.jsonl.bak`). If a pruned session fails to resume in Claude Code, the user can restore from backup:

- In the Session Picker, a session with a `.bak` sibling shows a small `↶` indicator next to the token count.
- Pressing `R` (shift-r) on such a session opens a confirmation dialog: "Restore from backup? This will overwrite the current session file with `<filename>.bak`."
- On confirm, the `.bak` is copied over the original and the backup is preserved until the next successful prune.
- If the user prunes again, the old `.bak` is replaced with a fresh one from the pre-prune state.

The restore action is also surfaced in the prune confirmation dialog's success message: "Session pruned. If resume fails, press R on this session to restore."

---

## 5. Topic Clustering Algorithm

Topic clustering is the core intelligence of the tool. It takes a flat list of turns and groups them into logical conversation topics using heuristics (no LLM calls, fully offline).

### 5.1 Clustering Signals

**Time gaps (strongest signal):** If more than 3 minutes elapse between consecutive turns, a new topic boundary is created. Threshold is configurable via settings (`gap_threshold`).

**File context shifts:** If the set of files referenced in tool calls changes significantly between turns (Jaccard similarity below 0.3 between consecutive file sets), this suggests a topic change.

**Explicit markers:** User messages containing phrases like "let's move on", "new topic", "switching to", "actually, can you", or "unrelated but" are treated as strong topic boundary signals.

**Keyword divergence (weak signal):** Extract the top 5 keywords from each turn using simple frequency analysis. If keyword overlap with the previous turn drops below 20%, this contributes to a boundary score. Only used to confirm boundaries suggested by other signals.

### 5.2 Topic Labels

Generated by extracting the most frequent meaningful keywords from the turns in each topic. For example, if a topic's turns heavily reference "auth", "JWT", and "middleware", the label might be "JWT auth middleware." The first user message is used as a fallback label if keyword extraction produces nothing useful.

---

## 6. Parser Specification

### 6.1 Parser Interface

```go
type SessionParser interface {
    // Parse reads a session from the given path and returns structured data
    Parse(path string) (*Session, error)

    // Write writes a modified session back to disk
    Write(path string, session *Session) error

    // Detect returns true if this parser can handle the given path
    Detect(path string) bool

    // Source returns a human-readable source name ("Claude Code", "OpenCode")
    Source() string
}
```

### 6.2 Claude Code JSONL Format

Each line in a Claude Code JSONL session file is a JSON object with a `type` field:

| Type          | Description                | Key Fields                                                              |
| ------------- | -------------------------- | ----------------------------------------------------------------------- |
| `user`        | User message               | `message.role`, `message.content`, `timestamp`                          |
| `assistant`   | Claude response            | `message.content` (array of text/tool_use/thinking blocks), `timestamp` |
| `tool_result` | Result of a tool call      | `message.content`, `tool_use_id`                                        |
| `system`      | System/compaction messages | `message.content`, `isCompactSummary`                                   |
| `summary`     | Session summary            | Summary text, generated asynchronously                                  |

The parser should ignore unknown types gracefully and log a warning via slog.

#### Session Continuation Chains

Claude Code sessions can span multiple JSONL files when a session is continued after compaction. To reconstruct a complete conversation:

1. Parse all JSONL files in a project directory
2. For each file, extract the session ID from the filename (strip `.jsonl`)
3. If the first `sessionId` in the file differs from the filename ID, the first ID is the parent session
4. Build a parent → child map and follow the chain chronologically
5. Skip records where `isCompactSummary` is true (synthetic summaries)

The parser should present chained sessions as a single logical session in the UI, with a note showing "continued across N files."

### 6.3 Adding Future Parsers

The parser interface is designed so additional formats can be added without modifying existing code. To add a new parser: implement the `SessionParser` interface, add detection logic, and register it in the parser registry. The registry tries each parser's `Detect` method in order until one matches.

---

## 7. Token Estimation

AgentLens estimates token counts for display purposes using a character-based heuristic: divide character count by 3.5 for English text. This gives an approximation within 10-15% of actual Claude tokenization. All token counts are prefixed with `~` in the UI to indicate they are approximate.

If the JSONL records contain actual token usage data (some Claude Code versions include usage fields in assistant messages), the parser should prefer those values over the heuristic.

---

## 8. Color Scheme

### 8.1 Default Theme: Catppuccin Mocha

AgentLens uses Catppuccin Mocha as the default color scheme. It's the most widely adopted terminal color scheme, has excellent contrast, and looks professional across different terminal emulators.

All colors are defined using `lipgloss.AdaptiveColor` so they degrade gracefully on light terminal backgrounds.

```go
// internal/tui/theme.go
var Theme = struct {
    Text        lipgloss.AdaptiveColor // Primary text
    Subtext     lipgloss.AdaptiveColor // Secondary/dim text
    Accent      lipgloss.AdaptiveColor // Highlights, active items
    Surface     lipgloss.AdaptiveColor // Selected row background
    Overlay     lipgloss.AdaptiveColor // Help overlay background
    Border      lipgloss.AdaptiveColor // Box borders
    UserBadge   lipgloss.AdaptiveColor // User message badge
    AsstBadge   lipgloss.AdaptiveColor // Assistant message badge
    ToolBadge   lipgloss.AdaptiveColor // Tool call badge
    ErrorColor  lipgloss.AdaptiveColor // Errors, delete actions
    TokenBar    lipgloss.AdaptiveColor // Token usage bar fill
    TokenEmpty  lipgloss.AdaptiveColor // Token usage bar empty
}{
    Text:       lipgloss.AdaptiveColor{Light: "#4C4F69", Dark: "#CDD6F4"},
    Subtext:    lipgloss.AdaptiveColor{Light: "#6C6F85", Dark: "#A6ADC8"},
    Accent:     lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"},
    Surface:    lipgloss.AdaptiveColor{Light: "#E6E9EF", Dark: "#313244"},
    Overlay:    lipgloss.AdaptiveColor{Light: "#DCE0E8", Dark: "#1E1E2E"},
    Border:     lipgloss.AdaptiveColor{Light: "#ACB0BE", Dark: "#585B70"},
    UserBadge:  lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"},
    AsstBadge:  lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"},
    ToolBadge:  lipgloss.AdaptiveColor{Light: "#DF8E1D", Dark: "#F9E2AF"},
    ErrorColor: lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"},
    TokenBar:   lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"},
    TokenEmpty: lipgloss.AdaptiveColor{Light: "#E6E9EF", Dark: "#313244"},
}
```

### 8.2 Theme Switching

Themes are selectable via the settings popup (`,`) or `--theme` CLI flag. v1 ships with three themes: `catppuccin` (default), `nord`, and `dracula`. Themes are defined as structs implementing the same shape as above. The active theme is stored in `~/.agentlens/config.json`.

### 8.3 Style Constants

Define all styles in `internal/tui/styles.go` using the theme colors. Never use hardcoded color values in view code — always reference the theme struct. Key styles:

- **Title bar:** Bold, accent foreground, border bottom
- **Selected row:** Surface background, accent foreground, bold
- **Role badges:** Colored background with white text, 1-char padding, rounded
- **Token bars:** Block characters (`█` for filled, `░` for empty) using TokenBar/TokenEmpty colors
- **Borders:** Rounded border style (`lipgloss.RoundedBorder()`) using Border color
- **Help overlay:** Overlay background, centered, rounded border, accent-colored header
- **Footer:** Subtext color, keybinding keys in bold accent

---

## 9. Responsive Layout

The TUI must adapt to different terminal sizes. Use `tea.WindowSizeMsg` to detect the terminal dimensions and adjust layout accordingly.

**Minimum size:** 60 columns × 15 rows. Below this, show a "terminal too small" message.

**Replay split pane:** The topic sidebar takes 20% of width (minimum 16 columns, maximum 24 columns). The main pane takes the remainder. If the terminal is narrower than 80 columns, hide the sidebar and show topics as a header bar instead.

**Long content:** Use `bubbles/viewport` for scrollable content in the replay main pane and log viewer. Handle word wrapping with `muesli/reflow` based on available width.

**Loading states:** Large sessions take time to parse. Show a `bubbles/spinner` with "Parsing session..." while loading. For sessions over 10,000 lines, show a progress indicator.

---

## 9.5 Error UX Standard

All user-facing errors go through a single channel so the app feels consistent.

- **Non-blocking errors** (parse warning, fuzzy search no-match, clipboard failure) → one-line toast at the bottom of the screen for 3 seconds, `ErrorColor` foreground, dismissible with any key. Implemented once in `internal/tui/toast.go` and reused by all screens.
- **Blocking errors** (file write failed, prune validation failed, config save failed) → centered modal with the error message, a `What happened` / `What to do` pair of lines, and an `[ OK ]` button. Modal state is owned by the root app model.
- **Log correlation:** Every displayed error writes a matching `error`-level slog entry with the same message and an `err` field. Users can cross-reference in `L` log viewer.
- **Never** use `fmt.Println` or `log.Fatal` from TUI code — the terminal is owned. Use `tea.Println` only for post-exit output.

---

## 9.6 Privacy & Telemetry

AgentLens collects **no telemetry**. No analytics, no crash reporting, no network calls, no update pings. The only network-capable dependency is `glamour` (for rendering images in markdown, which is disabled). If any future feature would phone home, it is opt-in and documented in this section before landing.

Session content never leaves disk. All processing is local. The log file at `~/.agentlens/debug.log` contains metadata only — see LOGGING.md for the "no raw content" rule.

---

## 10. CLI Specification

AgentLens uses Cobra for CLI argument parsing.

| Command / Flag          | Description                    | Default                |
| ----------------------- | ------------------------------ | ---------------------- |
| `agentlens`             | Launch TUI with session picker | Scans default dirs     |
| `agentlens <file>`      | Open a specific JSONL file     | Goes to Topic Overview |
| `--dir <path>`          | Scan a custom directory        | Auto-detected          |
| `--gap-threshold <dur>` | Time gap for topic boundaries  | `3m`                   |
| `--theme <name>`        | Color theme                    | `catppuccin`           |
| `--debug`               | Enable debug logging           | `false`                |
| `--version`             | Print version and exit         |                        |

---

## 11. Tech Stack & Go Best Practices

### 11.1 Go Version

AgentLens targets **Go 1.26** (latest stable, released February 2026). The `go.mod` file should specify `go 1.26`.

> **Note on import paths:** Current Charm library releases use `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2` module paths. Check the latest Charm documentation at release time — if the `charm.land` paths are stable, use those. Otherwise fall back to `github.com/charmbracelet/*`. Pin to specific versions in `go.mod`.

### 11.2 Dependencies

| Package                           | Purpose                                                             |
| --------------------------------- | ------------------------------------------------------------------- |
| `charmbracelet/bubbletea v2`      | TUI framework, application model and event loop                     |
| `charmbracelet/lipgloss v2`       | Terminal styling, colors, borders, layout                           |
| `charmbracelet/bubbles`           | Pre-built components: viewport, list, help, key, textinput, spinner |
| `charmbracelet/glamour`           | Markdown rendering for Claude's responses in replay view            |
| `muesli/termenv`                  | Terminal capability detection (color depth, background)             |
| `muesli/reflow`                   | Word wrapping and text truncation                                   |
| `github.com/spf13/cobra v1`       | CLI argument parsing and subcommands                                |
| `github.com/stretchr/testify v1`  | Testing: `assert`, `require`, `mock` packages — standard everywhere |
| `log/slog` (stdlib)               | Structured logging to file (TUI owns stdout)                        |
| `github.com/sahilm/fuzzy`         | Fuzzy string matching for `/` search                                |
| `github.com/alecthomas/chroma/v2` | Syntax highlighting for code blocks in replay                       |
| `github.com/dustin/go-humanize`   | Human-friendly formatting: "2h ago", "47k", "1.2 MB"                |
| `github.com/fsnotify/fsnotify v1` | OS-native filesystem notifications (future live watching)           |

**Shared with dotcor:** bubbletea, bubbles, lipgloss, termenv, reflow, sahilm/fuzzy, testify, go-humanize — reuse versions/patterns from there where applicable.

**Explicitly not used:** no third-party logging library (stdlib `log/slog` is sufficient), no YAML (config is JSON), no pty/clipboard/diff libraries (not needed for v1 scope).

### 11.3 Logging

**Library choice:** Use stdlib `log/slog` only. No third-party logging library (zap, zerolog, logrus). slog covers structured logging, levels, and handlers; adding another library would be dead weight for a tool this size.

**Conventions:**

- **Destination:** Always `~/.agentlens/debug.log`. Never stdout/stderr — the TUI owns the terminal.
- **Levels:** `info` by default, `debug` when `--debug` is passed. `warn` for recoverable parser issues (unknown JSONL types, malformed records skipped). `error` for failures the user should see (file read errors, prune validation failures) — these also surface in the UI, never only in the log.
- **Structured fields:** Use key/value pairs, not formatted strings. Prefer `slog.Info("parsed session", "path", p, "turns", n)` over `slog.Info(fmt.Sprintf(...))`.
- **Standard keys:** `path` (file path), `session_id`, `turns`, `topics`, `duration_ms`, `err`. Keep keys consistent across the codebase so log grep works.
- **Logger passing:** Create once at startup, set via `slog.SetDefault`. Packages call `slog.Info/Debug` directly — no per-struct logger fields unless a package needs a scoped logger with preset attrs.
- **No secrets or full message content:** log metadata (turn counts, IDs, sizes), not the raw conversation. Session content can include sensitive data from the user's work.

```go
logFile, _ := os.OpenFile(
    filepath.Join(home, ".agentlens", "debug.log"),
    os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644,
)
handler := slog.NewTextHandler(logFile, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
slog.SetDefault(slog.New(handler))
```

### 11.4 Testing

**Framework:** `github.com/stretchr/testify` is the project's testing library. All tests use it — do not mix in other assertion libraries or write raw `if got != want { t.Fatalf(...) }` style checks. Consistency matters more than the specific choice.

**Conventions:**

- `testify/require` for assertions that must pass before continuing (fail-fast: parse succeeded, file exists, no error returned).
- `testify/assert` for non-critical checks where the test should keep running to surface multiple failures.
- `testify/mock` for mocking the `SessionParser` interface and other boundaries.
- Table-driven tests for parser and clustering logic — each case gets a `name` field used as the subtest name.
- Test files sit next to the code they test (`claude_test.go` beside `claude.go`).
- `testdata/` holds sample JSONL fixtures. Fixtures are checked in, not generated.
- Run tests with `go test ./...` before any commit (see CLAUDE.md pre-commit gate).

```go
func TestClaudeParser_ParseUserMessage(t *testing.T) {
    require := require.New(t)

    session, err := parser.Parse("testdata/simple.jsonl")
    require.NoError(err)
    require.Len(session.Turns, 4)

    assert.Equal(t, "user", session.Turns[0].Role)
    assert.Contains(t, session.Turns[0].Content, "build a REST API")
}
```

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

## 12. Implementation Roadmap

Phases are executed sequentially by AI agents. Each phase must be fully complete (including tests) before moving to the next.

### Phase 1: Foundation

- Set up Go project with Bubbletea boilerplate
- Implement Claude Code JSONL parser (`types.go`, `claude.go`)
- Implement token estimation (`estimate.go`) — prefer `usage` fields when present, fall back to char heuristic
- Build Session Picker screen with session scanning
- Session deletion with confirmation dialog
- Set up slog logging and config file

### Phase 2: Topic Clustering

- Implement time-gap based clustering
- Add file-context shift detection
- Add explicit marker detection
- Implement topic labeling
- Build Topic Overview screen
- Add loading spinner for large sessions

### Phase 3: Replay Mode

- Build split-pane replay view with topic sidebar
- Add syntax highlighting via chroma
- Add markdown rendering via glamour
- Implement auto-play with configurable speed
- Add topic jumping and thinking block toggle
- Add expandable tool results viewport

### Phase 4: Edit Mode

- Add selection checkboxes to Topic Overview
- Implement safe message pairing logic
- Build pruner with JSONL rewriting
- Add `.bak` backup file creation
- Add confirmation dialog with token savings
- Add backup restore flow (see §4.5)

### Phase 5: Polish & Global Features

- Implement `?` help overlay component
- Implement `/` fuzzy search with sahilm/fuzzy
- Implement `,` settings popup
- Implement `L` log viewer
- Add Catppuccin, Nord, Dracula themes
- Add responsive layout handling

### Phase 6: Launch

- Write README with screenshots and GIF demos
- Create goreleaser config and Homebrew formula
- Add session continuation chain support for Claude Code
- Submit to awesome-claude-code list
- Write launch blog post / social thread

---

## 13. Future Features (Post-Launch)

These are explicitly out of scope for v1 but are natural extensions:

- **OpenCode parser:** SQLite + file-based + CLI export fallback. Deferred from v1 to keep scope focused on Claude Code.
- **Live session watching:** Use fsnotify to watch an active session and update the TUI in real time, similar to claude-esp. Turns AgentLens from post-hoc analysis into a live companion.
- **Additional parsers:** LangChain traces, Cursor conversation logs, generic JSONL agent logs.
- **Session comparison:** Side-by-side diff of two sessions to understand how different approaches played out.
- **Export:** Generate clean markdown or HTML reports from sessions for documentation or sharing.
- **Token budget visualization:** Show how token usage maps against different model context windows (200k, 1M).
- **PreCompact hook integration:** Optionally install a Claude Code hook that injects a "deprioritize" instruction for topics marked for pruning.

---

## 14. Risks & Mitigations

| Risk                                        | Impact                               | Mitigation                                                                                                              |
| ------------------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| JSONL format changes in Claude Code updates | Parser breaks, sessions fail to load | Pin parser to known format versions. Add format detection with fallback to raw display. Monitor Claude Code changelogs. |
| Large session files (100k+ lines)           | Slow parsing, high memory usage      | Stream-parse JSONL instead of loading entire file. Show spinner/progress during parse. Paginate displayed turns.        |
| Accidental deletion of important sessions   | Data loss                            | Require confirmation dialog. Create `.bak` backup before any write. Show undo hint after deletion.                      |
| Invalid JSONL after pruning breaks resume   | Session cannot be resumed            | Enforce strict message pairing rules. Validate output structure before writing. Always keep `.bak`.                     |
| Topic clustering produces poor groupings    | Confusing UI                         | Make clustering configurable (`gap_threshold`). Allow manual topic boundary insertion in future version.                |
| Charm library import path migration         | Build failures                       | Pin exact versions in go.mod. Document which import paths are used.                                                     |

