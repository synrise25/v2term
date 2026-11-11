package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/preview"
	"v2term/internal/v2ex"
)

func navigationModel() *Model {
	pages := map[string]v2ex.Page{"/t/1": readingPage()}
	for _, c := range categories {
		pages[c.path] = listPage(c.path)
	}
	m := New(context.Background(), &fakeAPI{pages: pages}, sessionAt("/?tab=tech"))
	deliver(m, m.Init())
	return m
}

func TestCategoryFocusSelectsWithoutNavigatingUntilEnter(t *testing.T) {
	m := navigationModel()
	m.Update(key(tea.KeyUp, ""))
	if m.focus != focusCategories || m.category != 3 {
		t.Fatal("up from the first topic did not focus the current category")
	}
	_, cmd := m.Update(key(tea.KeyLeft, ""))
	if cmd != nil || m.category != 2 || m.session.Current.Path != "/?tab=tech" {
		t.Fatal("left navigated before confirming the category")
	}
	m.Update(key(tea.KeyDown, ""))
	if m.focus != focusContent || m.session.Current.Selected != 1 {
		t.Fatal("down did not return to the selected topic")
	}
	m.Update(key(tea.KeyUp, ""))
	m.Update(key(tea.KeyLeft, ""))
	m.Update(key(tea.KeyLeft, ""))
	_, cmd = m.Update(key(tea.KeyEnter, ""))
	deliver(m, cmd)
	if m.session.Current.Path != "/recent" || m.focus != focusContent {
		t.Fatal("Enter did not open the selected category")
	}
	m.Update(key(tea.KeyUp, ""))
	for range 8 {
		m.Update(key(tea.KeyLeft, ""))
	}
	if m.category != 0 {
		t.Fatal("category selection wrapped past the first category")
	}
}

func TestPostHorizontalFocusUnwindsBeforeGoingBack(t *testing.T) {
	m := navigationModel()
	_, cmd := m.Update(key(tea.KeyRight, ""))
	deliver(m, cmd)
	m.Update(key(tea.KeyDown, ""))
	for _, step := range []struct {
		key   rune
		focus focusTarget
	}{{tea.KeyRight, focusReply}, {tea.KeyRight, focusThank}, {tea.KeyRight, focusThank}, {tea.KeyLeft, focusReply}, {tea.KeyLeft, focusContent}} {
		_, cmd = m.Update(key(step.key, ""))
		if cmd != nil || m.focus != step.focus || m.session.Current.Path != "/t/1" || m.mode != "" {
			t.Fatalf("arrow %d did not stay in selection state %d", step.key, step.focus)
		}
	}
	_, cmd = m.Update(key(tea.KeyLeft, ""))
	deliver(m, cmd)
	if m.session.Current.Path != "/?tab=tech" {
		t.Fatal("left from content did not go back")
	}
	_, cmd = m.Update(key(tea.KeyLeft, ""))
	if cmd == nil {
		t.Fatal("left from the list did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok || m.session.Current.Path != "/?tab=tech" {
		t.Fatal("left from the list should quit without navigating")
	}
	api := m.api.(*fakeAPI)
	if api.replies != 0 || api.thanks != 0 {
		t.Fatal("moving action focus performed a write")
	}
}

func TestListLeftQuitsLikeQAndPreservesReadingState(t *testing.T) {
	for _, path := range []string{"/?tab=hot", "/recent", "/?tab=qna", "/?tab=tech", "/go/golang"} {
		for _, loading := range []bool{false, true} {
			m := testTopicModel(listPage(path))
			m.moveSelection(2)
			m.setOffset(2)
			m.session.Current.Filter = "中文"
			m.session.Drafts["/t/1"] = "未发送的草稿"
			m.loading = loading
			if loading {
				m.page = v2ex.Page{}
			}
			before := m.Session()
			for _, press := range []tea.KeyPressMsg{key(tea.KeyLeft, ""), key('q', "q")} {
				_, cmd := m.Update(press)
				if cmd == nil {
					t.Fatalf("%s, loading %t: %s did not quit", path, loading, press.String())
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatalf("%s, loading %t: %s navigated instead of quitting", path, loading, press.String())
				}
				if got := m.Session(); got.Current != before.Current || got.Drafts["/t/1"] != "未发送的草稿" || len(got.Back) != len(before.Back) || len(got.Forward) != len(before.Forward) {
					t.Fatal("quit changed the reading state to save")
				}
			}
		}
	}
}

func TestLeftReturnsFromLoadingPostWithoutQuitting(t *testing.T) {
	m := navigationModel()
	_, pending := m.Update(key(tea.KeyRight, ""))
	if pending == nil || !m.loading || m.page.Topic != nil {
		t.Fatal("post was not loading")
	}
	_, cmd := m.Update(key(tea.KeyLeft, ""))
	deliver(m, cmd)
	if m.session.Current.Path != "/?tab=tech" || m.loading {
		t.Fatal("left did not return from the loading post")
	}
	m.Update(pending())
	if m.session.Current.Path != "/?tab=tech" {
		t.Fatal("stale post load replaced the list after going back")
	}
}

func TestEmptyListCanStillFocusCategories(t *testing.T) {
	m := navigationModel()
	m.page.Topics = nil
	m.rebuild()
	m.Update(key(tea.KeyUp, ""))
	if m.focus != focusCategories {
		t.Fatal("empty list trapped keyboard focus outside the category bar")
	}
	m.Update(key(tea.KeyDown, ""))
	if m.focus != focusContent {
		t.Fatal("could not leave categories for an empty list")
	}
}

func TestCategoryFocusFallsBackToHotWithoutCategoryHistory(t *testing.T) {
	pages := map[string]v2ex.Page{"/go/go": listPage("/go/go"), "/?tab=hot": listPage("/?tab=hot")}
	m := New(context.Background(), &fakeAPI{pages: pages}, sessionAt("/go/go"))
	deliver(m, m.Init())
	m.Update(key(tea.KeyUp, ""))
	if m.focus != focusCategories || m.category != 0 {
		t.Fatal("category focus should start at hot without a previous category")
	}
	_, cmd := m.Update(key(tea.KeyEnter, ""))
	deliver(m, cmd)
	if m.page.Path != "/?tab=hot" {
		t.Fatal("Enter did not open the default hot category")
	}
}

func TestEnterActivatesTheFocusedFloorAction(t *testing.T) {
	m := testTopicModel(readingPage())
	m.Update(key(tea.KeyDown, ""))
	m.Update(key(tea.KeyRight, ""))
	_, cmd := m.Update(key(tea.KeyEnter, ""))
	if m.mode != "reply" || m.editor.Value() != "@alice #1 " || m.focus != focusContent {
		t.Fatal("Enter did not open the focused floor reply")
	}
	if cmd != nil {
		m.Update(cmd())
	}
	m.Update(key(tea.KeyEscape, ""))
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyRight, ""))
	m.Update(key(tea.KeyEnter, ""))
	if m.mode != "thank" || m.confirmation.kind != "reply" || m.confirmation.id != 101 {
		t.Fatal("Enter did not confirm thanks for the focused floor")
	}
	if m.api.(*fakeAPI).thanks != 0 {
		t.Fatal("focus activation sent thanks before confirmation")
	}
}

func TestCategoryAndActionFocusPreserveScreenAndHitGeometry(t *testing.T) {
	for _, width := range []int{24, 40, 80, 160} {
		m := testTopicModel(readingPage())
		m.resize(width, 18)
		m.Update(key(tea.KeyUp, ""))
		for _, b := range m.navigation() {
			if b.end > m.contentLeft()+m.contentWidth() {
				t.Fatal("category focus clipped a tab")
			}
		}
		m.Update(key(tea.KeyDown, ""))
		for range 2 {
			m.Update(key(tea.KeyRight, ""))
			rows := strings.Split(m.View().Content, "\n")
			if len(rows) != m.height {
				t.Fatal("focused action changed screen height")
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != width {
					t.Fatal("focused action broke terminal cell width")
				}
			}
			index := m.selectionIndex()
			row := m.selections[index].actions
			if row < m.session.Current.Offset || row >= m.session.Current.Offset+m.bodyHeight() {
				t.Fatal("keyboard action was focused outside the viewport")
			}
			l := m.lines[row]
			focused := l.focused[m.focus-focusReply]
			base := l.normal
			if l.heading {
				base = l.highlight
			}
			z := l.zones[m.focus-focusReply]
			label := v2ex.SafeText(ansi.Cut(base, z.start, z.end))
			if v2ex.SafeText(focused) != v2ex.SafeText(base) || !strings.Contains(focused, focusedStyle.Render(label)) {
				t.Fatal("action focus changed its label or lost the focus style")
			}
		}
	}
}

func TestWheelSelectionFollowsVisibleFloorsIncludingImages(t *testing.T) {
	p := readingPage()
	p.Topic.Body = []v2ex.Element{{Text: strings.Repeat("OP 正文\n", 15)}}
	p.Replies[0].Body = []v2ex.Element{{Text: "开始"}, {Image: true, URL: "https://example.org/p.png"}, {Text: "结束"}}
	p.Replies[1].Body = []v2ex.Element{{Text: strings.Repeat("更多回复\n", 20)}}
	m := testTopicModel(p)
	m.resize(80, 12)
	m.images["reply:101:1"] = &imageState{expanded: true, image: &preview.Image{Columns: 8, Rows: []preview.Row{{Native: "A"}, {Native: "B"}, {Native: "C"}}}}
	m.rebuild()
	first := m.selections[1].at
	for m.session.Current.Offset < first {
		m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	if m.session.Current.Selected != 101 {
		t.Fatal("scrolling past OP did not highlight the first visible reply")
	}
	m.Update(key(tea.KeyRight, ""))
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.focus != focusContent {
		t.Fatal("reading with the wheel left action focus active")
	}
	for at, l := range m.lines {
		if l.imageRow != nil {
			m.setOffset(at)
			if m.session.Current.Selected != 101 {
				t.Fatal("image-only rows lost their owning floor selection")
			}
		}
	}
	m.setOffset(m.selections[2].at)
	if m.session.Current.Selected != 102 {
		t.Fatal("scrolling past one floor did not select the next")
	}
}
