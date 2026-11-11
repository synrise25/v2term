package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"v2term/internal/v2ex"
)

func (m *Model) appendTopics(topics []v2ex.Topic) {
	query := strings.ToLower(m.session.Current.Filter)
	badgeWidth := 4
	for _, t := range topics {
		badgeWidth = max(badgeWidth, len(strconv.Itoa(max(0, t.Replies)))+2)
	}
	badgeWidth = min(badgeWidth, max(4, m.contentWidth()-8))
	leftWidth := max(4, m.contentWidth()-badgeWidth-2)
	for _, t := range topics {
		h := hit{kind: "topic", path: t.Path, id: t.ID}
		title := strings.TrimSpace(v2ex.SafeText(t.Title))
		start := len(m.lines)
		for _, row := range visualRows(title, max(2, leftWidth-2)) {
			text := strings.TrimRight(title[row.start:row.end], " ")
			if query != "" && strings.Contains(strings.ToLower(text), query) {
				text = matchStyle.Render(text)
			}
			m.lines = append(m.lines, line{text: "  " + text, hit: h, heading: true, first: len(m.lines) == start})
		}
		count := ansi.Truncate(strconv.Itoa(max(0, t.Replies)), badgeWidth-2, "…")
		m.lines[start].right = controlLabel(strings.Repeat(" ", max(0, badgeWidth-2-ansi.StringWidth(count))) + count)
		var metadata []string
		for _, value := range []string{t.Node, t.Author, compactAge(t.Age)} {
			if value != "" {
				metadata = append(metadata, v2ex.SafeText(value))
			}
		}
		text := strings.Join(metadata, " · ")
		if text != "" {
			for _, row := range visualRows(text, max(2, leftWidth-2)) {
				part := text[row.start:row.end]
				style := muted
				if query != "" && strings.Contains(strings.ToLower(part), query) {
					style = matchStyle
				}
				m.lines = append(m.lines, line{text: "  " + style.Render(part), hit: h})
			}
		}
		m.lines = append(m.lines, line{})
	}
}
