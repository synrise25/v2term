package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"v2term/internal/preview"
	"v2term/internal/state"
	"v2term/internal/v2ex"
)

type API interface {
	Fetch(context.Context, string) (v2ex.Page, error)
	Reply(context.Context, string, string) error
	Thank(context.Context, string, string, int) error
	Authenticated() bool
}

type hit struct {
	kind, path, target string
	id                 int
	hot                bool
}
type line struct {
	text                 string
	hit                  hit
	heading, first       bool
	indent               int
	threadIndent         int
	threadPrefix         string
	zones                []zone
	imageRow             *preview.Row
	imageColumns, imageX int
	normal, highlight    string
	focused              [2]string
	right                string
}
type button struct {
	label, action string
	start, end    int
	active        bool
	focused       bool
}

type selection struct {
	hit     hit
	at      int
	actions int
}
type cached struct {
	page v2ex.Page
	at   time.Time
}
type loadedMsg struct {
	request int
	page    v2ex.Page
	err     error
}
type actionMsg struct {
	path, kind string
	draftKey   string
	id         int
	err        error
}

type Options struct {
	CacheTTL     time.Duration
	CacheSize    int
	ScrollLines  int
	HotReplies   int
	HotMinThanks int
	ImageOutput  *preview.Output
}

func DefaultOptions() Options {
	return Options{CacheTTL: time.Minute, CacheSize: 32, ScrollLines: 1, HotReplies: 3, HotMinThanks: 3}
}

type Model struct {
	ctx                      context.Context
	api                      API
	session                  state.Session
	page                     v2ex.Page
	cache                    map[string]cached
	lines                    []line
	selections               []selection
	width, height            int
	mode                     string
	focus                    focusTarget
	category                 int
	input                    textinput.Model
	editor                   textarea.Model
	status                   string
	loading, writing         bool
	request                  int
	cancel                   context.CancelFunc
	confirmation             hit
	options                  Options
	images                   map[string]*imageState
	imageLoader              *preview.Loader
	composerKey, replyTarget string
	chromeKey                chromeKey
	chrome                   []string
	actionsText              string
}

func New(ctx context.Context, api API, session state.Session, opts ...Options) *Model {
	options := DefaultOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	options.ScrollLines = min(20, max(1, options.ScrollLines))
	options.HotReplies = min(100, max(0, options.HotReplies))
	options.HotMinThanks = max(0, options.HotMinThanks)
	in := textinput.New()
	in.CharLimit = 1024
	ed := textarea.New()
	ed.Placeholder = "写下回复；Esc 保存草稿并返回"
	ed.CharLimit = 20000
	ed.ShowLineNumbers = false
	return &Model{ctx: ctx, api: api, session: session, cache: map[string]cached{}, width: 100, height: 30, input: in, editor: ed, options: options, images: map[string]*imageState{}, imageLoader: preview.NewLoader()}
}

func (m *Model) Init() tea.Cmd { return m.load(true) }

func (m *Model) Session() state.Session {
	if m.mode == "reply" {
		m.saveDraft()
	}
	session := m.session
	session.Current = m.savedLocation()
	return session
}

func (m *Model) draftKey() string {
	if m.mode == "reply" && m.composerKey != "" {
		return m.composerKey
	}
	return strings.Split(m.session.Current.Path, "?")[0]
}
func (m *Model) saveDraft() {
	if text := m.editor.Value(); text != "" {
		m.session.Drafts[m.draftKey()] = text
	} else {
		delete(m.session.Drafts, m.draftKey())
	}
}

func (m *Model) load(force bool) tea.Cmd {
	m.focus = focusContent
	if m.page.Path == m.session.Current.Path {
		m.session.Current = m.savedLocation()
	}
	m.clearImages()
	if m.cancel != nil {
		m.cancel()
	}
	m.request++
	m.loading, m.status = false, ""
	path := m.session.Current.Path
	if item, ok := m.cache[path]; !force && ok && time.Since(item.at) < m.options.CacheTTL {
		m.page = item.page
		m.rebuild()
		return nil
	}
	m.loading = true
	m.page, m.lines = v2ex.Page{}, nil
	m.selections = nil
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	request, api := m.request, m.api
	return func() tea.Msg {
		page, err := api.Fetch(ctx, path)
		return loadedMsg{request: request, page: page, err: err}
	}
}

func (m *Model) navigate(path string) tea.Cmd {
	canonical, err := v2ex.Route(path)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if canonical == m.session.Current.Path {
		return m.load(false)
	}
	m.session.Back = state.Trim(append(m.session.Back, m.savedLocation()))
	m.session.Forward = nil
	m.session.Current = state.Location{Path: canonical}
	return m.load(false)
}

func (m *Model) history(back bool) tea.Cmd {
	from, to := &m.session.Forward, &m.session.Back
	if back {
		from, to = &m.session.Back, &m.session.Forward
	}
	if len(*from) == 0 {
		return nil
	}
	*to = state.Trim(append(*to, m.savedLocation()))
	m.session.Current = (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	return m.load(false)
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.input.SetWidth(max(10, m.contentWidth()-4))
	m.editor.SetWidth(max(10, m.contentWidth()-4))
	m.editor.SetHeight(max(3, height-10))
	if !m.loading {
		m.rebuild()
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imageMsg:
		state := m.images[msg.key]
		if msg.request != m.request || state == nil || state.generation != msg.generation || !state.expanded {
			return m, nil
		}
		state.loading = false
		if msg.err != nil {
			state.err = msg.err.Error()
		} else {
			state.image = msg.image
		}
		m.rebuild()
		if msg.err == nil && (msg.columns != m.imageWidth() || msg.rows != max(1, m.bodyHeight()-3)) {
			return m, m.resizeImages()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, m.resizeImages()
	case loadedMsg:
		if msg.request != m.request {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.page = msg.page
		m.session.Current.Path = msg.page.Path
		if m.options.CacheSize > 0 && m.options.CacheTTL > 0 {
			if len(m.cache) >= m.options.CacheSize {
				var oldest string
				var at time.Time
				for path, item := range m.cache {
					if oldest == "" || item.at.Before(at) {
						oldest, at = path, item.at
					}
				}
				delete(m.cache, oldest)
			}
			m.cache[msg.page.Path] = cached{page: msg.page, at: time.Now()}
		}
		m.rebuild()
		return m, nil
	case actionMsg:
		m.writing = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		// Invalidate all lists and pages: reply counts and balances may have changed.
		clear(m.cache)
		if msg.kind == "reply" {
			delete(m.session.Drafts, msg.draftKey)
			m.editor.Reset()
			m.mode = ""
			m.editor.Blur()
		}
		cmd := m.load(true)
		m.status = "操作成功"
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "q" && m.mode != "reply" && m.mode != "address" && m.mode != "filter" {
			return m, tea.Quit
		}
		if m.mode != "" {
			return m.modalKey(msg)
		}
		if m.writing && msg.String() != "q" {
			return m, nil
		}
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "?":
			m.focus = focusContent
			m.mode = "help"
			return m, nil
		case "g":
			return m, m.prompt("address")
		case "/":
			return m, m.prompt("filter")
		case "esc":
			if m.focus != focusContent {
				m.focus = focusContent
				return m, nil
			}
			m.session.Current.Filter = ""
			m.rebuild()
			return m, nil
		case "alt+left", "backspace", "h":
			return m, m.history(true)
		case "left":
			return m, m.moveHorizontal(-1)
		case "right":
			if !m.loading {
				return m, m.moveHorizontal(1)
			}
			return m, nil
		case "l":
			return m, m.history(false)
		case "R", "ctrl+r":
			return m, m.load(true)
		case "1":
			return m, m.navigate("/?tab=hot")
		case "2":
			return m, m.navigate("/recent")
		case "3":
			return m, m.navigate("/?tab=qna")
		case "4":
			return m, m.navigate("/?tab=tech")
		case "[", "p":
			return m, m.paginate(-1)
		case "]", "n":
			return m, m.paginate(1)
		case "v":
			m.toggleConversation()
			return m, nil
		case "r":
			return m, m.compose()
		case "a":
			return m, m.composeTo(0)
		case "t":
			m.confirmThank(false)
			return m, nil
		case "T":
			m.confirmThank(true)
			return m, nil
		}
		if m.loading {
			return m, nil
		}
		switch msg.String() {
		case "enter":
			return m, m.activateFocus()
		case "tab", "down", "j":
			m.moveVertical(1)
		case "shift+tab", "up", "k":
			m.moveVertical(-1)
		case "pgdown", "space", "ctrl+d":
			m.scroll(m.bodyHeight() - 1)
		case "pgup", "ctrl+u":
			m.scroll(-m.bodyHeight() + 1)
		case "home":
			m.scroll(-len(m.lines))
		case "end", "G":
			m.scroll(len(m.lines))
		}
		return m, nil
	case tea.MouseClickMsg:
		if m.mode != "" || m.writing {
			return m, nil
		}
		if msg.Button == tea.MouseRight && !m.loading && m.page.Topic == nil {
			return m, m.activate(m.selected())
		}
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		if msg.Y == 1 {
			for _, b := range m.navigation() {
				if msg.X >= b.start && msg.X < b.end {
					return m, m.action(b.action)
				}
			}
		}
		if msg.Y == 2 {
			if m.status == "" && m.session.Current.Filter == "" {
				return m, m.prompt("address")
			}
			return m, nil
		}
		if msg.Y == m.height-1 {
			for _, b := range m.actions() {
				if msg.X >= b.start && msg.X < b.end {
					return m, m.action(b.action)
				}
			}
		}
		if !m.loading && msg.Y >= 4 && msg.Y < m.height-1 && msg.X >= m.contentLeft() && msg.X < m.contentLeft()+m.contentWidth() {
			at := m.session.Current.Offset + msg.Y - 4
			if at < len(m.lines) {
				l := m.lines[at]
				for _, z := range l.zones {
					if msg.X >= z.start && msg.X < z.end {
						return m, m.activate(z.hit)
					}
				}
				h := l.hit
				if h.kind == "reply" || h.kind == "op" {
					m.focus = focusContent
					m.selectHit(h)
					return m, nil
				}
				return m, m.activate(h)
			}
		}
	case tea.MouseWheelMsg:
		if m.mode != "" || m.loading {
			return m, nil
		}
		if msg.Button == tea.MouseWheelUp {
			m.scroll(-m.options.ScrollLines)
		} else if msg.Button == tea.MouseWheelDown {
			m.scroll(m.options.ScrollLines)
		}
		return m, nil
	}
	if !m.writing {
		var cmd tea.Cmd
		if paste, ok := msg.(tea.PasteMsg); ok {
			paste.Content = v2ex.SafeText(paste.Content)
			msg = paste
		}
		switch m.mode {
		case "reply":
			m.editor, cmd = m.editor.Update(msg)
		case "address", "filter":
			m.input, cmd = m.input.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) bodyHeight() int { return max(1, m.height-5) }
func (m *Model) scroll(delta int) {
	m.focus = focusContent
	m.setOffset(m.session.Current.Offset + delta)
}

func (m *Model) paginate(delta int) tea.Cmd {
	if m.loading || m.page.Path == "" {
		return nil
	}
	n := m.page.Number + delta
	if n < 1 || n > m.page.Pages {
		m.status = "没有更多分页"
		return nil
	}
	return m.navigate(v2ex.PagePath(m.page.Path, n))
}

func (m *Model) activate(h hit) tea.Cmd {
	m.focus = focusContent
	switch h.kind {
	case "reply-author":
		return m.composeAuthor()
	case "thank-topic":
		m.confirmThank(true)
	case "image":
		return m.toggleImage(h)
	case "reply-to":
		m.selectHit(h)
		return m.composeTo(h.id)
	case "thank-reply":
		m.selectHit(h)
		m.confirmThank(false)
	case "topic", "node":
		return m.navigate(h.path)
	}
	return nil
}

func (m *Model) action(action string) tea.Cmd {
	switch action {
	case "refresh":
		return m.load(true)
	case "tech":
		return m.navigate("/?tab=tech")
	case "hot":
		return m.navigate("/?tab=hot")
	case "recent":
		return m.navigate("/recent")
	case "qna":
		return m.navigate("/?tab=qna")
	case "address":
		return m.prompt("address")
	case "previous":
		return m.paginate(-1)
	case "next":
		return m.paginate(1)
	case "reply":
		return m.compose()
	case "reply-topic":
		return m.composeTo(0)
	case "thank":
		m.confirmThank(false)
	case "thank-topic":
		m.confirmThank(true)
	case "conversation":
		m.toggleConversation()
	case "quit":
		return tea.Quit
	case "help":
		m.mode = "help"
	}
	return nil
}

func (m *Model) toggleConversation() {
	if m.loading {
		return
	}
	m.focus = focusContent
	m.session.Conversation = !m.session.Conversation
	m.rebuild()
	if h := m.selected(); h.kind == "reply" {
		for index, s := range m.selections {
			if s.hit == h {
				m.revealSelection(index)
				break
			}
		}
	}
}

func (m *Model) prompt(mode string) tea.Cmd {
	m.focus = focusContent
	m.mode = mode
	m.input.SetValue("")
	m.input.Placeholder = "节点名、帖子编号或 V2EX 地址"
	if mode == "filter" {
		m.input.SetValue(m.session.Current.Filter)
		m.input.Placeholder = "查找当前页标题或正文"
	}
	return m.input.Focus()
}

func (m *Model) compose() tea.Cmd {
	id := 0
	if h := m.selected(); h.kind == "reply" {
		id = h.id
	}
	return m.composeTo(id)
}

func (m *Model) composeTo(id int) tea.Cmd {
	if m.loading || m.page.Topic == nil || m.writing {
		return nil
	}
	if !m.api.Authenticated() {
		m.status = "回复需要登录：使用 --cookie-file 或 V2TERM_COOKIE"
		return nil
	}
	if !m.page.CanReply {
		m.status = "当前帖子无法回复，请检查登录和帖子权限"
		return nil
	}
	key := strings.Split(m.page.Path, "?")[0]
	prefix := ""
	m.replyTarget = "主题"
	if id > 0 {
		found := false
		for _, r := range m.page.Replies {
			if r.ID == id {
				found = true
				key += fmt.Sprintf("#reply-%d", id)
				prefix = fmt.Sprintf("@%s #%d ", r.Author, r.Floor)
				m.replyTarget = fmt.Sprintf("@%s · #%d", r.Author, r.Floor)
				break
			}
		}
		if !found {
			m.status = "目标回复已不在当前页，请刷新核对"
			return nil
		}
	}
	m.mode, m.composerKey = "reply", key
	m.focus = focusContent
	content := m.session.Drafts[key]
	if content == "" {
		content = prefix
	}
	m.editor.SetValue(content)
	return m.editor.Focus()
}

func (m *Model) composeAuthor() tea.Cmd {
	if m.page.Topic == nil || m.page.Topic.Author == "" {
		return nil
	}
	cmd := m.composeTo(0)
	if m.mode != "reply" {
		return cmd
	}
	m.composerKey = strings.Split(m.page.Path, "?")[0] + "#author"
	m.replyTarget = "@" + m.page.Topic.Author + " · 主题作者"
	content := m.session.Drafts[m.composerKey]
	if content == "" {
		content = "@" + m.page.Topic.Author + " "
	}
	m.editor.SetValue(content)
	return cmd
}

func (m *Model) confirmThank(topic bool) {
	if m.loading || m.page.Topic == nil || m.writing {
		return
	}
	if !m.api.Authenticated() {
		m.status = "感谢需要登录：使用 --cookie-file 或 V2TERM_COOKIE"
		return
	}
	m.confirmation = hit{kind: "topic", id: m.page.Topic.ID}
	if !topic {
		h := m.selected()
		if h.kind != "reply" && h.kind != "op" {
			m.status = "先点击一条回复，或按 T 感谢主题"
			return
		}
		if h.kind == "reply" {
			m.confirmation = h
		}
	}
	m.mode = "thank"
	m.focus = focusContent
}

func (m *Model) modalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.mode == "help" {
		m.mode = ""
		return m, nil
	}
	if key == "esc" && !m.writing {
		if m.mode == "reply" {
			m.saveDraft()
			m.status = "回复" + m.replyTarget + "的草稿已保留"
		}
		m.input.Blur()
		m.editor.Blur()
		m.mode = ""
		return m, nil
	}
	if m.writing {
		return m, nil
	}
	if m.mode == "thank" {
		if key == "y" || key == "enter" {
			m.writing, m.mode = true, ""
			m.status = "正在发送感谢…"
			path, target, api, ctx := m.page.Path, m.confirmation, m.api, m.ctx
			return m, func() tea.Msg {
				return actionMsg{path: path, kind: "thank", id: target.id, err: api.Thank(ctx, path, target.kind, target.id)}
			}
		}
		if key == "n" || key == "q" {
			m.mode = ""
		}
		return m, nil
	}
	if m.mode == "reply" {
		if key == "ctrl+s" {
			m.saveDraft()
			content := m.editor.Value()
			if strings.TrimSpace(content) == "" {
				m.status = "回复不能为空"
				return m, nil
			}
			m.writing = true
			m.status = "正在发送回复…"
			path, api, ctx, draftKey := m.page.Path, m.api, m.ctx, m.draftKey()
			return m, func() tea.Msg {
				return actionMsg{path: path, kind: "reply", draftKey: draftKey, err: api.Reply(ctx, path, content)}
			}
		}
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
	if key == "enter" {
		value, mode := m.input.Value(), m.mode
		m.input.Blur()
		m.mode = ""
		if mode == "address" {
			return m, m.navigate(value)
		}
		m.session.Current.Filter, m.session.Current.Offset = v2ex.SafeText(value), 0
		m.rebuild()
		if m.page.Topic != nil && value != "" {
			for at, l := range m.lines {
				if strings.Contains(strings.ToLower(v2ex.SafeText(l.text)), strings.ToLower(value)) {
					m.session.Current.Offset = at
					m.scroll(0)
					break
				}
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) replyOrder() []v2ex.ConversationReply {
	if m.session.Conversation {
		return v2ex.Conversation(m.page.Replies)
	}
	ordered := make([]v2ex.ConversationReply, len(m.page.Replies))
	for i, r := range m.page.Replies {
		ordered[i] = v2ex.ConversationReply{Reply: r}
	}
	return ordered
}
