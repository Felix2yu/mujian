// Package huozhi talks to the user's self-hosted 货殖 (huozhi) accounting site
// over its public bill API:
//
//	curl -H "X-API-Key: <key>" http://your-domain/api/public/bills/123
//	{"code":0,"message":"ok","data":{"id":123,"description":"海宁站-杭州站",
//	 "amount":11,"currency":"CNY","type":"expense","tx_date":"2021-05-02 22:26:14",
//	 "merchant":"","remark":"海宁站-杭州站"}}
//
// The integration is strictly optional and read-only: mujian never writes to
// 货殖. When 货殖 is disabled or unreachable, callers fall back to the
// locally stored cost fields.
package huozhi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Settings is a point-in-time snapshot of the integration config.
type Settings struct {
	Enabled bool
	Domain  string // normalized base URL, e.g. https://huozhi.example.com
	APIKey  string
	// BillPath is the bill page path template; "{id}" is replaced by the bill
	// id (default "/bills/{id}").
	BillPath string
	// TicketCategories is a comma-separated list of category keywords that map a
	// 货殖 bill to the "ticket" role (门票实付). Anything else (transport,
	// hotel, …) is treated as "other" (其他开销). Empty means use
	// DefaultTicketCategories.
	TicketCategories string
}

// DefaultTicketCategories covers the common 娱乐/演出 family so most ticket
// bills land in 门票实付 without extra configuration.
const DefaultTicketCategories = "娱乐,演出,门票,戏,剧,综艺,电影,演唱会,音乐会,话剧,曲艺,相声"

// Configured reports whether the integration can actually issue requests.
func (s Settings) Configured() bool {
	return s.Enabled && s.Domain != "" && s.APIKey != ""
}

// ClassifyBillRole maps a bill to "ticket" (门票实付) or "other" (其他开销)
// based on its category. A bill is a ticket payment when its category contains
// any of the configured ticket keywords (substring match, case-insensitive).
// An empty category defaults to "ticket", because the primary reason to
// associate a 货殖 bill with a performance is the ticket itself; users can set
// an explicit category in 货殖 (e.g. 交通/酒店) to push a bill into 其他开销.
func (s Settings) ClassifyBillRole(b *Bill) string {
	return ClassifyBillRole(b, s.TicketCategories)
}

// ClassifyBillRole is the stateless form of Settings.ClassifyBillRole, used by
// callers that already resolved the ticket-category list.
func ClassifyBillRole(b *Bill, ticketCategories string) string {
	cats := strings.TrimSpace(ticketCategories)
	if cats == "" {
		cats = DefaultTicketCategories
	}
	cat := strings.TrimSpace(b.Category)
	if cat == "" {
		return "ticket"
	}
	for _, kw := range strings.Split(cats, ",") {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(strings.ToLower(cat), strings.ToLower(kw)) {
			return "ticket"
		}
	}
	return "other"
}

// Bill is one 货殖 bill. Fields are the ones the public API documents; unknown
// keys are preserved in Raw so the UI can surface extra detail without another
// release.
type Bill struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Type        string  `json:"type"`
	TxDate      string  `json:"tx_date"`
	Merchant    string  `json:"merchant"`
	Remark      string  `json:"remark"`
	// Account 是账单所属账户（部分部署返回对象，如 {"id":1,"name":"支付宝"}，
	// 解析时统一压平成字符串）。
	Account string `json:"account"`
	// Category 是货侧的消费分类（若接口返回）。
	Category string `json:"category"`
	// URL 是账单在货殖网页端的地址，供前端跳转。
	URL string `json:"url"`
}

// UnmarshalJSON decodes a bill tolerantly: 货殖 部署版本不同，amount 可能是
// 数字也可能是字符串，account 可能是字符串也可能是对象。任何无法识别的形态
// 都降级为空值而不是让整条记录解析失败。
func (b *Bill) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	b.ID = int64Of(raw["id"])
	b.Description = stringOf(raw["description"])
	b.Amount = floatOf(raw["amount"])
	b.Currency = stringOf(raw["currency"])
	b.Type = stringOf(raw["type"])
	b.TxDate = stringOf(raw["tx_date"])
	if b.TxDate == "" {
		b.TxDate = stringOf(raw["date"])
	}
	b.Merchant = stringOf(raw["merchant"])
	b.Remark = stringOf(raw["remark"])
	b.Category = stringOf(raw["category"])
	b.Account = accountOf(raw["account"])
	return nil
}

// accountOf flattens the account field: objects contribute their name/title,
// scalars their string form.
func accountOf(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, k := range []string{"name", "title", "account_name"} {
			if v := stringOf(obj[k]); v != "" {
				return v
			}
		}
	}
	return ""
}

func stringOf(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}

func floatOf(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return v
		}
	}
	return 0
}

func int64Of(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return v
		}
	}
	return 0
}

var (
	// ErrNotConfigured is returned when the integration is off or incomplete.
	ErrNotConfigured = errors.New("货殖未配置或未启用")
	// ErrNotFound means the API answered but has no such bill.
	ErrNotFound = errors.New("货殖中不存在该账单")
	// ErrUnauthorized means the API key was rejected.
	ErrUnauthorized = errors.New("货殖 API Key 无效或无权限")
)

// Client issues authenticated requests against one 货殖 deployment.
type Client struct {
	Settings
	HTTPClient *http.Client
}

// New builds a client for s. Domain is normalized (trailing slash trimmed,
// https:// added for bare hosts).
func New(s Settings) *Client {
	domain := strings.TrimRight(strings.TrimSpace(s.Domain), "/")
	if domain != "" && !strings.Contains(domain, "://") {
		domain = "https://" + domain
	}
	s.Domain = domain
	if s.BillPath == "" {
		s.BillPath = "/bills/{id}"
	}
	return &Client{Settings: s, HTTPClient: &http.Client{Timeout: 10 * time.Second}}
}

// BillURL returns the human-facing bill page URL in 货殖.
func (c *Client) BillURL(id int64) string {
	if c.Domain == "" {
		return ""
	}
	path := c.BillPath
	if path == "" {
		path = "/bills/{id}"
	}
	return c.Domain + strings.ReplaceAll(path, "{id}", strconv.FormatInt(id, 10))
}

// GetBill fetches one bill by id. Results (including failures) are cached for a
// short window so repeated detail-page views do not hammer 货殖.
func (c *Client) GetBill(ctx context.Context, id int64) (*Bill, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if id <= 0 {
		return nil, fmt.Errorf("账单 ID 无效：%d", id)
	}
	if b, ok := cacheGet(c.Domain, id); ok {
		return b.bill, b.err
	}
	bill, err := c.fetch(ctx, id)
	cachePut(c.Domain, id, bill, err)
	return bill, err
}

// GetBills fetches several bills concurrently, preserving the requested order
// and skipping duplicates. Per-id failures are returned in errors so the caller
// can still show the bills that resolved.
func (c *Client) GetBills(ctx context.Context, ids []int64) ([]*Bill, map[int64]error) {
	out := make([]*Bill, 0, len(ids))
	errs := map[int64]error{}
	seen := map[int64]bool{}
	type result struct {
		id   int64
		bill *Bill
		err  error
	}
	results := make([]result, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		if id <= 0 || seen[id] {
			results[i] = result{id: id, err: fmt.Errorf("账单 ID 无效：%d", id)}
			continue
		}
		seen[id] = true
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			b, err := c.GetBill(ctx, id)
			results[i] = result{id: id, bill: b, err: err}
		}(i, id)
	}
	wg.Wait()
	for _, r := range results {
		if r.err != nil {
			errs[r.id] = r.err
			continue
		}
		out = append(out, r.bill)
	}
	return out, errs
}

func (c *Client) fetch(ctx context.Context, id int64) (*Bill, error) {
	url := fmt.Sprintf("%s/api/public/bills/%d", c.Domain, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求货殖失败：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取货殖响应失败：%w", err)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("货殖返回 %d：%s", resp.StatusCode, truncate(string(body), 200))
	}

	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析货殖响应失败：%w", err)
	}
	if env.Code != 0 {
		msg := strings.TrimSpace(env.Message)
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		if resp.StatusCode == http.StatusUnauthorized || strings.Contains(strings.ToLower(msg), "unauthor") {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("货殖接口错误 %d：%s", env.Code, truncate(msg, 200))
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, ErrNotFound
	}
	var b Bill
	if err := json.Unmarshal(env.Data, &b); err != nil {
		return nil, fmt.Errorf("解析账单失败：%w", err)
	}
	if b.ID == 0 {
		b.ID = id
	}
	b.URL = c.BillURL(id)
	return &b, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---------- short-lived response cache ----------

const cacheTTL = 60 * time.Second

type cacheEntry struct {
	bill   *Bill
	err    error
	expiry time.Time
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

func cacheGet(domain string, id int64) (cacheEntry, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	e, ok := cache[cacheKey(domain, id)]
	if !ok || time.Now().After(e.expiry) {
		return cacheEntry{}, false
	}
	return e, true
}

func cachePut(domain string, id int64, bill *Bill, err error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	// Cheap bound: a stale/never-reaped map could otherwise grow unbounded if
	// someone paged through thousands of bill ids.
	if len(cache) > 512 {
		now := time.Now()
		for k, v := range cache {
			if now.After(v.expiry) {
				delete(cache, k)
			}
		}
	}
	cache[cacheKey(domain, id)] = cacheEntry{bill: bill, err: err, expiry: time.Now().Add(cacheTTL)}
}

func cacheKey(domain string, id int64) string {
	return domain + "#" + strconv.FormatInt(id, 10)
}
