package tui

import (
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/seshr/internal/parser"
)

type ProjectGroup struct {
	Name        string
	DisplayName string
	Sessions    []parser.SessionMeta
	TotalTokens int
	Color       lipgloss.TerminalColor
}

type RowKind int

const (
	RowGroup RowKind = iota
	RowSession
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

func GroupByProject(metas []parser.SessionMeta, th Theme) []ProjectGroup {
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
			return g.Sessions[i].ModifiedAt.After(g.Sessions[j].ModifiedAt)
		})
		groups = append(groups, *g)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Sessions[0].ModifiedAt.After(groups[j].Sessions[0].ModifiedAt)
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

func ComputeSummary(metas []parser.SessionMeta) SummaryStats {
	if len(metas) == 0 {
		return SummaryStats{}
	}
	var s SummaryStats
	projTokens := map[string]int64{}
	for _, m := range metas {
		s.TotalSessions++
		s.TotalTokens += int64(m.TokenCount)
		s.TotalBytes += m.Size
		projTokens[m.Project] += int64(m.TokenCount)
		if m.ModifiedAt.After(s.MostRecent) {
			s.MostRecent = m.ModifiedAt
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
