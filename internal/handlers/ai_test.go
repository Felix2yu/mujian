package handlers

import (
	"strings"
	"testing"
)

func TestExtractURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"没有链接的普通文本", ""},
		{"演出预告 https://mp.weixin.qq.com/s/abcDEF123 快来看", "https://mp.weixin.qq.com/s/abcDEF123"},
		{"详情见（https://mp.weixin.qq.com/s/xyz）速来", "https://mp.weixin.qq.com/s/xyz"},
		{"https://example.com/a,以及后续", "https://example.com/a"},
		{"链接https://example.com/。",
			"https://example.com/"},
		{"ftp://example.com/file", ""},
	}
	for _, c := range cases {
		if got := extractURL(c.in); got != c.want {
			t.Errorf("extractURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsPrivateHost(t *testing.T) {
	blocked := []string{"localhost", "foo.localhost", "nas.local", "127.0.0.1", "10.1.2.3", "192.168.1.1", "172.16.0.9", "::1", "169.254.1.1", "0.0.0.0"}
	for _, h := range blocked {
		if !isPrivateHost(h) {
			t.Errorf("isPrivateHost(%q) = false, want true", h)
		}
	}
	allowed := []string{"mp.weixin.qq.com", "example.com", "8.8.8.8", "172.67.1.1"}
	for _, h := range allowed {
		if isPrivateHost(h) {
			t.Errorf("isPrivateHost(%q) = true, want false", h)
		}
	}
}

func TestExtractHTMLText(t *testing.T) {
	page := `<!DOCTYPE html><html><head><title>演出预告 | 公众号</title>
<script>window.__DATA__="x";</script><style>.a{color:red}</style></head>
<body><h1 id="activity-name">昆剧《牡丹亭》精选</h1>
<div id="js_content"><p>时间：2026-05-01 19:30</p>
<p>地点：上海大剧院</p><p>主演：张军、沈昳丽</p>
折子：《游园》《惊梦》</div>
<script>var footer="lots of js";</script></body></html>`
	title, text, err := extractHTMLText(page)
	if err != nil {
		t.Fatalf("extractHTMLText error: %v", err)
	}
	if title != "演出预告 | 公众号" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"昆剧《牡丹亭》精选", "时间：2026-05-01 19:30", "上海大剧院", "张军、沈昳丽", "《游园》《惊梦》"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q\ntext = %q", want, text)
		}
	}
	if strings.Contains(text, "window.__DATA__") || strings.Contains(text, "lots of js") || strings.Contains(text, "color:red") {
		t.Errorf("script/style leaked into text:\n%s", text)
	}
}

func TestExtractHTMLTextTooShort(t *testing.T) {
	if _, _, err := extractHTMLText("<html><body>短</body></html>"); err == nil {
		t.Error("expected error for near-empty page, got nil")
	}
}
