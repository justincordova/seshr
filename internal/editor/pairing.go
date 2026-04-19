package editor

import (
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
)

type Selection struct {
	Topics map[int]bool
	Turns  map[int]bool
}

func ExpandSelection(sess *parser.Session, ts []topics.Topic, in Selection) Selection {
	out := Selection{Turns: map[int]bool{}}

	for topicIdx, sel := range in.Topics {
		if !sel || topicIdx < 0 || topicIdx >= len(ts) {
			continue
		}
		for _, turnIdx := range ts[topicIdx].TurnIndices {
			out.Turns[turnIdx] = true
		}
	}
	for turnIdx, sel := range in.Turns {
		if sel {
			out.Turns[turnIdx] = true
		}
	}

	for idx := range out.Turns {
		if idx < 0 || idx >= len(sess.Turns) {
			delete(out.Turns, idx)
			continue
		}
		switch sess.Turns[idx].Role {
		case parser.RoleSystem, parser.RoleSummary:
			delete(out.Turns, idx)
		}
	}

	for idx := range out.Turns {
		pullUserAssistantPartner(sess, idx, out.Turns)
	}

	pullToolPartners(sess, out.Turns)

	return out
}

// pullUserAssistantPartner ensures user and assistant turns are always deleted
// as a pair. Given a turn at idx, it searches forward then backward for the
// nearest partner of the opposite role (user↔assistant), stopping when it hits
// another turn of the same role as idx. Tool result turns between them are
// skipped over.
//
// Limitation: in sessions with non-sequential interleaving (e.g. parallel
// subagent invocations sharing the same turn list), this adjacency-based search
// may pull in a partner from a different conversation branch. This is acceptable
// for v1 because Claude Code sessions are strictly sequential.
func pullUserAssistantPartner(sess *parser.Session, idx int, sel map[int]bool) {
	role := sess.Turns[idx].Role
	var want parser.Role
	switch role {
	case parser.RoleUser:
		want = parser.RoleAssistant
	case parser.RoleAssistant:
		want = parser.RoleUser
	default:
		return
	}
	for j := idx + 1; j < len(sess.Turns); j++ {
		if sess.Turns[j].Role == want {
			sel[j] = true
			return
		}
		if sess.Turns[j].Role == role {
			break
		}
	}
	for j := idx - 1; j >= 0; j-- {
		if sess.Turns[j].Role == want {
			sel[j] = true
			return
		}
		if sess.Turns[j].Role == role {
			break
		}
	}
}

func pullToolPartners(sess *parser.Session, sel map[int]bool) {
	useIdx := map[string]int{}
	resultIdx := map[string]int{}
	for i, turn := range sess.Turns {
		for _, tc := range turn.ToolCalls {
			useIdx[tc.ID] = i
		}
		for _, tr := range turn.ToolResults {
			resultIdx[tr.ID] = i
		}
	}
	initial := make([]int, 0, len(sel))
	for i := range sel {
		initial = append(initial, i)
	}
	for _, i := range initial {
		for _, tc := range sess.Turns[i].ToolCalls {
			if j, ok := resultIdx[tc.ID]; ok {
				sel[j] = true
			}
		}
		for _, tr := range sess.Turns[i].ToolResults {
			if j, ok := useIdx[tr.ID]; ok {
				sel[j] = true
			}
		}
	}
}
