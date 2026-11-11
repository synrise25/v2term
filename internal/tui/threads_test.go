package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"v2term/internal/preview"
	"v2term/internal/v2ex"
)

func TestThreadGuidesConnectSiblingsAndStopAtLastChild(t *testing.T) {
	p := readingPage()
	p.Replies = []v2ex.Reply{
		{ID: 8, Floor: 8, Author: "alice", Content: "独立回复"},
		{ID: 9, Floor: 9, Author: "bob", Content: "@alice #8 第一个子回复", Body: []v2ex.Element{{Text: "第一段\n\n第二段"}, {Image: true, URL: "https://example.org/p.png"}}},
		{ID: 10, Floor: 10, Author: "carol", Content: "@alice #8 最后一个子回复"},
		{ID: 13, Floor: 13, Author: "alice", Content: "@bob #9 回复子楼层"},
		{ID: 20, Floor: 20, Author: "dave", Content: "另一个独立回复"},
	}
	m := testTopicModel(p)
	m.images["reply:9:1"] = &imageState{expanded: true, image: &preview.Image{Columns: 8, Rows: []preview.Row{{Native: "IMAGE"}}}}
	m.rebuild()
	want := map[int]threadGuide{9: {head: "├─", body: "│ "}, 13: {head: "│ └─", body: "│   "}, 10: {head: "└─", body: "  "}}
	for _, s := range m.selections {
		if s.hit.kind != "reply" {
			continue
		}
		l := m.lines[s.at]
		expected := want[s.hit.id]
		guide := ansi.StringWidth(expected.head)
		if got := v2ex.SafeText(ansi.Cut(l.normal, m.contentLeft(), m.contentLeft()+guide)); got != expected.head {
			t.Fatalf("floor %d header guide: %q, want %q", s.hit.id, got, expected.head)
		}
		end := len(m.lines)
		for _, next := range m.selections {
			if next.at > s.at {
				end = next.at
				break
			}
		}
		for _, row := range m.lines[s.at+1 : end] {
			if got := v2ex.SafeText(ansi.Cut(row.normal, m.contentLeft(), m.contentLeft()+guide)); got != expected.body {
				t.Fatalf("floor %d paragraph/image/spacing guide: %q, want %q", s.hit.id, got, expected.body)
			}
		}
	}
}

func TestSingleChildChainHasNoDanglingVerticalStems(t *testing.T) {
	replies := []v2ex.Reply{{ID: 8, Floor: 8, Author: "a"}, {ID: 9, Floor: 9, Author: "b", Content: "@a #8"}, {ID: 13, Floor: 13, Author: "a", Content: "@b #9"}}
	guides := threadGuides(v2ex.Conversation(replies), 4)
	if guides[1].head != "└─" || guides[1].body != "  " || guides[2].head != "  └─" || guides[2].body != "    " {
		t.Fatalf("single-child chain has misleading continuation: %+v", guides)
	}
}

func TestThreadIndentCapDoesNotInventSiblingBranches(t *testing.T) {
	var replies []v2ex.Reply
	for i := 1; i <= 7; i++ {
		content := "root"
		if i > 1 {
			content = fmt.Sprintf("#%d child", i-1)
		}
		replies = append(replies, v2ex.Reply{ID: i, Floor: i, Author: "a", Content: content})
	}
	replies = append(replies, v2ex.Reply{ID: 20, Floor: 20, Author: "b", Content: "new root"})
	guides := threadGuides(v2ex.Conversation(replies), 2)
	for _, g := range guides[3:7] {
		if ansi.StringWidth(g.head) != 4 || strings.ContainsAny(g.head, "├└│") || g.head != g.body {
			t.Fatalf("flattened descendant showed a false sibling connection: %+v", g)
		}
	}
	if guides[7].head != "" || guides[7].body != "" {
		t.Fatal("previous thread's guides leaked into a new root")
	}
}
