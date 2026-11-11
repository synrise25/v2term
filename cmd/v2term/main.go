package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"v2term/internal/preview"
	"v2term/internal/state"
	"v2term/internal/storage"
	"v2term/internal/tui"
	"v2term/internal/v2ex"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "v2term:", err)
		os.Exit(1)
	}
}

func run(args []string, input io.Reader, output io.Writer) error {
	options, positional, err := parseOptions(args, output)
	if err != nil {
		return err
	}
	if options.help {
		return nil
	}
	if options.version {
		fmt.Fprintln(output, "v2term", version)
		return nil
	}
	cfg, err := options.config()
	if err != nil {
		return err
	}
	if handled, err := command(options, cfg, positional, input, output); handled {
		return err
	}
	if len(positional) > 1 {
		return errors.New("只支持一个节点或帖子地址")
	}
	path := filepath.Join(cfg.StateDir, "state.json")
	session := state.Default()
	if !options.fresh {
		session, err = state.Load(path)
		if err != nil {
			return err
		}
	}
	if len(positional) == 1 {
		route, err := v2ex.Route(positional[0])
		if err != nil {
			return err
		}
		if route != session.Current.Path {
			session.Back = state.Trim(append(session.Back, session.Current))
			session.Forward = nil
			session.Current = state.Location{Path: route}
		}
	}
	cookie, managed, err := options.cookie(cfg)
	if err != nil {
		return err
	}
	client, err := v2ex.New(cookie)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	imageOutput := preview.NewOutput(output, preview.Protocol(cfg.ImageProtocol))
	model := tui.New(ctx, client, session, tui.Options{CacheTTL: cfg.TTL(), CacheSize: cfg.CacheSize, ScrollLines: cfg.ScrollLines, ImageOutput: imageOutput, HotReplies: cfg.HotReplies, HotMinThanks: cfg.HotMinThanks})
	final, runErr := tea.NewProgram(model, tea.WithFPS(120), tea.WithOutput(imageOutput)).Run()
	cancel()
	if final == nil {
		return runErr
	}
	if err := state.Save(path, final.(*tui.Model).Session()); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("无法保存阅读进度: %w", err))
	}
	if managed {
		if err := storage.WriteFile(cfg.CookieFile, []byte(client.CookieHeader()+"\n")); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("无法保存 Cookie: %w", err))
		}
	}
	return runErr
}
