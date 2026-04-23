package opencode

import (
	"encoding/json"
	"log/slog"
	"sort"
)

// findLeaf returns the message with the largest time_created among those
// that are not anyone's parent. Ties are broken by lexical id.
//
// OC stores parentID inside assistant message.data JSON. User messages never
// have a parent. Branches are formed when a single user message has multiple
// assistant children (regen). We pick the most-recent leaf assuming the user
// always continues down the latest branch. Empty input returns the zero row.
func findLeaf(msgs []messageRow) messageRow {
	if len(msgs) == 0 {
		return messageRow{}
	}

	// Collect all IDs that are a parent of someone.
	parents := make(map[string]struct{}, len(msgs))
	for _, m := range msgs {
		env, err := decodeEnvelope(m.Data)
		if err != nil || env.ParentID == "" {
			continue
		}
		parents[env.ParentID] = struct{}{}
	}

	// Pick leaves: messages whose id is not in `parents`.
	var leaves []messageRow
	for _, m := range msgs {
		if _, isParent := parents[m.ID]; !isParent {
			leaves = append(leaves, m)
		}
	}
	if len(leaves) == 0 {
		// Pathological: every message is someone's parent (cycle). Fall back
		// to the most-recent message overall.
		leaves = append([]messageRow{}, msgs...)
	}
	sort.Slice(leaves, func(i, j int) bool {
		if leaves[i].TimeCreated != leaves[j].TimeCreated {
			return leaves[i].TimeCreated > leaves[j].TimeCreated
		}
		return leaves[i].ID > leaves[j].ID
	})
	return leaves[0]
}

// walkParents walks parent links from leafID back to the root via the
// parentID field inside each message's data JSON, then returns the chain in
// root-to-leaf order.
//
// Bounded by len(all) iterations to defend against accidental cycles. A
// cycle logs at error level and truncates the chain at the repeat.
//
// NOTE: Primary chain construction uses takeCurrentBranch (branch.go);
// walkParents is retained for Phase 10 branch-change detection where we
// need to verify that a newly-appended message's ancestry still passes
// through the previous-leaf cursor.
//
//nolint:unused // used by Phase 10 LoadIncremental branch-change detection
func walkParents(all []messageRow, leafID string) []messageRow {
	if leafID == "" || len(all) == 0 {
		return nil
	}
	byID := make(map[string]messageRow, len(all))
	for _, m := range all {
		byID[m.ID] = m
	}

	seen := make(map[string]struct{}, len(all))
	var reversed []messageRow
	current := leafID
	for i := 0; i < len(all) && current != ""; i++ {
		msg, ok := byID[current]
		if !ok {
			// Parent missing — truncate silently; orphaned row is still
			// loadable for the user.
			break
		}
		if _, dup := seen[current]; dup {
			slog.Error("opencode: cycle detected while walking parents",
				"session", msg.SessionID, "msg", current)
			break
		}
		seen[current] = struct{}{}
		reversed = append(reversed, msg)
		// Look up parent via message envelope.
		env, err := decodeEnvelope(msg.Data)
		if err != nil {
			// Can't decode envelope → can't follow parent chain further.
			break
		}
		current = env.ParentID
	}

	// Reverse so index 0 is root.
	out := make([]messageRow, len(reversed))
	for i, m := range reversed {
		out[len(reversed)-1-i] = m
	}
	return out
}

// parentIDOf is a small helper for callers (branch-change detection in
// Phase 10) that need only the parent without a full envelope decode.
func parentIDOf(m messageRow) string {
	// Quick path: look for the "parentID" field without a full unmarshal.
	// Fall back to a proper decode if the heuristic misses.
	var head struct {
		ParentID string `json:"parentID"`
	}
	if err := json.Unmarshal(m.Data, &head); err != nil {
		return ""
	}
	return head.ParentID
}
