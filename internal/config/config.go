package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"v2term/internal/preview"
	"v2term/internal/storage"
)

type Config struct {
	StateDir      string `json:"state_dir"`
	CookieFile    string `json:"cookie_file"`
	CacheTTL      string `json:"cache_ttl"`
	CacheSize     int    `json:"cache_size"`
	ScrollLines   int    `json:"scroll_lines"`
	ImageProtocol string `json:"image_protocol"`
	HotReplies    int    `json:"hot_replies"`
	HotMinThanks  int    `json:"hot_min_thanks"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "v2term", "config.json"), nil
}

func defaults(path string) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	stateDir := filepath.Join(home, ".local", "state")
	if xdg := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(xdg) {
		stateDir = xdg
	}
	return Config{StateDir: filepath.Join(stateDir, "v2term"), CookieFile: filepath.Join(filepath.Dir(path), "cookies.txt"), CacheTTL: "1m", CacheSize: 32, ScrollLines: 1, ImageProtocol: "auto", HotReplies: 3, HotMinThanks: 3}, nil
}

func Load(path string) (Config, error) {
	c, err := defaults(path)
	if err != nil {
		return c, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return c, err
	}
	if len(data) > 65536 {
		return c, errors.New("配置文件过大")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("读取配置失败: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c, errors.New("配置文件包含多余内容")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if !preview.ValidProtocol(c.ImageProtocol) {
		return errors.New("image_protocol 应为 auto、iterm2、kitty 或 blocks")
	}
	if !filepath.IsAbs(c.StateDir) || !filepath.IsAbs(c.CookieFile) {
		return errors.New("state_dir 和 cookie_file 必须为绝对路径")
	}
	ttl, err := time.ParseDuration(c.CacheTTL)
	if err != nil || ttl < 0 || ttl > 24*time.Hour {
		return errors.New("cache_ttl 应为 0s 至 24h 的时间，例如 5m")
	}
	if c.CacheSize < 0 || c.CacheSize > 1000 {
		return errors.New("cache_size 应为 0 至 1000（0 关闭缓存）")
	}
	if c.ScrollLines < 1 || c.ScrollLines > 20 {
		return errors.New("scroll_lines 应为 1 至 20")
	}
	if c.HotReplies < 0 || c.HotReplies > 100 {
		return errors.New("hot_replies 应为 0 至 100（0 关闭热评）")
	}
	if c.HotMinThanks < 0 {
		return errors.New("hot_min_thanks 应为非负整数；热评感谢数须达到此值")
	}
	return nil
}

func (c Config) TTL() time.Duration { ttl, _ := time.ParseDuration(c.CacheTTL); return ttl }

func (c *Config) Set(key, value string) error {
	next := *c
	switch key {
	case "state_dir", "cookie_file":
		path, err := filepath.Abs(value)
		if err != nil || value == "" {
			return errors.New("路径不能为空")
		}
		if key == "state_dir" {
			next.StateDir = path
		} else {
			next.CookieFile = path
		}
	case "cache_ttl":
		next.CacheTTL = value
	case "image_protocol":
		next.ImageProtocol = value
	case "cache_size", "scroll_lines", "hot_replies", "hot_min_thanks":
		n, err := strconv.Atoi(value)
		if err != nil {
			return errors.New("请输入整数")
		}
		switch key {
		case "cache_size":
			next.CacheSize = n
		case "scroll_lines":
			next.ScrollLines = n
		case "hot_replies":
			next.HotReplies = n
		case "hot_min_thanks":
			next.HotMinThanks = n
		}
	default:
		return fmt.Errorf("未知配置项 %q，可配置 state_dir、cookie_file、cache_ttl、cache_size、scroll_lines、image_protocol、hot_replies、hot_min_thanks", key)
	}
	if err := next.Validate(); err != nil {
		return err
	}
	*c = next
	return nil
}

func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(path, append(data, '\n'))
}
