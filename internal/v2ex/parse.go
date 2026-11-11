package v2ex

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var topicIDPattern = regexp.MustCompile(`^/t/([0-9]+)`)

func number(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func parsePage(doc *goquery.Document, path string) (Page, error) {
	p := Page{Path: path, Number: 1, Pages: 1}
	main := doc.Find("#Main")
	if main.Length() == 0 {
		return p, fmt.Errorf("无法识别页面：可能遇到了登录或人机验证，请在浏览器检查")
	}
	p.Title = label(doc.Find("title").First().Text())
	p.Username = label(doc.Find("#Top a[href^='/member/']").First().Text())
	if p.Username == "" {
		p.Username, _ = doc.Find("#Top a[href^='/member/'] img").First().Attr("alt")
		p.Username = label(p.Username)
	}
	p.Number = max(1, number(main.Find(".page_current").First().Text()))
	p.Pages = p.Number
	main.Find(".page_normal, .page_current").Each(func(_ int, s *goquery.Selection) {
		p.Pages = max(p.Pages, number(s.Text()))
	})
	main.Find("input.page_input").Each(func(_ int, s *goquery.Selection) {
		p.Pages = max(p.Pages, number(s.AttrOr("max", "1")))
	})
	if strings.HasPrefix(path, "/t/") {
		id := topicIDPattern.FindStringSubmatch(path)
		h := main.Find("h1").First()
		if len(id) < 2 || h.Length() == 0 {
			return p, fmt.Errorf("帖子不可见、已删除或需要登录")
		}
		header := h.Closest(".header")
		node := header.Find("a[href^='/go/']").First()
		t := &Topic{ID: number(id[1]), Path: strings.Split(path, "?")[0], Title: label(h.Text()),
			Author: label(header.Find("small a[href^='/member/']").First().Text()),
			Node:   label(node.Text()), NodePath: node.AttrOr("href", ""),
			Age: label(header.Find("small span[title]").First().AttrOr("title", ""))}
		if t.Author == "" {
			t.Author = label(header.Find("a[href^='/member/']").First().Text())
		}
		t.Content, t.Body = richText(main.Find(".topic_content").First())
		main.Find(".subtle").Each(func(_ int, s *goquery.Selection) {
			text, body := richText(s)
			t.Content += "\n\n── 附言 ──\n" + text
			t.Body = append(t.Body, Element{Text: "\n\n── 附言 ──\n"})
			t.Body = append(t.Body, body...)
		})
		p.Topic, p.Title = t, t.Title
		main.Find("[id^='r_']").Each(func(_ int, s *goquery.Selection) {
			content := s.Find(".reply_content").First()
			if content.Length() == 0 {
				return
			}
			heart := s.Find(".small.fade img, .thank_area img").First().Parent()
			text, body := richText(content)
			p.Replies = append(p.Replies, Reply{ID: number(strings.TrimPrefix(s.AttrOr("id", ""), "r_")),
				Floor: number(s.Find(".no").First().Text()), Author: label(s.Find("strong a[href^='/member/']").First().Text()),
				Age: label(s.Find(".ago").First().AttrOr("title", "")), Thanks: number(heart.Text()), Content: text, Body: body})
		})
		p.CanReply = main.Find("form textarea[name='content']").Length() > 0
		return p, nil
	}
	seen := map[int]bool{}
	main.Find(".item_title a[href^='/t/']").Each(func(_ int, s *goquery.Selection) {
		id := topicIDPattern.FindStringSubmatch(s.AttrOr("href", ""))
		if len(id) < 2 || seen[number(id[1])] {
			return
		}
		seen[number(id[1])] = true
		cell := s.Closest(".cell")
		node := cell.Find("a.node").First()
		p.Topics = append(p.Topics, Topic{ID: number(id[1]), Path: "/t/" + id[1], Title: label(s.Text()),
			Author: label(cell.Find("strong a[href^='/member/']").First().Text()),
			Node:   label(node.Text()), NodePath: node.AttrOr("href", ""), Replies: number(cell.Find(".count_livid, .count_orange").First().Text()),
			Age: label(cell.Find("span[title]").First().AttrOr("title", ""))})
	})
	if len(p.Topics) == 0 && main.Find(".node_header, #Tabs, .page_current").Length() == 0 {
		return p, fmt.Errorf("没有识别到帖子列表：可能需要登录或网站结构已变化")
	}
	return p, nil
}
