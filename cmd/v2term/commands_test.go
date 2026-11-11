package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"v2term/internal/config"
)

func TestCommandConfigurationAndCookieLifecycle(t *testing.T) {
	t.Setenv("V2TERM_COOKIE", "")
	path := filepath.Join(t.TempDir(), "v2term", "config.json")
	runCommand := func(args []string, stdin string) string {
		t.Helper()
		var out bytes.Buffer
		if err := run(append([]string{"--config", path}, args...), strings.NewReader(stdin), &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	runCommand([]string{"config", "set", "cache_ttl", "5m"}, "")
	runCommand([]string{"config", "set", "scroll_lines", "1"}, "")
	runCommand([]string{"config", "set", "hot_replies", "5"}, "")
	runCommand([]string{"config", "set", "hot_min_thanks", "10"}, "")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.CacheTTL != "5m" || c.ScrollLines != 1 || c.HotReplies != 5 || c.HotMinThanks != 10 {
		t.Fatalf("config not persisted: %+v", c)
	}
	secret := "session=synthetic-secret"
	result := runCommand([]string{"login", "--cookie-file", "-"}, secret)
	if strings.Contains(result, secret) {
		t.Fatal("Cookie echoed")
	}
	data, err := os.ReadFile(c.CookieFile)
	if err != nil || string(data) != secret+"\n" {
		t.Fatal("Cookie not imported")
	}
	info, _ := os.Stat(c.CookieFile)
	if info.Mode().Perm() != 0600 {
		t.Fatal("Cookie is not private")
	}
	result = runCommand([]string{"config", "show"}, "")
	if strings.Contains(result, secret) {
		t.Fatal("config show leaked Cookie")
	}
	var view map[string]any
	if err := json.Unmarshal([]byte(result), &view); err != nil {
		t.Fatal(err)
	}
	if view["cookie_saved"] != true || view["cache_storage"] != "memory (退出即清空)" {
		t.Fatalf("wrong status: %s", result)
	}
	runCommand([]string{"logout"}, "")
	if _, err := os.Stat(c.CookieFile); !os.IsNotExist(err) {
		t.Fatal("Cookie not removed")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("logout removed config")
	}
}

func TestRuntimeOverridesDoNotModifyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.CacheSize = 99
	c.CacheTTL = "5m"
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	o, _, err := parseOptions([]string{"--config", path, "--cache-size", "2", "--cache-ttl", "0s", "--scroll-lines", "1", "--hot-replies", "0", "--hot-min-thanks", "0"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got, err := o.config()
	if err != nil {
		t.Fatal(err)
	}
	if got.CacheSize != 2 || got.CacheTTL != "0s" || got.ScrollLines != 1 || got.HotReplies != 0 || got.HotMinThanks != 0 {
		t.Fatalf("overrides: %+v", got)
	}
	stored, err := config.Load(path)
	if err != nil || stored != c {
		t.Fatal("runtime flags changed persistent config")
	}
}

func TestHotReplyCLIOverridesAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, args := range [][]string{
		{"--hot-replies", "5", "--hot-min-thanks", "10", "config", "show"},
		{"--hot-replies", "-1", "config", "show"},
		{"--hot-replies", "101", "config", "show"},
		{"--hot-min-thanks", "-1", "config", "show"},
	} {
		var out bytes.Buffer
		err := run(append([]string{"--config", path}, args...), strings.NewReader(""), &out)
		if args[1] == "5" {
			var c config.Config
			if err != nil || json.Unmarshal(out.Bytes(), &c) != nil || c.HotReplies != 5 || c.HotMinThanks != 10 {
				t.Fatalf("runtime hot reply options not applied: %s, %v", out.String(), err)
			}
		} else if err == nil {
			t.Fatalf("accepted invalid runtime options: %v", args)
		}
	}
}

func TestCookieSourcePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.CookieFile, []byte("session=saved"), 0600); err != nil {
		t.Fatal(err)
	}
	o := options{}
	t.Setenv("V2TERM_COOKIE", "")
	got, managed, err := o.cookie(c)
	if err != nil || got != "session=saved" || !managed {
		t.Fatal("saved Cookie not loaded")
	}
	t.Setenv("V2TERM_COOKIE", "session=environment")
	got, managed, err = o.cookie(c)
	if err != nil || got != "session=environment" || managed {
		t.Fatal("environment precedence is wrong")
	}
	o.cookieFile = filepath.Join(t.TempDir(), "override.txt")
	if err := os.WriteFile(o.cookieFile, []byte("session=override"), 0600); err != nil {
		t.Fatal(err)
	}
	got, managed, err = o.cookie(c)
	if err != nil || got != "session=override" || managed {
		t.Fatal("file override precedence is wrong")
	}
}

func TestInvalidLoginDoesNotReplaceExistingCookie(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.CookieFile, []byte("session=previous"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"", "not-a-cookie", "session=value\nother=value", strings.Repeat("x", 65537)} {
		if err := run([]string{"--config", path, "login", "--cookie-file", "-"}, strings.NewReader(input), io.Discard); err == nil {
			t.Fatal("invalid Cookie accepted")
		}
		data, _ := os.ReadFile(c.CookieFile)
		if string(data) != "session=previous" {
			t.Fatal("invalid import replaced existing Cookie")
		}
	}
}

func TestHelpUsesDoubleDash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, args := range [][]string{{"--help"}, {"login", "--help"}} {
		var output bytes.Buffer
		if err := run(append([]string{"--config", path}, args...), strings.NewReader(""), &output); err != nil {
			t.Fatal(err)
		}
		help := output.String()
		if !strings.Contains(help, "  --cookie-file string") || regexp.MustCompile(`(?m)^\s+-[^-\s]`).MatchString(help) {
			t.Fatalf("help does not consistently use --: %s", help)
		}
	}
}
