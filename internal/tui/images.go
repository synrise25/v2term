package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"v2term/internal/preview"
	"v2term/internal/state"
)

type imageState struct {
	expanded, loading bool
	image             *preview.Image
	err               string
	cancel            context.CancelFunc
	generation        int
	at                time.Time
}
type imageMsg struct {
	key                 string
	generation, request int
	columns, rows       int
	image               *preview.Image
	err                 error
}

func (m *Model) imageProtocol() preview.Protocol {
	if m.options.ImageOutput != nil {
		return m.options.ImageOutput.Protocol()
	}
	return preview.Blocks
}

func (m *Model) toggleImage(h hit) tea.Cmd {
	state := m.images[h.path]
	if state == nil {
		if len(m.images) >= 8 {
			oldest := ""
			for key, other := range m.images {
				if !other.expanded && (oldest == "" || other.at.Before(m.images[oldest].at)) {
					oldest = key
				}
			}
			if oldest == "" {
				m.status = "请先收起一张图片再展开新图片"
				return nil
			}
			delete(m.images, oldest)
		}
		state = &imageState{}
		m.images[h.path] = state
	}
	state.at = time.Now()
	if state.expanded && state.err == "" {
		state.expanded = false
		state.generation++
		if state.cancel != nil {
			state.cancel()
		}
		state.loading = false
		for at, l := range m.lines {
			if l.hit.kind == "image" && l.hit.path == h.path {
				m.session.Current.Offset = min(m.session.Current.Offset, at)
				break
			}
		}
		m.rebuild()
		return nil
	}
	state.expanded = true
	state.err = ""
	if state.image != nil {
		m.rebuild()
		return nil
	}
	state.loading = true
	state.generation++
	ctx, cancel := context.WithCancel(m.ctx)
	state.cancel = cancel
	key, target, generation, request := h.path, h.target, state.generation, m.request
	columns, rows := m.imageWidth(), max(1, m.bodyHeight()-3)
	loader, protocol := m.imageLoader, m.imageProtocol()
	m.rebuild()
	return func() tea.Msg {
		im, err := loader.Load(ctx, target, key, columns, rows, protocol)
		return imageMsg{key: key, generation: generation, request: request, image: im, err: err, columns: columns, rows: rows}
	}
}

func (m *Model) clearImages() {
	for _, state := range m.images {
		if state.cancel != nil {
			state.cancel()
		}
	}
	clear(m.images)
}

// Images start folded on the next page load; persist the corresponding text row.
func (m *Model) savedLocation() state.Location {
	loc := m.session.Current
	for i := 0; i < min(m.session.Current.Offset, len(m.lines)); i++ {
		if m.lines[i].imageRow != nil {
			loc.Offset--
		}
	}
	loc.Offset = max(0, loc.Offset)
	return loc
}

func (m *Model) resizeImages() tea.Cmd {
	var commands []tea.Cmd
	for key, state := range m.images {
		if state.image == nil || !state.expanded {
			continue
		}
		state.generation++
		generation, request := state.generation, m.request
		image := state.image
		columns, rows, protocol := m.imageWidth(), max(1, m.bodyHeight()-3), m.imageProtocol()
		commands = append(commands, func() tea.Msg {
			im, err := image.Resize(columns, rows, protocol)
			return imageMsg{key: key, generation: generation, request: request, image: im, err: err, columns: columns, rows: rows}
		})
	}
	return tea.Batch(commands...)
}
