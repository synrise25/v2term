package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/state"
	"v2term/internal/v2ex"
)

type fakeAPI struct {
	pages           map[string]v2ex.Page
	writeErr        error
	replies, thanks int
	content         string
	thankID         int
	thankKind       string
}

func (f *fakeAPI) Fetch(_ context.Context, path string) (v2ex.Page, error) {
	p, ok := f.pages[path]
	if !ok {
		return p, fmt.Errorf("missing %s", path)
	}
	return p, nil
}
func (f *fakeAPI) Reply(_ context.Context, path, content string) error {
	f.replies++
	f.content = content
	return f.writeErr
}
func (f *fakeAPI) Thank(_ context.Context, path, kind string, id int) error {
	f.thanks++
	f.thankID, f.thankKind = id, kind
	return f.writeErr
}
func (f *fakeAPI) Authenticated() bool { return true }

func listPage(path string) v2ex.Page {
	p := v2ex.Page{Path: path, Title: "帖子列表", Number: 1, Pages: 2}
	for i := 1; i <= 30; i++ {
		p.Topics = append(p.Topics, v2ex.Topic{ID: i, Path: fmt.Sprintf("/t/%d", i), Title: fmt.Sprintf("第 %d 个中文帖子", i), Author: "alice", Node: "Go"})
	}
	return p
}
func topicPage() v2ex.Page {
	return v2ex.Page{Path: "/t/1", Title: "主题", Number: 1, Pages: 2, CanReply: true, Topic: &v2ex.Topic{ID: 1, Title: "主题", Content: strings.Repeat("正文\n", 30)}, Replies: []v2ex.Reply{{ID: 900, Floor: 1, Author: "alice", Content: "一个回复"}, {ID: 901, Floor: 2, Author: "bob", Content: "@alice #1 第二个回复"}}}
}
func key(code rune, text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Text: text} }
func deliver(m *Model, cmd tea.Cmd) {
	if cmd != nil {
		m.Update(cmd())
	}
}

func sessionAt(path string) state.Session {
	s := state.Default()
	s.Current.Path = path
	return s
}

func TestLateResponseDoesNotReplaceCurrentPage(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/?tab=tech": listPage("/?tab=tech"), "/?tab=hot": listPage("/?tab=hot")}}
	m := New(context.Background(), f, sessionAt("/?tab=tech"))
	old := m.Init()
	next := m.navigate("/?tab=hot")
	deliver(m, next)
	deliver(m, old)
	if m.page.Path != "/?tab=hot" || m.session.Current.Path != "/?tab=hot" {
		t.Fatal("stale page replaced current navigation")
	}
}

func TestHistoryRestoresReadingPosition(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/?tab=tech": listPage("/?tab=tech"), "/t/1": topicPage()}}
	m := New(context.Background(), f, sessionAt("/?tab=tech"))
	deliver(m, m.Init())
	m.session.Current.Offset, m.session.Current.Selected = 30, 15
	deliver(m, m.navigate("/t/1"))
	m.scroll(12)
	if m.session.Current.Offset != 12 {
		t.Fatal("topic not scrolled")
	}
	deliver(m, m.history(true))
	if m.session.Current.Offset != 30 || m.session.Current.Selected != 15 {
		t.Fatalf("lost list position: %+v", m.session.Current)
	}
	deliver(m, m.history(false))
	if m.session.Current.Path != "/t/1" || m.session.Current.Offset != 12 {
		t.Fatalf("lost forward position: %+v", m.session.Current)
	}
}

func TestMouseAndViewportGeometry(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/?tab=tech": listPage("/?tab=tech"), "/?tab=hot": listPage("/?tab=hot"), "/t/1": topicPage()}}
	m := New(context.Background(), f, sessionAt("/?tab=tech"))
	m.resize(80, 24)
	deliver(m, m.Init())
	view := m.View()
	rows := strings.Split(view.Content, "\n")
	if len(rows) != 24 {
		t.Fatalf("height: %d", len(rows))
	}
	for i, row := range rows {
		if ansi.StringWidth(row) != 80 {
			t.Fatalf("row %d has width %d", i, ansi.StringWidth(row))
		}
	}
	_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 4})
	deliver(m, cmd)
	if m.page.Path != "/t/1" {
		t.Fatal("mouse did not open topic")
	}
	for _, b := range m.navigation() {
		if b.action == "hot" {
			_, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: b.start + 1, Y: 1})
			deliver(m, cmd)
		}
	}
	if m.page.Path != "/?tab=hot" {
		t.Fatal("CJK navigation button hit area is wrong")
	}
}

func TestReplyDraftPasteAndUncertainSend(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/t/1": topicPage()}, writeErr: errors.New("结果未确认")}
	s := state.Default()
	s.Current.Path = "/t/1"
	m := New(context.Background(), f, s)
	deliver(m, m.Init())
	m.compose()
	m.Update(tea.PasteMsg{Content: "中文草稿\x1b[2J"})
	m.Update(key('q', "q"))
	if m.mode != "reply" || m.editor.Value() != "中文草稿q" {
		t.Fatalf("editor: %q", m.editor.Value())
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	deliver(m, cmd)
	if f.replies != 1 || m.mode != "reply" || m.Session().Drafts["/t/1"] != "中文草稿q" {
		t.Fatal("failed reply lost draft or was replayed")
	}
	m.Update(key(tea.KeyEscape, ""))
	_, cmd = m.Update(key('q', "q"))
	if cmd == nil {
		t.Fatal("q did not quit")
	}
}

func TestThankNeedsConfirmationAndSelection(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/t/1": topicPage()}}
	s := state.Default()
	s.Current.Path = "/t/1"
	m := New(context.Background(), f, s)
	deliver(m, m.Init())
	m.Update(key(tea.KeyTab, ""))
	m.Update(key('t', "t"))
	if m.mode != "thank" || f.thanks != 0 || m.confirmation.id != 900 {
		t.Fatal("confirmation or reply selection failed")
	}
	m.Update(key('n', "n"))
	if f.thanks != 0 {
		t.Fatal("cancel sent thanks")
	}
	m.Update(key('t', "t"))
	_, cmd := m.Update(key(tea.KeyEnter, ""))
	if f.thanks != 0 {
		t.Fatal("network I/O ran in Update")
	}
	deliver(m, cmd)
	if f.thanks != 1 {
		t.Fatal("thank not sent once")
	}
}

func TestConversationPreferenceAndSelectedReplySurviveToggle(t *testing.T) {
	f := &fakeAPI{pages: map[string]v2ex.Page{"/t/1": topicPage()}}
	s := state.Default()
	s.Current.Path = "/t/1"
	m := New(context.Background(), f, s)
	deliver(m, m.Init())
	if !m.session.Conversation {
		t.Fatal("conversation not default")
	}
	m.session.Current.Selected = 901
	m.toggleConversation()
	if m.session.Conversation || m.session.Current.Selected != 901 {
		t.Fatal("toggle lost selected reply")
	}
	m.Update(key(tea.KeyEnd, ""))
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.session.Current.Offset > max(0, len(m.lines)-m.bodyHeight()) {
		t.Fatal("scroll escaped bounds")
	}
}

func TestQuitFromHelpAndThankConfirmation(t *testing.T) {
	for _, mode := range []string{"", "help", "thank"} {
		m := New(context.Background(), &fakeAPI{}, state.Default())
		m.mode = mode
		_, cmd := m.Update(key('q', "q"))
		if cmd == nil {
			t.Fatalf("q did not quit from %q", mode)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("q did not return QuitMsg from %q", mode)
		}
	}
}
