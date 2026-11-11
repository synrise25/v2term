package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"v2term/internal/v2ex"
)

func TestSelectionFollowsVisibleConversationAndTimeOrder(t *testing.T) {
	p := readingPage()
	p.Replies[1].Content = "独立回复"
	p.Replies[2].Content = "@alice #1 回复第一条"
	m := testTopicModel(p)
	m.resize(100, 12)
	for _, id := range []int{101, 103, 102} {
		m.Update(key(tea.KeyDown, ""))
		if m.session.Current.Selected != id {
			t.Fatalf("conversation selection: got %d, want %d", m.session.Current.Selected, id)
		}
		for _, s := range m.selections {
			if s.hit.id == id && (s.at < m.session.Current.Offset || s.at >= m.session.Current.Offset+m.bodyHeight()) {
				t.Fatal("selected heading was outside the viewport")
			}
		}
	}
	m.toggleConversation()
	m.Update(key(tea.KeyDown, ""))
	if m.session.Current.Selected != 103 {
		t.Fatal("time-order selection still followed the conversation tree")
	}
	m.Update(key(tea.KeyHome, ""))
	selected, offset := m.session.Current.Selected, m.session.Current.Offset
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.session.Current.Selected != selected || m.session.Current.Offset != offset+1 {
		t.Fatal("wheel changed the selected floor or stopped scrolling by row")
	}
}

func TestTopicAndFloorReplyShortcutsKeepIndependentDrafts(t *testing.T) {
	m := testTopicModel(readingPage())
	m.Update(key(tea.KeyDown, ""))
	m.Update(key('r', "r"))
	if m.editor.Value() != "@alice #1 " || m.draftKey() != "/t/1#reply-101" {
		t.Fatal("r did not target the selected floor")
	}
	m.Update(tea.PasteMsg{Content: "楼层草稿"})
	m.Update(key(tea.KeyEscape, ""))
	m.Update(key('a', "a"))
	if m.editor.Value() != "" || m.draftKey() != "/t/1" || m.replyTarget != "主题" {
		t.Fatal("topic reply inherited a floor mention or draft")
	}
	m.Update(tea.PasteMsg{Content: "主题草稿"})
	m.Update(key(tea.KeyEscape, ""))
	m.Update(key('r', "r"))
	if m.editor.Value() != "@alice #1 楼层草稿" {
		t.Fatal("topic reply overwrote the floor draft")
	}
	m.Update(key(tea.KeyEscape, ""))
	m.action("reply-topic")
	if m.editor.Value() != "主题草稿" || m.session.Current.Selected != 101 {
		t.Fatal("topic toolbar action lost the draft or floor selection")
	}
	m.Update(key(tea.KeyEscape, ""))
	m.Update(key(tea.KeyUp, ""))
	m.Update(key('r', "r"))
	if m.editor.Value() != "主题草稿" || m.draftKey() != "/t/1" {
		t.Fatal("r on OP did not use the topic draft")
	}
}

func TestClickingOPUpdatesSelectionAndContextActions(t *testing.T) {
	m := testTopicModel(readingPage())
	m.resize(160, 30)
	m.moveSelection(1)
	m.View()
	if !strings.Contains(m.actionsText, "r 回复楼层") || !strings.Contains(m.actionsText, "a 回复主题") {
		t.Fatal("floor and topic reply actions were not distinguished")
	}
	m.setOffset(0)
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.contentLeft() + 3, Y: 4})
	view := m.View().Content
	if m.session.Current.Selected != 0 || m.selected().kind != "op" || !strings.Contains(view, "▸") || !strings.Contains(m.actionsText, "r 回复主题") {
		t.Fatal("clicking OP did not update its highlight and actions")
	}
	m.Update(key('t', "t"))
	if m.mode != "thank" || m.confirmation.kind != "topic" || m.confirmation.id != 1 {
		t.Fatal("thank on OP did not use topic confirmation")
	}
	if m.api.(*fakeAPI).thanks != 0 {
		t.Fatal("selecting OP sent thanks without confirmation")
	}
}

func TestOPIsSelectableWithoutRepliesAndAfterStaleSelection(t *testing.T) {
	p := readingPage()
	p.Replies = nil
	p.Topic.Body = []v2ex.Element{{Text: strings.Repeat("长正文\n", 100)}}
	m := testTopicModel(p)
	m.session.Current.Selected = 999
	m.rebuild()
	if m.selected().kind != "op" {
		t.Fatal("stale selection did not fall back to OP")
	}
	m.moveSelection(1)
	m.moveSelection(-1)
	if m.session.Current.Selected != 0 || len(m.selections) != 1 {
		t.Fatal("empty reply list did not retain OP selection")
	}
}
