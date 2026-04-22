package backend

// Selection carries the set of turn indices to prune, as accepted by SessionEditor.
// TurnIndices are indices into session.Session.Turns.
type Selection struct {
	TurnIndices []int
}
