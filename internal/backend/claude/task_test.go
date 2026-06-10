package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractLastToolUse_RealRecordShape(t *testing.T) {
	// Records nest role/content under "message" — the fixture exercises the
	// shape Claude Code actually writes.
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "multi_topic.jsonl"))
	require.NoError(t, err)

	got := extractLastToolUse(strings.NewReader(string(data)))

	assert.NotEmpty(t, got, "fixture contains tool_use blocks; the task label must not be empty")
}

func TestExtractLastToolUse_ReturnsLastToolUse(t *testing.T) {
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls /tmp"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/home/u/main.go"}}]}}`,
	}, "\n") + "\n"

	got := extractLastToolUse(strings.NewReader(lines))

	assert.Equal(t, "Read main.go", got)
}

func TestExtractLastToolUse_NoToolUseReturnsEmpty(t *testing.T) {
	lines := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}` + "\n"

	got := extractLastToolUse(strings.NewReader(lines))

	assert.Empty(t, got)
}

func TestFormatToolTask_FallbackIsDeterministic(t *testing.T) {
	input := json.RawMessage(`{"zeta":"zzz","alpha":"aaa"}`)

	for range 20 {
		assert.Equal(t, "Tool aaa", formatToolTask("Tool", input))
	}
}
