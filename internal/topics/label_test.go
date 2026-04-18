package topics_test

import (
	"testing"

	"github.com/justincordova/seshly/internal/parser"
	"github.com/justincordova/seshly/internal/topics"
	"github.com/stretchr/testify/assert"
)

func TestLabelFor_KeywordRich_UsesKeywords(t *testing.T) {
	turns := []parser.Turn{
		{Role: parser.RoleUser, Content: "add jwt auth middleware"},
		{Role: parser.RoleAssistant, Content: "adding jwt middleware; auth token verified"},
		{Role: parser.RoleUser, Content: "also add a jwt refresh endpoint"},
	}
	got := topics.LabelFor(turns, 0)
	assert.Contains(t, got, "jwt")
	assert.Contains(t, got, "auth")
	assert.Contains(t, got, "middleware")
}

func TestLabelFor_OnlyStopwords_FallsBackToUserMessage(t *testing.T) {
	turns := []parser.Turn{
		{Role: parser.RoleUser, Content: "is it the one?"},
	}
	got := topics.LabelFor(turns, 3)
	assert.Equal(t, "is it the one?", got)
}

func TestLabelFor_LongUserMessage_Truncates(t *testing.T) {
	turns := []parser.Turn{
		{Role: parser.RoleUser, Content: "the the the the the the the the the the the the the the the the the the the the"},
	}
	got := topics.LabelFor(turns, 0)
	assert.LessOrEqual(t, len([]rune(got)), 40)
}

func TestLabelFor_EmptyTurns_UsesIndexFallback(t *testing.T) {
	got := topics.LabelFor(nil, 2)
	assert.Equal(t, "Topic 3", got)
}

func TestLabelFor_NoUserTurn_UsesAssistantForFallback(t *testing.T) {
	turns := []parser.Turn{
		{Role: parser.RoleAssistant, Content: "the and of to"},
	}
	got := topics.LabelFor(turns, 4)
	assert.Equal(t, "Topic 5", got)
}
