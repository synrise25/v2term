package v2ex

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var routePattern = regexp.MustCompile(`^/(?:t/[1-9][0-9]*|go/[A-Za-z0-9_.-]+|recent)?$`)
var nodePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

// Route limits navigation to readable pages, so a remote link can never trigger an action.
func Route(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "/?tab=tech", nil
	}
	if id, err := strconv.Atoi(input); err == nil && id > 0 {
		input = "/t/" + input
	} else if nodePattern.MatchString(input) {
		input = "/go/" + input
	}
	u, err := url.Parse(input)
	if err != nil || u == nil {
		return "", fmt.Errorf("无效地址")
	}
	if u.IsAbs() && (u.Scheme != "https" || u.Host != "www.v2ex.com") || u.User != nil || !u.IsAbs() && u.Host != "" {
		return "", fmt.Errorf("请输入 www.v2ex.com 的帖子或节点地址")
	}
	if !routePattern.MatchString(u.Path) || u.RawPath != "" {
		return "", fmt.Errorf("支持首页、/recent、/go/节点名 或 /t/帖子编号")
	}
	q := url.Values{}
	if p := u.Query().Get("p"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 1000000 {
			return "", fmt.Errorf("无效页码")
		}
		if n > 1 {
			q.Set("p", strconv.Itoa(n))
		}
	}
	if tab := u.Query().Get("tab"); tab != "" && u.Path == "/" {
		switch tab {
		case "tech", "creative", "play", "apple", "jobs", "deals", "city", "qna", "hot", "all", "r2":
			q.Set("tab", tab)
		default:
			return "", fmt.Errorf("未知首页分类")
		}
	}
	if len(q) == 0 {
		return u.Path, nil
	}
	return u.Path + "?" + q.Encode(), nil
}

func PagePath(path string, number int) string {
	u, _ := url.Parse(path)
	q := u.Query()
	q.Del("p")
	if number > 1 {
		q.Set("p", strconv.Itoa(number))
	}
	u.RawQuery = q.Encode()
	return u.String()
}
