package backend

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePS_ParsesAllRows(t *testing.T) {
	// Arrange
	raw, err := os.ReadFile("../../testdata/ps_output.txt")
	require.NoError(t, err)

	// Act
	procs, err := parsePS(raw)

	// Assert
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(procs), 6)

	var claudeProc *ProcInfo
	for i := range procs {
		if procs[i].PID == 850 {
			claudeProc = &procs[i]
		}
	}
	require.NotNil(t, claudeProc)
	assert.Equal(t, 421, claudeProc.PPID)
	assert.InDelta(t, 2.1, claudeProc.CPU, 0.01)
}

func TestProcessScanner_OnlyCallsReadCWDForAgents(t *testing.T) {
	// Arrange
	raw, err := os.ReadFile("../../testdata/ps_output.txt")
	require.NoError(t, err)

	var cwdCalls []int
	scanner := &ProcessScanner{
		now: time.Now,
		runPS: func(_ context.Context) ([]byte, error) {
			return raw, nil
		},
		readCWD: func(_ context.Context, pid int) (string, error) {
			cwdCalls = append(cwdCalls, pid)
			return "/tmp/fake", nil
		},
	}

	// Act
	snap, err := scanner.Scan(context.Background())

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, snap.ByPID)

	// Only agent PIDs should trigger readCWD: 850 (claude), 851 (node/opencode), 852 (opencode).
	for _, pid := range cwdCalls {
		proc := snap.ByPID[pid]
		assert.True(t, isAgentCandidate(proc.Command),
			"unexpected CWD call for pid %d cmd %q", pid, proc.Command)
	}
	// Non-agent PIDs must NOT appear in cwdCalls.
	nonAgents := []int{1, 312, 421, 900}
	for _, pid := range nonAgents {
		assert.NotContains(t, cwdCalls, pid)
	}
}

func TestProcessScanner_PSFails_ReturnsErrorWithNilByPID(t *testing.T) {
	// Arrange
	scanner := &ProcessScanner{
		now:     time.Now,
		runPS:   func(_ context.Context) ([]byte, error) { return nil, os.ErrNotExist },
		readCWD: func(_ context.Context, _ int) (string, error) { return "", nil },
	}

	// Act
	snap, err := scanner.Scan(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Nil(t, snap.ByPID)
	assert.False(t, snap.At.IsZero(), "At should be set even on error")
}

func TestIsAgentCandidate_DetectsClaudeAndOpencode(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"claude --resume abc", true},
		{"/usr/local/bin/claude", true},
		{"opencode serve", true},
		{"node /opt/.../opencode/dist/index.js", true},
		{"bash", false},
		{"zsh", false},
		{"/usr/bin/sshd -D", false},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			assert.Equal(t, tc.want, isAgentCandidate(tc.cmd))
		})
	}
}
