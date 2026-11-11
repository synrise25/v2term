package preview

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/x/ansi"
)

func Detect(getenv func(string) string) Protocol {
	term, program := strings.ToLower(getenv("TERM")), strings.ToLower(getenv("TERM_PROGRAM"))
	if getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty") || strings.Contains(program, "ghostty") {
		return Kitty
	}
	switch program {
	case "warpterminal":
		return Kitty
	case "iterm.app", "wezterm", "vscode", "tabby", "bobcat":
		return ITerm2
	}
	return Blocks
}

type Placement struct {
	X, Y, Columns int
	Row           *Row
}
type Frame struct{ Rows []Placement }

var frameMarker = regexp.MustCompile("\\x1b\\]8;[^;]*;v2term-frame:([0-9]+)(?:\\x07|\\x1b\\\\)")

// Output composites trusted image tiles after the terminal renderer's text diff.
// Keeping tiles to one terminal row makes cropping and scroll clipping exact.
type Output struct {
	w        io.Writer
	fd       uintptr
	protocol Protocol
	framesMu sync.Mutex
	frames   map[uint64]*Frame
	serial   uint64
	used     atomic.Bool
	frame    *Frame
	mu       sync.Mutex
	shown    []Placement
	active   bool
	uploaded map[int]bool
}

func NewOutput(w io.Writer, protocol Protocol) *Output {
	if protocol == Auto {
		protocol = Detect(os.Getenv)
	}
	o := &Output{w: w, fd: ^uintptr(0), protocol: protocol, frames: map[uint64]*Frame{}, uploaded: map[int]bool{}}
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		o.fd = f.Fd()
	}
	return o
}
func (o *Output) Fd() uintptr        { return o.fd }
func (o *Output) Protocol() Protocol { return o.protocol }
func (o *Output) Read(p []byte) (int, error) {
	if r, ok := o.w.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, io.EOF
}

// The caller owns the wrapped terminal file.
func (o *Output) Close() error { return nil }

// Frame associates images with the exact text frame emitted by Bubble Tea. A
// private hyperlink on an existing header glyph survives the renderer's cell diff;
// Write consumes it, so the terminal never sees this private marker. This avoids
// displaying images from a newer model while an older frame is still flushing.
func (o *Output) Frame(content string, rows []Placement) string {
	if len(rows) > 0 {
		o.used.Store(true)
	}
	if !o.used.Load() {
		return content
	}
	at, size := 0, 0
	var state byte
	for at < len(content) {
		seq, width, n, next := ansi.DecodeSequence(content[at:], state, nil)
		if n == 0 {
			break
		}
		if width > 0 && strings.TrimSpace(seq) != "" {
			size = n
			break
		}
		at, state = at+n, next
	}
	if size == 0 {
		return content
	}
	o.framesMu.Lock()
	defer o.framesMu.Unlock()
	o.serial++
	id := o.serial
	o.frames[id] = &Frame{Rows: rows}
	if id > 64 {
		delete(o.frames, id-64)
	}
	marker := ansi.SetHyperlink(fmt.Sprintf("v2term-frame:%d", id)) + content[at:at+size] + ansi.ResetHyperlink()
	return content[:at] + marker + content[at+size:]
}

func (o *Output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, match := range frameMarker.FindAllSubmatch(p, -1) {
		id, _ := strconv.ParseUint(string(match[1]), 10, 64)
		o.framesMu.Lock()
		frame := o.frames[id]
		o.framesMu.Unlock()
		if frame != nil {
			o.frame = frame
		}
	}
	payload := frameMarker.ReplaceAll(p, nil)
	if bytes.Contains(p, []byte(ansi.SetModeAltScreenSaveCursor)) {
		o.active = true
	}
	exiting := bytes.Contains(p, []byte(ansi.ResetModeAltScreenSaveCursor))
	invalidate := bytes.Contains(p, []byte("\x1b[2J")) || bytes.Contains(p, []byte(ansi.SetModeAltScreenSaveCursor))
	var next []Placement
	if f := o.frame; f != nil && o.active && !exiting {
		next = f.Rows
	}
	if len(next) == 0 && len(o.shown) == 0 {
		n, err := o.w.Write(payload)
		if exiting {
			o.active = false
		}
		if err == nil {
			return len(p), nil
		}
		return n, err
	}
	same := func(a, b Placement) bool { return a.X == b.X && a.Y == b.Y && a.Columns == b.Columns && a.Row == b.Row }
	has := func(rows []Placement, p Placement) bool {
		for _, other := range rows {
			if same(p, other) {
				return true
			}
		}
		return false
	}
	var buf strings.Builder
	if invalidate {
		clear(o.uploaded)
	}
	buf.WriteString(ansi.SetModeSynchronizedOutput)
	buf.WriteString(ansi.SaveCursor + "\x1b[0m")
	for _, old := range o.shown {
		if !has(next, old) || invalidate {
			if o.protocol == Kitty {
				remains := false
				for _, p := range next {
					if p.Row == old.Row {
						remains = true
						break
					}
				}
				deletion := 'I'
				if remains && !invalidate {
					deletion = 'i'
				} else {
					delete(o.uploaded, old.Row.ID)
				}
				fmt.Fprintf(&buf, "\x1b_Ga=d,d=%c,i=%d,q=2;\x1b\\", deletion, old.Row.ID)
			}
			fmt.Fprintf(&buf, "\x1b[%d;%dH%s", old.Y+1, old.X+1, strings.Repeat(" ", old.Columns))
		}
	}
	buf.WriteString(ansi.RestoreCursor)
	text := strings.ReplaceAll(string(payload), ansi.SetModeSynchronizedOutput, "")
	text = strings.ReplaceAll(text, ansi.ResetModeSynchronizedOutput, "")
	buf.WriteString(text)
	buf.WriteString(ansi.SaveCursor + "\x1b[0m")
	for _, at := range next {
		if !has(o.shown, at) || invalidate {
			fmt.Fprintf(&buf, "\x1b[%d;%dH", at.Y+1, at.X+1)
			if o.protocol == Kitty && o.uploaded[at.Row.ID] {
				buf.WriteString(kittyPlacement(at.Row.ID, at.Columns))
			} else {
				buf.WriteString(at.Row.Native)
				if o.protocol == Kitty {
					o.uploaded[at.Row.ID] = true
				}
			}
		}
	}
	buf.WriteString(ansi.RestoreCursor + ansi.ResetModeSynchronizedOutput)
	if _, err := io.WriteString(o.w, buf.String()); err != nil {
		return 0, err
	}
	o.shown = append(o.shown[:0], next...)
	if exiting {
		o.active = false
	}
	return len(p), nil
}
