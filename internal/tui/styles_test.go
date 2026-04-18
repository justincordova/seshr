package tui_test

import (
	"testing"

	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestNewStyles_IncludesReplayStyles(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())

	for name, fn := range map[string]func() string{
		"Thinking":                 func() string { return s.Thinking.Render("…") },
		"ToolResultExpandedHeader": func() string { return s.ToolResultExpandedHeader.Render("expanded") },
	} {
		assert.NotEmpty(t, fn(), "style %s must render non-empty output", name)
	}
}
