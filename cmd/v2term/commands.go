package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"v2term/internal/config"
	"v2term/internal/storage"
	"v2term/internal/v2ex"
)

func command(o options, c config.Config, args []string, input io.Reader, output io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "config":
		if len(args) == 1 || len(args) == 2 && args[1] == "show" {
			_, err := os.Stat(c.CookieFile)
			view := struct {
				config.Config
				ConfigFile   string `json:"config_file"`
				StateFile    string `json:"state_file"`
				CookieSaved  bool   `json:"cookie_saved"`
				CookieSource string `json:"cookie_source"`
				CacheStorage string `json:"cache_storage"`
			}{Config: c, ConfigFile: o.configPath, StateFile: filepath.Join(c.StateDir, "state.json"), CookieSaved: err == nil, CacheStorage: "memory (退出即清空)"}
			view.CookieSource = c.CookieFile
			if os.Getenv("V2TERM_COOKIE") != "" {
				view.CookieSource = "V2TERM_COOKIE"
			}
			if o.cookieFile != "" {
				view.CookieSource = o.cookieFile
			}
			encoder := json.NewEncoder(output)
			encoder.SetIndent("", "  ")
			return true, encoder.Encode(view)
		}
		if len(args) == 4 && args[1] == "set" {
			if err := c.Set(args[2], args[3]); err != nil {
				return true, err
			}
			if err := config.Save(o.configPath, c); err != nil {
				return true, err
			}
			fmt.Fprintln(output, "配置已保存:", o.configPath)
			return true, nil
		}
		return true, errors.New("用法: config show 或 config set 配置项 值")
	case "login":
		fs := flag.NewFlagSet("login", flag.ContinueOnError)
		fs.SetOutput(output)
		source := fs.String("cookie-file", o.cookieFile, "Cookie 文件，- 表示标准输入")
		fs.Usage = func() {
			fmt.Fprintln(output, "用法: v2term login --cookie-file 文件（- 表示标准输入）")
			printFlagDefaults(fs, output)
		}
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return true, nil
			}
			return true, err
		}
		if fs.NArg() != 0 || *source == "" {
			return true, errors.New("用法: login --cookie-file 文件（- 表示标准输入）")
		}
		var cookie string
		var err error
		if *source == "-" {
			cookie, err = readCookieFrom(input)
		} else {
			cookie, err = readCookie(*source)
		}
		if err != nil {
			return true, err
		}
		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			return true, errors.New("Cookie 不能为空")
		}
		if _, err := v2ex.New(cookie); err != nil {
			return true, err
		}
		if err := storage.WriteFile(c.CookieFile, []byte(cookie+"\n")); err != nil {
			return true, err
		}
		fmt.Fprintln(output, "Cookie 已保存:", c.CookieFile, "（有效性将在访问 V2EX 时检查）")
		return true, nil
	case "logout":
		if len(args) != 1 {
			return true, errors.New("用法: logout")
		}
		if err := os.Remove(c.CookieFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, err
		}
		fmt.Fprintln(output, "已清除保存的 Cookie:", c.CookieFile)
		return true, nil
	}
	return false, nil
}
