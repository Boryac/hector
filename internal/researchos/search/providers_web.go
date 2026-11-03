package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/verikod/hector/internal/researchos/textutil"
)

// ---------------------------------------------------------------------------
// Bing HTML 结果页（无密钥、尽力而为）
//
// 说明：Bing 对匿名抓取有反爬与地域化行为，本 Provider 只做「尽力而为」，
// 失败会被 Engine 记录 trace 并降级，不影响其它 Provider。
// ---------------------------------------------------------------------------

// Bing 抓取 Bing 搜索结果页。
type Bing struct{ opt ProviderOptions }

// NewBing 创建 Bing Provider。
func NewBing(opt ProviderOptions) *Bing { return &Bing{opt: opt} }

// Name 返回 provider 名。
func (b *Bing) Name() string { return "bing" }

// Available 无需凭据。
func (b *Bing) Available() bool { return true }

var (
	bingBlockRe = regexp.MustCompile(`(?is)<li class="b_algo".*?</li>`)
	bingLinkRe  = regexp.MustCompile(`(?is)<h2[^>]*>\s*<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	bingSnipRe  = regexp.MustCompile(`(?is)<p[^>]*class="[^"]*b_lineclamp[^"]*"[^>]*>(.*?)</p>`)
	tagStripRe  = regexp.MustCompile(`(?s)<[^>]*>`)
	hanRe       = regexp.MustCompile(`\p{Han}`)
)

// bingMarket 依据检索式语种选择市场。
//
// 之前固定 mkt=en-US，中文检索式在英文市场里会被弱化处理，返回的是
// 「和查询无关的本地化页面」（实测一条中文关键词能召回到完全无关的站点）。
func bingMarket(query string) string {
	if hanRe.MatchString(query) {
		return "mkt=zh-CN&setlang=zh-Hans"
	}
	return "mkt=en-US&setlang=en"
}

// Search 抓取并解析 Bing 结果页。
func (b *Bing) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	u := "https://www.bing.com/search?q=" + queryEscape(query) + "&" + bingMarket(query) + "&count=" + fmt.Sprint(maxInt(limit, 5))
	req, err := NewRequest(ctx, http.MethodGet, u, b.opt.ua())
	if err != nil {
		return nil, err
	}
	resp, err := b.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("bing request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing status %d", resp.StatusCode)
	}
	page := string(body)
	blocks := bingBlockRe.FindAllString(page, -1)
	results := make([]Result, 0, len(blocks))
	for _, blk := range blocks {
		m := bingLinkRe.FindStringSubmatch(blk)
		if len(m) < 3 {
			continue
		}
		link := htmlUnescape(m[1])
		if strings.HasPrefix(link, "/") || strings.Contains(link, "bing.com/aclick") {
			continue
		}
		title := textutil.CleanText(stripTags(m[2]))
		if title == "" || link == "" {
			continue
		}
		snippet := ""
		if sm := bingSnipRe.FindStringSubmatch(blk); len(sm) > 1 {
			snippet = textutil.CleanText(stripTags(sm[1]))
		}
		results = append(results, Result{
			Title:    title,
			URL:      link,
			Snippet:  snippet,
			Provider: "bing",
			Fields:   map[string]any{"kind": "webpage"},
		})
		if len(results) >= limit {
			break
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("bing 未解析到结果（可能被反爬或返回本地化页面）")
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Tavily（真实搜索 API，需要 Key）
// ---------------------------------------------------------------------------

// Tavily 使用 Tavily Search API。
type Tavily struct{ opt ProviderOptions }

// NewTavily 创建 Tavily Provider。
func NewTavily(opt ProviderOptions) *Tavily { return &Tavily{opt: opt} }

// Name 返回 provider 名。
func (t *Tavily) Name() string { return "tavily" }

// Available 需要 API Key。
func (t *Tavily) Available() bool { return t.opt.TavilyKey != "" }

// Search 调用 Tavily。
func (t *Tavily) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if t.opt.TavilyKey == "" {
		return nil, errNoCredential
	}
	payload := map[string]any{
		"api_key":             t.opt.TavilyKey,
		"query":               query,
		"search_depth":        "advanced",
		"max_results":         limit,
		"include_answer":      false,
		"include_raw_content": false,
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", t.opt.ua())
	resp, err := t.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily status %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}
	var out struct {
		Results []struct {
			Title         string  `json:"title"`
			URL           string  `json:"url"`
			Content       string  `json:"content"`
			Score         float64 `json:"score"`
			PublishedDate string  `json:"published_date"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("tavily decode: %w", err)
	}
	results := make([]Result, 0, len(out.Results))
	for _, r := range out.Results {
		results = append(results, Result{
			Title: r.Title, URL: r.URL, Snippet: textutil.CleanText(r.Content),
			Provider: "tavily",
			Fields:   map[string]any{"score": r.Score, "kind": "webpage", "published_date": r.PublishedDate},
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// SerpAPI（Google 结果，需要 Key）
// ---------------------------------------------------------------------------

// SerpAPI 使用 SerpAPI。
type SerpAPI struct{ opt ProviderOptions }

// NewSerpAPI 创建 SerpAPI Provider。
func NewSerpAPI(opt ProviderOptions) *SerpAPI { return &SerpAPI{opt: opt} }

// Name 返回 provider 名。
func (s *SerpAPI) Name() string { return "serpapi" }

// Available 需要 API Key。
func (s *SerpAPI) Available() bool { return s.opt.SerpAPIKey != "" }

// Search 调用 SerpAPI。
func (s *SerpAPI) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if s.opt.SerpAPIKey == "" {
		return nil, errNoCredential
	}
	u := "https://serpapi.com/search.json?engine=google&q=" + queryEscape(query) +
		"&num=" + fmt.Sprint(limit) + "&api_key=" + queryEscape(s.opt.SerpAPIKey)
	req, err := NewRequest(ctx, http.MethodGet, u, s.opt.ua())
	if err != nil {
		return nil, err
	}
	resp, err := s.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("serpapi request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serpapi status %d", resp.StatusCode)
	}
	var out struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
			Date    string `json:"date"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("serpapi decode: %w", err)
	}
	results := make([]Result, 0, len(out.Organic))
	for _, r := range out.Organic {
		results = append(results, Result{
			Title: r.Title, URL: r.Link, Snippet: textutil.CleanText(r.Snippet),
			Provider: "serpapi", Fields: map[string]any{"kind": "webpage", "date": r.Date},
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Brave Search API（需要 Key）
// ---------------------------------------------------------------------------

// Brave 使用 Brave Search API。
type Brave struct{ opt ProviderOptions }

// NewBrave 创建 Brave Provider。
func NewBrave(opt ProviderOptions) *Brave { return &Brave{opt: opt} }

// Name 返回 provider 名。
func (b *Brave) Name() string { return "brave" }

// Available 需要 API Key。
func (b *Brave) Available() bool { return b.opt.BraveKey != "" }

// Search 调用 Brave。
func (b *Brave) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if b.opt.BraveKey == "" {
		return nil, errNoCredential
	}
	u := "https://api.search.brave.com/res/v1/web/search?q=" + queryEscape(query) + "&count=" + fmt.Sprint(limit)
	req, err := NewRequest(ctx, http.MethodGet, u, b.opt.ua())
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", b.opt.BraveKey)
	req.Header.Set("Accept", "application/json")
	resp, err := b.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave status %d", resp.StatusCode)
	}
	var out struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Age         string `json:"age"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("brave decode: %w", err)
	}
	results := make([]Result, 0, len(out.Web.Results))
	for _, r := range out.Web.Results {
		results = append(results, Result{
			Title: r.Title, URL: r.URL, Snippet: stripTags(r.Description),
			Provider: "brave", Fields: map[string]any{"kind": "webpage", "age": r.Age},
		})
	}
	return results, nil
}

func stripTags(s string) string {
	return textutil.CleanText(htmlUnescape(tagStripRe.ReplaceAllString(s, " ")))
}

func htmlUnescape(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&#x27;", "'",
		"&ensp;", " ", "&#0183;", "·", "&nbsp;", " ",
	)
	return r.Replace(s)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// NormalizeURLForDedupe 暴露给上层，保证与存储层同一套规则。
func NormalizeURLForDedupe(raw string) string {
	if n, err := textutil.NormalizeURL(raw); err == nil {
		return n
	}
	if u, err := url.Parse(raw); err == nil {
		return u.String()
	}
	return raw
}

var _ = time.Second
