package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"v2term/internal/v2ex"
)

// Hot replies are an additional view of the loaded page. Keep the original
// forest intact, including descendants and stable floor references.
func (m *Model) hotReplies(ordered []v2ex.ConversationReply) []v2ex.ConversationReply {
	if m.options.HotReplies == 0 {
		return nil
	}
	var hot []v2ex.ConversationReply
	for _, r := range ordered {
		if r.Thanks >= m.options.HotMinThanks {
			r.Depth = 0
			hot = append(hot, r)
		}
	}
	slices.SortStableFunc(hot, func(a, b v2ex.ConversationReply) int {
		if order := cmp.Compare(b.Thanks, a.Thanks); order != 0 {
			return order
		}
		return cmp.Compare(a.Floor, b.Floor)
	})
	return hot[:min(len(hot), m.options.HotReplies)]
}

// Attach complete descendant trees to the ranked hot replies. If both an
// ancestor and its descendant qualify, render their shared branch only once,
// ordered by its highest-ranked hot reply. Descendants do not consume the quota.
func (m *Model) hotThreads(conversation []v2ex.ConversationReply) ([]v2ex.ConversationReply, int) {
	hot := m.hotReplies(conversation)
	if len(hot) == 0 {
		return nil, 0
	}
	selected := make(map[int]bool, len(hot))
	for _, r := range hot {
		selected[r.Floor] = true
	}
	owners := make(map[int]int, len(conversation))
	depths := make(map[int]int, len(hot))
	branches := make(map[int][]v2ex.ConversationReply, len(hot))
	for _, r := range conversation {
		owner := owners[r.ParentFloor]
		if owner == 0 && selected[r.Floor] {
			owner = r.Floor
			depths[owner] = r.Depth
		}
		owners[r.Floor] = owner
		if owner != 0 {
			r.Depth -= depths[owner]
			branches[owner] = append(branches[owner], r)
		}
	}
	var replies []v2ex.ConversationReply
	for _, r := range hot {
		owner := owners[r.Floor]
		replies = append(replies, branches[owner]...)
		delete(branches, owner)
	}
	return replies, len(hot)
}

func (m *Model) appendDivider(label string) {
	m.lines = append(m.lines,
		line{text: muted.Render(label + strings.Repeat("─", max(0, m.contentWidth()-ansi.StringWidth(label))))},
		line{},
	)
}

func (m *Model) appendReplies(ordered []v2ex.ConversationReply, hot bool) {
	limit := 4
	if m.contentWidth() < 36 {
		limit = 2
	}
	guides := threadGuides(ordered, limit)
	for index, r := range ordered {
		indent := min(r.Depth, limit) * 2
		h := hit{kind: "reply", id: r.ID, hot: hot}
		relation := ""
		if r.ParentFloor > 0 {
			relation = fmt.Sprintf("  ↪ #%d", r.ParentFloor)
			if r.Inferred {
				relation += " ≈"
			}
		}
		thanks := ""
		if r.Thanks > 0 {
			thanks = fmt.Sprintf("  ♥ %d", r.Thanks)
		}
		start := len(m.lines)
		label := muted.Render(fmt.Sprintf("#%d", r.Floor)) + " "
		if author := m.page.Topic.Author; author != "" && strings.EqualFold(r.Author, author) {
			label += accent.Render("OP") + " "
		}
		label += strong.Render("@"+r.Author) + muted.Render(thanks+relation)
		m.appendHeader(label, r.Age, indent, h,
			inlineAction{label: "回复", hit: hit{kind: "reply-to", id: r.ID, hot: hot}},
			inlineAction{label: "感谢", hit: hit{kind: "thank-reply", id: r.ID, hot: hot}},
		)
		m.lines[start].heading, m.lines[start].first, m.lines[start].indent = true, true, indent
		owner := fmt.Sprintf("reply:%d", r.ID)
		if hot {
			owner = "hot-" + owner
		}
		m.appendContent(r.Body, r.Content, owner, h, indent+2)
		m.lines = append(m.lines, line{})
		for i := start; i < len(m.lines); i++ {
			m.lines[i].threadIndent = indent
			m.lines[i].threadPrefix = guides[index].body
			if i == start {
				m.lines[i].threadPrefix = guides[index].head
			}
		}
	}
}
