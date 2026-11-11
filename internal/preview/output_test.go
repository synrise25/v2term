package preview

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func TestOutputUsesMatchingTextFrameAndClearsHiddenImages(t *testing.T) {
	var buf bytes.Buffer
	o := NewOutput(&buf, ITerm2)
	a, b := &Row{Native: "IMAGE_A"}, &Row{Native: "IMAGE_B"}
	old := o.Frame(" old frame", []Placement{{X: 2, Y: 4, Columns: 6, Row: a}})
	newer := o.Frame(" newer frame", []Placement{{X: 2, Y: 5, Columns: 6, Row: b}})
	if _, err := o.Write([]byte(ansi.SetModeAltScreenSaveCursor + old)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "IMAGE_A") || strings.Contains(buf.String(), "IMAGE_B") || strings.Contains(buf.String(), "v2term-frame") {
		t.Fatal("newer frame images leaked into older text")
	}
	buf.Reset()
	o.Write([]byte(newer))
	if !strings.Contains(buf.String(), "IMAGE_B") || !strings.Contains(buf.String(), "      ") {
		t.Fatal("previous image not erased before new placement")
	}
	buf.Reset()
	o.Write([]byte(o.Frame(" hidden", nil)))
	if strings.Contains(buf.String(), "IMAGE_") || !strings.Contains(buf.String(), "      ") {
		t.Fatal("hidden image was redrawn or not cleared")
	}
}

func TestOutputReusesUnchangedTilesAndRestoresAfterErase(t *testing.T) {
	var buf bytes.Buffer
	o := NewOutput(&buf, ITerm2)
	row := &Row{Native: "IMAGE", ID: 11}
	placements := []Placement{{X: 3, Y: 4, Columns: 5, Row: row}}
	o.Write([]byte(ansi.SetModeAltScreenSaveCursor + o.Frame(" first", placements)))
	buf.Reset()
	o.Write([]byte(o.Frame(" metadata changed", placements)))
	if strings.Contains(buf.String(), "IMAGE") {
		t.Fatal("unchanged image retransmitted")
	}
	buf.Reset()
	o.Write([]byte("\x1b[2J" + o.Frame(" redraw", placements)))
	if !strings.Contains(buf.String(), "IMAGE") {
		t.Fatal("image not restored after screen erase")
	}
	buf.Reset()
	o.Write([]byte(ansi.ResetModeAltScreenSaveCursor))
	if strings.Contains(buf.String(), "IMAGE") {
		t.Fatal("image redrawn after program exit")
	}
}

func TestKittyResourcesRemovedOnCollapse(t *testing.T) {
	var buf bytes.Buffer
	o := NewOutput(&buf, Kitty)
	row := &Row{Native: "IMAGE", ID: 11}
	o.Write([]byte(ansi.SetModeAltScreenSaveCursor + o.Frame(" image", []Placement{{Columns: 5, Row: row}})))
	buf.Reset()
	o.Write([]byte(o.Frame(" collapsed", nil)))
	if !strings.Contains(buf.String(), "a=d,d=I,i=11") {
		t.Fatal("Kitty image resources were left behind")
	}
}

func TestProtocolDetection(t *testing.T) {
	for _, tc := range []struct {
		program, term string
		want          Protocol
	}{{"WarpTerminal", "xterm-256color", Kitty}, {"Apple_Terminal", "xterm-256color", Blocks}, {"WezTerm", "xterm-256color", ITerm2}, {"ghostty", "xterm-ghostty", Kitty}, {"", "xterm-kitty", Kitty}, {"", "xterm-256color", Blocks}} {
		got := Detect(func(key string) string {
			switch key {
			case "TERM_PROGRAM":
				return tc.program
			case "TERM":
				return tc.term
			}
			return ""
		})
		if got != tc.want {
			t.Fatalf("%s/%s: %s", tc.program, tc.term, got)
		}
	}
}

func TestKittyScrollingMovesStoredTilesWithoutRetransmitting(t *testing.T) {
	var buf bytes.Buffer
	o := NewOutput(&buf, Kitty)
	row := &Row{Native: "IMAGE_PAYLOAD", ID: 17}
	o.Write([]byte(ansi.SetModeAltScreenSaveCursor + o.Frame(" first", []Placement{{X: 2, Y: 8, Columns: 5, Row: row}})))
	buf.Reset()
	o.Write([]byte(o.Frame(" scrolled", []Placement{{X: 2, Y: 7, Columns: 5, Row: row}})))
	got := buf.String()
	if strings.Contains(got, "IMAGE_PAYLOAD") || !strings.Contains(got, "a=p,i=17") || !strings.Contains(got, "z=-1") {
		t.Fatal("scroll did not reuse background image")
	}
	if !strings.Contains(got, "a=d,d=i,i=17") || strings.Contains(got, "a=d,d=I,i=17") {
		t.Fatal("scroll discarded uploaded pixels")
	}
}

type observingWriter struct {
	bytes.Buffer
	once sync.Once
	seen chan struct{}
}

func (w *observingWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func (w *observingWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if bytes.Contains(p, []byte("FRAME_IMAGE")) {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

type nativeTestModel struct {
	o   *Output
	row *Row
}

func (m nativeTestModel) Init() tea.Cmd                       { return nil }
func (m nativeTestModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m nativeTestModel) View() tea.View {
	v := tea.NewView(m.o.Frame("  \x1b[1mV2TERM\x1b[0m\n        \nfooter", []Placement{{Y: 1, Columns: 8, Row: m.row}}))
	v.AltScreen = true
	return v
}

func TestNativeFrameSurvivesBubbleTeaCellRenderer(t *testing.T) {
	w := &observingWriter{seen: make(chan struct{})}
	o := NewOutput(w, ITerm2)
	p := tea.NewProgram(nativeTestModel{o: o, row: &Row{Native: "FRAME_IMAGE"}}, tea.WithInput(nil), tea.WithOutput(o), tea.WithWindowSize(40, 10), tea.WithColorProfile(colorprofile.TrueColor))
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	select {
	case <-w.seen:
	case err := <-done:
		t.Fatalf("renderer exited without image: %v", err)
	case <-time.After(3 * time.Second):
		p.Kill()
		<-done
		t.Fatalf("image frame marker did not survive cell rendering; output=%q", w.String())
	}
	p.Send(tea.QuitMsg{})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		p.Kill()
		<-done
		t.Fatal("program did not terminate")
	}
	if strings.Contains(w.String(), "v2term-frame:") {
		t.Fatal("private frame marker leaked to terminal")
	}
}
