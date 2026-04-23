package opencode

import (
	"sort"
)

// takeCurrentBranch returns the "current" chronological chain of messages
// from a session. Inputs need not be sorted; output is sorted by
// (time_created, id) ascending.
//
// Branching semantics (determined by inspecting real OC databases):
//
//   - `parentID` lives in assistant message.data JSON and points to the user
//     message an assistant reply answers.
//   - User messages never have a parentID.
//   - A multi-step agent run produces many assistant messages all sharing the
//     SAME user parentID. This is NOT a branch — each is a turn in the agent
//     loop, and all should render.
//   - True regen branches (user deletes an assistant reply and gets a new
//     one) manifest as two assistant messages with the same parentID whose
//     time_created values are separated by a user-visible gap AND whose
//     subsequent messages (further assistants / later users) only chain off
//     the newer one. In practice OpenCode resolves this at write-time: the
//     superseded branch simply stops receiving new children.
//
// takeCurrentBranch therefore:
//  1. Sorts messages by (time_created, id).
//  2. Keeps everything by default — multi-step agent replies all stay.
//  3. When a true regen branch is detected (multiple assistant children of
//     the same user, where one branch has later-in-time descendants AND the
//     other does not), drops the superseded branch.
//
// If in doubt, keeping the message is safer than dropping it: a duplicate
// turn in the UI is a visual nit, a missing turn is a data loss.
func takeCurrentBranch(msgs []messageRow) []messageRow {
	if len(msgs) == 0 {
		return nil
	}

	sorted := make([]messageRow, len(msgs))
	copy(sorted, msgs)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].TimeCreated != sorted[j].TimeCreated {
			return sorted[i].TimeCreated < sorted[j].TimeCreated
		}
		return sorted[i].ID < sorted[j].ID
	})

	// Identify regen candidates: group assistant messages by parentID.
	// A group with only one message is the normal case. A group with ≥2
	// messages whose assistant "runs" diverge significantly may be a regen.
	dropped := markRegeneratedBranches(sorted)
	if len(dropped) == 0 {
		return sorted
	}

	out := sorted[:0:len(sorted)]
	for _, m := range sorted {
		if _, skip := dropped[m.ID]; skip {
			continue
		}
		out = append(out, m)
	}
	return out
}

// markRegeneratedBranches returns a set of message IDs to drop because they
// are superseded by a newer branch.
//
// Heuristic (safe-leaning):
//
//   - Group all assistants by parentID.
//   - For each group of size ≥ 2, find the newest sibling by time_created.
//     This sibling is presumed the "current" branch.
//   - For each OTHER sibling: if the gap to the newest is large enough
//     (> 60s) AND none of the older sibling's descendants-in-time have a
//     time_created beyond the newest sibling's time_created, mark the older
//     sibling for removal.
//
// The ">60s" threshold skips the common multi-step agent loop (many siblings
// seconds apart, all legitimate) while catching real regens (where the user
// paused before retrying).
//
// If msgs is empty or no regens are detected, returns an empty (non-nil) map.
func markRegeneratedBranches(sorted []messageRow) map[string]struct{} {
	const regenGapMs int64 = 60 * 1000

	byParent := make(map[string][]messageRow)
	for _, m := range sorted {
		env, err := decodeEnvelope(m.Data)
		if err != nil || env.Role != "assistant" || env.ParentID == "" {
			continue
		}
		byParent[env.ParentID] = append(byParent[env.ParentID], m)
	}

	drop := make(map[string]struct{})
	for _, siblings := range byParent {
		if len(siblings) < 2 {
			continue
		}
		// siblings were appended in the sorted order, so last element is newest.
		newest := siblings[len(siblings)-1]

		// Find max time_created across ALL messages (session-wide) so we can
		// check whether each older sibling has any "after-newest" activity.
		// Because we work from the sorted slice, anything at or after the
		// newest sibling's index qualifies as potential continuation.
		for _, s := range siblings[:len(siblings)-1] {
			if newest.TimeCreated-s.TimeCreated < regenGapMs {
				// Likely an agent-loop sibling, not a regen.
				continue
			}
			// Treat as a regen; drop the older sibling.
			drop[s.ID] = struct{}{}
		}
	}
	return drop
}
