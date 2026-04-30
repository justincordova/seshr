# Picker Row Redesign

## Context

The session picker rows have poor visual hierarchy — session ID, source badge,
tokens, age, path, turns, and cost all blend together as an undifferentiated
string. Users can't quickly scan for claude vs opencode sessions or compare
token counts across rows.

## Goals

- Instant visual differentiation between claude and opencode sessions
- Clear information hierarchy: ID → source → tokens → age on line 1, path + metadata on line 2
- Right-aligned token count and age columns so values line up vertically
- Subtle source pill badge — readable without being loud

## Design

### Line 1 — Identity row

```
 ▌  ▸ sesh_639845b9  [claude]  ..............  1.4M tok  6 minutes ago  ↶
```

Elements (left to right):
1. **Gutter** — `▌` in project color (existing, unchanged)
2. **Glyph** — `▸` ended / `●` live (existing, unchanged)
3. **Session ID** — `sesh_` prefix + 8-char body in foreground (bold when selected)
4. **Source pill** — `[claude]` or `[opencode]` with surface0 background, source-colored foreground text. Rounded brackets via `[`/`]` chars. Width-padded to 10 chars so columns align. Claude = mauve foreground, OpenCode = blue foreground.
5. **Gap** — spaces filling to the right-aligned columns
6. **Token count** — right-aligned to a fixed column (e.g., col 60), dim style
7. **Age** — right-aligned after tokens, dim style
8. **Backup indicator** — `↶` in green when backup exists (existing, unchanged)

### Line 2 — Context row (indented)

```
       ~/cs/projects/riverineranch  ·  453 turns
       ~/cs/dotcor  ·  692 turns  ·  $0.11
```

- 6-space indent
- Path in subtext0, left-truncated (existing behavior)
- Turns count after `·` separator
- Cost after turns when present (opencode sessions)

### Source pill implementation

```go
func sourcePill(kind session.SourceKind) string {
    switch kind {
    case session.SourceClaude:
        // surface0 background, mauve foreground, 10-char padded
        return lipgloss.NewStyle().
            Background(colSurface0).
            Foreground(colMauve).
            Padding(0, 1).
            Render("claude")
    case session.SourceOpenCode:
        return lipgloss.NewStyle().
            Background(colSurface0).
            Foreground(colBlue).
            Padding(0, 1).
            Render("opencode")
    default:
        return lipgloss.NewStyle().
            Background(colSurface0).
            Foreground(colSubtext0).
            Padding(0, 1).
            Render(string(kind))
    }
}
```

### Right-alignment approach

Replace the current gap-fill logic with explicit right-alignment of the token
and age columns. Compute the right section width at render time and use
`lipgloss.Place` or manual padding to align tokens to a consistent column
position within the content width.

The right section is: `tokStr + "  " + ageStr + backup`. Compute its rendered
width, then the gap is `contentWidth - leftWidth - rightWidth`.

### Width tiers

- **≥ 80 cols:** Full 2-line layout with pill badge, tokens, age, path, turns
- **< 80 cols:** Drop to 1-line compact: gutter + glyph + id + pill + tokens

No longer need the 100-col tier since the badge is always inline after the ID.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Badge style | Subtle pill (surface0 bg, colored fg) | Readable without being loud; matches k9s conventions |
| Column alignment | Right-aligned tokens + age | Easier to scan vertically across rows |
| Pill color per source | Mauve=claude, Blue=opencode | Uses existing theme colors; distinguishable |
| Badge always visible | Yes | Source is primary info, not optional metadata |

## Rejected Alternatives

- **Icon glyphs instead of text badge** — less clear, emoji rendering varies across terminals
- **Color-coded session ID** — harder to read, conflicts with selection highlighting
- **Bold inverse pill** — too loud, draws attention away from tokens/age which matter more

## Scope

Changes are limited to `internal/tui/sessions.go`:
- `renderSessionRow` — restructure line 1 layout
- `renderTwoLineSessionRow` — restructure line 2 layout
- New `sourcePill` helper replaces `sourceBadge`
- Remove 3-tier width branching (simplify to 2-tier: ≥80 and <80)
