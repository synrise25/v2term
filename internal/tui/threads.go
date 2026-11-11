package tui

import (
	"strings"

	"v2term/internal/v2ex"
)

type threadGuide struct{ head, body string }

// A vertical stem means a later sibling still exists. Root threads are separate
// trees; stems end at the last child instead of following indentation blindly.
func threadGuides(replies []v2ex.ConversationReply, limit int) []threadGuide {
	last := make(map[int]int, len(replies))
	for i, r := range replies {
		if r.Depth > 0 {
			last[r.ParentFloor] = i
		}
	}
	guides := make([]threadGuide, len(replies))
	var stems []bool
	for i, r := range replies {
		if r.Depth == 0 {
			stems = stems[:0]
			continue
		}
		stems = stems[:min(len(stems), r.Depth-1)]
		for len(stems) < r.Depth-1 {
			stems = append(stems, false)
		}
		stems = append(stems, last[r.ParentFloor] != i)
		var head, body strings.Builder
		for depth, continues := range stems[:min(len(stems), limit)] {
			stem := "  "
			if continues {
				stem = "│ "
			}
			body.WriteString(stem)
			if depth == r.Depth-1 {
				stem = "└─"
				if continues {
					stem = "├─"
				}
			}
			head.WriteString(stem)
		}
		guides[i] = threadGuide{head: head.String(), body: body.String()}
		// Deeper levels keep their indentation cap and explicit parent floor;
		// do not draw a false sibling branch for a flattened descendant.
	}
	return guides
}
