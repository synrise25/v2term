package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/preview"
	"v2term/internal/v2ex"
)

var (
	accent        = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	guideStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	strong        = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Background(lipgloss.Color("236")).Bold(true)
	focusedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("75")).Bold(true)
	countStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("109")).Background(lipgloss.Color("236"))
	matchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
)

func (m *Model) rebuild() {
	offset := m.session.Current.Offset
	m.lines = nil
	m.selections = nil
	if m.page.Path == "" {
		return
	}
	width := m.contentWidth()
	add := func(text string, h hit, style lipgloss.Style, indent int) {
		prefix := strings.Repeat(" ", indent)
		text = strings.ReplaceAll(v2ex.SafeText(text), "\t", "    ")
		for _, r := range visualRows(text, max(4, width-indent)) {
			s := strings.TrimRight(text[r.start:r.end], " ")
			if q := strings.ToLower(m.session.Current.Filter); q != "" && strings.Contains(strings.ToLower(s), q) {
				m.lines = append(m.lines, line{text: prefix + matchStyle.Render(s), hit: h})
			} else {
				m.lines = append(m.lines, line{text: prefix + style.Render(s), hit: h})
			}
		}
	}
	blank := func() { m.lines = append(m.lines, line{}) }
	heading := func(text string, h hit, style lipgloss.Style, indent int) {
		start := len(m.lines)
		add("  "+text, h, style, indent)
		for i := start; i < len(m.lines); i++ {
			m.lines[i].heading = true
			m.lines[i].first = i == start
			m.lines[i].indent = indent
		}
	}
	if t := m.page.Topic; t != nil {
		op := hit{kind: "op"}
		heading(t.Title, op, strong, 0)
		if t.Author != "" {
			info := accent.Render("@" + t.Author)
			if t.Node != "" {
				info += muted.Render(" · " + t.Node)
			}
			m.appendHeader(info, t.Age, 0, hit{kind: "node", path: t.NodePath}, inlineAction{label: "回复作者", hit: hit{kind: "reply-author"}}, inlineAction{label: "感谢作者", hit: hit{kind: "thank-topic"}})
		} else if t.Node != "" || t.Age != "" {
			add(strings.Trim(t.Node+" · "+compactAge(t.Age), " ·"), hit{kind: "node", path: t.NodePath}, muted, 2)
		}
		blank()
		m.appendContent(t.Body, t.Content, fmt.Sprintf("topic:%d", t.ID), op, 2)
		blank()
		ordered := m.replyOrder()
		conversation := ordered
		if !m.session.Conversation && m.options.HotReplies > 0 {
			conversation = v2ex.Conversation(m.page.Replies)
		}
		if hot, count := m.hotThreads(conversation); count > 0 {
			m.appendDivider(fmt.Sprintf("── 热评 %d · 当前页 ", count))
			m.appendReplies(hot, true)
		}
		mode := "时间顺序"
		if m.session.Conversation {
			mode = "对话"
		}
		m.appendDivider(fmt.Sprintf("── 回复 %d · %s ", len(m.page.Replies), mode))
		m.appendReplies(ordered, false)
		if len(m.page.Replies) == 0 {
			add("还没有回复，按 r 写下第一条。", hit{}, muted, 2)
		}
	} else {
		m.session.Current.Hot = false
		query := strings.ToLower(m.session.Current.Filter)
		var topics []v2ex.Topic
		for _, t := range m.page.Topics {
			if query == "" || strings.Contains(strings.ToLower(t.Title+" "+t.Author+" "+t.Node), query) {
				topics = append(topics, t)
			}
		}
		found := false
		for _, t := range topics {
			if t.ID == m.session.Current.Selected {
				found = true
			}
		}
		if !found && len(topics) > 0 {
			m.session.Current.Selected = topics[0].ID
		}
		m.appendTopics(topics)
		if len(topics) == 0 {
			add("没有匹配的帖子，Esc 清除筛选或 R 刷新", hit{}, muted, 0)
		}
	}
	m.session.Current.Offset = offset
	m.session.Current.Offset = min(max(0, offset), max(0, len(m.lines)-m.bodyHeight()))
	for i := range m.lines {
		l := &m.lines[i]
		decorate := func(text string, highlight bool) string {
			prefix := ""
			if l.threadIndent > 0 {
				if len(text) < l.threadIndent {
					text = strings.Repeat(" ", l.threadIndent)
				}
				prefix = guideStyle.Render(l.threadPrefix)
				text = text[l.threadIndent:]
			}
			// Color the clean heading separately: a guide's ANSI reset must not
			// cancel the selection style of the text that follows it.
			if highlight {
				text = selectedStyle.Render(text)
			}
			if l.right != "" {
				style := countStyle
				if highlight {
					style = activeStyle
				}
				return m.frameLine(splitRow(prefix+text, style.Render(l.right), width))
			}
			return m.frameLine(prefix + text)
		}
		l.normal = decorate(l.text, false)
		if l.heading {
			if l.first {
				m.selections = append(m.selections, selection{hit: l.hit, at: i, actions: -1})
			}
			text := v2ex.SafeText(l.text)
			if l.first && len(text) >= l.indent+2 {
				text = text[:l.indent] + "▸ " + text[l.indent+2:]
			}
			l.highlight = decorate(text, true)
		}
		if len(l.zones) > 0 && len(m.selections) > 0 {
			m.selections[len(m.selections)-1].actions = i
			base := l.normal
			if l.heading {
				base = l.highlight
			}
			for action, z := range l.zones[:min(2, len(l.zones))] {
				l.focused[action] = ansi.Cut(base, 0, z.start) + focusedStyle.Render(v2ex.SafeText(ansi.Cut(base, z.start, z.end))) + ansi.Cut(base, z.end, m.width)
			}
		}
	}
	if m.page.Topic != nil && m.selected().kind == "" {
		// A changed threshold/ranking may remove the hot copy. Prefer its
		// original floor before falling back to the topic.
		m.session.Current.Hot = false
		if m.selected().kind == "" {
			m.selectHit(hit{kind: "op"})
		}
	}
}

func fit(s string, width int) string {
	s = ansi.Truncate(s, max(1, width), "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func (m *Model) View() tea.View {
	var placements []preview.Placement
	if m.width < 24 || m.height < 10 {
		v := tea.NewView(" 终端太小，请放大至至少 24×10。q 退出")
		if m.options.ImageOutput != nil {
			v.Content = m.options.ImageOutput.Frame(v.Content, nil)
		}
		v.AltScreen = true
		return v
	}
	rows := append(make([]string, 0, m.height), m.cachedChrome()...)
	if m.status != "" {
		rows[2] = m.frameLine(accent.Render(v2ex.SafeText(m.status)))
	} else if m.session.Current.Filter != "" {
		rows[2] = m.frameLine(muted.Render("查找: " + v2ex.SafeText(m.session.Current.Filter)))
	}
	body := make([]string, 0, m.bodyHeight())
	switch m.mode {
	case "help":
		body = []string{"键盘与鼠标", "", "↑/↓ 或 j/k、Tab/Shift+Tab 选择帖子或楼层", "首条 ↑ 分类栏；←/→ 选择，Enter 确认，↓ 返回", "列表 → / Enter / 鼠标右键打开；← 保存退出", "帖内 → 选择回复、感谢；← 逐级退出再返回", "Enter 执行；Backspace / Alt+← / h 直接返回", "滚轮按行滚动且高亮跟随；PgUp/PgDn 翻屏", "r 回复所选，a 回复主题；t 感谢所选，T 谢主题", "点击图片展开/收起；Command 点击正文链接", "R 刷新；1 最热；2 最新；3 问与答；4 技术", "g 地址；/ 查找；[ / ] 翻页；v 对话/时间", "Ctrl+S 发送回复；Esc 保留草稿或取消选择", "q 保存退出；Ctrl+C 编辑时退出", "", "对话优先引用楼层，其次 @用户名 最近前文。", "≈ 表示推断；仅关联当前页，始终保留楼层号。", "├─ 后面还有同级回复；└─ 同级最后一条", "任意键返回"}
	case "address", "filter":
		caption := "打开节点或帖子"
		if m.mode == "filter" {
			caption = "查找当前页"
		}
		body = []string{strong.Render(caption), "", m.input.View(), "", muted.Render("Enter 确认 · Esc 返回")}
	case "reply":
		body = append(body, strong.Render("回复 "+m.replyTarget+" · "+m.page.Title), "", m.editor.View(), "", muted.Render("Ctrl+S 发送 · Esc 保留草稿 · 编辑时 q 正常输入"))
		body = strings.Split(strings.Join(body, "\n"), "\n")
	case "thank":
		target := fmt.Sprintf("主题 #%d", m.confirmation.id)
		if m.page.Topic != nil && m.page.Topic.Author != "" {
			target += " @" + m.page.Topic.Author
		}
		if m.confirmation.kind == "reply" {
			for _, r := range m.page.Replies {
				if r.ID == m.confirmation.id {
					target = fmt.Sprintf("回复 #%d @%s", r.Floor, r.Author)
				}
			}
		}
		body = []string{strong.Render("发送感谢"), "", target, "", "将按 V2EX 规则扣除铜币。", "", accent.Render("Enter / y 确认 · Esc / n 取消")}
	default:
		if m.loading {
			body = []string{muted.Render("正在加载页面…  q 可随时退出")}
		} else if m.page.Path == "" {
			body = []string{"页面未加载。R 重试，g 更换地址。"}
		} else {
			end := min(len(m.lines), m.session.Current.Offset+m.bodyHeight())
			actionRow := -1
			if m.focus == focusReply || m.focus == focusThank {
				if index := m.selectionIndex(); index >= 0 {
					actionRow = m.selections[index].actions
				}
			}
			for i, l := range m.lines[m.session.Current.Offset:end] {
				if l.imageRow != nil && l.imageRow.Native != "" && l.imageX+l.imageColumns <= m.width {
					placements = append(placements, preview.Placement{X: l.imageX, Y: 4 + len(body), Columns: l.imageColumns, Row: l.imageRow})
				}
				text := l.normal
				if m.focus != focusCategories && l.heading && m.isSelected(l.hit) {
					text = l.highlight
				}
				if m.session.Current.Offset+i == actionRow {
					text = l.focused[m.focus-focusReply]
				}
				body = append(body, text)
			}
		}
	}
	blankRow := strings.Repeat(" ", m.width)
	for i := 0; i < m.bodyHeight(); i++ {
		if i >= len(body) {
			rows = append(rows, blankRow)
			continue
		}
		s := body[i]
		if m.mode != "" || m.loading || m.page.Path == "" {
			s = m.frameLine("  " + s)
		}
		rows = append(rows, s)
	}
	progress := min(100, (m.session.Current.Offset+m.bodyHeight())*100/max(1, len(m.lines)))
	rows = append(rows, m.frameLine(splitRow(m.actionsText, muted.Render(m.positionLabel(progress)), m.contentWidth())))
	v := tea.NewView(strings.Join(rows, "\n"))
	if m.options.ImageOutput != nil {
		v.Content = m.options.ImageOutput.Frame(v.Content, placements)
	}
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "v2term · V2EX"
	return v
}
