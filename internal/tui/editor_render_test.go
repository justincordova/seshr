package tui_test

import (
	"strings"
	"testing"

	"github.com/justincordova/agentlens/internal/topics"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestRenderCheckboxRow_Unselected(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	row := tui.RenderCheckboxRow(0, topics.Topic{Label: "Setup", TokenCount: 12400}, false, false, 60, s)
	assert.True(t, strings.Contains(row, "[ ]"))
	assert.Contains(t, row, "Setup")
	assert.Contains(t, row, "12,400")
}

func TestRenderCheckboxRow_Selected(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	row := tui.RenderCheckboxRow(2, topics.Topic{Label: "House", TokenCount: 2100}, true, false, 60, s)
	assert.True(t, strings.Contains(row, "[x]"))
}

func TestRenderSelectionFooter_SingularVsPlural(t *testing.T) {
	one := tui.RenderSelectionFooter(1, 0, 2100)
	many := tui.RenderSelectionFooter(3, 0, 42000)
	assert.Contains(t, one, "1 topic selected")
	assert.Contains(t, many, "3 topics selected")
	assert.Contains(t, one, "2,100")
	assert.Contains(t, many, "42,000")
}
