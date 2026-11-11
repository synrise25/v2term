package tui

import "sort"

func (m *Model) setOffset(offset int) {
	m.session.Current.Offset = min(max(0, offset), max(0, len(m.lines)-m.bodyHeight()))
	// Keep the current item while any of its block remains visible, including
	// wrapped text and images. When it leaves, select the first visible block.
	start := m.session.Current.Offset
	end := min(len(m.lines), start+m.bodyHeight())
	if len(m.selections) == 0 {
		return
	}
	first := max(0, sort.Search(len(m.selections), func(i int) bool { return m.selections[i].at > start })-1)
	for _, s := range m.selections[first:] {
		if s.at >= end {
			break
		}
		if m.isSelected(s.hit) {
			return
		}
	}
	m.selectHit(m.selections[first].hit)
}
