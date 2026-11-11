package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"v2term/internal/preview"
	"v2term/internal/v2ex"
)

type zone struct {
	start, end int
	hit        hit
}
type textSpan struct {
	start, end int
	target     string
}

// appendContent wraps text once and records exact cell hit areas for inline links.
func (m *Model) appendContent(body []v2ex.Element, fallback, owner string, base hit, indent int) {
	if len(body) == 0 {
		body = []v2ex.Element{{Text: fallback}}
	}
	var text strings.Builder
	var spans []textSpan
	appendText := func() {
		plain := text.String()
		if plain == "" {
			return
		}
		a, b := contentRange(plain)
		if a == b {
			text.Reset()
			spans = nil
			return
		}
		if len(m.lines) > 0 && m.lines[len(m.lines)-1].hit.kind == "image" {
			m.lines = append(m.lines, line{})
		}
		for _, r := range visualRows(plain[a:b], max(4, m.contentWidth()-indent)) {
			start := a + r.start
			row := plain[start : a+r.end]
			row = strings.TrimRight(row, " \t")
			if row == "" && (len(m.lines) == 0 || strings.TrimSpace(v2ex.SafeText(m.lines[len(m.lines)-1].text)) == "") {
				continue
			}
			l := line{hit: base}
			var rendered strings.Builder
			rendered.WriteString(strings.Repeat(" ", indent))
			cursor := 0
			for _, span := range spans {
				a, b := max(start, span.start)-start, min(start+len(row), span.end)-start
				if a >= b {
					continue
				}
				rendered.WriteString(row[cursor:a])
				rendered.WriteString(ansi.SetHyperlink(span.target) + accent.Render(row[a:b]) + ansi.ResetHyperlink())
				cursor = b
			}
			rendered.WriteString(row[cursor:])
			l.text = rendered.String()
			if q := strings.ToLower(m.session.Current.Filter); q != "" && strings.Contains(strings.ToLower(row), q) {
				l.text = matchStyle.Render(l.text)
			}
			m.lines = append(m.lines, l)
		}
		text.Reset()
		spans = nil
	}
	for i, e := range body {
		if !e.Image {
			t := strings.ReplaceAll(v2ex.SafeText(e.Text), "\t", "    ")
			if e.URL != "" {
				t = compactLink(t, e.URL, m.contentWidth()-indent)
				t = v2ex.SafeText(t)
				spans = append(spans, textSpan{start: text.Len(), end: text.Len() + len(t), target: e.URL})
			}
			text.WriteString(t)
			continue
		}
		appendText()
		key := fmt.Sprintf("%s:%d", owner, i)
		h := hit{kind: "image", path: key, target: e.URL}
		caption := "▸ 显示图片"
		if e.Text != "" {
			caption += " · " + v2ex.SafeText(e.Text)
		}
		state := m.images[key]
		if state != nil && state.expanded {
			caption = "▾ 收起图片"
			if state.loading {
				caption = "▾ 图片加载中 · 点击取消"
			} else if state.err != "" {
				caption = "▸ 加载失败 · 点击重试"
			}
		}
		if m.imageProtocol() == preview.Blocks {
			caption += " · 字符预览"
		}
		m.lines = append(m.lines, line{text: strings.Repeat(" ", indent) + ansi.SetHyperlink(e.URL) + accent.Render(caption) + ansi.ResetHyperlink(), hit: h})
		if state != nil && state.expanded && state.image != nil {
			for j := range state.image.Rows {
				row := &state.image.Rows[j]
				l := line{hit: h, imageRow: row, imageColumns: state.image.Columns, imageX: m.contentLeft() + indent}
				if row.Native != "" {
					l.text = strings.Repeat(" ", indent+state.image.Columns)
				} else {
					l.text = strings.Repeat(" ", indent) + row.Text
				}
				m.lines = append(m.lines, l)
			}
		}
		if state != nil && state.expanded && state.err != "" {
			m.lines = append(m.lines, line{text: strings.Repeat(" ", indent) + muted.Render(state.err), hit: h})
		}
	}
	appendText()
}

type inlineAction struct {
	label string
	hit   hit
}

func (m *Model) appendHeader(left, age string, indent int, h hit, actions ...inlineAction) {
	prefix := strings.Repeat(" ", indent+2)
	width := max(1, m.contentWidth()-indent-2)
	labelWidth := 2 * len(actions)
	for _, a := range actions {
		labelWidth += ansi.StringWidth(a.label)
	}
	var labels []string
	var widths []int
	actionWidth := 0
	for _, a := range actions {
		label := a.label
		if labelWidth > width {
			label = strings.TrimSuffix(label, "作者")
		}
		label = controlLabel(label)
		w := ansi.StringWidth(label)
		labels = append(labels, accent.Render(label))
		widths = append(widths, w)
		actionWidth += w
	}
	right := strings.Join(labels, "")
	if age = compactAge(v2ex.SafeText(age)); age != "" {
		budget := width - actionWidth - 2
		if budget >= ansi.StringWidth(age) || budget >= 5 {
			age = ansi.Truncate(age, budget, "…")
		} else {
			age = ""
		}
		if age != "" {
			right = muted.Render(age) + "  " + right
		}
	}
	l := line{hit: h}
	if ansi.StringWidth(left)+2+ansi.StringWidth(right) <= width {
		l.text = prefix + splitRow(left, right, width)
	} else {
		m.lines = append(m.lines, line{text: prefix + ansi.Truncate(left, width, "…"), hit: h})
		l.text = prefix + strings.Repeat(" ", max(0, width-ansi.StringWidth(right))) + right
	}
	x := m.contentLeft() + ansi.StringWidth(l.text) - actionWidth
	for i, a := range actions {
		w := widths[i]
		l.zones = append(l.zones, zone{start: x, end: x + w, hit: a.hit})
		x += w
	}
	m.lines = append(m.lines, l)
}
