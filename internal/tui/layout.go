package tui

import (
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Keep prose readable on wide terminals, with the same geometry for text,
// buttons and native image placements.
func (m *Model) contentWidth() int { return max(1, min(112, m.width-4)) }
func (m *Model) contentLeft() int  { return max(0, (m.width-m.contentWidth())/2) }
func (m *Model) imageWidth() int   { return max(1, m.contentWidth()-12) }

// End highlighted controls on an ASCII cell instead of a wide glyph's trailing
// cell. Include the padding in hit areas so rendering and mouse geometry agree.
func controlLabel(label string) string { return " " + label + " " }

func (m *Model) frameLine(s string) string {
	left, width := m.contentLeft(), m.contentWidth()
	return strings.Repeat(" ", left) + fit(s, width) + strings.Repeat(" ", max(0, m.width-left-width))
}

func splitRow(left, right string, width int) string {
	rightWidth := ansi.StringWidth(right)
	if right == "" || rightWidth+2 >= width {
		return fit(left, width)
	}
	n := width - rightWidth - 2
	return fit(left, n) + "  " + right
}

func compactAge(raw string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -07:00", "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("01-02 15:04")
		}
	}
	return raw
}

// Only abbreviate URLs used as their own label. Descriptive link text stays
// intact, and the native hyperlink always retains the complete destination.
func compactLink(text, target string, width int) string {
	limit := max(24, min(48, width/2))
	if strings.TrimRight(text, "/") != strings.TrimRight(target, "/") || ansi.StringWidth(text) <= limit {
		return text
	}
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return text
	}
	label := u.Host
	if name := path.Base(u.Path); name != "." && name != "/" {
		name, err = url.PathUnescape(name)
		if err != nil {
			return text
		}
		name = ansi.Truncate(name, max(8, limit-ansi.StringWidth(label)-3), "…")
		label += "/…/" + name
	}
	return ansi.Truncate(label, limit, "…")
}

// Strip paragraph margins introduced by HTML block elements, preserving code
// indentation and intentional blank lines inside a paragraph group.
func contentRange(s string) (start, end int) {
	end = len(s)
	for start < end {
		n := strings.IndexByte(s[start:end], '\n')
		if n < 0 || strings.TrimSpace(s[start:start+n]) != "" {
			break
		}
		start += n + 1
	}
	for end > start {
		n := strings.LastIndexByte(s[start:end], '\n')
		if n < 0 || strings.TrimSpace(s[start+n+1:end]) != "" {
			break
		}
		end = start + n
	}
	if strings.TrimSpace(s[start:end]) == "" {
		return end, end
	}
	return start, end
}
