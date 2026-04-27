# Picker Row Redesign

## Context

The session picker rows are functionally complete but visually bland — flat
colors, no visual hierarchy, and missing useful metadata (turns, cost). This
is a polish pass on the row rendering to make the picker easier to scan at a
glance.

Also fixes a bug where OpenCode live sessions show 0 turns / 0 tokens when
detected before their session appears in the Scan results (synthMetaFromLive
creates entries with no token/turn counts).

## Goals

- Stronger visual hierarchy: bold status text, dimmer metadata, better use of
  the Catppuccin palette
- Richer metadata: turns count on 2nd line, cost when non-zero, token unit
  label
- Status rendered as colored dot + bold text (brighter than current)
- Ended sessions: relative age for recent (< 24h), absolute date for older
- Fix 0 tokens / 0 turns for live OpenCode sessions

## Non-Goals

- Changing row height or layout structure (still 1-line and 2-line rows)
- Adding new columns or fields to SessionMeta
- Changing the divider, group header, or footer rendering

## Design

### Status area (right side of row)

**Live sessions:**
```
● working · fixing failing tests…
```
- Dot stays colored (green/yellow as now)
- Status word ("working" / "waiting") becomes **bold** + colored (brighter)
- CurrentTask remains dim text after the dot+status
- Context warning unchanged

**Ended sessions:**
```
2h ago          (updated < 24h ago)
Apr 24          (updated >= 24h ago)
```
- Relative age for recent sessions using existing `humanize.Time`
- Absolute `Mon DD` for older sessions
- Slightly brighter than current (use `colSubtext0` instead of `colOverlay1`)

### Token format

Add unit label:
```
15.7M tok    (was: 15.7M)
2.5K tok     (was: 2.5K)
47 tok       (was: 47)
```

### Second line (2-line rows)

```
      ~/cs/projects/seshr  ·  47 turns
      ~/cs/projects/other  ·  12 turns  ·  $0.42
```
- Path as now (dim, left-truncated)
- Turns count always shown (needs `TurnCount` on SessionMeta or deriving from
  the scan)
- Cost shown only when > 0, formatted as `$X.XX`

### OpenCode 0-tokens bug

`synthMetaFromLive` creates SessionMeta entries with TokenCount: 0 and no turn
count. When a live session IS already in the scan results, the scan metadata
should have tokens. The bug happens when:
1. The session was synthesized (not in scan)
2. OR the scan results don't have the session yet

Fix: after `syncLiveMetas` merges synthesized entries, the picker should
refresh metadata from the backend on slow ticks so tokens/turns populate once
the session is written to the DB.

Alternative simpler fix: have the live detector return token/turn counts
alongside the LiveSession so `synthMetaFromLive` can populate them directly.
For OpenCode this is a cheap SQL query (the detector already queries the DB).

Going with the simpler fix: add `TokenCount int` and `TurnCount int` to
`LiveSession`, populate them in the OpenCode detector, and use them in
`synthMetaFromLive`.

### Turn count on SessionMeta

Currently `SessionMeta` has no `TurnCount` field. We need one so the second
line can show it. Options:
1. Add `TurnCount int` to `SessionMeta`, populate in Scan
2. Derive from existing data somehow

Going with option 1 — add the field and populate it:
- Claude: count JSONL "type:assistant" messages (or estimate from token count)
- OpenCode: `SELECT COUNT(*) FROM message WHERE session_id = ? AND json_extract(data, '$.role') = 'assistant'`

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Status word style | Dot + bold colored text | User preference; brighter than current plain text |
| Ended time format | Relative < 24h, absolute otherwise | Best of both worlds per user preference |
| Token display | With unit (`15.7M tok`) | User preference |
| 2nd line info | Path + turns + cost (if > 0) | Max info density without clutter |
| 0-token fix | Populate from detector | Simpler than refresh-from-backend approach |
| Turn count source | New SessionMeta field | Clean; both backends can populate |

## Edge Cases

- Sessions with 0 turns (just started): show "0 turns" — honest, not confusing
- Cost is 0 (subscription): don't show cost — clean
- Very long paths + turns + cost on 2nd line: left-truncate path to fit
- Token count of 0: show "0 tok" — consistent

## Open Questions

None — all resolved in discussion.
