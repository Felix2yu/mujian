package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"mujian/internal/config"
)

// aiParseRequest is the body posted to POST /api/ai/parse.
type aiParseRequest struct {
	Text string `json:"text"`
}

// parseAI extracts structured performance fields from a pasted blob of text
// using the configured OpenAI-compatible chat model. The API key stays
// server-side; the client only ever sends the raw pasted text.
//
// If the pasted text contains an http(s) link (typically a WeChat
// official-account article), the page is fetched first and its readable text
// is appended to the prompt, so the model extracts from the article content
// (演出时间、演员、剧目、折子等) instead of just the URL itself.
func (h *Handler) parseAI(w http.ResponseWriter, r *http.Request) {
	var req aiParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		jsonErr(w, 400, "请粘贴演出信息文本")
		return
	}

	ai := h.cfg.GetAISettings()
	if !ai.Enabled || ai.APIKey == "" || ai.Model == "" || ai.BaseURL == "" {
		jsonErr(w, 400, "AI 未配置：请先在设置页填写 API 地址、密钥与模型")
		return
	}

	userMsg := text
	var srcURL, srcTitle string
	if pageURL := extractURL(text); pageURL != "" {
		title, pageText, err := fetchPageText(pageURL)
		if err != nil {
			jsonErr(w, 502, "抓取链接失败："+err.Error()+"。可改为直接复制文章正文文本重试")
			return
		}
		srcURL, srcTitle = pageURL, title
		rest := strings.TrimSpace(strings.Replace(text, pageURL, "", 1))
		var sb strings.Builder
		if rest != "" {
			sb.WriteString("用户附注文本：\n" + rest + "\n\n")
		}
		sb.WriteString("网页链接：" + pageURL + "\n")
		if title != "" {
			sb.WriteString("网页标题：" + title + "\n")
		}
		sb.WriteString("网页正文：\n" + pageText)
		userMsg = sb.String()
	}

	out, err := callLLM(ai, userMsg)
	if err != nil {
		jsonErr(w, 502, "AI 解析失败："+err.Error())
		return
	}
	if srcURL != "" {
		out["_source"] = map[string]string{"url": srcURL, "title": srcTitle}
	}
	jsonResp(w, 200, out)
}

// aiSystemPrompt instructs the model to return ONLY a JSON object whose keys
// match the editable record fields, so the client can map them 1:1.
const aiSystemPrompt = `你是一个演出信息提取助手。用户会提供一段与现场演出相关的材料：可能是粘贴的文本（购票短信、票务邮件、宣传文案、观演记录等），也可能包含一篇网页正文（如微信公众号演出预告/回顾文章）。若同时给出网页正文与用户附注文本，以网页正文为主要依据。请从中提取结构化字段，并只输出一个 JSON 对象（不要包含任何解释文字或 Markdown 代码块标记）。
字段说明（键名必须严格一致；无对应信息则省略该键，或使用给出的默认值）：
- name: 演出名称（字符串）
- date_local: 演出时间，格式必须为 "YYYY-MM-DDTHH:MM"（24 小时制，当地时区），无则省略
- city: 城市（字符串）
- address: 场馆 / 地址（字符串）
- channel: 购票渠道或平台（字符串）
- company: 剧团 / 演出团体（字符串）
- categoryNames: 剧种分类数组，如 ["昆剧","京剧"]
- artist_names: 演员姓名数组
- play: 剧目名称数组（整本剧目，不是折子）
- zhezi_names: 折子戏名数组（折子 / 选场，如 ["游园","惊梦"]；整本剧目放 play）
- guest: 嘉宾数组
- seat: 座位号（字符串）
- friends: 同行人（字符串，可用顿号或逗号分隔多人）
- remark: 备注（字符串）
- rating: 评分整数 0-5，未提及则为 0
- duration: 演出时长（整数分钟），未知为 0
- price: 票价（数字）
- pay_price: 实付金额（数字）
- other_cost: 其他花费（数字）
- active_status: 演出状态整数：0=正常 1=想看 2=已取消 3=未赴约，默认 0
- lat: 纬度数字，已知才给
- lng: 经度数字，已知才给
只返回 JSON。`

// callLLM posts to the configured OpenAI-compatible /chat/completions endpoint
// and returns the model's JSON content parsed into a generic map.
func callLLM(ai config.AISettings, text string) (map[string]interface{}, error) {
	base := strings.TrimRight(ai.BaseURL, "/")
	payload, err := json.Marshal(map[string]interface{}{
		"model": ai.Model,
		"messages": []map[string]string{
			{"role": "system", "content": aiSystemPrompt},
			{"role": "user", "content": text},
		},
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ai.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 AI 服务失败：%w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI 服务返回 %d：%s", resp.StatusCode, truncateStr(string(raw), 300))
	}

	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, fmt.Errorf("解析 AI 响应失败：%w", err)
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("AI 未返回内容")
	}

	content := cr.Choices[0].Message.Content
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("AI 返回非 JSON：%s", truncateStr(content, 200))
	}
	return out, nil
}

// urlPattern matches http(s) links inside the pasted text. CJK punctuation and
// full-width brackets are excluded so a 「（链接）」wrap doesn't get swallowed;
// trailing ASCII punctuation is trimmed separately.
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>，。；！？、【】（）「」,]+`)
var urlTailTrim = ".,;:!?、，。；！？）)】]"

// extractURL returns the first http(s) link in the text (punctuation-trimmed),
// or "" if none. Only the first link is used; pasting several is not expected.
func extractURL(text string) string {
	m := urlPattern.FindString(text)
	if m == "" {
		return ""
	}
	return strings.TrimRight(m, urlTailTrim)
}

const pageUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
const maxPageChars = 15000

var (
	headRe     = regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	scriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	commentRe  = regexp.MustCompile(`(?s)<!--.*?-->`)
	titleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	// WeChat article pages ship an empty <title> (filled client-side); the
	// real title sits in the og:title meta and the #activity-name h1.
	ogTitleRe  = regexp.MustCompile(`(?is)<meta[^>]*og:title[^>]*>`)
	metaContRe = regexp.MustCompile(`(?is)content=["']([^"']*)["']`)
	h1Re       = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	blockTagRe = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|tr|td|th|h[1-6]|section|article|header|footer|blockquote|table)[^>]*>`)
	tagRe      = regexp.MustCompile(`(?s)<[^>]*>`)
)

// fetchPageText downloads the page at rawURL and returns (title, readable
// text). Tags are stripped and whitespace collapsed — enough for
// server-rendered article pages such as WeChat official-account posts
// (mp.weixin.qq.com), which render their body HTML on the server.
func fetchPageText(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", fmt.Errorf("不是有效的 http(s) 链接")
	}
	if isPrivateHost(u.Hostname()) {
		return "", "", fmt.Errorf("不允许访问内网地址")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", pageUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("网页返回 %d", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text") {
		return "", "", fmt.Errorf("链接不是网页（%s）", truncateStr(resp.Header.Get("Content-Type"), 60))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return "", "", fmt.Errorf("读取网页失败：%w", err)
	}
	return extractHTMLText(string(body))
}

// extractHTMLText strips the page down to visible text and returns
// (title, text), one line per block element, capped at maxPageChars. Returns
// an error when too little text survives (e.g. the link is a card, not an
// article).
func extractHTMLText(page string) (string, string, error) {
	title := ""
	if m := titleRe.FindStringSubmatch(page); len(m) > 1 {
		title = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	if title == "" {
		if m := ogTitleRe.FindString(page); m != "" {
			if c := metaContRe.FindStringSubmatch(m); len(c) > 1 {
				title = strings.TrimSpace(html.UnescapeString(c[1]))
			}
		}
	}
	if title == "" {
		if m := h1Re.FindStringSubmatch(page); len(m) > 1 {
			inner := tagRe.ReplaceAllString(m[1], "")
			title = strings.TrimSpace(html.UnescapeString(inner))
		}
	}
	page = headRe.ReplaceAllString(page, " ")
	page = commentRe.ReplaceAllString(page, " ")
	page = scriptRe.ReplaceAllString(page, " ")
	page = styleRe.ReplaceAllString(page, " ")
	page = blockTagRe.ReplaceAllString(page, "\n")
	page = tagRe.ReplaceAllString(page, "")
	page = html.UnescapeString(page)

	var b strings.Builder
	prevBlank := true
	for _, line := range strings.Split(page, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			prevBlank = true
			continue
		}
		if !prevBlank {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		prevBlank = false
	}
	text := b.String()
	if len([]rune(text)) < 50 {
		return title, "", fmt.Errorf("网页正文为空或过短")
	}
	if r := []rune(text); len(r) > maxPageChars {
		text = string(r[:maxPageChars]) + "…（正文过长已截断）"
	}
	return title, text, nil
}

// isPrivateHost blocks loopback/private literal IPs and localhost names so a
// pasted link can't be used to probe the server's own network.
func isPrivateHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
