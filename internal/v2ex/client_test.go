package v2ex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const topicHTML = `<html><title>测试主题 - V2EX</title><div id="Top"><a href="/member/me">me</a></div><div id="Main">
<div class="box"><div class="header"><a href="/go/go">Go</a><h1>测试主题</h1><small><a href="/member/alice">alice</a><span title="2026-01-01">today</span></small></div>
<div class="topic_content"><p>中文正文</p><pre><code>line1
  line2</code></pre><a href="https://example.org">资料</a><script>bad()</script><img src="//images.example.org/a.png" alt="示例"></div>
<div class="subtle"><span>附言正文</span></div></div>
<a class="page_current">2</a><a class="page_normal">3</a><input class="page_input" max="8">
<div class="cell" id="r_901"><strong><a href="/member/bob">bob</a></strong><span class="ago" title="today">now</span><span class="small fade"><img src="heart.png">2</span><span class="no">101</span><div class="reply_content">@alice #1<br>一个回复</div></div>
<form action="/t/42" method="post"><input type="hidden" name="once" value="old"><textarea name="content"></textarea></form>
</div></html>`

func testClient(t *testing.T, handler http.HandlerFunc, auth bool) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	c.base, _ = url.Parse(srv.URL)
	c.auth = auth
	c.http.Jar.SetCookies(c.base, []*http.Cookie{{Name: "session", Value: "test", Path: "/"}})
	return c
}

func TestFetchTopicAndPagination(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, topicHTML) }, false)
	p, err := c.Fetch(context.Background(), "/t/42?p=2")
	if err != nil {
		t.Fatal(err)
	}
	if p.Topic == nil || p.Topic.ID != 42 || p.Topic.Author != "alice" || p.Number != 2 || p.Pages != 8 || p.Username != "me" || !p.CanReply {
		t.Fatalf("page: %+v", p)
	}
	if len(p.Replies) != 1 || p.Replies[0].Floor != 101 || p.Replies[0].Thanks != 2 {
		t.Fatalf("replies: %+v", p.Replies)
	}
	targets := 0
	for _, e := range p.Topic.Body {
		if e.URL != "" {
			targets++
		}
	}
	if targets != 2 || !strings.Contains(p.Topic.Content, "line1\n  line2") || !strings.Contains(p.Topic.Content, "附言正文") || strings.Contains(p.Topic.Content, "bad()") {
		t.Fatalf("body: %q; elements: %+v", p.Topic.Content, p.Topic.Body)
	}
}

func TestFetchList(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<title>Go</title><div id="Main"><div class="cell item"><span class="item_title"><a href="/t/42#reply9">中文标题</a></span><a class="node" href="/go/go">Go</a><strong><a href="/member/alice">alice</a></strong><span title="today">now</span><a class="count_livid">9</a></div></div>`)
	}, false)
	p, err := c.Fetch(context.Background(), "/go/go")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Topics) != 1 || p.Topics[0].Path != "/t/42" || p.Topics[0].Replies != 9 || p.Topics[0].Author != "alice" {
		t.Fatalf("topics: %+v", p.Topics)
	}
}

func TestReplyRefreshesOnceAndDoesNotReplay(t *testing.T) {
	var posts atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/poll_once" {
			fmt.Fprint(w, "12345")
			return
		}
		if r.Method == http.MethodGet {
			fmt.Fprint(w, topicHTML)
			return
		}
		posts.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("once") != "12345" || r.Form.Get("content") != "回复 q 中文" {
			t.Errorf("form: %v", r.Form)
		}
		if r.Header.Get("Origin") == "" || !strings.HasSuffix(r.Referer(), "/t/42?p=2") {
			t.Errorf("missing CSRF headers")
		}
		if _, err := r.Cookie("session"); err != nil {
			t.Error("missing cookie")
		}
		http.Redirect(w, r, "/t/42#reply102", http.StatusSeeOther)
	}, true)
	if err := c.Reply(context.Background(), "/t/42?p=2", "回复 q 中文"); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 {
		t.Fatal("POST replayed")
	}
}

func TestThankUsesPostAndFreshToken(t *testing.T) {
	var posts atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/poll_once" {
			fmt.Fprint(w, "56789")
			return
		}
		posts.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/thank/reply/901" || r.URL.Query().Get("once") != "56789" {
			t.Errorf("bad request: %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `{"success":true,"once":67890}`)
	}, true)
	if err := c.Thank(context.Background(), "/t/42", "reply", 901); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 {
		t.Fatal("unexpected requests")
	}
}

func TestWriteErrorsAreNotRetried(t *testing.T) {
	for _, mode := range []string{"server-error", "rejected", "unexpected", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var posts atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/poll_once" {
					fmt.Fprint(w, "123")
					return
				}
				posts.Add(1)
				switch mode {
				case "server-error":
					http.Error(w, "failed", 500)
				case "rejected":
					fmt.Fprint(w, `{"success":false,"message":"余额不足"}`)
				case "unexpected":
					fmt.Fprint(w, "<html>captcha</html>")
				case "redirect":
					http.Redirect(w, r, "/signin", 307)
				}
			}, true)
			if err := c.Thank(context.Background(), "/t/42", "topic", 42); err == nil {
				t.Fatal("unconfirmed operation reported success")
			}
			if posts.Load() != 1 {
				t.Fatal("write retried")
			}
		})
	}
}

func TestClientRejectsCredentialRedirect(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }, true)
	if _, err := c.Fetch(context.Background(), "/t/42"); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	if leaked.Load() != 0 {
		t.Fatal("redirect followed")
	}
}

func TestRouteAndTerminalSafety(t *testing.T) {
	for in, want := range map[string]string{"42": "/t/42", "go": "/go/go", "/t/42#reply3": "/t/42", "https://www.v2ex.com/go/go?p=2": "/go/go?p=2", "/?tab=hot": "/?tab=hot"} {
		got, err := Route(in)
		if err != nil || got != want {
			t.Errorf("Route(%q)=%q,%v", in, got, err)
		}
	}
	for _, in := range []string{"//evil.test/t/42", "https://evil.test/t/42", "/thank/reply/42", "/signout", "/t/42?p=-1", "http://www.v2ex.com/t/42", "/go/%2e%2e"} {
		if _, err := Route(in); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	got := SafeText("中文\x1b[2Jtext\x1b]52;c;secret\a\x00\u009b\u202e\nnext")
	if got != "中文text\nnext" {
		t.Fatalf("unsafe text %q", got)
	}
}

func TestReplyDoesNotFollowOrReplay307(t *testing.T) {
	var posts atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/poll_once" {
			fmt.Fprint(w, "123")
			return
		}
		if r.Method == http.MethodGet {
			fmt.Fprint(w, topicHTML)
			return
		}
		posts.Add(1)
		http.Redirect(w, r, "/t/42", http.StatusTemporaryRedirect)
	}, true)
	if err := c.Reply(context.Background(), "/t/42", "hello"); err == nil {
		t.Fatal("307 reported as success")
	}
	if posts.Load() != 1 {
		t.Fatal("POST replayed on redirect")
	}
}

func TestFetchCanBeCancelled(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, false)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Fetch(ctx, "/t/42")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
}

func TestCookieHeaderPreservesRotatedSession(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "rotated", Path: "/"})
		fmt.Fprint(w, topicHTML)
	}, true)
	if _, err := c.Fetch(context.Background(), "/t/42"); err != nil {
		t.Fatal(err)
	}
	header := c.CookieHeader()
	if header != "session=rotated" {
		t.Fatal("session rotation not captured")
	}
	restored, err := New(header)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CookieHeader() != header {
		t.Fatal("exported session cannot be restored")
	}
}

func TestLivePublicPages(t *testing.T) {
	if os.Getenv("V2TERM_LIVE") != "1" {
		t.Skip("set V2TERM_LIVE=1 to verify public V2EX pages")
	}
	c, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/?tab=tech", "/?tab=hot", "/recent", "/go/go?p=2", "/t/1254603"} {
		t.Run(path, func(t *testing.T) {
			p, err := c.Fetch(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if p.Topic == nil && len(p.Topics) == 0 {
				t.Fatal("no topics")
			}
			if p.Topic != nil && (p.Topic.Content == "" || len(p.Replies) == 0) {
				t.Fatal("topic or replies missing")
			}
			t.Logf("%s: topics=%d replies=%d page=%d/%d", p.Path, len(p.Topics), len(p.Replies), p.Number, p.Pages)
		})
	}
}
