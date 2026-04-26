package tui_test

import (
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupByProject_TwoProjects_SortedByMostRecent(t *testing.T) {
	// Arrange
	metas := []backend.SessionMeta{
		{ID: "a", Project: "proj-b", UpdatedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Project: "proj-a", UpdatedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "c", Project: "proj-a", UpdatedAt: time.Now().Add(-3 * time.Hour)},
		{ID: "d", Project: "proj-b", UpdatedAt: time.Now().Add(-48 * time.Hour)},
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
	metas := []backend.SessionMeta{
		{ID: "old", Project: "proj", UpdatedAt: time.Now().Add(-72 * time.Hour)},
		{ID: "new", Project: "proj", UpdatedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "mid", Project: "proj", UpdatedAt: time.Now().Add(-24 * time.Hour)},
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
	metas := []backend.SessionMeta{
		{ID: "a", Project: "proj", TokenCount: 100, UpdatedAt: time.Now()},
		{ID: "b", Project: "proj", TokenCount: 200, UpdatedAt: time.Now().Add(-1 * time.Hour)},
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
	metas := []backend.SessionMeta{
		{ID: "a", Project: "proj-a", UpdatedAt: time.Now()},
		{ID: "b", Project: "proj-b", UpdatedAt: time.Now().Add(-1 * time.Hour)},
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
	metas := []backend.SessionMeta{
		{ID: "a", Project: "-Users-x-projects-myapp", UpdatedAt: time.Now()},
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
	metas := []backend.SessionMeta{
		{ID: "a", Project: "p1", TokenCount: 100, SizeBytes: 1000, UpdatedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Project: "p1", TokenCount: 200, SizeBytes: 2000, UpdatedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "c", Project: "p2", TokenCount: 300, SizeBytes: 3000, UpdatedAt: time.Now().Add(-3 * time.Hour)},
		{ID: "d", Project: "p2", TokenCount: 400, SizeBytes: 4000, UpdatedAt: time.Now().Add(-4 * time.Hour)},
		{ID: "e", Project: "p3", TokenCount: 500, SizeBytes: 5000, UpdatedAt: time.Now().Add(-5 * time.Hour)},
	}
	s := tui.ComputeSummary(metas)
	assert.Equal(t, 5, s.TotalSessions)
	assert.Equal(t, int64(1500), s.TotalTokens)
	assert.Equal(t, int64(15000), s.TotalBytes)
	assert.Equal(t, 3, s.Projects)
	assert.Equal(t, "p2", s.BiggestProj)
}

// ── BuildRecentRows ─────────────────────────────────────────────────────────

func TestBuildRecentRows_LivePinnedToTop(t *testing.T) {
	// Arrange — 2 ended (newer than the live ones) + 2 live.
	now := time.Now()
	metas := []backend.SessionMeta{
		{ID: "ended-1", Project: "p", UpdatedAt: now},
		{ID: "live-1", Project: "p", UpdatedAt: now.Add(-1 * time.Hour)},
		{ID: "ended-2", Project: "p", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "live-2", Project: "p", UpdatedAt: now.Add(-3 * time.Hour)},
	}
	live := map[string]*backend.LiveSession{
		"live-1": {SessionID: "live-1", Status: backend.StatusWorking},
		"live-2": {SessionID: "live-2", Status: backend.StatusWaiting},
	}

	// Act
	rows, ordered := tui.BuildRecentRows(metas, live)

	// Assert — 2 live rows, divider, 2 ended rows = 5 total.
	require.Len(t, rows, 5)
	assert.Equal(t, tui.RowSession, rows[0].Kind)
	assert.Equal(t, tui.RowSession, rows[1].Kind)
	assert.Equal(t, tui.RowDivider, rows[2].Kind)
	assert.Equal(t, tui.RowSession, rows[3].Kind)
	assert.Equal(t, tui.RowSession, rows[4].Kind)
	// Live block: Working before Waiting.
	assert.Equal(t, "live-1", ordered[rows[0].SessionIdx].ID)
	assert.Equal(t, "live-2", ordered[rows[1].SessionIdx].ID)
	// Ended block: ended-1 (newer) before ended-2.
	assert.Equal(t, "ended-1", ordered[rows[3].SessionIdx].ID)
	assert.Equal(t, "ended-2", ordered[rows[4].SessionIdx].ID)
}

func TestBuildRecentRows_StatusClassOrder(t *testing.T) {
	// Arrange — Working, Waiting, Ambiguous all live; Working & Ambiguous
	// are older than Waiting to prove status class beats UpdatedAt.
	now := time.Now()
	metas := []backend.SessionMeta{
		{ID: "amb", UpdatedAt: now.Add(-1 * time.Minute)},
		{ID: "wait", UpdatedAt: now.Add(-2 * time.Minute)},
		{ID: "work", UpdatedAt: now.Add(-3 * time.Minute)},
	}
	live := map[string]*backend.LiveSession{
		"work": {SessionID: "work", Status: backend.StatusWorking},
		"wait": {SessionID: "wait", Status: backend.StatusWaiting},
		"amb":  {SessionID: "amb", Status: backend.StatusWaiting, Ambiguous: true},
	}

	// Act
	rows, ordered := tui.BuildRecentRows(metas, live)

	// Assert
	require.Len(t, rows, 3) // no divider, no ended
	assert.Equal(t, "work", ordered[rows[0].SessionIdx].ID)
	assert.Equal(t, "wait", ordered[rows[1].SessionIdx].ID)
	assert.Equal(t, "amb", ordered[rows[2].SessionIdx].ID)
}

func TestBuildRecentRows_NoLiveOmitsDivider(t *testing.T) {
	// Arrange
	now := time.Now()
	metas := []backend.SessionMeta{
		{ID: "a", UpdatedAt: now},
		{ID: "b", UpdatedAt: now.Add(-1 * time.Hour)},
	}

	// Act
	rows, _ := tui.BuildRecentRows(metas, nil)

	// Assert — only sessions, no divider.
	require.Len(t, rows, 2)
	assert.Equal(t, tui.RowSession, rows[0].Kind)
	assert.Equal(t, tui.RowSession, rows[1].Kind)
}

func TestBuildRecentRows_NoEndedOmitsDivider(t *testing.T) {
	// Arrange
	metas := []backend.SessionMeta{{ID: "live", UpdatedAt: time.Now()}}
	live := map[string]*backend.LiveSession{
		"live": {SessionID: "live", Status: backend.StatusWorking},
	}

	// Act
	rows, _ := tui.BuildRecentRows(metas, live)

	// Assert
	require.Len(t, rows, 1)
	assert.Equal(t, tui.RowSession, rows[0].Kind)
}

func TestBuildRecentRows_EmptyMetas(t *testing.T) {
	// Arrange / Act
	rows, ordered := tui.BuildRecentRows(nil, nil)

	// Assert
	assert.Empty(t, rows)
	assert.Empty(t, ordered)
}

// ── SplitLiveGroup ──────────────────────────────────────────────────────────

func TestSplitLiveGroup_PrependsLiveGroup(t *testing.T) {
	// Arrange
	now := time.Now()
	groups := []tui.ProjectGroup{
		{Name: "p1", DisplayName: "p1", Sessions: []backend.SessionMeta{
			{ID: "live-a", UpdatedAt: now},
			{ID: "ended-a", UpdatedAt: now.Add(-1 * time.Hour)},
		}},
		{Name: "p2", DisplayName: "p2", Sessions: []backend.SessionMeta{
			{ID: "live-b", UpdatedAt: now.Add(-30 * time.Minute)},
			{ID: "ended-b", UpdatedAt: now.Add(-2 * time.Hour)},
		}},
	}
	live := map[string]*backend.LiveSession{
		"live-a": {SessionID: "live-a", Status: backend.StatusWorking},
		"live-b": {SessionID: "live-b", Status: backend.StatusWaiting},
	}

	// Act
	got := tui.SplitLiveGroup(groups, live, tui.CatppuccinMocha())

	// Assert
	require.Len(t, got, 3, "LIVE group prepended + 2 project groups")
	assert.Equal(t, "◉ LIVE", got[0].Name)
	assert.Equal(t, "LIVE", got[0].DisplayName)
	require.Len(t, got[0].Sessions, 2)
	// Working before Waiting in LIVE group.
	assert.Equal(t, "live-a", got[0].Sessions[0].ID)
	assert.Equal(t, "live-b", got[0].Sessions[1].ID)
}

func TestSplitLiveGroup_RemovesLiveFromProjectGroups(t *testing.T) {
	// Arrange
	groups := []tui.ProjectGroup{
		{Name: "p1", Sessions: []backend.SessionMeta{
			{ID: "live-a"}, {ID: "ended-a"},
		}},
	}
	live := map[string]*backend.LiveSession{
		"live-a": {SessionID: "live-a", Status: backend.StatusWorking},
	}

	// Act
	got := tui.SplitLiveGroup(groups, live, tui.CatppuccinMocha())

	// Assert — p1 has only ended-a now.
	require.Len(t, got, 2)
	assert.Equal(t, "p1", got[1].Name)
	require.Len(t, got[1].Sessions, 1)
	assert.Equal(t, "ended-a", got[1].Sessions[0].ID)
}

func TestSplitLiveGroup_NoLiveReturnsUnchanged(t *testing.T) {
	groups := []tui.ProjectGroup{
		{Name: "p1", Sessions: []backend.SessionMeta{{ID: "a"}}},
	}

	got := tui.SplitLiveGroup(groups, nil, tui.CatppuccinMocha())

	assert.Equal(t, groups, got)
}

func TestSplitLiveGroup_DropsEmptyProjectGroups(t *testing.T) {
	// Arrange — p1 has only live sessions; should disappear after split.
	groups := []tui.ProjectGroup{
		{Name: "p1", Sessions: []backend.SessionMeta{{ID: "live-a"}}},
		{Name: "p2", Sessions: []backend.SessionMeta{{ID: "ended-b"}}},
	}
	live := map[string]*backend.LiveSession{
		"live-a": {SessionID: "live-a", Status: backend.StatusWorking},
	}

	// Act
	got := tui.SplitLiveGroup(groups, live, tui.CatppuccinMocha())

	// Assert
	require.Len(t, got, 2, "LIVE + p2 only; empty p1 dropped")
	assert.Equal(t, "◉ LIVE", got[0].Name)
	assert.Equal(t, "p2", got[1].Name)
}

func TestSplitLiveGroup_StatusClassOrderInsideLive(t *testing.T) {
	// Arrange
	now := time.Now()
	groups := []tui.ProjectGroup{
		{Name: "p", Sessions: []backend.SessionMeta{
			{ID: "amb", UpdatedAt: now},
			{ID: "wait", UpdatedAt: now.Add(-1 * time.Minute)},
			{ID: "work", UpdatedAt: now.Add(-2 * time.Minute)},
		}},
	}
	live := map[string]*backend.LiveSession{
		"work": {SessionID: "work", Status: backend.StatusWorking},
		"wait": {SessionID: "wait", Status: backend.StatusWaiting},
		"amb":  {SessionID: "amb", Status: backend.StatusWaiting, Ambiguous: true},
	}

	// Act
	got := tui.SplitLiveGroup(groups, live, tui.CatppuccinMocha())

	// Assert — Working → Waiting → Ambiguous
	require.Len(t, got, 1) // p had only live sessions, dropped
	require.Equal(t, "◉ LIVE", got[0].Name)
	require.Len(t, got[0].Sessions, 3)
	assert.Equal(t, "work", got[0].Sessions[0].ID)
	assert.Equal(t, "wait", got[0].Sessions[1].ID)
	assert.Equal(t, "amb", got[0].Sessions[2].ID)
}
