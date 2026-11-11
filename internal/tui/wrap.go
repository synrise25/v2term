package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type textRange struct{ start, end int }

// visualRows preserves byte ranges for links while applying Unicode line breaks.
// Unlike space-only wrapping, Chinese prose can fill the available columns.
func visualRows(text string, width int) []textRange {
	var rows []textRange
	base := 0
	for _, paragraph := range strings.Split(text, "\n") {
		start, offset, cells := base, base, 0
		rest, state := paragraph, -1
		for rest != "" {
			var segment string
			segment, rest, _, state = uniseg.FirstLineSegmentInString(rest, state)
			segmentWidth := ansi.StringWidth(segment)
			if cells+ansi.StringWidth(strings.TrimRight(segment, " ")) > width && cells > 0 && strings.TrimSpace(text[start:offset]) != "" {
				rows = append(rows, textRange{start, offset})
				start, cells = offset, 0
			}
			if cells+segmentWidth <= width {
				cells += segmentWidth
				offset += len(segment)
				continue
			}
			g := uniseg.NewGraphemes(segment)
			for g.Next() {
				w := g.Width()
				if cells+w > width && cells > 0 {
					rows = append(rows, textRange{start, offset})
					start, cells = offset, 0
				}
				cells += w
				offset += len(g.Str())
			}
		}
		rows = append(rows, textRange{start, offset})
		base += len(paragraph) + 1
	}
	return rows
}
