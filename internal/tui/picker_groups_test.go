package tui_test

import (
	"testing"
	"time"

	"github.com/justincordova/seshly/internal/parser"
	"github.com/justincordova/seshly/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupByProject_TwoProjects_SortedByMostRecent(t *testing.T) {
	// Arrange
	metas := []parser.SessionMeta{
		{ID: "a", Project: "proj-b", ModifiedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Project: "proj-a", ModifiedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "c", Project: "proj-a", ModifiedAt: time.Now().Add(-3 * time.Hour)},
		{ID: "d", Project: "proj-b", ModifiedAt: time.Now().Add(-48 * time.Hour)},
	}
	th := tui.CatppuccinMocha()

	// Act
	groups := tui.GroupByProject(metas, th)

	// Assert
	require.Len(t, groups, 2)
	assert.Equal(t, "proj-b", groups[0].Name, "proj-b has the most-recent session")
	assert.Equal(t, "proj-a", groups[1].Name)
	assert.Len(t, groups[0].Sessions, 2)
	assert.Len(t, groups[1].Sessions, 2)
	assert.Equal(t, "a", groups[0].Sessions[0].ID, "sessions sorted by ModifiedAt desc")
}

func TestGroupByProject_SessionsWithinGroup_SortedByModifiedAt(t *testing.T) {
	// Arrange
	metas := []parser.SessionMeta{
		{ID: "old", Project: "proj", ModifiedAt: time.Now().Add(-72 * time.Hour)},
		{ID: "new", Project: "proj", ModifiedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "mid", Project: "proj", ModifiedAt: time.Now().Add(-24 * time.Hour)},
	}
	th := tui.CatppuccinMocha()

	// Act
	groups := tui.GroupByProject(metas, th)

	// Assert
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Sessions, 3)
	assert.Equal(t, "new", groups[0].Sessions[0].ID)
	assert.Equal(t, "mid", groups[0].Sessions[1].ID)
	assert.Equal(t, "old", groups[0].Sessions[2].ID)
}

func TestGroupByProject_TotalTokens(t *testing.T) {
	// Arrange
	metas := []parser.SessionMeta{
		{ID: "a", Project: "proj", TokenCount: 100, ModifiedAt: time.Now()},
		{ID: "b", Project: "proj", TokenCount: 200, ModifiedAt: time.Now().Add(-1 * time.Hour)},
	}
	th := tui.CatppuccinMocha()

	// Act
	groups := tui.GroupByProject(metas, th)

	// Assert
	require.Len(t, groups, 1)
	assert.Equal(t, 300, groups[0].TotalTokens)
}

func TestBuildFlatRows_ExpandAndCollapse(t *testing.T) {
	// Arrange
	metas := []parser.SessionMeta{
		{ID: "a", Project: "proj-a", ModifiedAt: time.Now()},
		{ID: "b", Project: "proj-b", ModifiedAt: time.Now().Add(-1 * time.Hour)},
	}
	th := tui.CatppuccinMocha()
	groups := tui.GroupByProject(metas, th)

	// Act — expand all
	rows := tui.BuildFlatRows(groups, map[string]bool{})
	require.Len(t, rows, 4)
	assert.Equal(t, tui.RowGroup, rows[0].Kind)
	assert.Equal(t, tui.RowSession, rows[1].Kind)
	assert.Equal(t, tui.RowGroup, rows[2].Kind)
	assert.Equal(t, tui.RowSession, rows[3].Kind)

	// Act — collapse proj-a
	rows = tui.BuildFlatRows(groups, map[string]bool{"proj-a": true})
	require.Len(t, rows, 3)
	assert.Equal(t, tui.RowGroup, rows[0].Kind)
	assert.Equal(t, tui.RowGroup, rows[1].Kind)
	assert.Equal(t, tui.RowSession, rows[2].Kind)
}

func TestProjectDisplayName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"simple", "simple"},
		{"-Users-justincordova-cs-projects-boot", "boot"},
		{"-Users-justincordova-cs-njit-dartly", "dartly"},
		{"no-dash-prefix", "no-dash-prefix"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tui.ProjectDisplayName(tc.input))
	}
}

func TestGroupByProject_PopulatesDisplayName(t *testing.T) {
	metas := []parser.SessionMeta{
		{ID: "a", Project: "-Users-x-projects-myapp", ModifiedAt: time.Now()},
	}
	th := tui.CatppuccinMocha()
	groups := tui.GroupByProject(metas, th)
	require.Len(t, groups, 1)
	assert.Equal(t, "-Users-x-projects-myapp", groups[0].Name)
	assert.Equal(t, "myapp", groups[0].DisplayName)
}

func TestComputeSummary_Empty(t *testing.T) {
	s := tui.ComputeSummary(nil)
	assert.Equal(t, 0, s.TotalSessions)
}

func TestComputeSummary_FiveMetas(t *testing.T) {
	metas := []parser.SessionMeta{
		{ID: "a", Project: "p1", TokenCount: 100, Size: 1000, ModifiedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Project: "p1", TokenCount: 200, Size: 2000, ModifiedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "c", Project: "p2", TokenCount: 300, Size: 3000, ModifiedAt: time.Now().Add(-3 * time.Hour)},
		{ID: "d", Project: "p2", TokenCount: 400, Size: 4000, ModifiedAt: time.Now().Add(-4 * time.Hour)},
		{ID: "e", Project: "p3", TokenCount: 500, Size: 5000, ModifiedAt: time.Now().Add(-5 * time.Hour)},
	}
	s := tui.ComputeSummary(metas)
	assert.Equal(t, 5, s.TotalSessions)
	assert.Equal(t, int64(1500), s.TotalTokens)
	assert.Equal(t, int64(15000), s.TotalBytes)
	assert.Equal(t, 3, s.Projects)
	assert.Equal(t, "p2", s.BiggestProj)
}
