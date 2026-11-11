package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/v2ex"
)

func TestSingleFooterKeepsActionsAndProgressSeparate(t *testing.T) {
	for width := 24; width <= 220; width++ {
		for _, mode := range []string{"list", "op", "reply"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				p := readingPage()
				if mode == "list" {
					p = listPage("/?tab=tech")
				}
				p.Number, p.Pages = 87, 1234
				m := testTopicModel(p)
				m.resize(width, 18)
				if mode == "reply" {
					m.moveSelection(1)
				}
				rows := strings.Split(m.View().Content, "\n")
				if len(rows) != m.height || m.bodyHeight() != m.height-5 {
					t.Fatal("footer did not release a row for content")
				}
				footer := rows[len(rows)-1]
				if strings.Contains(v2ex.SafeText(footer), "推断关联") {
					t.Fatal("old conversation status remains in footer")
				}
				progress := min(100, (m.session.Current.Offset+m.bodyHeight())*100/max(1, len(m.lines)))
				position := m.positionLabel(progress)
				positionStart := m.contentLeft() + m.contentWidth() - ansi.StringWidth(position)
				if got := v2ex.SafeText(ansi.Cut(footer, positionStart, m.contentLeft()+m.contentWidth())); got != position {
					t.Fatalf("progress was clipped: %q", got)
				}
				for _, b := range m.actions() {
					if b.end > positionStart-2 || b.end > m.contentLeft()+m.actionWidth() {
						t.Fatalf("control %s overlaps progress", b.label)
					}
					if got, want := v2ex.SafeText(ansi.Cut(footer, b.start, b.end)), " "+b.label+" "; got != want {
						t.Fatalf("control hit area covers %q, want %q", got, want)
					}
					if b.action == "help" {
						m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: b.start, Y: m.height - 1})
					}
				}
				if m.mode != "help" {
					t.Fatal("compact footer button click missed")
				}
			})
		}
	}
}

func TestLastContentRowIsClickableAndProgressIsNotAnAction(t *testing.T) {
	m := testTopicModel(topicPage())
	m.resize(80, 16)
	for _, s := range m.selections {
		if s.hit.kind != "reply" {
			continue
		}
		m.setOffset(s.at - m.bodyHeight() + 1)
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft() + 3, Y: m.height - 2})
		if m.session.Current.Selected != s.hit.id {
			t.Fatal("new last content row cannot select a floor")
		}
		_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft() + m.contentWidth() - 1, Y: m.height - 1})
		if cmd != nil || m.mode != "" {
			t.Fatal("clicking progress triggered a hidden action")
		}
		return
	}
	t.Fatal("no reply to select")
}

func TestOperationMessagesAndFilterRemainVisibleAboveContent(t *testing.T) {
	m := testTopicModel(readingPage())
	m.status = "发送失败\x1b[2J，请重试"
	rows := strings.Split(m.View().Content, "\n")
	if !strings.Contains(v2ex.SafeText(rows[2]), "发送失败，请重试") || strings.Contains(rows[2], "\x1b[2J") {
		t.Fatal("operation message was hidden or not sanitized")
	}
	if !strings.Contains(rows[len(rows)-1], "r 回复主题") {
		t.Fatal("operation message displaced the footer controls")
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft(), Y: 2})
	if m.mode != "" {
		t.Fatal("message row unexpectedly opened the address input")
	}
	m.status, m.session.Current.Filter = "", "查找词"
	rows = strings.Split(m.View().Content, "\n")
	if !strings.Contains(v2ex.SafeText(rows[2]), "查找: 查找词") {
		t.Fatal("filter became invisible when its footer row was removed")
	}
}

func TestTopicAuthorRepliesHaveAnOPMarker(t *testing.T) {
	for _, conversation := range []bool{true, false} {
		for _, width := range []int{24, 80, 160} {
			p := readingPage()
			p.Topic.Author = "maker"
			p.Replies[0].Author, p.Replies[1].Author, p.Replies[2].Author = "maker", "MaKeR", "OP"
			m := testTopicModel(p)
			m.session.Conversation = conversation
			m.resize(width, 30)
			for _, l := range m.lines {
				if !l.heading || !l.first || l.hit.kind != "reply" {
					continue
				}
				want := l.hit.id == 101 || l.hit.id == 102
				for _, rendered := range []string{l.normal, l.highlight} {
					if got := strings.Contains(v2ex.SafeText(rendered), " OP @"); got != want {
						t.Fatalf("width %d, floor %d, OP marker=%t, want %t", width, l.hit.id, got, want)
					}
				}
			}
		}
	}
	p := readingPage()
	p.Topic.Author, p.Replies[0].Author = "", ""
	m := testTopicModel(p)
	for _, l := range m.lines {
		if l.hit.kind == "reply" && strings.Contains(v2ex.SafeText(l.normal), " OP @") {
			t.Fatal("missing author information produced an OP marker")
		}
	}
}
