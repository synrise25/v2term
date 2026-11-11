package tui

// Selection follows the rendered document, including OP and conversation order.
// Anchors are rebuilt with layout, so key repeats never re-infer the reply tree.
func (m *Model) moveSelection(delta int) {
	if len(m.selections) == 0 {
		return
	}
	current := max(0, m.selectionIndex())
	next := min(max(0, current+delta), len(m.selections)-1)
	m.selectHit(m.selections[next].hit)
	m.revealSelection(next)
}

func (m *Model) revealSelection(index int) {
	s := m.selections[index]
	if s.hit.kind == "op" {
		m.setOffset(0)
		return
	}
	end := len(m.lines)
	if index+1 < len(m.selections) {
		end = m.selections[index+1].at
	}
	end = min(end, s.at+m.bodyHeight())
	if s.at < m.session.Current.Offset {
		m.setOffset(s.at)
	} else if end > m.session.Current.Offset+m.bodyHeight() {
		m.setOffset(end - m.bodyHeight())
	}
}

func (m *Model) selected() hit {
	if index := m.selectionIndex(); index >= 0 {
		return m.selections[index].hit
	}
	return hit{}
}

func (m *Model) selectionIndex() int {
	for i, s := range m.selections {
		if m.isSelected(s.hit) {
			return i
		}
	}
	return -1
}

func (m *Model) isSelected(h hit) bool {
	return h.id == m.session.Current.Selected && h.hot == m.session.Current.Hot
}

func (m *Model) selectHit(h hit) {
	m.session.Current.Selected, m.session.Current.Hot = h.id, h.hot
}
