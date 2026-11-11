package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"v2term/internal/storage"
	"v2term/internal/v2ex"
)

const historyLimit = 50

type Location struct {
	Path     string `json:"path"`
	Offset   int    `json:"offset,omitempty"`
	Selected int    `json:"selected,omitempty"`
	Hot      bool   `json:"hot,omitempty"`
	Filter   string `json:"filter,omitempty"`
}

type Session struct {
	Version      int               `json:"version"`
	Current      Location          `json:"current"`
	Back         []Location        `json:"back,omitempty"`
	Forward      []Location        `json:"forward,omitempty"`
	Drafts       map[string]string `json:"drafts,omitempty"`
	Conversation bool              `json:"conversation"`
}

func Default() Session {
	return Session{Version: 1, Current: Location{Path: "/?tab=hot"}, Drafts: map[string]string{}, Conversation: true}
}

func Load(path string) (Session, error) {
	s := Default()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil {
		return s, err
	}
	if len(data) > 4<<20 {
		return s, errors.New("状态文件过大")
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("状态文件损坏: %w（可用 --fresh 启动）", err)
	}
	if s.Version != 1 {
		return s, fmt.Errorf("不支持的状态版本 %d", s.Version)
	}
	if err := validate(&s); err != nil {
		return s, err
	}
	return s, nil
}

func validate(s *Session) error {
	locations := []*Location{&s.Current}
	for i := range s.Back {
		locations = append(locations, &s.Back[i])
	}
	for i := range s.Forward {
		locations = append(locations, &s.Forward[i])
	}
	for _, loc := range locations {
		path, err := v2ex.Route(loc.Path)
		if err != nil {
			return fmt.Errorf("状态包含无效页面: %w", err)
		}
		loc.Path = path
		loc.Offset, loc.Selected = max(0, loc.Offset), max(0, loc.Selected)
		if loc.Selected == 0 || !strings.HasPrefix(path, "/t/") {
			loc.Hot = false
		}
		loc.Filter = v2ex.SafeText(loc.Filter)
	}
	if s.Drafts == nil {
		s.Drafts = map[string]string{}
	}
	s.Back, s.Forward = Trim(s.Back), Trim(s.Forward)
	return nil
}

func Trim(history []Location) []Location {
	if len(history) > historyLimit {
		return history[len(history)-historyLimit:]
	}
	return history
}

// Save uses a same-directory rename so an interrupted write cannot truncate progress.
func Save(path string, s Session) error {
	if err := validate(&s); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return errors.New("草稿和历史记录过大，无法保存")
	}
	return storage.WriteFile(path, data)
}
