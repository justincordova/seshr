# Seshr Manual Testing Guide

Automated tests (`go test ./...`) cover parser, clustering, pruner, and TUI `Update` logic. They do **not** catch rendering bugs, flicker, color regressions, or keybinding ergonomics. Manual verification is required before declaring any phase complete.

Run these checks on a real terminal (iTerm2, Alacritty, or kitty) at normal size before marking a phase done.

---

## Environment

- macOS or Linux
- Terminal ≥ 80 cols × 24 rows
- A real `~/.claude/projects/` with at least one multi-topic session
- Fresh build: `just build && ./seshr`

Tail the log file in a second pane while testing:

```bash
tail -f ~/.seshr/debug.log
```

---

## Phase 1 — Scaffolding

- [ ] `./seshr --version` prints a version and exits 0
- [ ] `./seshr --debug` launches the placeholder TUI
- [ ] `q` quits cleanly, exit code 0, terminal is restored (no garbled state)
- [ ] Log file exists at `~/.seshr/debug.log` and contains a structured `seshr starting` line
- [ ] Resizing the terminal during runtime does not crash

## Phase 2 — Parser & Session Picker

- [ ] Real sessions from `~/.claude/projects/` appear in the picker
- [ ] Token counts are in the right ballpark (estimated from file size in picker; exact on open)
- [ ] Timestamps show "2h ago", "1d ago" (not raw RFC3339)
- [ ] `j/k` navigation wraps or clamps without crashing at list boundaries
- [ ] `d` on a throwaway session → confirmation dialog → file is deleted
- [ ] Deleting the last session in a project directory cleans up the empty dir
- [ ] Pressing `d` then cancelling leaves the file untouched
- [ ] Malformed JSONL line is skipped with a `warn` in the log, not a crash

## Phase 3 — Topic Overview

- [ ] Multi-topic session shows sensible topic boundaries (spot-check 3 real sessions)
- [ ] Topic labels are meaningful, not empty, not truncated mid-word
- [ ] Token counts per topic render correctly; pre-compact topics show ░ indicator
- [ ] `enter`/`→`/`l` expands a topic inline showing turn previews
- [ ] `f` folds/unfolds all expanded topics
- [ ] `space` toggles topic selection; selection strip updates with token count and safety indicator
- [ ] `a` toggles select all/deselect all
- [ ] `p` shows confirmation with token savings and context-aware safety message
- [ ] `tab` toggles stats panel; numbers sum to session totals (±1%)
- [ ] `/` fuzzy-searches topic labels and turn content
- [ ] Clustering a 100+ turn session completes in < 2s

## Phase 4 — Replay Mode

- [ ] Markdown renders (headings, code fences, lists, bold/italic)
- [ ] Code blocks render in a bordered panel with formatted JSON
- [ ] Role badges are colored correctly (user green, assistant blue, tool result lavender)
- [ ] `→/←` or `l/h` step forward/back without flicker
- [ ] `space` toggles auto-play; `+/-` adjusts speed during auto-play
- [ ] `]`/`[` jump to next/prev topic
- [ ] `t` toggles thinking blocks (collapsed by default)
- [ ] `c` toggles slim mode (hides non-Agent tool calls/results)
- [ ] `enter` on a long tool result or continuation summary opens full-screen viewport; `esc` returns
- [ ] `tab` toggles sidebar focus; `j/k` navigates topic list; `enter` jumps to topic
- [ ] Resizing mid-replay reflows text without corruption

## Phase 5 — Prune + Restore

- [ ] `space` selects a topic; selection strip shows token savings and safety indicator
- [ ] Selecting a topic containing a `tool_use` also auto-selects its `tool_result`
- [ ] `p` shows confirmation with context-aware safety message (pre-compact vs active vs mixed)
- [ ] After prune: `.bak` file exists next to original
- [ ] After prune: reopen the pruned session in Claude Code (`claude --resume`) — it resumes without error
- [ ] Session Picker shows `↶` indicator on sessions with a `.bak` sibling
- [ ] `R` on such a session → confirmation → original content restored byte-for-byte
- [ ] Pruning an already-pruned session replaces the old `.bak` with a fresh one

## Phase 6 — Polish

- [ ] `?` on each screen shows the correct context-sensitive keybindings
- [ ] `/` search filters in real time; `esc` clears; `enter` commits
- [ ] `,` settings popup reads current config and writes changes to `~/.seshr/config.json`
- [ ] `L` log viewer shows tail of debug.log; `j/k`, `g/G` work
- [ ] Theme switch (`--theme nord` / `--theme dracula`) changes all colors
- [ ] Narrow terminal (60×15): sidebar collapses, layout still legible
- [ ] Terminal below minimum: friendly "too small" message, no crash

## Phase 7 — Launch

- [ ] Fresh binary from goreleaser artifact launches on clean macOS + Linux
- [ ] Homebrew install flow end-to-end
- [ ] `--version` prints the git tag via ldflags (not `dev`)
- [ ] Continuation chain session from multiple JSONL files presents as one session with "continued across N files" note

## Phase 7 — Landing page, resume overlay, and picker polish

- [ ] Enter on picker opens landing page (not topics directly)
- [ ] Landing page shows correct content for live vs ended sessions
- [ ] `c` key on landing opens resume overlay; enter copies to clipboard; `✓ copied` appears and fades after 2s
- [ ] `esc esc` from landing page returns to picker at the same row
- [ ] First-launch welcome banner shows on fresh config; dismissed on any keypress; never shown again
- [ ] Resize terminal across 70 / 90 / 120 cols; picker rows adapt layout correctly
- [ ] Badge column visible at ≥100 cols, hidden at 80-99, narrow mode at <80
- [ ] `t` key on landing goes to Topic Overview; `r` goes to Replay

---

## Phase 6 — Live tickers, hysteresis, and detection banner

- [ ] Fast tick: send a message in claude; seshr reflects the new turn within ~2s (once fully wired in Phase 6 fast tick)
- [ ] Hysteresis: briefly make Claude idle (status momentarily Waiting); the picker row does not flicker to Waiting for at least 20s
- [ ] Slow tick failure banner: temporarily alias ps to a non-existent bin; after ~30s the banner appears; restore and confirm it disappears on next success
- [ ] Overlays suspend tickers: open help (?); verify no rerender storms during a live tail
- [ ] Context warning: synthesize a live session with ContextTokens > 80% of ContextWindow; row shows `ctx N% ⚠`

---

## Phase 4 — Claude live detection

- [ ] Launch `claude` in another terminal; seshr shows the session as live within 10s
- [ ] Kill claude; the row reverts to ended within 20s (hysteresis lands in Phase 6; pre-hysteresis this is immediate)
- [ ] `./seshr --no-live` launches with all sessions shown as ended; no pulse dots visible
- [ ] Badge column reads `claude` on Claude Code sessions
- [ ] Live row shows `● working · <task>` (green) or `● waiting` (yellow) correctly

---

## Regression Pass (before any tag)

Run the full Phase 1–6 checklist end-to-end before cutting a release. Manual testing is cheap; a broken release is expensive.
