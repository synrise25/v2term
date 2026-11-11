package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"v2term/internal/state"
	"v2term/internal/v2ex"
)

func TestBareArrowsAndCategoryShortcuts(t *testing.T) {
	pages := map[string]v2ex.Page{}
	for _, path := range []string{"/?tab=hot", "/recent", "/?tab=qna", "/?tab=tech"} {
		pages[path] = listPage(path)
	}
	pages["/t/1"] = topicPage()
	m := New(context.Background(), &fakeAPI{pages: pages}, state.Default())
	deliver(m, m.Init())
	if m.page.Path != "/?tab=hot" || !strings.Contains(v2ex.SafeText(m.View().Content), "V2TERM / 最热") {
		t.Fatal("initial page should be hot topics")
	}
	for _, tc := range []struct {
		key  rune
		path string
	}{{'1', "/?tab=hot"}, {'2', "/recent"}, {'3', "/?tab=qna"}, {'4', "/?tab=tech"}} {
		_, cmd := m.Update(key(tc.key, string(tc.key)))
		deliver(m, cmd)
		if m.page.Path != tc.path {
			t.Errorf("%c opened %s", tc.key, m.page.Path)
		}
	}
	_, cmd := m.Update(key(tea.KeyBackspace, ""))
	deliver(m, cmd)
	if m.page.Path != "/?tab=qna" {
		t.Fatal("backspace did not go back")
	}
	_, cmd = m.Update(key('l', "l"))
	deliver(m, cmd)
	if m.page.Path != "/?tab=tech" {
		t.Fatal("history did not go forward")
	}
	_, cmd = m.Update(key(tea.KeyRight, ""))
	deliver(m, cmd)
	if m.page.Path != "/t/1" {
		t.Fatal("right arrow did not open highlighted topic")
	}
}

func TestRightMouseOpensHighlightedTopic(t *testing.T) {
	p := topicPage()
	p.Path = "/t/2"
	p.Topic.ID = 2
	m := New(context.Background(), &fakeAPI{pages: map[string]v2ex.Page{"/?tab=tech": listPage("/?tab=tech"), "/t/2": p}}, sessionAt("/?tab=tech"))
	deliver(m, m.Init())
	m.Update(key(tea.KeyDown, ""))
	_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseRight, X: 79, Y: 1})
	deliver(m, cmd)
	if m.page.Path != "/t/2" {
		t.Fatal("right mouse did not open the highlighted topic")
	}
}

func TestWheelUsesVisualLinesAndKeyboardSelectsTopics(t *testing.T) {
	p := listPage("/?tab=tech")
	p.Topics[0].Title = strings.Repeat("长中文标题 ", 20)
	m := New(context.Background(), &fakeAPI{pages: map[string]v2ex.Page{p.Path: p}}, sessionAt(p.Path))
	m.resize(40, 14)
	deliver(m, m.Init())
	lines := &m.lines[0]
	for i := 1; i <= 5; i++ {
		_, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		if cmd != nil {
			t.Fatal("wheel scheduled delayed work")
		}
		if m.session.Current.Offset != i {
			t.Fatalf("jumped by topic instead of visual line at %d: %d", i, m.session.Current.Offset)
		}
	}
	if lines != &m.lines[0] {
		t.Fatal("scroll rebuilt layout")
	}
	count := len(m.lines)
	m.Update(key(tea.KeyDown, ""))
	if m.session.Current.Selected != 2 {
		t.Fatal("down key should select the next topic")
	}
	if count != len(m.lines) || lines != &m.lines[0] {
		t.Fatal("selection changed wrapping or rebuilt layout")
	}
}

func TestWheelIsImmediateAndRespectsConfiguredStep(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/?tab=tech": listPage("/?tab=tech"), "/?tab=hot": listPage("/?tab=hot")}}
	m := New(context.Background(), f, sessionAt("/?tab=tech"))
	deliver(m, m.Init())
	_, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.session.Current.Offset != 1 || cmd != nil {
		t.Fatal("wheel did not react by one row immediately")
	}
	m.options.ScrollLines = 3
	_, cmd = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.session.Current.Offset != 4 || cmd != nil {
		t.Fatal("wheel distance was deferred or ignored")
	}
	_, cmd = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.session.Current.Offset != 1 || cmd != nil {
		t.Fatal("wheel reversal was delayed")
	}
	deliver(m, m.navigate("/?tab=hot"))
	if m.session.Current.Offset != 0 {
		t.Fatal("wheel position leaked to a new page")
	}
}

func TestKeyboardSelectsOPAndReplies(t *testing.T) {
	s := state.Default()
	s.Current.Path = "/t/1"
	m := New(context.Background(), &fakeAPI{pages: map[string]v2ex.Page{"/t/1": topicPage()}}, s)
	deliver(m, m.Init())
	before := &m.lines[0]
	m.Update(key(tea.KeyDown, ""))
	if m.session.Current.Selected != 900 || m.selected().kind != "reply" {
		t.Fatal("down did not select the first reply")
	}
	m.Update(key(tea.KeyDown, ""))
	m.Update(key(tea.KeyDown, ""))
	if m.session.Current.Selected != 901 {
		t.Fatal("selection did not stop at the last reply")
	}
	m.Update(key(tea.KeyUp, ""))
	m.Update(key(tea.KeyUp, ""))
	if m.session.Current.Selected != 0 || m.selected().kind != "op" || m.session.Current.Offset != 0 {
		t.Fatal("up from the first reply did not select and reveal OP")
	}
	m.Update(key(tea.KeyUp, ""))
	if m.session.Current.Selected != 0 || &m.lines[0] != before {
		t.Fatal("OP selection wrapped or navigation rebuilt layout")
	}
}

func TestCacheSettingsAndExpiry(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/?tab=tech": listPage("/?tab=tech"), "/?tab=hot": listPage("/?tab=hot")}}
	opts := DefaultOptions()
	opts.CacheSize = 1
	m := New(context.Background(), f, sessionAt("/?tab=tech"), opts)
	deliver(m, m.Init())
	deliver(m, m.navigate("/?tab=hot"))
	if len(m.cache) != 1 {
		t.Fatal("cache size limit ignored")
	}
	item := m.cache["/?tab=hot"]
	item.at = time.Now().Add(-2 * time.Minute)
	m.cache["/?tab=hot"] = item
	if cmd := m.load(false); cmd == nil {
		t.Fatal("expired cache used")
	}
	opts.CacheSize = 0
	m = New(context.Background(), f, sessionAt("/?tab=tech"), opts)
	deliver(m, m.Init())
	if len(m.cache) != 0 || m.load(false) == nil {
		t.Fatal("cache disabled but still retained pages")
	}
}

func TestZeroThanksHiddenAndSelectionDoesNotReflowReplies(t *testing.T) {
	s := state.Default()
	s.Current.Path = "/t/1"
	p := topicPage()
	p.Replies[1].Thanks = 2
	m := New(context.Background(), &fakeAPI{pages: map[string]v2ex.Page{p.Path: p}}, s)
	deliver(m, m.Init())
	all := ""
	for _, l := range m.lines {
		all += l.text + "\n"
	}
	if strings.Contains(all, "♥ 0") || !strings.Contains(all, "♥ 2") {
		t.Fatal("incorrect thanks display")
	}
	before := &m.lines[0]
	count := len(m.lines)
	m.moveSelection(1)
	if before != &m.lines[0] || count != len(m.lines) {
		t.Fatal("reply selection rebuilt content")
	}
}
