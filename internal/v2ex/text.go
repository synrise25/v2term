package v2ex

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/net/html"
)

// SafeText drops ANSI sequences and terminal controls, including OSC hyperlinks.
func SafeText(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

func label(s string) string { return strings.Join(strings.Fields(SafeText(s)), " ") }

func richText(s *goquery.Selection) (string, []Element) {
	var elements []Element
	write := func(text, target string) {
		if text == "" {
			return
		}
		if len(elements) > 0 && !elements[len(elements)-1].Image && elements[len(elements)-1].URL == target {
			elements[len(elements)-1].Text += text
		} else {
			elements = append(elements, Element{Text: text, URL: target})
		}
	}
	var walk func(*html.Node, bool, string)
	walk = func(n *html.Node, pre bool, target string) {
		if n.Type == html.TextNode {
			text := SafeText(n.Data)
			if !pre {
				text = strings.ReplaceAll(text, "\n", " ")
				text = strings.ReplaceAll(text, "\t", " ")
			}
			write(text, target)
			return
		}
		if n.Type != html.ElementNode {
			return
		}
		if n.Data == "script" || n.Data == "style" || n.Data == "iframe" {
			return
		}
		attr := func(name string) string {
			for _, a := range n.Attr {
				if a.Key == name {
					return a.Val
				}
			}
			return ""
		}
		if n.Data == "a" {
			target = contentURL(attr("href"))
			if u, err := url.Parse(target); err == nil && u.Host == "www.v2ex.com" && strings.HasPrefix(u.Path, "/member/") {
				target = ""
			}
		}
		block := strings.Contains("|p|div|pre|blockquote|h1|h2|h3|h4|ul|ol|table|tr|", "|"+n.Data+"|")
		if block || n.Data == "br" || n.Data == "li" {
			write("\n", "")
		}
		if n.Data == "li" {
			write("• ", target)
		}
		if n.Data == "img" {
			if src := contentURL(attr("src")); src != "" {
				write("\n", "")
				elements = append(elements, Element{Image: true, Text: label(attr("alt")), URL: src})
				write("\n", "")
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, pre || n.Data == "pre", target)
		}
		if n.Data == "td" || n.Data == "th" {
			write("  ", "")
		}
		if block {
			write("\n", "")
		}
	}
	for _, n := range s.Nodes {
		walk(n, false, "")
	}
	var plain strings.Builder
	for _, e := range elements {
		if e.Image {
			plain.WriteString("[图片]")
		} else {
			plain.WriteString(e.Text)
		}
	}
	lines := strings.Split(plain.String(), "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")), elements
}

func contentURL(href string) string {
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	base, _ := url.Parse("https://www.v2ex.com")
	u = base.ResolveReference(u)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || strings.ContainsAny(u.String(), "\x1b\r\n") {
		return ""
	}
	return u.String()
}
