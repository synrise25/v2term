package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestConfigDefaultsAndRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.CacheSize != 32 || c.TTL() != time.Minute || c.ScrollLines != 1 || c.HotReplies != 3 || c.HotMinThanks != 3 || c.CookieFile != filepath.Join(filepath.Dir(path), "cookies.txt") {
		t.Fatalf("defaults: %+v", c)
	}
	for key, value := range map[string]string{"cache_ttl": "5m", "cache_size": "64", "scroll_lines": "2", "state_dir": t.TempDir(), "hot_replies": "5", "hot_min_thanks": "10"} {
		if err := c.Set(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, c) {
		t.Fatalf("round trip: %+v", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("config file is not private")
	}
}

func TestConfigRejectsInvalidValuesWithoutMutation(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := c
	for key, value := range map[string]string{"cache_ttl": "-1m", "cache_size": "1001", "scroll_lines": "0", "hot_replies": "101", "hot_min_thanks": "-1", "unknown": "value"} {
		if err := c.Set(key, value); err == nil {
			t.Errorf("accepted %s=%s", key, value)
		}
		if c != before {
			t.Fatal("invalid config mutated settings")
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	for _, body := range []string{`{"cache_ttl":"oops"}`, `{"scroll_lines":0}`, `{"hot_replies":-1}`, `{"hot_min_thanks":-1}`, `{"cookie":"secret"}`, `{} {}`, `broken`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("accepted invalid config %q", body)
		}
	}
}

func TestHotReplyDefaultsForExistingConfigAndExplicitZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"scroll_lines":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || c.HotReplies != 3 || c.HotMinThanks != 3 {
		t.Fatalf("existing config did not inherit hot reply defaults: %+v, %v", c, err)
	}
	for _, key := range []string{"hot_replies", "hot_min_thanks"} {
		if err := c.Set(key, "0"); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.HotReplies != 0 || got.HotMinThanks != 0 {
		t.Fatalf("explicit zeros were replaced by defaults: %+v, %v", got, err)
	}
}
