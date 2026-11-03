// Package fetch 负责抓取网页并抽取正文。
//
// 安全与礼貌约束：
//   - 只允许 http/https，禁止访问内网/回环地址（SSRF 防护）
//   - 限制响应大小与超时
//   - 默认遵守 robots.txt
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/verikod/hector/internal/researchos/textutil"
)

// Options 是 Fetcher 的配置。
type Options struct {
	UserAgent     string
	Timeout       time.Duration
	MaxBytes      int64
	RespectRobots bool
	DataDir       string
	Client        *http.Client
}

// Document 是一次抓取的结果。
type Document struct {
	URL         string    `json:"url"`
	FinalURL    string    `json:"final_url"`
	Title       string    `json:"title"`
	Text        string    `json:"text"`
	Hash        string    `json:"content_hash"`
	HTTPStatus  int       `json:"http_status"`
	ContentType string    `json:"content_type"`
	Bytes       int       `json:"bytes"`
	FetchedAt   time.Time `json:"fetched_at"`
	Snapshot    string    `json:"snapshot,omitempty"`
	RobotsBlock bool      `json:"robots_blocked,omitempty"`
}

// Fetcher 抓取文档。
type Fetcher struct {
	opt Options

	mu          sync.Mutex
	robotsCache map[string]*robotsRules
}

// New 创建 Fetcher。
func New(opt Options) *Fetcher {
	if opt.Timeout <= 0 {
		opt.Timeout = 25 * time.Second
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = 3 << 20
	}
	if opt.UserAgent == "" {
		opt.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"
	}
	if opt.Client == nil {
		opt.Client = &http.Client{Timeout: opt.Timeout, Transport: guardTransport()}
	}
	return &Fetcher{opt: opt, robotsCache: map[string]*robotsRules{}}
}

// Fetch 抓取单个 URL 并抽取正文。
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("非法 URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("不支持的协议 %q", u.Scheme)
	}
	if err := guardHost(u.Hostname()); err != nil {
		return nil, err
	}
	if err := guardResolved(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	if f.opt.RespectRobots {
		allowed, err := f.allowedByRobots(ctx, u)
		if err == nil && !allowed {
			return &Document{URL: rawURL, FinalURL: rawURL, RobotsBlock: true, FetchedAt: time.Now().UTC()}, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.opt.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8")

	resp, err := f.opt.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抓取失败: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, f.opt.MaxBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	ctype := resp.Header.Get("Content-Type")
	doc := &Document{
		URL:         rawURL,
		FinalURL:    resp.Request.URL.String(),
		HTTPStatus:  resp.StatusCode,
		ContentType: ctype,
		Bytes:       len(body),
		FetchedAt:   time.Now().UTC(),
	}
	if resp.StatusCode >= 400 {
		return doc, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	doc.Title, doc.Text = render(ctype, body)
	doc.Hash = textutil.Hash(doc.Text)

	// 原文快照：便于审计与离线复现（限制大小，避免磁盘膨胀）。
	if f.opt.DataDir != "" && len(doc.Text) > 0 {
		path, err := f.snapshot(rawURL, doc.Text)
		if err == nil {
			doc.Snapshot = path
		}
	}
	return doc, nil
}

func render(ctype string, body []byte) (title, text string) {
	lower := strings.ToLower(ctype)
	switch {
	case strings.Contains(lower, "application/json"), strings.Contains(lower, "+json"):
		title, text = renderJSON(body)
	case strings.Contains(lower, "text/html"), strings.Contains(lower, "xml"), lower == "":
		title, text = textutil.ExtractHTML(string(body))
		if len(text) > 200000 {
			text = text[:200000]
		}
	default:
		title = "纯文本文档"
		text = textutil.CleanText(string(body))
		if len(text) > 200000 {
			text = text[:200000]
		}
	}
	if title == "" {
		title = "（无标题）"
	}
	return title, text
}

// jsonTitleKeys / jsonTextKeys 是常见 API 载荷中标题与正文的字段名。
//
// 直接把 JSON 原文当正文会污染后续抽取（键名、括号、转义符都会命中关键词），
// 所以优先取语义字段；取不到才退回紧凑原文（去掉结构噪声）。
var (
	jsonTitleKeys = []string{"title", "name", "full_name", "headline", "display_title"}
	jsonTextKeys  = []string{"description", "summary", "body", "readme", "content", "text", "abstract", "extract"}
)

// renderJSON 从 JSON 文档中尽量提炼标题与正文。
func renderJSON(body []byte) (title, text string) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return "JSON 文档", textutil.CleanText(string(body))
	}
	var parts []string
	var walk func(node any, depth int)
	walk = func(node any, depth int) {
		if depth > 4 {
			return
		}
		switch n := node.(type) {
		case map[string]any:
			for _, k := range jsonTitleKeys {
				if s, ok := n[k].(string); ok && title == "" && strings.TrimSpace(s) != "" {
					title = textutil.CleanText(s)
				}
			}
			for _, k := range jsonTextKeys {
				if s, ok := n[k].(string); ok {
					if s = strings.TrimSpace(s); len([]rune(s)) >= 20 {
						parts = append(parts, textutil.CleanText(s))
					}
				}
			}
			// 递归子节点（items / results / data / nodes 等）。
			for _, k := range []string{"items", "results", "data", "nodes", "hits", "entries", "children"} {
				if child, ok := n[k]; ok {
					walk(child, depth+1)
				}
			}
		case []any:
			for _, item := range n {
				walk(item, depth+1)
			}
		}
	}
	walk(v, 0)
	if len(parts) == 0 {
		text = textutil.CleanText(string(body))
	} else {
		text = strings.Join(parts, "\n")
	}
	if len(text) > 20000 {
		text = text[:20000]
	}
	return title, text
}

func (f *Fetcher) snapshot(rawURL, text string) (string, error) {
	dir := filepath.Join(f.opt.DataDir, "snapshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := textutil.Hash(rawURL) + ".txt"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(rawURL+"\n\n"+text), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// guardHost 阻止访问本机/内网地址，避免 SSRF。
func guardHost(host string) error {
	if host == "" {
		return errors.New("URL 缺少主机名")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".internal") {
		return errors.New("拒绝访问本地/内网地址")
	}
	if ip := net.ParseIP(lower); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return errors.New("拒绝访问本地/内网地址")
		}
	}
	return nil
}

type robotsRules struct {
	disallow []string
	fetched  time.Time
}

func (f *Fetcher) allowedByRobots(ctx context.Context, u *url.URL) (bool, error) {
	host := u.Host
	f.mu.Lock()
	rules, ok := f.robotsCache[host]
	f.mu.Unlock()
	if ok && time.Since(rules.fetched) < 30*time.Minute {
		return !matchDisallow(rules.disallow, u.Path), nil
	}

	robotsURL := u.Scheme + "://" + host + "/robots.txt"
	rctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return true, err
	}
	req.Header.Set("User-Agent", f.opt.UserAgent)
	resp, err := f.opt.Client.Do(req)
	if err != nil {
		// robots.txt 不可达时按允许处理（不因对方服务异常阻断研究）。
		return true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.cache(host)
		return true, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	r := parseRobots(string(body), f.opt.UserAgent)
	r.fetched = time.Now()
	f.mu.Lock()
	f.robotsCache[host] = r
	f.mu.Unlock()
	return !matchDisallow(r.disallow, u.Path), nil
}

func (f *Fetcher) cache(host string) {
	f.mu.Lock()
	f.robotsCache[host] = &robotsRules{fetched: time.Now()}
	f.mu.Unlock()
}

func parseRobots(body, ua string) *robotsRules {
	r := &robotsRules{}
	// 简化实现：合并 "User-agent: *" 与匹配当前 UA token 的 Disallow 规则。
	uaToken := "hectorresearchos"
	active := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			// 空行是分组分隔符：后续 Disallow 必须重新声明 User-agent 才生效。
			active = false
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		switch key {
		case "user-agent":
			v := strings.ToLower(val)
			active = v == "*" || strings.Contains(strings.ToLower(ua), v) || v == uaToken
		case "disallow":
			if active && val != "" {
				r.disallow = append(r.disallow, val)
			}
		}
	}
	return r
}

func matchDisallow(patterns []string, path string) bool {
	for _, p := range patterns {
		if p == "/" {
			return true
		}
		base := strings.TrimSuffix(p, "*")
		if base == "" {
			continue
		}
		if strings.HasPrefix(path, base) && !strings.HasPrefix(p, "/*") {
			return true
		}
		if strings.HasPrefix(p, "/*") && strings.HasSuffix(path, strings.TrimPrefix(p, "*")) {
			return true
		}
	}
	return false
}

// guardTransport 返回在拨号阶段再校验一次目标 IP 的 Transport。
//
// 只校验 URL 里的主机名不够：域名可以解析到内网地址（如 127.0.0.1.nip.io），
// 也可能被 DNS rebinding 在两次解析之间换掉。在 Dial 阶段拿到真实 IP 再判断，
// 才能挡住指向回环/内网/链路本地（含云元数据 169.254.169.254）的请求。
func guardTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if err := guardHost(host); err != nil {
				return nil, err
			}
			if ips, err := net.DefaultResolver.LookupIPAddr(ctx, host); err == nil {
				for _, ipAddr := range ips {
					if err := guardIP(ipAddr.IP); err != nil {
						return nil, err
					}
				}
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          32,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// guardIP 拒绝回环、私有、链路本地与未指定地址。
func guardIP(ip net.IP) error {
	if ip == nil {
		return errors.New("无法解析目标地址")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("拒绝访问内网/保留地址 %s", ip.String())
	}
	return nil
}

// guardResolved 解析域名并校验所有解析结果（提前失败，省去无谓请求）。
// 解析失败时放行，交由实际请求报错，避免因 DNS 抖动误伤正常站点。
func guardResolved(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		return guardIP(ip)
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(rctx, host)
	if err != nil || len(ips) == 0 {
		return nil
	}
	for _, addr := range ips {
		if err := guardIP(addr.IP); err != nil {
			return err
		}
	}
	return nil
}
