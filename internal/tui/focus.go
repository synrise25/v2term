package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type focusTarget uint8

const (
	focusContent focusTarget = iota
	focusCategories
	focusReply
	focusThank
)

var categories = [...]struct{ path, action string }{
	{"/?tab=hot", "hot"}, {"/recent", "recent"}, {"/?tab=qna", "qna"}, {"/?tab=tech", "tech"},
}

func (m *Model) enterCategories() {
	m.category = 0
	paths := []string{m.session.Current.Path}
	for i := len(m.session.Back) - 1; i >= 0; i-- {
		paths = append(paths, m.session.Back[i].Path)
	}
	for _, path := range paths {
		for i, c := range categories {
			if path == c.path {
				m.category = i
				m.focus = focusCategories
				m.setOffset(0)
				return
			}
		}
	}
	m.focus = focusCategories
	m.setOffset(0)
}

func (m *Model) moveVertical(delta int) {
	if m.focus == focusCategories {
		if delta > 0 {
			m.focus = focusContent
			if index := m.selectionIndex(); index >= 0 {
				m.revealSelection(index)
			}
		}
		return
	}
	if delta < 0 && m.selectionIndex() <= 0 {
		m.enterCategories()
		return
	}
	m.focus = focusContent
	m.moveSelection(delta)
}

func (m *Model) moveHorizontal(delta int) tea.Cmd {
	if m.focus == focusCategories {
		m.category = min(max(0, m.category+delta), len(categories)-1)
		return nil
	}
	if delta < 0 {
		switch m.focus {
		case focusThank:
			m.focus = focusReply
		case focusReply:
			m.focus = focusContent
		default:
			if strings.HasPrefix(m.session.Current.Path, "/t/") {
				return m.history(true)
			}
			return tea.Quit
		}
		return nil
	}
	if m.page.Topic == nil {
		return m.activate(m.selected())
	}
	index := m.selectionIndex()
	if index < 0 || m.selections[index].actions < 0 {
		return nil
	}
	if m.focus == focusContent {
		m.focus = focusReply
	} else {
		m.focus = focusThank
	}
	row := m.selections[index].actions
	if row < m.session.Current.Offset {
		m.setOffset(max(m.selections[index].at, row-m.bodyHeight()+1))
	} else if row >= m.session.Current.Offset+m.bodyHeight() {
		m.setOffset(row - m.bodyHeight() + 1)
	}
	return nil
}

func (m *Model) activateFocus() tea.Cmd {
	if m.focus == focusCategories {
		return m.navigate(categories[m.category].path)
	}
	if m.focus == focusReply || m.focus == focusThank {
		index := m.selectionIndex()
		if index < 0 || m.selections[index].actions < 0 {
			return nil
		}
		zones := m.lines[m.selections[index].actions].zones
		action := int(m.focus - focusReply)
		m.focus = focusContent
		if action < len(zones) {
			return m.activate(zones[action].hit)
		}
		return nil
	}
	return m.activate(m.selected())
}
