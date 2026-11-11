package tui

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"v2term/internal/preview"
	"v2term/internal/v2ex"
)

func hotPage() v2ex.Page {
	p := readingPage()
	p.Replies = []v2ex.Reply{
		{ID: 101, Floor: 1, Author: "alice", Content: "独立回复"},
		{ID: 102, Floor: 2, Author: "bob", Content: "@alice #1 子回复", Thanks: 6},
		{ID: 103, Floor: 3, Author: "carol", Content: "另一个独立回复", Thanks: 5},
		{ID: 104, Floor: 4, Author: "dave", Content: "@bob #2 深层回复", Thanks: 10, Body: []v2ex.Element{{Text: "热评正文"}, {Image: true, URL: "https://example.org/p.png"}}},
	}
	return p
}

func hotConversationPage() v2ex.Page {
	p := hotPage()
	p.Replies = append(p.Replies,
		v2ex.Reply{ID: 105, Floor: 5, Author: "eve", Content: "@dave #4 后续回复"},
		v2ex.Reply{ID: 106, Floor: 6, Author: "frank", Content: "@bob #2 同级回复", Thanks: 1},
		v2ex.Reply{ID: 107, Floor: 7, Author: "grace", Content: "@carol #3 另一条热评的回复"},
		v2ex.Reply{ID: 108, Floor: 8, Author: "heidi", Content: "不相关的独立回复"},
		v2ex.Reply{ID: 109, Floor: 9, Author: "ivan", Content: "@eve #5 更深的回复"},
	)
	return p
}

func TestHotThreadsIncludeDescendantsAndMergeOverlappingBranches(t *testing.T) {
	m := testTopicModel(hotConversationPage())
	conversation := m.replyOrder()
	before := slices.Clone(conversation)
	for _, tc := range []struct {
		limit int
		ids   []int
		depth []int
		count int
	}{
		{1, []int{104, 105, 109}, []int{0, 1, 2}, 1},
		{2, []int{102, 104, 105, 109, 106}, []int{0, 1, 2, 3, 1}, 2},
		{3, []int{102, 104, 105, 109, 106, 103, 107}, []int{0, 1, 2, 3, 1, 0, 1}, 3},
		{0, nil, nil, 0},
	} {
		m.options.HotReplies = tc.limit
		replies, count := m.hotThreads(conversation)
		var ids, depth []int
		for _, r := range replies {
			ids, depth = append(ids, r.ID), append(depth, r.Depth)
			for _, original := range conversation {
				if original.ID == r.ID && (r.ParentFloor != original.ParentFloor || r.Inferred != original.Inferred) {
					t.Fatal("hot thread changed a parent reference")
				}
			}
		}
		if count != tc.count || !slices.Equal(ids, tc.ids) || !slices.Equal(depth, tc.depth) || !reflect.DeepEqual(conversation, before) {
			t.Fatalf("limit %d: got count %d, ids %v, depth %v; want count %d, ids %v, depth %v", tc.limit, count, ids, depth, tc.count, tc.ids, tc.depth)
		}
	}
	m.options.HotReplies = 3
	replies, _ := m.hotThreads(conversation)
	guides := threadGuides(replies, 4)
	for i, want := range []string{"", "├─", "│ └─", "│   └─", "└─", "", "└─"} {
		if guides[i].head != want {
			t.Fatalf("hot floor %d guide %q, want %q", replies[i].Floor, guides[i].head, want)
		}
	}
}

func TestHotRepliesRankThresholdAndLimitPreserveOriginalTree(t *testing.T) {
	p := hotPage()
	p.Replies = append(p.Replies,
		v2ex.Reply{ID: 105, Floor: 5, Author: "eve", Thanks: 6},
		v2ex.Reply{ID: 106, Floor: 6, Author: "frank", Thanks: 3},
		v2ex.Reply{ID: 107, Floor: 7, Author: "grace", Thanks: 4},
	)
	// Input order should not affect ties or mutate the source replies.
	slices.Reverse(p.Replies)
	m := testTopicModel(p)
	ordered := m.replyOrder()
	before := slices.Clone(ordered)
	for _, tc := range []struct {
		limit, threshold int
		want             []int
	}{
		{3, 3, []int{104, 102, 105}},
		{5, 3, []int{104, 102, 105, 103, 107}},
		{100, 3, []int{104, 102, 105, 103, 107, 106}},
		{5, 4, []int{104, 102, 105, 103, 107}},
		{3, 10, []int{104}},
		{3, 11, nil},
		{0, 0, nil},
		{100, 0, []int{104, 102, 105, 103, 107, 106, 101}},
	} {
		m.options.HotReplies, m.options.HotMinThanks = tc.limit, tc.threshold
		var got []int
		for _, r := range m.hotReplies(ordered) {
			got = append(got, r.ID)
			if r.Depth != 0 || r.ID == 104 && r.ParentFloor != 2 {
				t.Fatal("hot reply lost its original reference or kept nested indentation")
			}
		}
		if !slices.Equal(got, tc.want) || !reflect.DeepEqual(ordered, before) {
			t.Fatalf("limit %d, threshold %d: got %v, want %v, or original tree changed", tc.limit, tc.threshold, got, tc.want)
		}
	}
	for i := range m.page.Replies {
		m.page.Replies[i].Thanks = 2
	}
	m.options = DefaultOptions()
	m.rebuild()
	for _, s := range m.selections {
		if s.hit.hot {
			t.Fatal("reply below the threshold was featured")
		}
	}
	for _, l := range m.lines {
		if strings.Contains(v2ex.SafeText(l.text), "── 热评") {
			t.Fatal("empty hot reply section was rendered")
		}
	}
}

func TestHotReplyMinimumIncludesExactBoundaryInBothViews(t *testing.T) {
	p := hotPage()
	p.Replies = []v2ex.Reply{
		{ID: 101, Floor: 1, Author: "alice", Content: "恰好三次感谢", Thanks: 3},
		{ID: 102, Floor: 2, Author: "bob", Content: "低于最低感谢数", Thanks: 2},
		{ID: 103, Floor: 3, Author: "carol", Content: "四次感谢", Thanks: 4},
	}
	m := testTopicModel(p)
	for _, conversation := range []bool{true, false} {
		m.session.Conversation = conversation
		m.rebuild()
		var hot []int
		for _, s := range m.selections {
			if s.hit.hot {
				hot = append(hot, s.hit.id)
			}
		}
		if !slices.Equal(hot, []int{103, 101}) || !strings.Contains(v2ex.SafeText(m.View().Content), "── 热评 2") {
			t.Fatalf("conversation %t: minimum of 3 excluded a reply with exactly 3 thanks: %v", conversation, hot)
		}
	}
}

func TestHotReplyNavigationSelectsEachCopyExactlyOnce(t *testing.T) {
	for _, width := range []int{24, 80, 160} {
		m := testTopicModel(hotConversationPage())
		m.resize(width, 12)
		want := []hit{{kind: "op"}}
		for _, id := range []int{102, 104, 105, 109, 106, 103, 107} {
			want = append(want, hit{kind: "reply", id: id, hot: true})
		}
		for _, id := range []int{101, 102, 104, 105, 109, 106, 103, 107, 108} {
			want = append(want, hit{kind: "reply", id: id})
		}
		for i, h := range want {
			if i > 0 {
				m.Update(key(tea.KeyDown, ""))
			}
			if m.selected() != h {
				t.Fatalf("width %d: selection %d got %+v, want %+v", width, i, m.selected(), h)
			}
			matches := 0
			for _, s := range m.selections {
				if m.isSelected(s.hit) {
					matches++
				}
			}
			s := m.selections[m.selectionIndex()]
			row := 4 + s.at - m.session.Current.Offset
			rows := strings.Split(m.View().Content, "\n")
			if matches != 1 || row < 4 || row >= m.height-1 || rows[row] != m.lines[s.at].highlight {
				t.Fatalf("width %d, selection %d: hot and original highlight collided or selected heading left the screen", width, i)
			}
		}
		for i := len(want) - 2; i >= 0; i-- {
			m.Update(key(tea.KeyUp, ""))
			if m.selected() != want[i] {
				t.Fatal("up skipped a hot or original reply")
			}
		}
	}
}

func TestHotReplyMouseWheelActionsAndRestoreUseTheVisibleCopy(t *testing.T) {
	m := testTopicModel(hotPage())
	m.resize(80, 12)
	hot := m.selections[2]
	m.setOffset(hot.at)
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft() + 3, Y: 4})
	if m.selected() != hot.hit {
		t.Fatal("click selected the original instead of the hot reply")
	}
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyEnter, ""))
	if m.mode != "reply" || m.editor.Value() != "@dave #4 " {
		t.Fatal("hot reply action did not target the original floor")
	}
	m.Update(key(tea.KeyEscape, ""))
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyEnter, ""))
	if m.mode != "thank" || m.confirmation.id != 104 || m.confirmation.kind != "reply" || !m.session.Current.Hot || m.api.(*fakeAPI).thanks != 0 {
		t.Fatal("hot reply thanks skipped confirmation or changed selection")
	}
	m.Update(key(tea.KeyEscape, ""))
	saved := m.Session()
	restored := New(context.Background(), m.api, saved)
	restored.resize(m.width, m.height)
	deliver(restored, restored.Init())
	if restored.selected() != hot.hit || restored.Session().Current != saved.Current {
		t.Fatalf("restoring hot copy %+v at %+v got %+v at %+v", hot.hit, saved.Current, restored.selected(), restored.Session().Current)
	}
	restored.toggleConversation()
	if restored.selected() != hot.hit {
		t.Fatal("changing reply order lost the selected hot copy")
	}
	restored.options.HotReplies = 0
	restored.rebuild()
	if restored.selected().id != 104 || restored.selected().hot {
		t.Fatal("disabling hot replies did not fall back to the original floor")
	}
	for _, s := range m.selections {
		if !s.hit.hot && s.hit.id == 104 {
			m.setOffset(s.at)
			if m.selected() != s.hit {
				t.Fatal("scrolling to the original selected its hot copy")
			}
		}
	}
}

func TestHotDescendantActionsAndSelectionSurviveTimeViewAndRestore(t *testing.T) {
	m := testTopicModel(hotConversationPage())
	m.resize(80, 14)
	for _, s := range m.selections {
		if s.hit.hot && s.hit.id == 105 {
			m.setOffset(s.at)
		}
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft() + 6, Y: 4})
	if m.selected() != (hit{kind: "reply", id: 105, hot: true}) {
		t.Fatal("hot descendant mouse selection missed")
	}
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyEnter, ""))
	if m.mode != "reply" || m.editor.Value() != "@eve #5 " {
		t.Fatal("hot descendant reply targeted its parent")
	}
	m.Update(key(tea.KeyEscape, ""))
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyEnter, ""))
	if m.mode != "thank" || m.confirmation.id != 105 || m.confirmation.kind != "reply" || m.api.(*fakeAPI).thanks != 0 {
		t.Fatal("hot descendant thanks targeted its parent or skipped confirmation")
	}
	m.Update(key(tea.KeyEscape, ""))
	m.toggleConversation()
	var hotIDs, indents []int
	for _, s := range m.selections {
		if s.hit.hot {
			hotIDs = append(hotIDs, s.hit.id)
			indents = append(indents, m.lines[s.at].indent)
		}
	}
	if !slices.Equal(hotIDs, []int{102, 104, 105, 109, 106, 103, 107}) || !slices.Equal(indents, []int{0, 2, 4, 6, 2, 0, 2}) {
		t.Fatalf("time view lost hot conversation order or indentation: %v, %v", hotIDs, indents)
	}
	saved := m.Session()
	restored := New(context.Background(), m.api, saved)
	restored.resize(m.width, m.height)
	deliver(restored, restored.Init())
	if restored.selected() != m.selected() || restored.Session().Current != saved.Current {
		t.Fatal("restoring the time view lost the hot descendant selection")
	}
}

func TestHotReplyImageExpansionIsIndependentOfOriginal(t *testing.T) {
	m := testTopicModel(hotPage())
	m.resize(80, 12)
	im := &preview.Image{Columns: 4, Rows: []preview.Row{{Native: "IMAGE"}}}
	for _, key := range []string{"hot-reply:104:1", "reply:104:1"} {
		m.images[key] = &imageState{image: im}
	}
	m.toggleImage(hit{kind: "image", path: "hot-reply:104:1"})
	if !m.images["hot-reply:104:1"].expanded || m.images["reply:104:1"].expanded {
		t.Fatal("expanding a hot reply image also expanded the original")
	}
	for at, l := range m.lines {
		if l.imageRow != nil {
			m.setOffset(at)
			if m.selected().id != 104 || !m.selected().hot {
				t.Fatal("hot image scrolling lost its owning hot reply")
			}
		}
	}
	m.toggleImage(hit{kind: "image", path: "hot-reply:104:1"})
	for _, l := range m.lines {
		if l.imageRow != nil {
			t.Fatal("collapsed hot image left a visible row")
		}
	}
}
