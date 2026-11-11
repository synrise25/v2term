package tui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/preview"
	"v2term/internal/state"
	"v2term/internal/v2ex"
)

func testTopicModel(p v2ex.Page) *Model {
	s := state.Default()
	s.Current.Path = p.Path
	m := New(context.Background(), &fakeAPI{pages: map[string]v2ex.Page{p.Path: p}}, s)
	deliver(m, m.Init())
	return m
}

func TestInlineLinksWithoutFootnotesOrBrowserActions(t *testing.T) {
	p := topicPage()
	p.Topic.Body = []v2ex.Element{{Text: "中文前文 "}, {Text: "一个链接", URL: "https://example.org/article"}, {Text: " 和后文"}}
	m := testTopicModel(p)
	text := m.View().Content
	if strings.Contains(text, "链接 · 点击打开") || strings.Contains(text, "[1]") {
		t.Fatal("link footnotes still shown")
	}
	if !strings.Contains(text, ansi.SetHyperlink("https://example.org/article")) {
		t.Fatal("terminal-native link missing")
	}
	for _, b := range m.navigation() {
		if b.action == "forward" || strings.Contains(b.label, "Alt+") {
			t.Fatal("forward navigation still visible")
		}
		if b.action == "back" || b.action == "open" {
			t.Fatal("redundant back/action hints still occupy the navigation bar")
		}
	}
	if len(m.navigation()) == 0 || m.navigation()[0].action != "hot" {
		t.Fatal("navigation should start with category tabs")
	}
}

func TestReplyTargetsHaveIndependentDraftsAndThankActions(t *testing.T) {
	p := topicPage()
	m := testTopicModel(p)
	f := m.api.(*fakeAPI)
	m.activate(hit{kind: "reply-to", id: 900})
	if m.editor.Value() != "@alice #1 " || !strings.Contains(m.replyTarget, "alice") {
		t.Fatalf("missing reply target: %q", m.editor.Value())
	}
	m.Update(tea.PasteMsg{Content: "first draft"})
	m.Update(key(tea.KeyEscape, ""))
	m.activate(hit{kind: "reply-to", id: 901})
	if m.editor.Value() != "@bob #2 " {
		t.Fatal("draft for another user reused")
	}
	m.Update(tea.PasteMsg{Content: "second draft"})
	m.Update(key(tea.KeyEscape, ""))
	m.composeTo(900)
	if m.editor.Value() != "@alice #1 first draft" {
		t.Fatal("targeted draft not restored")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	deliver(m, cmd)
	if f.replies != 1 || f.content != "@alice #1 first draft" {
		t.Fatal("reply target not sent through topic form")
	}
	if _, ok := m.session.Drafts["/t/1#reply-900"]; ok {
		t.Fatal("sent draft retained")
	}
	if m.session.Drafts["/t/1#reply-901"] != "@bob #2 second draft" {
		t.Fatal("sending removed another user's draft")
	}
	// Finish the read refresh before starting another action.
	deliver(m, m.load(true))
	m.activate(hit{kind: "thank-reply", id: 901})
	if m.mode != "thank" || f.thanks != 0 {
		t.Fatal("thank skipped confirmation")
	}
	_, cmd = m.Update(key(tea.KeyEnter, ""))
	deliver(m, cmd)
	if f.thankID != 901 || f.thankKind != "reply" {
		t.Fatal("wrong user reply thanked")
	}
}

func TestInlineReplyButtonsUseActualMouseHitAreas(t *testing.T) {
	m := testTopicModel(topicPage())
	for at, l := range m.lines {
		if len(l.zones) == 2 && l.zones[0].hit.kind == "reply-to" {
			m.session.Current.Offset = at
			m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: l.zones[0].start, Y: 4})
			if m.mode != "reply" || !strings.HasPrefix(m.editor.Value(), "@alice #1 ") {
				t.Fatal("inline reply click missed user")
			}
			return
		}
	}
	t.Fatal("no inline action buttons")
}

func imageServer(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	im.Set(0, 0, color.White)
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.Write(data.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv.URL, calls
}

func TestImageExpansionCollapseAndStaleLoad(t *testing.T) {
	url, calls := imageServer(t)
	p := topicPage()
	p.Topic.Body = []v2ex.Element{{Text: "before\n"}, {Image: true, URL: url}, {Text: "\nafter"}}
	m := testTopicModel(p)
	var target hit
	for _, l := range m.lines {
		if l.hit.kind == "image" {
			target = l.hit
			break
		}
	}
	if target.path == "" {
		t.Fatal("image control missing")
	}
	cmd := m.activate(target)
	if calls.Load() != 0 || cmd == nil {
		t.Fatal("image I/O ran in Update")
	}
	deliver(m, cmd)
	if !m.images[target.path].expanded || m.images[target.path].image == nil {
		t.Fatal("image not displayed")
	}
	count := len(m.lines)
	m.activate(target)
	if m.images[target.path].expanded || len(m.lines) >= count {
		t.Fatal("image did not collapse")
	}
	m.activate(target)
	if calls.Load() != 1 {
		t.Fatal("expanding downloaded cached image again")
	}
	m.activate(target)
	delete(m.images, target.path)
	stale := m.activate(target)
	m.activate(target)
	deliver(m, stale)
	if m.images[target.path].expanded || m.images[target.path].image != nil {
		t.Fatal("cancelled load reopened image")
	}
}

func TestNativeImagePlacementAndModalCleanup(t *testing.T) {
	p := topicPage()
	p.Replies = nil
	p.Topic.Body = []v2ex.Element{{Image: true, URL: "https://example.org/image.png"}, {Text: strings.Repeat("\n图片之后的正文", 20)}}
	m := testTopicModel(p)
	m.resize(40, 16)
	var target hit
	for _, l := range m.lines {
		if l.hit.kind == "image" {
			target = l.hit
			break
		}
	}
	var terminal bytes.Buffer
	out := preview.NewOutput(&terminal, preview.ITerm2)
	m.options.ImageOutput = out
	m.images[target.path] = &imageState{expanded: true, image: &preview.Image{Columns: 8, Rows: []preview.Row{{Native: "NATIVE_TILE_A"}, {Native: "NATIVE_TILE_B"}}}}
	m.rebuild()
	view := m.View()
	if strings.Contains(view.Content, "NATIVE_TILE") {
		t.Fatal("native data leaked into cell renderer")
	}
	out.Write([]byte(ansi.SetModeAltScreenSaveCursor + view.Content))
	if !strings.Contains(terminal.String(), "NATIVE_TILE_A") {
		t.Fatal("inline image was not composited")
	}
	before := m.session.Current.Offset
	_, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 3, Y: 7})
	if cmd != nil || m.session.Current.Offset != before+1 {
		t.Fatal("wheel over an image did not scroll the document immediately")
	}
	if !m.images[target.path].expanded {
		t.Fatal("scroll collapsed the image")
	}
	terminal.Reset()
	m.mode = "help"
	out.Write([]byte(m.View().Content))
	if strings.Contains(terminal.String(), "NATIVE_TILE") {
		t.Fatal("image remained over a modal")
	}
}

func TestAuthorReplyAndSavedImagePosition(t *testing.T) {
	p := topicPage()
	p.Topic.Author = "OP"
	m := testTopicModel(p)
	m.activate(hit{kind: "reply-author"})
	if m.editor.Value() != "@OP " || m.draftKey() != "/t/1#author" {
		t.Fatal("author reply not targeted independently")
	}
	m.Update(key(tea.KeyEscape, ""))
	m.session.Current.Offset = 5
	m.lines[1].imageRow = &preview.Row{}
	m.lines[2].imageRow = &preview.Row{}
	if got := m.Session().Current.Offset; got != 3 {
		t.Fatalf("persisted expanded-image offset: %d", got)
	}
	if m.session.Current.Offset != 5 {
		t.Fatal("saving changed the visible reading position")
	}
}

func BenchmarkScrollView(b *testing.B) {
	p := topicPage()
	p.Replies = nil
	for i := 1; i <= 1000; i++ {
		p.Replies = append(p.Replies, v2ex.Reply{ID: i, Floor: i, Author: "member", Content: "多行中文内容，包含浏览与滚动。\nsecond line\nthird line"})
	}
	m := testTopicModel(p)
	m.resize(100, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.scroll(1)
		if m.session.Current.Offset >= len(m.lines)-m.bodyHeight() {
			m.scroll(-len(m.lines))
		}
		view := m.View()
		runtime.KeepAlive(view.Content)
	}
}

func TestUnicodeWrappingPreservesTextAndLinkRanges(t *testing.T) {
	text := "这是 English 混合中文段落🙂，还包含 e\u0301 组合字符。\nsecond paragraph"
	var rebuilt strings.Builder
	foundEnglish := false
	for _, r := range visualRows(text, 12) {
		row := text[r.start:r.end]
		if !utf8.ValidString(row) || ansi.StringWidth(strings.TrimRight(row, " ")) > 12 {
			t.Fatalf("invalid wrapped row: %q", row)
		}
		if strings.Contains(row, "English") {
			foundEnglish = true
		}
		rebuilt.WriteString(row)
	}
	if rebuilt.String() != strings.ReplaceAll(text, "\n", "") || !foundEnglish {
		t.Fatal("wrapping lost text or split a short English word")
	}
	long := "  " + strings.Repeat("long_word", 20)
	rows := visualRows(long, 12)
	if strings.TrimSpace(long[rows[0].start:rows[0].end]) == "" {
		t.Fatal("long title starts with an empty selection row")
	}
	for _, r := range rows {
		if ansi.StringWidth(strings.TrimRight(long[r.start:r.end], " ")) > 12 {
			t.Fatal("long word exceeded line width")
		}
	}
}
