package opencode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// msg constructs a messageRow helper for table-driven tests.
func msg(id, role, parent string, tc int64) messageRow {
	data := map[string]any{"role": role}
	if parent != "" {
		data["parentID"] = parent
	}
	raw, _ := json.Marshal(data)
	return messageRow{ID: id, TimeCreated: tc, Data: raw}
}

func TestTakeCurrentBranch_Empty(t *testing.T) {
	assert.Nil(t, takeCurrentBranch(nil))
}

func TestTakeCurrentBranch_AgentLoopSiblings_AllKept(t *testing.T) {
	// Three assistant messages sharing a parent, 2s apart each — agent loop.
	msgs := []messageRow{
		msg("u1", "user", "", 1000),
		msg("a1", "assistant", "u1", 2000),
		msg("a2", "assistant", "u1", 4000),
		msg("a3", "assistant", "u1", 6000),
	}

	chain := takeCurrentBranch(msgs)

	assert.Len(t, chain, 4, "agent-loop siblings must all be kept")
}

func TestTakeCurrentBranch_RegenSiblings_OldestDropped(t *testing.T) {
	// a1 and a2 share a parent but are 90s apart — regen.
	msgs := []messageRow{
		msg("u1", "user", "", 0),
		msg("a1", "assistant", "u1", 1000),
		msg("a2", "assistant", "u1", 91_000),
	}

	chain := takeCurrentBranch(msgs)

	assert.Len(t, chain, 2, "regen drops the older sibling")
	assert.Equal(t, "u1", chain[0].ID)
	assert.Equal(t, "a2", chain[1].ID, "expected newer assistant to survive")
}

func TestTakeCurrentBranch_RegenGapButLaterContinuation_AllKept(t *testing.T) {
	// a1 and a2 share parent u1 and are >60s apart (would look like a regen),
	// but the conversation CONTINUES past a2 (u2/a3). Because parentID only
	// links assistant→user, we cannot prove a1 was superseded, so dropping it
	// would risk losing a legitimate turn. The safe-leaning rule keeps all.
	msgs := []messageRow{
		msg("u1", "user", "", 0),
		msg("a1", "assistant", "u1", 1000),
		msg("a2", "assistant", "u1", 91_000),
		msg("u2", "user", "", 95_000),
		msg("a3", "assistant", "u2", 96_000),
	}

	chain := takeCurrentBranch(msgs)

	assert.Len(t, chain, 5, "must not drop a sibling when the session continued past it")
}

func TestTakeCurrentBranch_SortsByTimeThenID(t *testing.T) {
	// Same time_created: sort by ID ascending.
	msgs := []messageRow{
		msg("b", "user", "", 1000),
		msg("a", "user", "", 1000),
	}

	chain := takeCurrentBranch(msgs)

	assert.Equal(t, "a", chain[0].ID)
	assert.Equal(t, "b", chain[1].ID)
}

func TestFindLeaf_PicksNewestNonParent(t *testing.T) {
	msgs := []messageRow{
		msg("u1", "user", "", 1000),
		msg("a1", "assistant", "u1", 2000), // a1 is NOT a parent of anything → leaf candidate
		msg("a2", "assistant", "u1", 3000), // newer leaf candidate
	}

	leaf := findLeaf(msgs)

	assert.Equal(t, "a2", leaf.ID)
}

func TestParentIDOf_ExtractsWithoutFullDecode(t *testing.T) {
	m := msg("a1", "assistant", "u1", 1000)

	assert.Equal(t, "u1", parentIDOf(m))
}

func TestParentIDOf_UserMessage_EmptyString(t *testing.T) {
	m := msg("u1", "user", "", 1000)

	assert.Equal(t, "", parentIDOf(m))
}
