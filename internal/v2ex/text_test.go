package v2ex

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestRichTextKeepsInlineTargetsAndImageBoundaries(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<div id="body"><p>你好 @<a href="/member/alice">alice</a>，<a href="https://example.org">点击阅读</a>。</p><a href="https://example.org/full"><img src="//images.example.org/a.png" alt="示例"></a><script>bad()</script><pre>first
  second</pre></div>`))
	if err != nil {
		t.Fatal(err)
	}
	text, body := richText(doc.Find("#body"))
	if strings.Contains(text, "[1]") || strings.Contains(text, "bad()") || !strings.Contains(text, "@alice") || !strings.Contains(text, "first\n  second") {
		t.Fatalf("unexpected plain text: %q", text)
	}
	images, links := 0, 0
	for _, e := range body {
		if strings.Contains(e.URL, "/member/") {
			t.Fatal("user mention became a separate navigation link")
		}
		if e.Image {
			images++
			if e.URL != "https://images.example.org/a.png" || e.Text != "示例" {
				t.Fatalf("image: %+v", e)
			}
		} else if e.URL != "" {
			links++
		}
	}
	if images != 1 || links != 1 {
		t.Fatalf("images=%d links=%d", images, links)
	}
}
