package v2ex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
)

const maxResponse = 8 << 20

type Client struct {
	http *http.Client
	base *url.URL
	auth bool
}

func New(cookie string) (*Client, error) {
	base, _ := url.Parse("https://www.v2ex.com")
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}
	cookie = strings.TrimSpace(cookie)
	if strings.ContainsAny(cookie, "\r\n") {
		return nil, errors.New("Cookie 文件应只包含一行 Cookie header 的值")
	}
	if len(cookie) > 65536 {
		return nil, errors.New("Cookie 过长")
	}
	if cookie != "" {
		cookies, err := http.ParseCookie(cookie)
		if err != nil || len(cookies) == 0 {
			return nil, errors.New("无法解析 Cookie")
		}
		for _, item := range cookies {
			item.Secure, item.Path = true, "/"
		}
		jar.SetCookies(base, cookies)
	}
	c := &Client{base: base, auth: cookie != "", http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
	c.http.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("重定向次数过多")
		}
		if req.URL.Scheme != c.base.Scheme || req.URL.Host != c.base.Host {
			return errors.New("页面跳转到了其他站点，请在浏览器检查登录或验证状态")
		}
		return nil
	}
	return c, nil
}

func (c *Client) Authenticated() bool { return c.auth }

// CookieHeader exports the current same-origin session, including server rotations.
func (c *Client) CookieHeader() string {
	r := &http.Request{Header: make(http.Header)}
	for _, cookie := range c.http.Jar.Cookies(c.base) {
		r.AddCookie(cookie)
	}
	return r.Header.Get("Cookie")
}

func (c *Client) request(ctx context.Context, method, path, referer string, values url.Values) ([]byte, *http.Response, error) {
	u := c.base.ResolveReference(&url.URL{Path: "/"})
	rel, err := url.Parse(path)
	if err != nil || rel.IsAbs() || rel.Host != "" || !strings.HasPrefix(path, "/") {
		return nil, nil, errors.New("无效请求地址")
	}
	u = u.ResolveReference(rel)
	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "v2term/0.1 (+https://www.v2ex.com/; terminal client)")
	req.Header.Set("Accept", "text/html, application/json;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", c.base.String()+referer)
	}
	if method == http.MethodPost {
		req.Header.Set("Origin", c.base.String())
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	hc := *c.http
	if method != http.MethodGet {
		// Inspect the redirect before following it: a POST must never be replayed.
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	resp, err := hc.Do(req)
	if err != nil {
		var requestError *url.Error
		if errors.As(err, &requestError) {
			err = requestError.Err
		}
		return nil, nil, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, resp, err
	}
	if len(data) > maxResponse {
		return nil, resp, errors.New("页面过大，已停止读取")
	}
	if resp.StatusCode >= 400 {
		if resp.StatusCode == 429 {
			return nil, resp, fmt.Errorf("请求过于频繁，请稍后重试（Retry-After: %s）", label(resp.Header.Get("Retry-After")))
		}
		return nil, resp, fmt.Errorf("V2EX 返回 HTTP %d；请检查登录、访问权限或人机验证", resp.StatusCode)
	}
	return data, resp, nil
}

func (c *Client) document(ctx context.Context, path string) (*goquery.Document, string, error) {
	data, resp, err := c.request(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, "", err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, "", err
	}
	canonical, err := Route(resp.Request.URL.RequestURI())
	if err != nil {
		return nil, "", errors.New("请在浏览器完成登录或验证，再更新 Cookie")
	}
	return doc, canonical, nil
}

func (c *Client) Fetch(ctx context.Context, path string) (Page, error) {
	path, err := Route(path)
	if err != nil {
		return Page{}, err
	}
	doc, canonical, err := c.document(ctx, path)
	if err != nil {
		return Page{}, err
	}
	return parsePage(doc, canonical)
}

func (c *Client) once(ctx context.Context, path string) (string, error) {
	data, _, err := c.request(ctx, http.MethodGet, "/poll_once", path, nil)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if _, err := strconv.ParseUint(token, 10, 64); err != nil || token == "" {
		return "", errors.New("无法获取会话令牌，请更新 Cookie")
	}
	return token, nil
}

func (c *Client) Reply(ctx context.Context, path, content string) error {
	if !c.auth {
		return errors.New("回复需要登录，请设置 V2TERM_COOKIE 或 --cookie-file")
	}
	path, err := Route(path)
	if err != nil || !strings.HasPrefix(path, "/t/") {
		return errors.New("只能回复帖子")
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("回复不能为空")
	}
	doc, _, err := c.document(ctx, path)
	if err != nil {
		return err
	}
	form := doc.Find("#Main form").FilterFunction(func(_ int, s *goquery.Selection) bool { return s.Find("textarea[name='content']").Length() > 0 }).First()
	if form.Length() == 0 {
		return errors.New("当前帖子无法回复，请检查 Cookie、权限或帖子是否关闭")
	}
	action := form.AttrOr("action", strings.Split(path, "?")[0])
	if action == "" {
		action = strings.Split(path, "?")[0]
	}
	action, err = Route(action)
	if err != nil || strings.Split(action, "?")[0] != strings.Split(path, "?")[0] {
		return errors.New("无法识别回复表单的目标地址")
	}
	values := url.Values{}
	form.Find("input[type='hidden'][name]").Each(func(_ int, s *goquery.Selection) { values.Set(s.AttrOr("name", ""), s.AttrOr("value", "")) })
	token, err := c.once(ctx, path)
	if err != nil {
		return err
	}
	values.Set("once", token)
	values.Set("content", content)
	data, resp, err := c.request(ctx, http.MethodPost, action, path, values)
	if err != nil {
		return fmt.Errorf("%w；发送结果可能不确定，请刷新核对后再决定是否重发", err)
	}
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		location, err := resp.Location()
		if err == nil && location.Scheme == c.base.Scheme && location.Host == c.base.Host && location.Path == strings.Split(path, "?")[0] {
			return nil
		}
	}
	result, _ := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if result != nil {
		if message := label(result.Find(".problem, .message").First().Text()); message != "" {
			return fmt.Errorf("回复未确认: %.300s", message)
		}
	}
	return errors.New("未收到回复成功的确认；草稿已保留，请刷新帖子核对后再决定是否重发")
}

func (c *Client) Thank(ctx context.Context, path, kind string, id int) error {
	if !c.auth {
		return errors.New("感谢需要登录，请设置 V2TERM_COOKIE 或 --cookie-file")
	}
	if (kind != "topic" && kind != "reply") || id <= 0 {
		return errors.New("无效的感谢目标")
	}
	path, err := Route(path)
	if err != nil || !strings.HasPrefix(path, "/t/") {
		return errors.New("只能在帖子中感谢")
	}
	token, err := c.once(ctx, path)
	if err != nil {
		return err
	}
	data, _, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/thank/%s/%d?once=%s", kind, id, token), path, url.Values{})
	if err != nil {
		return fmt.Errorf("%w；请刷新核对感谢结果，客户端不会自动重试", err)
	}
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return errors.New("感谢结果无法确认，请刷新核对，客户端不会自动重试")
	}
	if !result.Success {
		return fmt.Errorf("感谢失败: %s", label(result.Message))
	}
	return nil
}
