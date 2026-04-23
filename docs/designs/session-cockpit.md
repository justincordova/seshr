# Session Cockpit

> **Status:** Vision doc, deferred. Ship after OpenCode parity (Phases 8–11) lands.
> **Captured:** April 2026.

## Context

The current per-session "Landing Page" (SPEC.md §3.2) is a thin summary screen
reached by pressing `enter` on a picker row. It shows the same information as
Topic Overview plus a one-line current-action. For ended sessions it adds
nothing meaningful over Topics; for live sessions it surfaces only `CurrentTask`
+ `LastActivity` + a context-% warning. This makes the landing page feel like
an unnecessary intermediate step — which it currently is.

This doc proposes evolving the Landing Page into a **Session Cockpit**: an
abtop-inspired live dashboard that shows token rate, context window with
compaction marks, quota (Claude only), current task, recent action timeline,
subagent tree, child processes, and memory. The cockpit is shown only for
live sessions. Ended sessions skip it and go directly to Topic Overview.

Inspiration: [graykode/abtop](https://github.com/graykode/abtop) — a
multi-session htop-style monitor for AI agents. The cockpit applies that
information density to a single-session view inside seshr.

## Goals

- Make the landing screen earn its place in the navigation. If we make the
  user press `enter` for a screen, that screen needs to show something they
  can't get elsewhere.
- Surface real-time telemetry (token rate, quota %, current tool, child
  processes) for live sessions in one place.
- Build on data we already collect — `Session.Turns`, `LiveSession`,
  `ProcessSnapshot.Children`, `CompactBoundaries` — minimize new collectors.
- Preserve the conditional-routing rule: ended sessions skip the cockpit.

## Non-Goals

- A multi-session view (that's the picker; not duplicating).
- Network calls to providers for live quota — Claude quota comes from the
  local Anthropic CLI state file only. If reverse-engineering proves brittle,
  the quota panel ships hidden behind a config flag.
- OpenCode quota panel. OpenCode supports many providers; presenting one
  unified quota gauge is misleading. OpenCode cockpit shows everything
  except the quota panel.
- Theme cycling, --once mode, tmux jump — out of scope.
- Open-port detection — possibly v1.1+, deferred.

## Design

### Routing change

Picker `enter` behavior becomes conditional on session state:

- **Live session →** Session Cockpit (`stateCockpit`, formerly `stateLanding`).
- **Ended session →** Topic Overview (`stateOverview`) directly.

Topic Overview gains a `c` resume keybinding so the resume action remains
reachable without forcing every user through an intermediate screen.

### Layout (target ≈100 cols, boxed and centered)

```
┌─ ◆ Seshr · Cockpit ─────────────────────────────────────────┐
│  bb859dee-… · boot · claude · WORKING ●        4s ago       │
│  838 turns · 65.7M tok · 4 compactions                      │
├─────────────────────────────────────────────────────────────┤
│  Context  ████████████████████████████░░░  85% ⚠            │
│           ↑c ↑c ↑c ↑c   (4 compactions)                     │
│                                                              │
│  Quota    5h:  ███████░░░  72%  resets in 1h 14m            │
│           7d:  ████░░░░░░  43%  resets Mon 9am              │
│                                                              │
│  Tokens   65.7M total · 24 tok/s last min · 18 tok/s 5m avg │
│           ▌ user 32.9K   ▌ AI 65.0M   ▌ tool 680K           │
│                                                              │
│  Memory   412 MB rss   PID 14821                            │
├─────────────────────────────────────────────────────────────┤
│  CURRENT                                                     │
│  ▶ Edit src/bot/strategy.go                       4s · now  │
├─────────────────────────────────────────────────────────────┤
│  TIMELINE   (last 8)                                         │
│  Edit  strategy.go              now                          │
│  Bash  go test ./...            8s ago                       │
│  Read  strategy_test.go         12s ago                      │
│  Edit  config.go                25s ago                      │
│  Task  refactor-pricing  ▸      40s ago                      │
│   └ Read pricing.go             36s ago                      │
│   └ Edit pricing.go             32s ago                      │
│  Bash  git status               1m ago                       │
├─────────────────────────────────────────────────────────────┤
│  CHILDREN   2 procs                                          │
│   14855  go test ./...                                       │
│   14903    └ go vet                                          │
├─────────────────────────────────────────────────────────────┤
│  t topics  ·  r replay  ·  c resume  ·  esc back            │
└─────────────────────────────────────────────────────────────┘
```

Each panel renders only when its data is available. Sessions with no
children, no compactions, or no quota data hide the corresponding panels —
the cockpit shrinks gracefully. Minimum useful render: header + context
gauge + tokens + current.

### Panel descriptions

**Header** — id, project, source, status, last-activity. Pulsing `●` per the
existing live-pulse animation.

**Context gauge** — proportional bar. Marks (`↑c`) along the bar at compaction
boundary positions, computed from `len(CompactBoundaries)` and the relative
position of each boundary in the turn sequence. Warning color at ≥80%, red
at ≥95%.

**Quota panel** (Claude only) — reads Anthropic CLI's local state file
(path TBD; see Open Questions). Two bars: 5-hour rolling and 7-day rolling.
Shows reset time. Hidden if state file not found or unreadable. Hidden
entirely for OpenCode.

**Tokens panel** — total + sliding-window rate. Two rates:
- **Last minute:** total tokens generated by turns whose timestamp is within
  the last 60s, divided by 60.
- **5-min average:** same, 300s window.

The ▌ segments duplicate the existing collapsed token bar from the current
landing page. Keep them — three small swatches read faster than three
labeled lines.

**Memory panel** — `ProcInfo.RSSKB` for the agent PID, formatted as MB.
Already collected by `ProcessScanner`.

**Current panel** — most-recent assistant turn's last `tool_use`. For Claude,
prefix is the tool name; arg is the first stringified parameter. Truncated
at 60 chars. Elapsed time = `now - LastActivity`.

**Timeline panel** — last N (default 8) tool calls across all assistant turns,
oldest at bottom. Indentation conveys subagent nesting (see below).

**Subagent tree** — when an assistant turn invokes the `Task` tool (Claude's
subagent spawner), subsequent turns until the matching `tool_result` are
nested one level under the `Task` line. Multiple concurrent Tasks are rare
but should each get their own indent root. OpenCode has no equivalent
construct in v1; subagent panel is Claude-only.

**Children panel** — derived from `ProcessSnapshot.Children[agentPID]`.
Shows command + PID for each child. Recursive one level (children of
children rendered nested).

### Refresh strategy

- Cockpit subscribes to the existing fast-tick (2s) for `Live`,
  `CurrentTask`, `LastActivity`, child-proc snapshot, RSS.
- Cockpit subscribes to slow-tick (10s) for context % refresh and quota
  re-read.
- Token-rate computation runs on every cockpit re-render — cheap, just
  walks the recent tail of `Session.Turns`.

### Keybindings

| Key | Action |
| --- | --- |
| `t` | Topic Overview |
| `r` | Replay Mode |
| `c` | Resume overlay |
| `i` | Info overlay (full session metadata) |
| `ctrl+l` | Jump to picker live-only |
| `esc` | Back to picker |
| `?` | Help |

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Conditional routing | Live → cockpit, ended → topics | The cockpit's value is real-time data. For ended sessions Topics is the right destination; the cockpit would be visual noise. |
| Rename | `Landing Page` → `Session Cockpit` | "Landing" implies a stop-over. "Cockpit" implies a workspace. Matches the actual purpose. |
| Per-source feature parity | Quota Claude-only; subagent tree Claude-only | OpenCode's multi-provider model breaks the "show one quota gauge" assumption. Subagent equivalent doesn't exist in OC v1. Hide rather than fake. |
| Token rate windows | 1-min and 5-min | One snapshot rate is misleading (flips wildly). Two windows give context: short = current burst, long = sustained pace. |
| Quota panel resilience | Hidden when state file unreadable | Better silent absence than a broken or stale gauge. Optional config flag to hard-disable for users on enterprise plans without local state. |

## Rejected Alternatives

- **Delete the landing page entirely.** Originally my recommendation when the
  page was just a redundant summary. Reversed once the abtop-inspired
  cockpit redesign was on the table — that version *does* earn its place.
- **Show the cockpit for ended sessions too (with degraded data).** Greys
  out 60% of the panels for the most common flow. Friction without payoff.
- **Replace Topic Overview with a cockpit-style screen.** Too ambitious;
  Topic Overview's pruning workflow is its own thing and doesn't belong in
  the cockpit.
- **Embed the cockpit as a sidebar in Topic Overview.** Overloads the screen,
  and the cockpit's refresh rate would force re-renders that disrupt scroll
  and selection state on Topics. Separate screens, separate concerns.

## Edge Cases & Constraints

- **No live data yet:** if the cockpit opens on a session that just
  transitioned ended → live, `Live` may be `nil` for one tick. Render the
  static panels (context, tokens, memory if PID known) and show a
  `waiting for live tick…` placeholder in the current/timeline/children
  panels.
- **Session became ended while cockpit is open:** transition to Topic
  Overview automatically with a one-shot toast `session ended — switched to
  topics`. Avoids leaving the user staring at frozen telemetry.
- **Tiny terminals (< 80 cols):** drop the timeline and children panels;
  keep header, context, tokens, current. Below 60 cols falls back to the
  existing min-size error message.
- **Quota state file changes format:** version-detect; fail closed (hide
  panel) rather than crash. Log via slog at warn level once per session.
- **Rate-limit reverse engineering ages out:** Anthropic ships a CLI
  update; our parser breaks; cockpit hides quota panel; user sees no
  regression on other panels. Acceptable degradation.

## Open Questions

- **Anthropic local state path.** `~/.claude/credentials.json`?
  `~/.claude/state.json`? Needs investigation against current Claude Code
  versions. Spike before scoping the quota panel into a phase.
- **Subagent detection from JSONL.** Need to confirm that nested `Task`
  invocations write recognizable parent-child markers in the JSONL. If
  not, the subagent tree degrades to a flat timeline.
- **Children panel update cadence.** `ProcessScanner.Scan` is currently
  driven only by the slow ticker. Children of agent processes can spawn
  and die quickly; consider promoting agent-PID child enumeration to the
  fast ticker.

## Implementation Phases (when picked up)

This doc deliberately does NOT scope the work into a phase plan — that's a
separate exercise once OpenCode parity (Phases 8–11) ships. Rough sketch
for future planning:

1. **Spike:** Anthropic quota state file investigation (1 session).
2. **Cockpit core:** Routing change, scaffold + header + context gauge +
   tokens + current panel.
3. **Cockpit advanced:** Timeline, subagent tree, children, memory, quota
   (if spike succeeded).
4. **Polish:** Edge cases, transitions (live → ended toast), responsive
   sub-80-col layout.

## Related

- Current landing implementation: `internal/tui/landing.go`.
- Live data sources: `internal/tui/live_index.go`,
  `internal/backend/process.go`.
- Conditional-routing change shipping ahead of this doc:
  `docs/plans/landing-fixes-plan.md`.
