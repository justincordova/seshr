package claude_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/justincordova/seshr/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetector_Layer1_SidecarMatchesPID(t *testing.T) {
	// Arrange: sidecar says PID=850, sessionId="abc123def456"
	sidecarDir := t.TempDir()
	require.NoError(t, copyFile(filepath.Join(testdataDir, "claude_live_sidecar.json"), filepath.Join(sidecarDir, "850.json")))

	det := claudeBackend.NewDetector(t.TempDir(), sidecarDir)

	snap := backend.ProcessSnapshot{
		At: time.Now(),
		ByPID: map[int]backend.ProcInfo{
			850: {PID: 850, PPID: 421, Command: "claude --resume abc123def456", CPU: 5.0},
		},
		Children: map[int][]int{},
	}

	// Act
	lives, err := det.DetectLive(context.Background(), snap)

	// Assert
	require.NoError(t, err)
	require.Len(t, lives, 1)
	assert.Equal(t, "abc123def456", lives[0].SessionID)
	assert.Equal(t, session.SourceClaude, lives[0].Kind)
	assert.Equal(t, 850, lives[0].PID)
	assert.Equal(t, backend.StatusWorking, lives[0].Status) // CPU > 1.0
}

func TestDetector_Layer1_NoMatchingPID_ReturnsEmpty(t *testing.T) {
	// Arrange: sidecar says PID=850, but snapshot has no claude processes.
	sidecarDir := t.TempDir()
	require.NoError(t, copyFile(filepath.Join(testdataDir, "claude_live_sidecar.json"), filepath.Join(sidecarDir, "850.json")))

	det := claudeBackend.NewDetector(t.TempDir(), sidecarDir)

	snap := backend.ProcessSnapshot{
		At:       time.Now(),
		ByPID:    map[int]backend.ProcInfo{},
		Children: map[int][]int{},
	}

	// Act
	lives, err := det.DetectLive(context.Background(), snap)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, lives)
}

func TestDetector_Layer2_CWDFallback(t *testing.T) {
	// Arrange: no sidecar; claude PID with CWD pointing to a project with a fresh transcript.
	// Create a projects root with encoded dir + session jsonl.
	projectsRoot := t.TempDir()
	// Use a simple CWD we control to predict encoding.
	// We create a directory matching the encoding of cwd.
	tmpCWD := t.TempDir()
	encoded := encodeForTest(tmpCWD)
	encodedDir := filepath.Join(projectsRoot, encoded)
	require.NoError(t, os.MkdirAll(encodedDir, 0o755))
	sessionFile := filepath.Join(encodedDir, "mysession.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), sessionFile))

	det := claudeBackend.NewDetector(projectsRoot, t.TempDir())

	snap := backend.ProcessSnapshot{
		At: time.Now(),
		ByPID: map[int]backend.ProcInfo{
			999: {PID: 999, PPID: 1, Command: "claude", CPU: 0.5, CWD: tmpCWD},
		},
		Children: map[int][]int{},
	}

	// Act
	lives, err := det.DetectLive(context.Background(), snap)

	// Assert: the transcript is fresh (just copied), so should be found.
	require.NoError(t, err)
	require.Len(t, lives, 1)
	assert.Equal(t, "mysession", lives[0].SessionID)
}

func TestDeriveStatus_TranscriptFresh_ReturnsWorking(t *testing.T) {
	// Use the detector directly; status derives from fresh transcript mtime.
	// A fresh transcript → StatusWorking.
	projectsRoot := t.TempDir()
	det := claudeBackend.NewDetector(projectsRoot, t.TempDir())

	freshDir := t.TempDir()
	encoded := encodeForTest(freshDir)
	encodedDir := filepath.Join(projectsRoot, encoded)
	require.NoError(t, os.MkdirAll(encodedDir, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), filepath.Join(encodedDir, "sess.jsonl")))

	snap := backend.ProcessSnapshot{
		At:       time.Now(),
		ByPID:    map[int]backend.ProcInfo{42: {PID: 42, Command: "claude", CPU: 0.0, CWD: freshDir}},
		Children: map[int][]int{},
	}

	lives, err := det.DetectLive(context.Background(), snap)
	require.NoError(t, err)
	require.Len(t, lives, 1)
	assert.Equal(t, backend.StatusWorking, lives[0].Status)
}

// encodeForTest replicates the encoding logic for test setup.
// /Users/foo/bar → -Users-foo-bar
func encodeForTest(cwd string) string {
	result := ""
	for _, ch := range cwd {
		switch ch {
		case '/', '_', '.':
			result += "-"
		default:
			result += string(ch)
		}
	}
	return result
}
