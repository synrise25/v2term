package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2term", "state.json")
	s := Default()
	s.Current = Location{Path: "/t/42?p=2", Offset: 72, Selected: 901, Hot: true}
	s.Back = []Location{{Path: "/go/go", Offset: 12, Selected: 42}}
	s.Forward = []Location{{Path: "/?tab=hot"}}
	s.Drafts["/t/42"] = "回复 q 中文"
	s.Conversation = false
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("round trip: %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions: %v", info.Mode())
	}
	if err := Save(path, s); err != nil {
		t.Fatal("atomic replacement:", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary files left behind")
	}
}

func TestDefaultAndCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(path)
	if err != nil || !s.Conversation {
		t.Fatal("conversation should be default")
	}
	if s.Current.Path != "/?tab=hot" {
		t.Fatalf("missing state should open hot topics, got %q", s.Current.Path)
	}
	for _, body := range []string{"broken", `{"version":9}`, `{"version":1,"current":{"path":"/signout"}}`, strings.Repeat("x", (4<<20)+1)} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("invalid state accepted")
		}
		data, _ := os.ReadFile(path)
		if string(data) != body {
			t.Fatal("corrupt state overwritten")
		}
	}
}

func TestSavedPageOverridesDefault(t *testing.T) {
	for _, route := range []string{"/?tab=hot", "/recent", "/?tab=qna", "/?tab=tech", "/t/42?p=2"} {
		t.Run(route, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			s := Default()
			s.Current = Location{Path: route, Offset: 12, Selected: 7}
			s.Drafts["/t/42"] = "未发送的草稿"
			s.Conversation = false
			if err := Save(path, s); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, s) {
				t.Fatalf("saved reading state replaced by defaults: %+v", got)
			}
		})
	}
}
