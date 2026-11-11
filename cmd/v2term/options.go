package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"v2term/internal/config"
)

type options struct {
	configPath, cookieFile, stateDir, imageProtocol string
	cacheTTL                                        time.Duration
	cacheSize, scrollLines                          int
	hotReplies, hotMinThanks                        int
	fresh, version, help, noCache                   bool
	changed                                         map[string]bool
}

func parseOptions(args []string, output io.Writer) (options, []string, error) {
	o := options{changed: map[string]bool{}}
	path, err := config.DefaultPath()
	if err != nil {
		return o, nil, err
	}
	fs := flag.NewFlagSet("v2term", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&o.configPath, "config", path, "配置文件路径")
	fs.StringVar(&o.cookieFile, "cookie-file", "", "本次运行的 Cookie 文件；login 时为导入源")
	fs.StringVar(&o.stateDir, "state-dir", "", "本次运行的阅读进度和草稿目录")
	fs.DurationVar(&o.cacheTTL, "cache-ttl", time.Minute, "内存页面缓存有效期，例如 5m")
	fs.IntVar(&o.cacheSize, "cache-size", 32, "内存缓存最多页数，0 关闭")
	fs.BoolVar(&o.noCache, "no-cache", false, "本次运行关闭页面缓存")
	fs.IntVar(&o.scrollLines, "scroll-lines", 1, "每个滚轮事件即时移动的显示行数")
	fs.StringVar(&o.imageProtocol, "image-protocol", "auto", "图片预览: auto、iterm2、kitty 或 blocks")
	fs.IntVar(&o.hotReplies, "hot-replies", 3, "正文下热评最多条数，0 关闭")
	fs.IntVar(&o.hotMinThanks, "hot-min-thanks", 3, "热评最低感谢数，包含此值；仅筛选当前页")
	fs.BoolVar(&o.fresh, "fresh", false, "忽略阅读进度，从最热开始")
	fs.BoolVar(&o.version, "version", false, "显示版本")
	fs.Usage = func() {
		fmt.Fprint(output, "用法: v2term [选项] [节点名 | 帖子编号 | V2EX 地址]\n       v2term [--config 路径] config show\n       v2term [--config 路径] config set 配置项 值\n       v2term [--config 路径] login --cookie-file 文件（- 表示标准输入）\n       v2term [--config 路径] logout\n\n无参数恢复上次页面；q 保存退出。选项放在节点或帖子地址之前。\n\n")
		printFlagDefaults(fs, output)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			o.help = true
			return o, nil, nil
		}
		return o, nil, err
	}
	fs.Visit(func(f *flag.Flag) { o.changed[f.Name] = true })
	o.configPath, err = filepath.Abs(o.configPath)
	return o, fs.Args(), err
}

func printFlagDefaults(fs *flag.FlagSet, output io.Writer) {
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		argument := ""
		if name != "" {
			argument = " " + name
		}
		fmt.Fprintf(output, "  --%s%s\n      %s", f.Name, argument, usage)
		if f.DefValue != "" {
			value := f.DefValue
			if name == "string" {
				value = strconv.Quote(value)
			}
			fmt.Fprintf(output, "（默认: %s）", value)
		}
		fmt.Fprintln(output)
	})
}

func (o options) config() (config.Config, error) {
	c, err := config.Load(o.configPath)
	if err != nil {
		return c, err
	}
	values := map[string]string{"state-dir": o.stateDir, "cache-ttl": o.cacheTTL.String(), "cache-size": fmt.Sprint(o.cacheSize), "scroll-lines": fmt.Sprint(o.scrollLines), "image-protocol": o.imageProtocol, "hot-replies": fmt.Sprint(o.hotReplies), "hot-min-thanks": fmt.Sprint(o.hotMinThanks)}
	keys := map[string]string{"state-dir": "state_dir", "cache-ttl": "cache_ttl", "cache-size": "cache_size", "scroll-lines": "scroll_lines", "image-protocol": "image_protocol", "hot-replies": "hot_replies", "hot-min-thanks": "hot_min_thanks"}
	for flagName, key := range keys {
		if o.changed[flagName] {
			if err := c.Set(key, values[flagName]); err != nil {
				return c, err
			}
		}
	}
	if o.noCache {
		c.CacheSize = 0
	}
	return c, nil
}

func readCookie(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readCookieFrom(f)
}

func readCookieFrom(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil {
		return "", err
	}
	if len(data) > 65536 {
		return "", errors.New("Cookie 过长")
	}
	return string(data), nil
}

func (o options) cookie(c config.Config) (string, bool, error) {
	if o.cookieFile != "" {
		value, err := readCookie(o.cookieFile)
		return value, false, err
	}
	if value := os.Getenv("V2TERM_COOKIE"); value != "" {
		return value, false, nil
	}
	value, err := readCookie(c.CookieFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	return value, err == nil, err
}
