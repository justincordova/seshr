package tui

import (
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/seshr/internal/backend"
)

// liveGroupName is the synthetic project name used for the pinned LIVE
// group at the top of Project view. No real project can have this name
// (project paths never start with "◉").
const liveGroupName = "◉ LIVE"

type ProjectGroup struct {
	Name        string
	DisplayName string
	Sessions    []backend.SessionMeta
	TotalTokens int
	Color       lipgloss.TerminalColor
}

type RowKind int

const (
	RowGroup RowKind = iota
	RowSession
	// RowDivider is a dim horizontal rule between the live block and the
	// ended block in Recent view. Has no associated GroupIdx/SessionIdx.
	RowDivider
)

type PickerRow struct {
	Kind       RowKind
	GroupIdx   int
	SessionIdx int
}

func ProjectDisplayName(raw string) string {
	if raw == "" {
		return raw
	}
	if !strings.HasPrefix(raw, "-") {
		return raw
	}
	s := strings.TrimPrefix(raw, "-")
	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return raw
	}
	return parts[len(parts)-1]
}

func GroupByProject(metas []backend.SessionMeta, th Theme) []ProjectGroup {
	groupMap := map[string]*ProjectGroup{}
	var order []string

	for i := range metas {
		m := &metas[i]
		g, ok := groupMap[m.Project]
		if !ok {
			order = append(order, m.Project)
			groupMap[m.Project] = &ProjectGroup{
				Name:        m.Project,
				DisplayName: ProjectDisplayName(m.Project),
				Color:       projectColor(m.Project, th),
			}
			g = groupMap[m.Project]
		}
		g.Sessions = append(g.Sessions, *m)
		g.TotalTokens += m.TokenCount
	}

	groups := make([]ProjectGroup, 0, len(order))
	for _, name := range order {
		g := groupMap[name]
		sort.Slice(g.Sessions, func(i, j int) bool {
			return g.Sessions[i].UpdatedAt.After(g.Sessions[j].UpdatedAt)
		})
		groups = append(groups, *g)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Sessions[0].UpdatedAt.After(groups[j].Sessions[0].UpdatedAt)
	})

	return groups
}

func projectColor(name string, th Theme) lipgloss.TerminalColor {
	if len(th.ProjectPalette) == 0 {
		return colMauve
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	idx := int(h.Sum32()) % len(th.ProjectPalette)
	return th.ProjectPalette[idx]
}

type SummaryStats struct {
	TotalSessions int
	TotalTokens   int64
	TotalBytes    int64
	Projects      int
	MostRecent    time.Time
	BiggestProj   string
}

func ComputeSummary(metas []backend.SessionMeta) SummaryStats {
	if len(metas) == 0 {
		return SummaryStats{}
	}
	var s SummaryStats
	projTokens := map[string]int64{}
	for _, m := range metas {
		s.TotalSessions++
		s.TotalTokens += int64(m.TokenCount)
		s.TotalBytes += m.SizeBytes
		projTokens[m.Project] += int64(m.TokenCount)
		if m.UpdatedAt.After(s.MostRecent) {
			s.MostRecent = m.UpdatedAt
		}
	}
	s.Projects = len(projTokens)
	var biggest int64
	for p, t := range projTokens {
		if t > biggest {
			biggest = t
			s.BiggestProj = p
		}
	}
	return s
}

func BuildFlatRows(groups []ProjectGroup, collapsed map[string]bool) []PickerRow {
	var rows []PickerRow
	for gi, g := range groups {
		rows = append(rows, PickerRow{Kind: RowGroup, GroupIdx: gi})
		if !collapsed[g.Name] {
			for si := range g.Sessions {
				rows = append(rows, PickerRow{Kind: RowSession, GroupIdx: gi, SessionIdx: si})
			}
		}
	}
	return rows
}

// liveStatusRank assigns a sort rank to a live session based on its status
// class. Lower ranks sort first: Working → Waiting → Ambiguous → other live
// → ended (nil). Used by Recent view and the synthetic LIVE group to keep
// the most actionable sessions visible.
func liveStatusRank(ls *backend.LiveSession) int {
	if ls == nil {
		return 99
	}
	if ls.Ambiguous {
		return 2
	}
	switch ls.Status {
	case backend.StatusWorking:
		return 0
	case backend.StatusWaiting:
		return 1
	default:
		return 3
	}
}

// SplitLiveGroup returns groups with a synthetic LIVE group prepended (containing
// all live sessions found in the input groups), and live sessions removed
// from their original project groups. Empty project groups produced by the
// extraction are dropped. When liveIndex is empty, groups are returned
// unchanged.
//
// The synthetic group uses theme.Success as its color and is keyed by
// liveGroupName so render code can detect it.
func SplitLiveGroup(
	groups []ProjectGroup,
	liveIndex map[string]*backend.LiveSession,
	th Theme,
) []ProjectGroup {
	if len(liveIndex) == 0 || len(groups) == 0 {
		return groups
	}

	var liveSessions []backend.SessionMeta
	var liveTokens int
	out := make([]ProjectGroup, 0, len(groups)+1)

	for _, g := range groups {
		kept := g.Sessions[:0:0]
		var keptTokens int
		for _, s := range g.Sessions {
			if liveIndex[s.ID] != nil {
				liveSessions = append(liveSessions, s)
				liveTokens += s.TokenCount
				continue
			}
			kept = append(kept, s)
			keptTokens += s.TokenCount
		}
		if len(kept) == 0 {
			continue
		}
		gg := g
		gg.Sessions = kept
		gg.TotalTokens = keptTokens
		out = append(out, gg)
	}

	if len(liveSessions) == 0 {
		return groups
	}

	sort.SliceStable(liveSessions, func(i, j int) bool {
		ri := liveStatusRank(liveIndex[liveSessions[i].ID])
		rj := liveStatusRank(liveIndex[liveSessions[j].ID])
		if ri != rj {
			return ri < rj
		}
		return liveSessions[i].UpdatedAt.After(liveSessions[j].UpdatedAt)
	})

	liveColor := lipgloss.TerminalColor(th.Success)
	live := ProjectGroup{
		Name:        liveGroupName,
		DisplayName: "LIVE",
		Sessions:    liveSessions,
		TotalTokens: liveTokens,
		Color:       liveColor,
	}
	return append([]ProjectGroup{live}, out...)
}

// BuildRecentRows produces a flat row list for Recent view. Output:
//   - Live sessions first, sorted by liveStatusRank then UpdatedAt desc.
//   - A RowDivider between live and ended (omitted when either bucket is
//     empty).
//   - Ended sessions, sorted by UpdatedAt desc.
//
// The returned []SessionMeta is the ordered metas slice referenced by
// PickerRow.SessionIdx (GroupIdx is unused, set to -1, in Recent rows).
func BuildRecentRows(
	metas []backend.SessionMeta,
	liveIndex map[string]*backend.LiveSession,
) ([]PickerRow, []backend.SessionMeta) {
	if len(metas) == 0 {
		return nil, nil
	}

	live := make([]backend.SessionMeta, 0, len(metas))
	ended := make([]backend.SessionMeta, 0, len(metas))
	for _, m := range metas {
		if liveIndex[m.ID] != nil {
			live = append(live, m)
		} else {
			ended = append(ended, m)
		}
	}

	sort.SliceStable(live, func(i, j int) bool {
		ri := liveStatusRank(liveIndex[live[i].ID])
		rj := liveStatusRank(liveIndex[live[j].ID])
		if ri != rj {
			return ri < rj
		}
		return live[i].UpdatedAt.After(live[j].UpdatedAt)
	})
	sort.SliceStable(ended, func(i, j int) bool {
		return ended[i].UpdatedAt.After(ended[j].UpdatedAt)
	})

	ordered := make([]backend.SessionMeta, 0, len(live)+len(ended))
	ordered = append(ordered, live...)
	ordered = append(ordered, ended...)

	rows := make([]PickerRow, 0, len(ordered)+1)
	for i := range live {
		rows = append(rows, PickerRow{Kind: RowSession, GroupIdx: -1, SessionIdx: i})
	}
	if len(live) > 0 && len(ended) > 0 {
		rows = append(rows, PickerRow{Kind: RowDivider, GroupIdx: -1, SessionIdx: -1})
	}
	for i := range ended {
		rows = append(rows, PickerRow{Kind: RowSession, GroupIdx: -1, SessionIdx: len(live) + i})
	}
	return rows, ordered
}
