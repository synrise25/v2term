package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/preview"
	"v2term/internal/v2ex"
)

func readingPage() v2ex.Page {
	return v2ex.Page{Path: "/t/1", Title: "终端里的阅读体验", Number: 1, Pages: 2, CanReply: true,
		Topic: &v2ex.Topic{ID: 1, Title: "终端里的阅读体验", Author: "OP", Node: "技术", Age: "2026-11-09 13:30:00 +08:00", Body: []v2ex.Element{{Text: "\n\n正文的第一段。\n\n正文的第二段。\n\n"}}},
		Replies: []v2ex.Reply{
			{ID: 101, Floor: 1, Author: "alice", Age: "2026-11-09 13:52:02 +08:00", Content: "第一条回复", Body: []v2ex.Element{{Text: "\n第一段回复。\n\n第二段回复。\n"}}},
			{ID: 102, Floor: 2, Author: "bob", Content: "@alice #1 回复内容", Age: "2026-11-09 13:58:34 +08:00", Body: []v2ex.Element{{Text: "\n@alice #1 回复内容\n"}}},
			{ID: 103, Floor: 3, Author: strings.Repeat("longname", 4), Thanks: 2, Content: "独立的第三条回复"},
		}}
}

func TestReadingLayoutFitsAndKeepsClickableControls(t *testing.T) {
	for _, width := range []int{24, 32, 40, 60, 64, 80, 100, 120, 160, 220} {
		for _, topic := range []bool{false, true} {
			t.Run(fmt.Sprintf("width_%d/topic_%t", width, topic), func(t *testing.T) {
				p := readingPage()
				if !topic {
					p = listPage("/?tab=tech")
				}
				m := testTopicModel(p)
				m.resize(width, 30)
				rows := strings.Split(m.View().Content, "\n")
				if len(rows) != 30 {
					t.Fatal("layout changed screen height")
				}
				for i, row := range rows {
					if got := ansi.StringWidth(row); got != width {
						t.Fatalf("row %d: width %d, want %d", i, got, width)
					}
				}
				for _, b := range append(m.navigation(), m.actions()...) {
					if b.start < m.contentLeft() || b.end > m.contentLeft()+m.contentWidth() {
						t.Fatalf("control %s was clipped", b.label)
					}
				}
				if topic {
					m.moveSelection(1)
					m.View()
					for _, b := range m.actions() {
						if b.end > m.contentLeft()+m.contentWidth() {
							t.Fatalf("selected-floor control %s was clipped", b.label)
						}
					}
				}
				for _, l := range m.lines {
					for _, z := range l.zones {
						label := strings.TrimSpace(v2ex.SafeText(ansi.Cut(l.normal, z.start, z.end)))
						if label != "回复" && label != "感谢" && label != "回复作者" && label != "感谢作者" {
							t.Fatalf("hit area covered %q (%s, %d:%d) in row %q", label, z.hit.kind, z.start, z.end, v2ex.SafeText(l.normal))
						}
					}
				}
			})
		}
	}
}

func TestReplyParagraphSpacingAndThreadSelection(t *testing.T) {
	m := testTopicModel(readingPage())
	m.resize(160, 30)
	blank := false
	foundParagraph, foundChild := false, false
	for at, l := range m.lines {
		isBlank := strings.TrimSpace(v2ex.SafeText(l.text)) == ""
		if isBlank && blank {
			t.Fatal("HTML margins and reply spacing produced duplicate empty rows")
		}
		blank = isBlank
		if strings.Contains(l.text, "第一段回复") {
			foundParagraph = at+2 < len(m.lines) && strings.Contains(m.lines[at+2].text, "第二段回复")
		}
		if l.heading && l.hit.id == 102 {
			foundChild = true
			if !strings.Contains(l.normal, "└─") || !strings.Contains(l.highlight, "└─") || !strings.Contains(l.highlight, "▸") {
				t.Fatal("conversation selection lost its branch or marker")
			}
			m.session.Current.Offset = at
			z := l.zones[0]
			m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: z.start, Y: 4})
			if m.mode != "reply" || m.editor.Value() != "@bob #2 " {
				t.Fatal("centered conversation reply button missed its user")
			}
		}
	}
	if !foundParagraph || !foundChild {
		t.Fatal("paragraph spacing or conversation child missing")
	}
}

func TestCompactLinksRetainCompleteNativeTarget(t *testing.T) {
	p := readingPage()
	target := "https://example.org/articles/" + strings.Repeat("long-path/", 15) + "reference.png"
	p.Topic.Body = []v2ex.Element{{Text: target, URL: target}, {Text: "\n"}, {Text: "原来的链接标题", URL: target}}
	m := testTopicModel(p)
	view := m.View().Content
	if !strings.Contains(view, ansi.SetHyperlink(target)) || !strings.Contains(view, "原来的链接标题") {
		t.Fatal("link abbreviation lost the destination or descriptive title")
	}
	if strings.Contains(v2ex.SafeText(view), target) || !strings.Contains(view, "example.org/…/reference.png") {
		t.Fatal("raw URL still dominates the reading column")
	}
	if got := compactAge("2026-11-09 13:52:02 +08:00"); got != "11-09 13:52" {
		t.Fatalf("timestamp was not abbreviated: %s", got)
	}
}

func TestViewModeUpdatesCachedActionLabel(t *testing.T) {
	m := testTopicModel(readingPage())
	m.View()
	before := m.actionsText
	m.toggleConversation()
	m.View()
	if before == m.actionsText || !strings.Contains(m.actionsText, "v 对话视图") {
		t.Fatal("mode switch left a stale toolbar label")
	}
}

func TestCenteredNativeImagePlacementAndClick(t *testing.T) {
	m := testTopicModel(readingPage())
	m.resize(160, 30)
	var terminal bytes.Buffer
	m.options.ImageOutput = preview.NewOutput(&terminal, preview.Kitty)
	m.page.Topic.Body = []v2ex.Element{{Image: true, URL: "https://example.org/image.png"}}
	m.images["topic:1:0"] = &imageState{expanded: true, image: &preview.Image{Columns: 8, Rows: []preview.Row{{Native: "IMAGE", ID: 7}}}}
	m.rebuild()
	m.options.ImageOutput.Write([]byte(ansi.SetModeAltScreenSaveCursor + m.View().Content))
	for at, l := range m.lines {
		if l.imageRow == nil {
			continue
		}
		if l.imageX != m.contentLeft()+2 || !strings.Contains(terminal.String(), fmt.Sprintf("\x1b[%d;%dH", at+5, l.imageX+1)) {
			t.Fatal("native image did not follow the centered content column")
		}
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: l.imageX, Y: at + 4})
		if m.images["topic:1:0"].expanded {
			t.Fatal("clicking the centered image did not collapse it")
		}
		return
	}
	t.Fatal("no image row")
}
