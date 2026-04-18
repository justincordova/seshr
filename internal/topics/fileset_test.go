package topics_test

import (
	"testing"

	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/stretchr/testify/assert"
)

func TestExtractFiles_ReadTool_ReturnsPath(t *testing.T) {
	// Arrange
	tc := parser.ToolCall{
		Name:  "Read",
		Input: []byte(`{"file_path":"/src/auth.go"}`),
	}
	// Act
	got := topics.ExtractFiles([]parser.ToolCall{tc})
	// Assert
	assert.ElementsMatch(t, []string{"/src/auth.go"}, got)
}

func TestExtractFiles_WriteAndEdit_ReturnsPaths(t *testing.T) {
	// Arrange
	calls := []parser.ToolCall{
		{Name: "Write", Input: []byte(`{"file_path":"/src/a.go","content":"x"}`)},
		{Name: "Edit", Input: []byte(`{"file_path":"/src/b.go","old_string":"a","new_string":"b"}`)},
	}
	// Act
	got := topics.ExtractFiles(calls)
	// Assert
	assert.ElementsMatch(t, []string{"/src/a.go", "/src/b.go"}, got)
}

func TestExtractFiles_Glob_UsesPattern(t *testing.T) {
	// Arrange
	tc := parser.ToolCall{
		Name:  "Glob",
		Input: []byte(`{"pattern":"src/**/*.go"}`),
	}
	// Act
	got := topics.ExtractFiles([]parser.ToolCall{tc})
	// Assert
	assert.ElementsMatch(t, []string{"src/**/*.go"}, got)
}

func TestExtractFiles_Deduplicates(t *testing.T) {
	// Arrange
	calls := []parser.ToolCall{
		{Name: "Read", Input: []byte(`{"file_path":"/a.go"}`)},
		{Name: "Read", Input: []byte(`{"file_path":"/a.go"}`)},
	}
	// Act
	got := topics.ExtractFiles(calls)
	// Assert
	assert.Len(t, got, 1)
}

func TestExtractFiles_BashCommand_Ignored(t *testing.T) {
	// Arrange
	tc := parser.ToolCall{
		Name:  "Bash",
		Input: []byte(`{"command":"cat /etc/hosts"}`),
	}
	// Act
	got := topics.ExtractFiles([]parser.ToolCall{tc})
	// Assert
	assert.Empty(t, got)
}

func TestExtractFiles_BashWithPathKey_Ignored(t *testing.T) {
	// Arrange — Bash input that contains a "path" key (should still be skipped)
	tc := parser.ToolCall{
		Name:  "Bash",
		Input: []byte(`{"command":"cat /etc/hosts","path":"/etc/hosts"}`),
	}
	// Act
	got := topics.ExtractFiles([]parser.ToolCall{tc})
	// Assert
	assert.Empty(t, got)
}

func TestExtractFiles_MalformedJSON_Skipped(t *testing.T) {
	// Arrange
	tc := parser.ToolCall{
		Name:  "Read",
		Input: []byte(`{not valid json`),
	}
	// Act
	got := topics.ExtractFiles([]parser.ToolCall{tc})
	// Assert
	assert.Empty(t, got)
}

func TestJaccard_IdenticalSets_Returns1(t *testing.T) {
	got := topics.Jaccard([]string{"a", "b"}, []string{"a", "b"})
	assert.InDelta(t, 1.0, got, 0.001)
}

func TestJaccard_DisjointSets_Returns0(t *testing.T) {
	got := topics.Jaccard([]string{"a"}, []string{"b"})
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestJaccard_PartialOverlap(t *testing.T) {
	// {a,b} ∩ {b,c} = {b}, union = {a,b,c} → 1/3
	got := topics.Jaccard([]string{"a", "b"}, []string{"b", "c"})
	assert.InDelta(t, 1.0/3.0, got, 0.001)
}

func TestJaccard_EmptyBothSets_Returns1(t *testing.T) {
	got := topics.Jaccard(nil, nil)
	assert.InDelta(t, 1.0, got, 0.001)
}

func TestJaccard_OneEmpty_Returns0(t *testing.T) {
	got := topics.Jaccard([]string{"a"}, nil)
	assert.InDelta(t, 0.0, got, 0.001)
}
