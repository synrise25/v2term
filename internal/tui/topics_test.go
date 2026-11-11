package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/v2ex"
)

func TestTopicReplyBadgesAlignWithoutCoveringWrappedTitles(t *testing.T) {
	for _, width := range []int{24, 40, 80, 160} {
		p := listPage("/?tab=tech")
		p.Topics = p.Topics[:3]
		for i, count := range []int{0, 12, 12345} {
			p.Topics[i].Replies = count
			p.Topics[i].Title = strings.Repeat("中文 mixed title ", 12)
		}
		m := testTopicModel(p)
		m.resize(width, 24)
		for _, s := range m.selections {
			l := m.lines[s.at]
			count := strconv.Itoa(p.Topics[s.hit.id-1].Replies)
			if strings.TrimSpace(l.right) != count {
				t.Fatalf("topic %d counter: %q", s.hit.id, l.right)
			}
			x := m.contentLeft() + m.contentWidth() - ansi.StringWidth(l.right)
			for _, rendered := range []string{l.normal, l.highlight} {
				if ansi.StringWidth(rendered) != width {
					t.Fatal("reply badge changed row width")
				}
				if got := v2ex.SafeText(ansi.Cut(rendered, x, m.contentLeft()+m.contentWidth())); got != l.right {
					t.Fatalf("right-aligned counter: %q, want %q", got, l.right)
				}
			}
			for i := s.at + 1; i < len(m.lines) && m.lines[i].hit.id == s.hit.id; i++ {
				row := m.lines[i]
				if row.right != "" || ansi.StringWidth(row.text) > x-m.contentLeft()-2 {
					t.Fatal("wrapped title or metadata intruded into the reply column")
				}
				if !row.heading && strings.Contains(v2ex.SafeText(row.text), "回复") {
					t.Fatal("reply count was duplicated in metadata")
				}
			}
		}
		_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft() + m.contentWidth() - 1, Y: 4})
		if cmd == nil || m.session.Current.Path != "/t/1" {
			t.Fatal("clicking the reply badge did not open its topic")
		}
	}
}

func TestTopicFilteringKeepsCountsWithTheirTopics(t *testing.T) {
	p := listPage("/?tab=tech")
	p.Topics[3].Title, p.Topics[3].Replies = "只保留这一篇", 42
	m := testTopicModel(p)
	m.session.Current.Filter = "只保留"
	m.rebuild()
	if len(m.selections) != 1 || m.selections[0].hit.id != 4 {
		t.Fatal("filter selected the wrong topic")
	}
	l := m.lines[m.selections[0].at]
	if strings.TrimSpace(l.right) != "42" || !strings.Contains(l.text, matchStyle.Render("只保留这一篇")) {
		t.Fatal("filtered topic lost its count or match highlight")
	}
}
