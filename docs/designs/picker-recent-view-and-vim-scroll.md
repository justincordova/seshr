# Picker Recent View, Pinned Live Group, and Vim Scroll Bindings

## Context

The session picker currently groups sessions by project. Within each project group, live sessions float to the top — but the *groups themselves* sort by `UpdatedAt`, so a live session in an old project ends up at the bottom of the list. The user has to scroll past stale projects to find active work. SPEC §3.1 calls for project groups containing live sessions to sort first; this was never implemented (`internal/tui/picker_groups.go:78-80`).

Additionally, session rows display full UUIDs (e.g. `bb859dee-0744-…`) which eat horizontal space and provide no project context when groups are collapsed.

This design introduces a default flat "Recent" view with live sessions pinned at the top, a toggle to the existing project-grouped view (now with a pinned `LIVE` group), shorter session ids, two-line rows in Recent view for project-path context, and vim-style scroll bindings (`g`, `G`, `ctrl+u`, `ctrl+d`) across all scrollable surfaces.

## Goals

- Live sessions are always immediately visible without scrolling.
- Default view is a flat, recency-sorted list (Recent) — no toggle needed for the common case.
- Project-grouped view remains available for users who think project-first.
- Session rows surface the project path inline so collapsed/flat views stay informative.
- Session ids are short and human-readable (`sesh_bb859d`, `ses_23afb0`).
- Vim scroll bindings work uniformly across every scrollable view in seshr.

## Non-Goals

- No live-only filter mode (`l` keybinding from SPEC §3.1 stays out of scope for this change).
- No live pulse animation (still deferred per existing TODOs).
- No changes to search, delete, replay, restore, or any non-display picker behavior.
- No changes to the topic overview, replay, landing, or settings *layouts* — only their scroll bindings.

## Design

### Two view modes

The picker tracks a `viewMode` field with two values:

- `ViewRecent` (default) — flat list of all sessions sorted by `UpdatedAt` desc, with live sessions pinned at the top.
- `ViewProject` — current grouped-by-project layout, plus a synthetic `◉ LIVE` group always pinned at the top containing every live session across all projects.

A new keybinding `v` cycles between the two modes. Mode is persisted to config (`PickerViewMode` in `~/.seshr/config.toml`) so it survives restarts.

### Recent view layout

```
┌─ seshr · sessions ─────────────────────────────────────────────────────────┐
│  47 sessions · 142M tokens · LIVE 2                                        │
│                                                                            │
│  ▌ ● sesh_bb859d                claude   15.7M   working · fixing tests    │
│  │   ~/cs/projects/seshr                                                   │
│  ▌ ● ses_23afb0                 opencode  8.2M   waiting                   │
│  │   ~/cs/projects/dotfiles                                                │
│  ────────────────────────────────────────────────────────────────────────  │
│  ▌ ▸ sesh_a91c4d                claude    4.1M   2 hours ago               │
│  │   ~/cs/projects/seshr                                                   │
│  ▌ ▸ ses_77eb31                 opencode  912K   yesterday                 │
│  │   ~/cs/projects/web-app                                                 │
└────────────────────────────────────────────────────────────────────────────┘
```

- Each session is a two-line row.
  - Line 1: gutter (project color) + status glyph + short id + source badge + tokens + status/age.
  - Line 2: dim, indented under the id; project path with `~` substitution and **left-truncation** (`…/projects/seshr`) when too long.
- Live sessions render at the top, sorted by status class (Working → Waiting → Ambiguous), then by `UpdatedAt` desc.
- A subtle dim divider line `─────` separates the live block from ended sessions. The green/yellow `●` glyphs remain on live rows — the divider is additive, not a replacement.
- Ended sessions render below the divider sorted by `UpdatedAt` desc.
- No section headers ("LIVE" / "RECENT" labels) — divider + glyphs are enough signal.

### Project view layout

```
┌─ seshr · sessions ─────────────────────────────────────────────────────────┐
│  47 sessions · 142M tokens · LIVE 2                                        │
│                                                                            │
│  ▌ ◉ LIVE                                       ▾ 2 sessions  23.9M tok    │
│  ▌ ● sesh_bb859d                claude   15.7M   working · fixing tests    │
│  │   ~/cs/projects/seshr                                                   │
│  ▌ ● ses_23afb0                 opencode  8.2M   waiting                   │
│  │   ~/cs/projects/dotfiles                                                │
│                                                                            │
│  ▌ SESHR                                        ▸ 12 sessions  47.2M tok   │
│  ▌ DOTFILES                                     ▸ 8 sessions   18.1M tok   │
└────────────────────────────────────────────────────────────────────────────┘
```

- A synthetic `◉ LIVE` group is always pinned at the top when at least one live session exists. Group color is `theme.Success` (green). Always starts expanded; collapse state is not persisted across runs.
- Live sessions appear **only** in the LIVE group, not duplicated in their project group below. When a session ends, it falls out of LIVE and reappears in its project group on the next reconcile.
- Inside the LIVE group, session rows are **two lines** (id + project path) since they span multiple projects.
- Regular project groups remain **one line** per session (project context already in the group header — adding it per-row would be redundant).
- Regular project groups sort by `UpdatedAt` desc as today. With live work hoisted to LIVE, the "old project at bottom" problem is solved.

### Short session ids

Today: `bb859dee-0744-4c12-9a3e-…` (UUIDs and similar opaque ids).

New format:
- Claude Code: `sesh_<first 6 hex of UUID, lowercase, dashes stripped>` → `sesh_bb859d`
- OpenCode: `ses_<first 6 chars of session id>` → `ses_23afb0`

The two prefixes (`sesh_` for claude, `ses_` for opencode) provide a glanceable visual distinction even when the source badge column is dropped at narrow widths. The full id remains in `SessionMeta.ID` and is used for all internal lookups; only the *display* changes.

A small helper `shortID(kind session.SourceKind, id string) string` lives in `internal/tui/sessions.go` next to `sourceBadge`. Collisions are theoretically possible but vanishingly rare for 6 hex chars (~16M space) within a single user's session list; we don't disambiguate.

### Two-line row mechanics

`renderSessionRow` currently returns a 1-line string. We change the signature to return `(string, int)` where the int is the row height (1 or 2). The caller (`Picker.View`) uses the height to advance its line cursor.

`rowHeight(row PickerRow) int` becomes:
- Group header: 1
- Session row in Recent view: 2
- Session row in LIVE pinned group (Project view): 2
- Session row in regular project group (Project view): 1

`visibleCount()` and `clampOffset()` need to account for variable row heights. Simplest approach: convert the offset/cursor model from "row index" to "row index" but compute pixel-style line totals when paginating. We track:
- `cursor int` — index into `flatRows` (unchanged).
- `offset int` — first visible `flatRows` index (unchanged).
- A new helper `visibleLineCount(rows, offset, height) int` walks rows from offset summing `rowHeight` until the available terminal height is exhausted, returning how many rows fit.

Selection highlighting spans both lines of a 2-line row — line 2 (project path) gets the same selected/unselected treatment as line 1 (just dimmer).

### Vim scroll bindings

A new `ScrollKeys` struct in `internal/tui/keys.go`:

```go
type ScrollKeys struct {
    Top      key.Binding  // "g"
    Bottom   key.Binding  // "G"
    PageDown key.Binding  // "ctrl+d"
    PageUp   key.Binding  // "ctrl+u"
}

func DefaultScrollKeys() ScrollKeys {
    return ScrollKeys{
        Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
        Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
        PageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "page down")),
        PageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "page up")),
    }
}
```

`ScrollKeys` is embedded into every keymap struct that owns a scrollable view: `PickerKeys`, `OverviewKeys`, `ReplayKeys`, plus the help, log viewer, settings, and landing/cockpit models. Each view's `Update` handles them by adjusting its `cursor`/`offset` (or scroll position) accordingly.

Conventions:
- `g` jumps to the first row, sets offset to 0.
- `G` jumps to the last row.
- `ctrl+d` moves cursor down by `visibleCount() / 2` rows, clamping.
- `ctrl+u` moves cursor up by `visibleCount() / 2` rows, clamping.
- For views without a "cursor" (transcript view in replay, log viewer, help overlay), the bindings adjust the scroll offset directly rather than a cursor.

`g` does **not** require a leading no-op as in vim (`gg`); a single `g` jumps to top. Simpler, and there's no other `g`-prefixed binding to disambiguate from.

### Search behavior in Recent view

Search remains live and incremental. Haystack stays as `project + " " + id` (using full id for matching, not shortened). When matches narrow the list, the Recent view still pins matched live sessions at the top and divider stays in place if any live session matches.

### Footer hints

Picker footer is updated to reflect new bindings. Existing footer (`sessions.go:735`) currently shows 5 hints; new version:

```
↑↓/jk nav · gG top/bot · ^d/^u page · enter open · v view · / search · q quit
```

Other views' footers add a compact `gG top/bot · ^d/^u page` segment.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Default view | `ViewRecent` (flat) | Solves the scrolling complaint without a toggle. Live always visible. |
| View toggle key | `v` | Unused in picker keymap. Mnemonic for "view." |
| Persist view mode | Yes, in config | Users settle on a preference; respect it across runs. |
| Live separator in Recent | Subtle dim divider line | User preference. Keeps glyphs as primary signal. |
| Live duplication in Project view | No — LIVE group only | Avoids two rows for the same session. Falls back to project group when ended. |
| LIVE group expanded by default | Yes | Defeats the point of pinning if collapsed. |
| Two-line rows scope | Recent view + LIVE group only | Project groups already show project name in header. |
| Two-line row order | id on top, path below (dim) | Identifying info first, metadata second. Standard scan pattern. |
| Path truncation | Left-truncate (`…/projects/seshr`) | Project basename is the meaningful part — keep it visible. |
| Short id format | `sesh_` / `ses_` prefix + 6 hex | Distinguishable at a glance, no badge needed. Display-only. |
| Vim scroll scope | All scrollable views | User explicit ask. Uniform mental model. |
| `g` vs `gg` | Single `g` | No conflict, simpler. |

## Rejected Alternatives

- **Tri-state toggle (Recent | Project | Live).** Rejected per user — live-only is unnecessary noise; pinning live to the top of both views covers the use case.
- **Live sessions duplicated in both LIVE group and project group.** Rejected — visual duplication is confusing and consumes vertical space.
- **Section labels ("LIVE" / "RECENT" headers) in Recent view.** Rejected — divider + glyphs already signal the boundary; labels would be loud.
- **Two-line rows everywhere including project-grouped sessions.** Rejected — redundant when project name is already in the group header.
- **Path-on-top, id-below row order.** Rejected after discussion — id is the primary identifier and should be on the brighter top line.
- **`gg` for top (vim-strict).** Rejected — single `g` is unambiguous in this app and avoids implementing a key-sequence buffer.
- **Per-view custom scroll bindings.** Rejected — uniform vim bindings across all scrollable surfaces matches user expectation and reduces cognitive load.

## Edge Cases & Constraints

- **Empty live set in Project view.** When no live sessions exist, the LIVE group is omitted entirely (not rendered as an empty group).
- **Empty live set in Recent view.** Divider line is omitted; rows render as a single recency-sorted block.
- **Synthesized live sessions** (live process detected but no transcript yet — `syncLiveMetas`) appear in both views as expected. In Project view they go into LIVE; in Recent they sit in the live block with whatever placeholder metadata they have.
- **Search results that match only ended sessions** (no live matches): no divider line in Recent view since there's nothing above it to separate.
- **Search results that match only live sessions:** still show the divider for visual consistency (dividing live block from empty ended block) — actually, no: if there are zero ended matches, omit the divider to avoid a stray line. Edge case codified in `renderRecentView`.
- **Two-line rows + narrow terminals (<80 cols).** Path line still renders but with aggressive left-truncation. If width drops below ~40, project path line is omitted entirely (1-line fallback).
- **Cursor on the path line of a 2-line row.** Not possible — a 2-line row is a single selectable unit. `cursor` indexes `flatRows`, which has one entry per logical row regardless of height.
- **`ctrl+d` / `ctrl+u` in views with `Quit` bound to `ctrl+c` only.** Safe — `ctrl+d` and `ctrl+u` are unbound today across all picker/overview/replay keymaps.
- **`g` conflict in OverviewKeys.** None — `g` is unbound. (`l` is bound there for "expand," but that's irrelevant.)
- **`G` conflict in ReplayKeys.** None — only `+`, `-`, `t`, `c`, brackets are used.
- **Persisted view mode + first-run.** If config is missing the field, default to `ViewRecent`.

## Open Questions

None — all resolved during brainstorm.
