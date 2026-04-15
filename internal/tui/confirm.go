package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Confirm is a minimal yes/no modal. Caller constructs with NewConfirm,
// passes key messages through Update, and on Done() inspects Confirmed().
type Confirm struct {
	title     string
	body      string
	done      bool
	confirmed bool
	styles    Styles
}

// NewConfirm returns an unresolved Confirm with the given strings.
func NewConfirm(title, body string) Confirm {
	return Confirm{title: title, body: body, styles: NewStyles(CatppuccinMocha())}
}

// Done reports whether the user has chosen yes or no.
func (c Confirm) Done() bool { return c.done }

// Confirmed returns true only after Done is true and the user chose yes.
func (c Confirm) Confirmed() bool { return c.confirmed }

// Init satisfies tea.Model.
func (c Confirm) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (c Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	switch km.String() {
	case "y", "Y":
		c.done = true
		c.confirmed = true
	case "n", "N", "esc":
		c.done = true
		c.confirmed = false
	}
	return c, nil
}

// View satisfies tea.Model.
func (c Confirm) View() string {
	return c.styles.App.Render(
		c.styles.Title.Render(c.title) + "\n\n" +
			c.body + "\n\n" +
			c.styles.Hint.Render("y/n"),
	)
}
