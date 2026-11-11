package tui

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"v2term/internal/v2ex"
)

// Keep the renderer alive between frames to exercise both initial output and
// cursor-based updates, as Bubble Tea does when keyboard focus moves.
func wireRenderer(t *testing.T, m *Model) func() string {
	t.Helper()
	var wire bytes.Buffer
	renderer := uv.NewTerminalRenderer(&wire, []string{"TERM=xterm-256color", "COLORTERM=truecolor"})
	renderer.SetColorProfile(colorprofile.TrueColor)
	renderer.SetFullscreen(true)
	screen := uv.NewScreenBuffer(m.width, m.height)
	return func() string {
		t.Helper()
		wire.Reset()
		uv.NewStyledString(m.View().Content).Draw(screen, screen.Bounds())
		renderer.Render(screen.RenderBuffer)
		if err := renderer.Flush(); err != nil {
			t.Fatal(err)
		}
		return wire.String()
	}
}

func backgroundRuns(wire, sample string) []string {
	style := uv.NewStyledString(sample).Lines(ansi.GraphemeWidth)[0][0].Style
	if style.Bg == nil {
		return nil
	}
	wantR, wantG, wantB, _ := style.Bg.RGBA()
	var runs []string
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			runs = append(runs, run.String())
			run.Reset()
		}
	}
	for _, line := range uv.NewStyledString(wire).Lines(ansi.GraphemeWidth) {
		for _, cell := range line {
			// Wide glyphs have empty continuation cells, not a style boundary.
			if cell.Width == 0 {
				continue
			}
			if cell.Style.Bg != nil {
				r, g, b, _ := cell.Style.Bg.RGBA()
				if r == wantR && g == wantG && b == wantB {
					run.WriteString(cell.Content)
					continue
				}
			}
			flush()
		}
		flush()
	}
	return runs
}

func TestCategoryHighlightWireIncludesPaddingForEveryTab(t *testing.T) {
	for _, width := range []int{24, 37, 80, 160} {
		m := navigationModel()
		m.resize(width, 18)
		render := wireRenderer(t, m)
		// The current category also has a background without keyboard focus.
		wire := render()
		for _, b := range m.navigation() {
			if b.active && !slices.Contains(backgroundRuns(wire, activeStyle.Render(" ")), " "+b.label+" ") {
				t.Fatalf("width %d: active category %q lost padding", width, b.label)
			}
		}
		m.Update(key(tea.KeyUp, ""))
		for _, delta := range []int{0, -1, -1, -1, 1, 1, 1} {
			m.moveHorizontal(delta)
			wire = render()
			rows := strings.Split(m.View().Content, "\n")
			for _, b := range m.navigation() {
				if got, want := v2ex.SafeText(ansi.Cut(rows[1], b.start, b.end)), " "+b.label+" "; got != want {
					t.Fatalf("width %d: tab hit area covers %q, want %q", width, got, want)
				}
				if b.focused && !slices.Contains(backgroundRuns(wire, focusedStyle.Render(" ")), " "+b.label+" ") {
					t.Fatalf("width %d: focused category %q lost padding", width, b.label)
				}
			}
		}
	}
}

func TestReplyBadgeHighlightWireIncludesBothEdges(t *testing.T) {
	for _, width := range []int{24, 80, 160} {
		p := listPage("/?tab=tech")
		p.Topics = p.Topics[:3]
		p.Topics[0].Replies, p.Topics[1].Replies, p.Topics[2].Replies = 0, 12, 12345
		m := testTopicModel(p)
		m.resize(width, 30)
		render := wireRenderer(t, m)
		wire := render()
		for _, s := range m.selections {
			badge := m.lines[s.at].right
			if !slices.Contains(backgroundRuns(wire, countStyle.Render(" ")), badge) {
				t.Fatalf("width %d: badge %q lost padding", width, badge)
			}
		}
		for range 2 {
			m.moveSelection(1)
			wire = render()
			badge := m.lines[m.selections[m.selectionIndex()].at].right
			if !slices.Contains(backgroundRuns(wire, activeStyle.Render(" ")), badge) {
				t.Fatalf("width %d: selected badge %q lost padding", width, badge)
			}
		}
	}
}

// Exercise the same cell renderer as Bubble Tea. ANSI text equality alone
// cannot check where a focused wide-character run ends on terminal output.
func TestFocusedActionWireEndsAfterPaddingNotInsideWideLabel(t *testing.T) {
	for _, width := range []int{24, 80, 160} {
		for _, selected := range []int{0, 101} {
			m := testTopicModel(readingPage())
			m.resize(width, 18)
			m.session.Current.Selected = selected
			m.revealSelection(m.selectionIndex())
			render := wireRenderer(t, m)
			render()
			for _, delta := range []int{1, 1, -1} {
				m.moveHorizontal(delta)
				wire := render()
				row := m.lines[m.selections[m.selectionIndex()].actions]
				z := row.zones[m.focus-focusReply]
				label := strings.TrimSpace(v2ex.SafeText(ansi.Cut(row.normal, z.start, z.end)))
				if runs := backgroundRuns(wire, focusedStyle.Render(" ")); !slices.Contains(runs, " "+label+" ") {
					t.Fatalf("width %d, selection %d: focused terminal runs %q need padding around %q", width, selected, runs, label)
				}
			}
		}
	}
}

func TestRootNestedAndHotReplyHeadingsShareSelectionColor(t *testing.T) {
	want := uv.NewStyledString(selectedStyle.Render("X")).Lines(ansi.GraphemeWidth)[0][0].Style
	for _, width := range []int{24, 80, 160} {
		p := readingPage()
		p.Replies[1].Thanks = 10
		m := testTopicModel(p)
		m.resize(width, 18)
		root, nested, hot := false, false, false
		for _, s := range m.selections {
			if s.hit.kind != "reply" {
				continue
			}
			l := m.lines[s.at]
			hot = hot || s.hit.hot
			nested = nested || l.indent > 0
			root = root || !s.hit.hot && l.indent == 0
			cells := uv.NewStyledString(l.highlight).Lines(ansi.GraphemeWidth)[0]
			for _, cell := range cells[m.contentLeft()+l.indent:] {
				if cell.Width == 0 || strings.TrimSpace(cell.Content) == "" {
					continue
				}
				if cell.Style.Fg != want.Fg || cell.Style.Attrs != want.Attrs {
					t.Fatalf("width %d, floor %d, indent %d, hot %t: %q has style %+v, want %+v", width, s.hit.id, l.indent, s.hit.hot, cell.Content, cell.Style, want)
				}
			}
		}
		if !root || !nested || !hot {
			t.Fatal("fixture did not exercise all reply placements")
		}
	}
}
