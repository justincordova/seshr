package opencode

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// newDetectorTestStore builds an in-memory DB with the minimal schema the
// detector needs, inserts the given sessions, and returns a Detector bound
// to it. All sessions share project_id="p1" for simplicity.
func newDetectorTestStore(t *testing.T, now time.Time, seeds []sessionSeed) *Detector {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE project (
			id TEXT PRIMARY KEY,
			worktree TEXT NOT NULL,
			name TEXT,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			sandboxes TEXT NOT NULL
		);
		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			slug TEXT NOT NULL,
			directory TEXT NOT NULL,
			title TEXT NOT NULL,
			version TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			time_archived INTEGER
		);
		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
		INSERT INTO project (id, worktree, name, time_created, time_updated, sandboxes)
			VALUES ('p1', '/x', 'p1', 0, 0, '{}');
	`)
	require.NoError(t, err)

	for _, s := range seeds {
		_, err := db.Exec(
			`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
			 VALUES (?, 'p1', 'slug', ?, 'title', '0.1.0', ?, ?)`,
			s.ID, s.Directory, s.TimeUpdated, s.TimeUpdated,
		)
		require.NoError(t, err)
	}

	store := &Store{
		conns: &connections{dbPath: ":memory:", read: db},
	}
	d := NewDetector(store)
	d.now = func() time.Time { return now }
	return d
}

type sessionSeed struct {
	ID          string
	Directory   string
	TimeUpdated int64 // Unix ms
}

// makeSnapshot builds a ProcessSnapshot with the given processes.
func makeSnapshot(procs ...backend.ProcInfo) backend.ProcessSnapshot {
	byPID := make(map[int]backend.ProcInfo, len(procs))
	children := make(map[int][]int)
	for _, p := range procs {
		byPID[p.PID] = p
		if p.PPID != 0 {
			children[p.PPID] = append(children[p.PPID], p.PID)
		}
	}
	return backend.ProcessSnapshot{
		At: time.Now(), ByPID: byPID, Children: children,
	}
}

func TestSelectOpenCodeProcs_NodeLauncherWithChild_OnlyChildKept(t *testing.T) {
	snap := makeSnapshot(
		backend.ProcInfo{PID: 100, Command: "node /opt/opencode/cli.js"},
		backend.ProcInfo{PID: 101, PPID: 100, Command: "/opt/opencode/bin/opencode"},
	)

	procs := selectOpenCodeProcs(snap)

	require.Len(t, procs, 1)
	assert.Equal(t, 101, procs[0].PID)
}

func TestSelectOpenCodeProcs_TwoUnrelated_BothKept(t *testing.T) {
	snap := makeSnapshot(
		backend.ProcInfo{PID: 100, Command: "opencode"},
		backend.ProcInfo{PID: 200, Command: "opencode --debug"},
	)

	procs := selectOpenCodeProcs(snap)

	assert.Len(t, procs, 2)
}

func TestSelectOpenCodeProcs_NoOpenCodeProcesses_Empty(t *testing.T) {
	snap := makeSnapshot(
		backend.ProcInfo{PID: 1, Command: "bash"},
		backend.ProcInfo{PID: 2, Command: "zsh"},
	)

	assert.Empty(t, selectOpenCodeProcs(snap))
}

func TestCommandMentionsOpenCode_MatchesFirstTwoArgv(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want bool
	}{
		{"bare", "opencode", true},
		{"with args", "opencode --debug", true},
		{"node launcher", "node /opt/opencode/cli.js", true},
		{"path to bin", "/usr/local/bin/opencode", true},
		{"unrelated", "bash -c something", false},
		{"third arg contains opencode ignored", "go run opencode cli", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, commandMentionsOpenCode(tc.cmd))
		})
	}
}

func TestDetectLive_SingleCandidate_NotAmbiguous(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	d := newDetectorTestStore(t, now, []sessionSeed{
		{ID: "ses_a", Directory: "/proj/a", TimeUpdated: now.Add(-10 * time.Second).UnixMilli()},
	})

	snap := makeSnapshot(
		backend.ProcInfo{PID: 100, Command: "opencode", CWD: "/proj/a"},
	)

	results, err := d.DetectLive(context.Background(), snap)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "ses_a", results[0].SessionID)
	assert.False(t, results[0].Ambiguous)
	assert.Equal(t, 100, results[0].PID)
	assert.Equal(t, session.SourceOpenCode, results[0].Kind)
}

func TestDetectLive_MultipleCandidates_AllAmbiguous(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	d := newDetectorTestStore(t, now, []sessionSeed{
		{ID: "ses_a", Directory: "/proj/a", TimeUpdated: now.Add(-10 * time.Second).UnixMilli()},
		{ID: "ses_b", Directory: "/proj/a", TimeUpdated: now.Add(-120 * time.Second).UnixMilli()},
	})

	snap := makeSnapshot(
		backend.ProcInfo{PID: 100, Command: "opencode", CWD: "/proj/a"},
	)

	results, err := d.DetectLive(context.Background(), snap)

	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.True(t, r.Ambiguous, "expected Ambiguous=true for session %s", r.SessionID)
		assert.Empty(t, r.CurrentTask, "ambiguous emissions must not display a task")
	}
}

func TestDetectLive_StaleSession_Excluded(t *testing.T) {
	// Session updated 10 minutes ago — outside the 5-minute window.
	now := time.UnixMilli(1_700_000_000_000)
	d := newDetectorTestStore(t, now, []sessionSeed{
		{ID: "ses_a", Directory: "/proj/a", TimeUpdated: now.Add(-10 * time.Minute).UnixMilli()},
	})

	snap := makeSnapshot(
		backend.ProcInfo{PID: 100, Command: "opencode", CWD: "/proj/a"},
	)

	results, err := d.DetectLive(context.Background(), snap)

	require.NoError(t, err)
	assert.Empty(t, results, "stale session must not appear in live results")
}

func TestDetectLive_NoCWD_Skipped(t *testing.T) {
	now := time.Now()
	d := newDetectorTestStore(t, now, nil)

	snap := makeSnapshot(
		backend.ProcInfo{PID: 100, Command: "opencode", CWD: ""},
	)

	results, err := d.DetectLive(context.Background(), snap)

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDeriveStatus_DBRecent_Working(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	cand := candidateRow{timeUpdated: now.Add(-5 * time.Second).UnixMilli()}

	status := deriveStatus(cand, backend.ProcInfo{CPU: 0.0}, nil, now)

	assert.Equal(t, backend.StatusWorking, status)
}

func TestDeriveStatus_AgentCPU_Working(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	cand := candidateRow{timeUpdated: now.Add(-2 * time.Minute).UnixMilli()}

	status := deriveStatus(cand, backend.ProcInfo{CPU: 15.0}, nil, now)

	assert.Equal(t, backend.StatusWorking, status)
}

func TestDeriveStatus_ChildCPU_Working(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	cand := candidateRow{timeUpdated: now.Add(-2 * time.Minute).UnixMilli()}

	status := deriveStatus(
		cand,
		backend.ProcInfo{CPU: 0.1},
		[]backend.ProcInfo{{CPU: 80.0}},
		now,
	)

	assert.Equal(t, backend.StatusWorking, status)
}

func TestDeriveStatus_AllQuiet_Waiting(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	cand := candidateRow{timeUpdated: now.Add(-2 * time.Minute).UnixMilli()}

	status := deriveStatus(cand, backend.ProcInfo{CPU: 0.1}, nil, now)

	assert.Equal(t, backend.StatusWaiting, status)
}

func TestCurrentTask_CacheHit_SkipsQuery(t *testing.T) {
	now := time.Now()
	d := newDetectorTestStore(t, now, nil)
	// Seed cache.
	d.cache["ses_x"] = taskCacheEntry{TimeUpdated: 12345, Task: "cached"}

	// Same timeUpdated → cache hit; DB not consulted (would return "").
	got := d.currentTask(context.Background(), "ses_x", 12345, now)

	assert.Equal(t, "cached", got)
}

func TestCurrentTask_CacheMiss_QueriesAndUpdates(t *testing.T) {
	// Arrange: session with a recent tool part.
	now := time.UnixMilli(1_700_000_000_000)
	d := newDetectorTestStore(t, now, nil)

	// Insert a tool part into the in-memory DB.
	_, err := d.store.conns.read.Exec(`
		INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		VALUES ('p1', 'm1', 'ses_x', ?, ?, ?)
	`, now.Add(-5*time.Second).UnixMilli(), now.Add(-5*time.Second).UnixMilli(),
		`{"type":"tool","callID":"c1","tool":"bash","state":{"status":"running","input":{"cmd":"ls -la"}}}`,
	)
	require.NoError(t, err)

	// Act
	got := d.currentTask(context.Background(), "ses_x", 42, now)

	// Assert
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "bash")
	// Cache now populated.
	entry, ok := d.cache["ses_x"]
	require.True(t, ok)
	assert.Equal(t, int64(42), entry.TimeUpdated)
}

func TestFormatToolPartForTask_Truncates(t *testing.T) {
	long := fmt.Sprintf(`{"type":"tool","callID":"c","tool":"bash","state":{"status":"running","input":{"cmd":"%s"}}}`,
		"a very very very very very very long command line here")

	got := formatToolPartForTask([]byte(long))

	assert.LessOrEqual(t, len([]rune(got)), 30)
	assert.Contains(t, got, "bash")
}

func TestFormatToolPartForTask_NoTool_Empty(t *testing.T) {
	assert.Empty(t, formatToolPartForTask([]byte(`{"type":"text"}`)))
	assert.Empty(t, formatToolPartForTask([]byte(`not json`)))
}
