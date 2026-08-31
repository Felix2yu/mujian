package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	out, err := callLLM(ai, text)
	if err != nil {
		jsonErr(w, 502, "AI 解析失败："+err.Error())
		return
	}
	jsonResp(w, 200, out)
}

// aiSystemPrompt instructs the model to return ONLY a JSON object whose keys
// match the editable record fields, so the client can map them 1:1.
const aiSystemPrompt = `你是一个演出信息提取助手。用户会粘贴一段与现场演出相关的文本（如购票短信、票务邮件、宣传文案、观演记录等）。请从中提取结构化字段，并只输出一个 JSON 对象（不要包含任何解释文字或 Markdown 代码块标记）。
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

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
