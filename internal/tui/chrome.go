package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type chromeKey struct {
	width         int
	path, user    string
	auth, topic   bool
	conversation  bool
	replySelected bool
	actionWidth   int
	focus         focusTarget
	category      int
}

func (m *Model) cachedChrome() []string {
	key := chromeKey{width: m.width, path: m.session.Current.Path, user: m.page.Username, auth: m.api.Authenticated(), topic: m.page.Topic != nil, conversation: m.session.Conversation, replySelected: m.page.Topic != nil && m.session.Current.Selected != 0, actionWidth: m.actionWidth(), focus: m.focus, category: m.category}
	if m.chrome != nil && key == m.chromeKey {
		return m.chrome
	}
	user := "游客"
	if key.user != "" {
		user = "@" + key.user
	} else if key.auth {
		user = "已配置 Cookie"
	}
	context := "浏览"
	for _, category := range []struct{ path, name string }{{"/?tab=hot", "最热"}, {"/recent", "最新"}, {"/?tab=qna", "问与答"}, {"/?tab=tech", "技术"}} {
		if key.path == category.path {
			context = category.name
		}
	}
	if key.topic {
		context = "阅读"
	}
	address := "https://www.v2ex.com" + key.path
	addressRow := ansi.SetHyperlink(address) + muted.Render("v2ex.com"+key.path) + ansi.ResetHyperlink()
	if !key.topic {
		addressRow = splitRow(addressRow, muted.Render("回复"), m.contentWidth())
	}
	m.chrome = []string{
		m.frameLine(splitRow(strong.Render("V2TERM")+muted.Render(" / "+context), muted.Render(user), m.contentWidth())),
		m.frameLine(renderButtons(m.navigation())),
		m.frameLine(addressRow),
		m.frameLine(guideStyle.Render(strings.Repeat("─", m.contentWidth()))),
	}
	m.actionsText = renderButtons(m.actions())
	m.chromeKey = key
	return m.chrome
}

func (m *Model) buttons(items ...button) []button {
	x := m.contentLeft()
	for i := range items {
		items[i].start = x
		x += ansi.StringWidth(controlLabel(items[i].label))
		items[i].end = x
	}
	return items
}

func (m *Model) navigation() []button {
	labels := []string{"1 最热", "2 最新", "3 问与答", "4 技术"}
	if m.contentWidth() < 36 {
		labels = []string{"1热", "2新", "3问", "4技"}
	}
	var items []button
	width := 0
	for i, c := range categories {
		items = append(items, button{label: labels[i], action: c.action, active: m.session.Current.Path == c.path, focused: m.focus == focusCategories && i == m.category})
		width += ansi.StringWidth(controlLabel(labels[i]))
	}
	if m.focus != focusCategories {
		if width+16 <= m.contentWidth() {
			items = append(items, button{label: "R 刷新", action: "refresh"}, button{label: "g 地址", action: "address"})
		} else if width+8 <= m.contentWidth() {
			items = append(items, button{label: "g 地址", action: "address"})
		}
	}
	return m.buttons(items...)
}

func (m *Model) actions() []button {
	width := m.actionWidth()
	if m.page.Topic != nil {
		reply := m.session.Current.Selected != 0
		label, thank := "r 回复主题", "t 感谢主题"
		if reply {
			label, thank = "r 回复楼层", "t 感谢楼层"
		}
		items := []button{{label: label, action: "reply"}}
		if reply {
			items = append(items, button{label: "a 回复主题", action: "reply-topic"})
		}
		mode := "v 时间顺序"
		if !m.session.Conversation {
			mode = "v 对话视图"
		}
		switch {
		case width >= 90:
			items = append(items, button{label: thank, action: "thank"}, button{label: mode, action: "conversation"}, button{label: "[ 上页", action: "previous"}, button{label: "] 下页", action: "next"}, button{label: "? 帮助", action: "help"}, button{label: "q 退出", action: "quit"})
		case width >= 58:
			if reply {
				items[1].label = "a 主题"
			}
			items = append(items, button{label: "t 感谢", action: "thank"}, button{label: mode, action: "conversation"}, button{label: "[", action: "previous"}, button{label: "]", action: "next"}, button{label: "?", action: "help"}, button{label: "q 退出", action: "quit"})
		case width >= 36:
			if reply {
				items[1].label = "a 主题"
			}
			items = append(items, button{label: "? 帮助", action: "help"}, button{label: "q 退出", action: "quit"})
		case width >= 22:
			items[0].label = "r 主题"
			if reply {
				items[0].label, items[1].label = "r 楼层", "a 主题"
			}
			items = append(items, button{label: "?", action: "help"}, button{label: "q", action: "quit"})
		default:
			items[0].label = "r"
			if reply {
				items[1].label = "a"
			}
			items = append(items, button{label: "?", action: "help"}, button{label: "q", action: "quit"})
		}
		return m.buttons(items...)
	}
	if width < 15 {
		return m.buttons(button{label: "[", action: "previous"}, button{label: "]", action: "next"}, button{label: "?", action: "help"}, button{label: "q", action: "quit"})
	}
	if width < 20 {
		return m.buttons(button{label: "[", action: "previous"}, button{label: "]", action: "next"}, button{label: "g", action: "address"}, button{label: "?", action: "help"}, button{label: "q", action: "quit"})
	}
	if width < 48 {
		return m.buttons(button{label: "[", action: "previous"}, button{label: "]", action: "next"}, button{label: "g 地址", action: "address"}, button{label: "?", action: "help"}, button{label: "q", action: "quit"})
	}
	return m.buttons(button{label: "[ 上页", action: "previous"}, button{label: "] 下页", action: "next"}, button{label: "g 节点/地址", action: "address"}, button{label: "? 帮助", action: "help"}, button{label: "q 退出", action: "quit"})
}

func (m *Model) positionLabel(progress int) string {
	format := "%d / %d 页 · %d%%"
	if m.contentWidth() < 60 {
		format = "%d/%d · %d%%"
	}
	page, pages := max(1, m.page.Number), max(1, m.page.Pages)
	// Reserve the width of 100% throughout scrolling so controls never shift
	// when the percentage gains a digit. Keep a usable key area on small screens.
	if m.contentWidth()-ansi.StringWidth(fmt.Sprintf(format, page, pages, 100))-2 < 14 {
		return fmt.Sprintf("%d%%", progress)
	}
	return fmt.Sprintf(format, page, pages, progress)
}

func (m *Model) actionWidth() int {
	return max(0, m.contentWidth()-ansi.StringWidth(m.positionLabel(100))-2)
}

func renderButtons(items []button) string {
	labels := make([]string, len(items))
	for i, b := range items {
		style := muted
		if b.active {
			style = activeStyle
		}
		if b.focused {
			style = focusedStyle
		}
		labels[i] = style.Render(controlLabel(b.label))
	}
	return strings.Join(labels, "")
}
