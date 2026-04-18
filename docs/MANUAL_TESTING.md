# Seshr Manual Testing Guide

Automated tests (`go test ./...`) cover parser, clustering, pruner, and TUI `Update` logic. They do **not** catch rendering bugs, flicker, color regressions, or keybinding ergonomics. Manual verification is required before declaring any phase complete.

Run these checks on a real terminal (iTerm2, Alacritty, or kitty) at normal size before marking a phase done.

---

## Environment

- macOS or Linux
- Terminal ≥ 80 cols × 24 rows
- A real `~/.claude/projects/` with at least one multi-topic session
- Fresh build: `go build -o seshr ./ && ./seshr`

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
- [ ] Token counts are reasonable (same order of magnitude as Claude Code's own count)
- [ ] Timestamps show "2h ago", "1d ago" (not raw RFC3339)
- [ ] `j/k` navigation wraps or clamps without crashing at list boundaries
- [ ] `d` on a throwaway session → confirmation dialog → file is deleted
- [ ] Deleting the last session in a project directory cleans up the empty dir
- [ ] Pressing `d` then cancelling leaves the file untouched
- [ ] Malformed JSONL line is skipped with a `warn` in the log, not a crash

## Phase 3 — Topic Overview

- [ ] Multi-topic session shows sensible topic boundaries (spot-check 3 real sessions)
- [ ] Topic labels are meaningful, not empty, not truncated mid-word
- [ ] Token bars render using block characters, colors match the theme
- [ ] `enter`/`→` expands a topic inline; `esc` collapses
- [ ] `tab` toggles stats panel; numbers sum to session totals (±1%)
- [ ] Clustering a 100+ turn session shows a spinner, then completes in < 2s

## Phase 4 — Replay Mode

- [ ] Markdown renders (headings, code fences, lists, bold/italic)
- [ ] Code blocks are syntax-highlighted per language
- [ ] Role badges are colored correctly (user green, assistant blue, tool yellow)
- [ ] `→/←` step forward/back without flicker
- [ ] `space` toggles auto-play; `1-9` changes speed audibly (visibly)
- [ ] `]`/`[` jump to next/prev topic
- [ ] `t` toggles thinking blocks (collapsed by default)
- [ ] `enter` on a long tool result opens full-screen viewport; `esc` returns
- [ ] `w` toggles word wrap without losing position
- [ ] Resizing mid-replay reflows text without corruption

## Phase 5 — Edit Mode + Restore

- [ ] `e` enters edit mode; checkboxes appear on topics
- [ ] `space` toggles; footer updates "N selected · ~X tokens freed" live
- [ ] `a` selects all, `A` deselects all
- [ ] Selecting a topic containing a `tool_use` also auto-selects its `tool_result`, visibly
- [ ] `p` shows confirmation with token savings and pair-expansion explanation
- [ ] After prune: `.bak` file exists next to original
- [ ] After prune: reopen the pruned session in Claude Code (`claude --resume`) — it resumes without error
- [ ] Session Picker shows `↶` indicator on sessions with a `.bak` sibling
- [ ] `R` on such a session → confirmation → original content restored byte-for-byte
- [ ] Pruning an already-pruned session replaces the old `.bak` with a fresh one

## Phase 6 — Polish

- [ ] `?` on each screen shows the correct context-sensitive keybindings
- [ ] `/` search filters in real time; `esc` clears; `enter` selects
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

---

## Regression Pass (before any tag)

Run the full Phase 1–6 checklist end-to-end before cutting a release. Manual testing is cheap; a broken release is expensive.
