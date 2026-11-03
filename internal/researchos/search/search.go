// Package search 提供真实检索能力。
//
// 关键原则：
//   - 每个 Provider 都是真实外部服务的调用实现，单元测试可以用 httptest 模拟，但默认链路不含 Mock。
//   - 任何 Provider 失败都不得使整次检索失败，只记录 trace 并降级到其它 Provider。
//   - 结果必须带 provider 名与原始 URL，供证据链回溯。
package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/verikod/hector/internal/researchos/textutil"
)

// Result 是一条检索结果。
type Result struct {
	Title       string         `json:"title"`
	URL         string         `json:"url"`
	Snippet     string         `json:"snippet"`
	Provider    string         `json:"provider"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

// Trace 记录单个 Provider 的调用情况，用于 tool.invoked / tool.completed 事件。
type Trace struct {
	Provider   string `json:"provider"`
	Count      int    `json:"count"`
	DurationMS int    `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Skipped    string `json:"skipped,omitempty"`
}

// Provider 是可插拔的检索实现。
type Provider interface {
	Name() string
	// Available 返回是否具备调用条件（如 API Key）。
	Available() bool
	Search(ctx context.Context, query string, limit int) ([]Result, error)
}

// Engine 聚合多个 Provider。
type Engine struct {
	providers []Provider
	timeout   time.Duration
	client    *http.Client
}

// Options 是 Engine 的构造参数。
type Options struct {
	Providers []Provider
	Timeout   time.Duration
	Client    *http.Client
}

// New 创建检索引擎。
func New(opts Options) *Engine {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: opts.Timeout}
	}
	return &Engine{providers: opts.Providers, timeout: opts.Timeout, client: opts.Client}
}

// Providers 返回已注册的 Provider 名称。
func (e *Engine) Names() []string {
	out := make([]string, 0, len(e.providers))
	for _, p := range e.providers {
		out = append(out, p.Name())
	}
	return out
}

// AvailableNames 返回当前具备调用条件的 Provider 名称（如缺少 Key 的不算）。
func (e *Engine) AvailableNames() []string {
	out := make([]string, 0, len(e.providers))
	for _, p := range e.providers {
		if p.Available() {
			out = append(out, p.Name())
		}
	}
	return out
}

// SearchAll 并发调用所有可用 Provider，合并去重后返回。
//
// perProvider 是单 Provider 的结果上限；失败不返回错误，而是写入 trace。
func (e *Engine) SearchAll(ctx context.Context, query string, perProvider int) ([]Result, []Trace) {
	if perProvider <= 0 {
		perProvider = 5
	}
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []Result
		traces  []Trace
	)
	for _, p := range e.providers {
		if !p.Available() {
			mu.Lock()
			traces = append(traces, Trace{Provider: p.Name(), Skipped: "未配置凭据或不可用"})
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, e.timeout)
			defer cancel()
			start := time.Now()
			res, err := p.Search(cctx, query, perProvider)
			tr := Trace{Provider: p.Name(), Count: len(res), DurationMS: int(time.Since(start).Milliseconds())}
			if err != nil {
				tr.Error = err.Error()
				slog.Warn("search provider failed", "provider", p.Name(), "err", err)
			}
			mu.Lock()
			results = append(results, res...)
			traces = append(traces, tr)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return Dedupe(results), traces
}

// Dedupe 按规范化 URL 去重，保留信息量最大的一条。
//
// 非 http(s) 或无法解析的 URL 一律丢弃：检索结果会进入来源库并被报告引用，
// 放行 javascript:/data: 之类协议等于把可注入内容带进前端链接。
func Dedupe(in []Result) []Result {
	seen := make(map[string]int, len(in))
	out := make([]Result, 0, len(in))
	for _, r := range in {
		normalized, err := textutil.NormalizeURL(r.URL)
		if err != nil || normalized == "" {
			continue
		}
		key := normalized
		r.URL = normalized
		if idx, ok := seen[key]; ok {
			if len(r.Snippet) > len(out[idx].Snippet) {
				out[idx].Snippet = r.Snippet
			}
			if out[idx].Title == "" {
				out[idx].Title = r.Title
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, r)
	}
	return out
}

// NewDefault 依据配置构造默认 Provider 链。
//
// names 中的未知项会被忽略并记录警告，绝不静默替换为 Mock。
func NewDefault(names []string, opt ProviderOptions) (*Engine, []string) {
	var providers []Provider
	var unknown []string
	for _, name := range names {
		switch name {
		case "github":
			providers = append(providers, NewGitHub(opt))
		case "hackernews", "hn":
			providers = append(providers, NewHackerNews(opt))
		case "wikipedia", "wiki":
			providers = append(providers, NewWikipedia(opt))
		case "bing":
			providers = append(providers, NewBing(opt))
		case "tavily":
			providers = append(providers, NewTavily(opt))
		case "serpapi":
			providers = append(providers, NewSerpAPI(opt))
		case "brave":
			providers = append(providers, NewBrave(opt))
		default:
			unknown = append(unknown, name)
			slog.Warn("unknown search provider ignored", "provider", name)
		}
	}
	return New(Options{Providers: providers, Timeout: opt.Timeout, Client: opt.Client}), unknown
}

// ProviderOptions 是各 Provider 的公共依赖。
type ProviderOptions struct {
	UserAgent   string
	Timeout     time.Duration
	Client      *http.Client
	TavilyKey   string
	SerpAPIKey  string
	BraveKey    string
	GitHubToken string
}

func (o ProviderOptions) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	to := o.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (o ProviderOptions) ua() string {
	if o.UserAgent == "" {
		return "HectorResearchOS/1.0 (+local research agent)"
	}
	return o.UserAgent
}

var errNoCredential = errors.New("provider credential not configured")

func queryEscape(q string) string { return url.QueryEscape(q) }

// NewRequest 构造带 UA 的 GET 请求（供各 Provider 复用）。
func NewRequest(ctx context.Context, method, rawURL, ua string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	return req, nil
}
