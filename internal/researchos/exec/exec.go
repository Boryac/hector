// Package exec 实现 ResearchOS 的节点执行器。
//
// 执行器只做三件事：读输入 → 调用真实外部能力（检索/抓取/LLM）→ 写回结构化产出。
// 所有产出一律落库（sources / claims / evidence / reports），不做内存态假数据。
package exec

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/fetch"
	"github.com/verikod/hector/internal/researchos/llm"
	"github.com/verikod/hector/internal/researchos/search"
	"github.com/verikod/hector/internal/researchos/store"
	"github.com/verikod/hector/internal/researchos/textutil"
)

// Config 是执行器参数。
type Config struct {
	FetchTopN            int
	FetchConcurrency     int
	MaxSourcesPerAnalyze int
	MaxClaimsPerNode     int
	ExcerptChars         int
	ProgressMinInterval  time.Duration
	// LeaseTTL 是进度心跳续租的时长，必须与 Worker 认领节点时的租约一致。
	// 曾经这里硬编码 3 分钟，导致 RESEARCHOS_STALE_LEASE 调小后崩溃恢复仍然
	// 要等 3 分钟——配置形同虚设。
	LeaseTTL time.Duration
}

func (c *Config) setDefaults() {
	if c.FetchTopN <= 0 {
		c.FetchTopN = 4
	}
	if c.FetchConcurrency <= 0 {
		c.FetchConcurrency = 4
	}
	if c.MaxSourcesPerAnalyze <= 0 {
		c.MaxSourcesPerAnalyze = 6
	}
	if c.MaxClaimsPerNode <= 0 {
		c.MaxClaimsPerNode = 12
	}
	if c.ExcerptChars <= 0 {
		c.ExcerptChars = 2500
	}
	if c.ProgressMinInterval <= 0 {
		c.ProgressMinInterval = 800 * time.Millisecond
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 3 * time.Minute
	}
}

// leaseTTL 返回心跳续租时长，供 Session 使用。
func (c Config) leaseTTL() time.Duration {
	if c.LeaseTTL <= 0 {
		return 3 * time.Minute
	}
	return c.LeaseTTL
}

// Engine 调度不同节点类型的执行器。
type Engine struct {
	Store   *store.Store
	Search  *search.Engine
	Fetcher *fetch.Fetcher
	LLM     llm.Client
	Cfg     Config
}

// New 创建执行引擎。
func New(st *store.Store, se *search.Engine, f *fetch.Fetcher, client llm.Client, cfg Config) *Engine {
	cfg.setDefaults()
	return &Engine{Store: st, Search: se, Fetcher: f, LLM: client, Cfg: cfg}
}

// Session 是一次节点执行的上下文。
type Session struct {
	Ctx     context.Context
	Task    *domain.Task
	Node    *domain.Node
	RunID   string
	AgentID string
	cfg     Config
	store   *store.Store
	engine  *Engine

	lastProgress int
	lastEmit     time.Time
	emitMu       sync.Mutex
}

// Emit 发送事件（节流由调用方决定）。
func (s *Session) Emit(eventType string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	ev := &domain.Event{TaskID: s.Task.ID, RunID: s.RunID, NodeID: s.Node.ID, Type: eventType, Payload: payload}
	if _, err := s.store.Emit(s.Ctx, ev); err != nil {
		slog.Error("emit event failed", "type", eventType, "err", err)
	}
}

// Progress 更新进度并发出 node.progress 事件（限频 + 单调递增）。
func (s *Session) Progress(pct int, text string) {
	if pct < s.lastProgress {
		pct = s.lastProgress
	}
	if pct > 99 {
		pct = 99
	}
	s.emitMu.Lock()
	samePhase := text == s.Node.ProgressText
	tooSoon := time.Since(s.lastEmit) < s.cfg.ProgressMinInterval
	s.emitMu.Unlock()
	if samePhase && tooSoon {
		return
	}
	s.lastProgress = pct
	s.lastEmit = time.Now()
	_ = s.store.UpdateNodeProgress(s.Ctx, s.Task.ID, s.Node.ID, pct, text, s.cfg.leaseTTL())
	s.Emit(domain.EvtNodeProgress, map[string]any{"progress": pct, "progress_text": text})
}

// Run 执行单个节点。
func (e *Engine) Run(ctx context.Context, task *domain.Task, node *domain.Node, runID string) (map[string]any, int, error) {
	s := &Session{Ctx: ctx, Task: task, Node: node, RunID: runID, cfg: e.Cfg, store: e.Store, engine: e, AgentID: ""}
	s.lastProgress = node.Progress

	var (
		out    map[string]any
		tokens int
		err    error
	)
	switch node.Type {
	case domain.NodeTypeSearch:
		out, tokens, err = e.runSearch(s)
	case domain.NodeTypeFetch:
		out, tokens, err = e.runFetch(s)
	case domain.NodeTypeAnalyze:
		out, tokens, err = e.runAnalyze(s)
	case domain.NodeTypeVerify:
		out, tokens, err = e.runVerify(s)
	case domain.NodeTypeSynthesize:
		out, tokens, err = e.runSynthesize(s)
	default:
		err = Permanent(fmt.Errorf("未知节点类型 %q", node.Type))
	}
	return out, tokens, err
}

// ---------------------------------------------------------------------------
// search 节点：检索 + 首批抓取
// ---------------------------------------------------------------------------

func (e *Engine) runSearch(s *Session) (map[string]any, int, error) {
	query := stringInput(s.Node.Input, "query")
	if query == "" {
		query = s.Node.Title
	}
	if query == "" {
		return nil, 0, Permanent(fmt.Errorf("search 节点缺少 query"))
	}
	s.Progress(5, "检索中")

	start := time.Now()
	results, traces := e.Search.SearchAll(s.Ctx, query, resultsPerProvider(s.Task.Depth))
	s.Emit(domain.EvtToolInvoked, map[string]any{
		"tool": "search", "query": query, "providers": e.Search.Names(),
	})
	if len(results) == 0 {
		if fb := stringInput(s.Node.Input, "fallback_query"); fb != "" && fb != query {
			s.Progress(12, "首选检索式无结果，使用简化检索式重试")
			query = fb
			results, traces = e.Search.SearchAll(s.Ctx, query, resultsPerProvider(s.Task.Depth))
		}
	}
	if len(results) == 0 {
		s.Emit(domain.EvtToolFailed, map[string]any{"tool": "search", "query": query, "trace": traces})
		return nil, 0, fmt.Errorf("检索无结果（query=%q）", query)
	}
	s.Emit(domain.EvtToolCompleted, map[string]any{
		"tool": "search", "query": query, "count": len(results),
		"duration_ms": int(time.Since(start).Milliseconds()), "trace": traces,
	})
	s.Progress(25, fmt.Sprintf("检索到 %d 条结果", len(results)))

	// 落库（检索阶段只保存元数据）
	srcs := make([]*domain.Source, 0, len(results))
	for _, r := range results {
		norm, err := textutil.NormalizeURL(r.URL)
		if err != nil {
			continue
		}
		src := &domain.Source{
			ID:            newID(),
			TaskID:        s.Task.ID,
			NodeID:        s.Node.ID,
			URL:           r.URL,
			NormalizedURL: norm,
			Title:         textutil.CleanText(r.Title),
			Site:          textutil.Host(r.URL),
			Provider:      r.Provider,
			Snippet:       clipRunes(textutil.CleanText(r.Snippet), maxSnippetRunes),
			Credibility:   credibilityFor(r.Provider, r.Fields),
			Status:        "discovered",
			PublishedAt:   r.PublishedAt,
		}
		saved, err := e.Store.UpsertSource(s.Ctx, src)
		if err != nil {
			slog.Warn("upsert source failed", "url", r.URL, "err", err)
			continue
		}
		srcs = append(srcs, saved)
	}
	if len(srcs) == 0 {
		return nil, 0, fmt.Errorf("检索结果均无法规范化入库")
	}

	// 抓取正文：按可信度排序 + 同站去重，先抓一批，若有效正文不足再补抓。
	sort.SliceStable(srcs, func(i, j int) bool {
		wi, wj := credibilityWeight(srcs[i].Credibility), credibilityWeight(srcs[j].Credibility)
		if wi != wj {
			return wi > wj
		}
		return providerRank(srcs[i].Provider) < providerRank(srcs[j].Provider)
	})
	picked := pickDiverse(srcs, fetchCapFor(s.Task.Depth))
	var (
		fetched int
		failed  int
	)
	fetched, failed = e.fetchBatch(s, picked, query)
	topN := len(picked)
	if fetched < 2 && len(picked) < len(srcs) {
		rest := pickDiverse(remaining(srcs, picked), 6)
		f2, ff2 := e.fetchBatch(s, rest, query)
		fetched += f2
		failed += ff2
		topN += len(rest)
	}

	ids := make([]string, 0, len(srcs))
	for _, s := range srcs {
		ids = append(ids, s.ID)
	}
	return map[string]any{
		"query":        query,
		"result_count": len(srcs),
		"fetched":      fetched,
		"failed":       failed,
		"source_ids":   ids,
		"providers":    e.Search.Names(),
		"trace":        traces,
	}, 0, nil
}

// ---------------------------------------------------------------------------
// fetch 节点：补抓（按 URL 列表）
// ---------------------------------------------------------------------------

func (e *Engine) runFetch(s *Session) (map[string]any, int, error) {
	urls := stringSliceInput(s.Node.Input, "urls")
	if len(urls) == 0 {
		return nil, 0, fmt.Errorf("fetch 节点缺少 urls")
	}
	s.Progress(10, "抓取中")
	fetched := 0
	for i, u := range urls {
		doc, err := e.Fetcher.Fetch(s.Ctx, u)
		if err != nil || doc == nil {
			continue
		}
		norm, _ := textutil.NormalizeURL(u)
		src := &domain.Source{
			ID: newID(), TaskID: s.Task.ID, NodeID: s.Node.ID, URL: u, NormalizedURL: norm,
			Title: doc.Title, Site: textutil.Host(u), Provider: "direct", Status: "ok",
			HTTPStatus: doc.HTTPStatus, ContentHash: doc.Hash, Excerpt: doc.Text,
			FetchedAt: &doc.FetchedAt, Credibility: "medium",
		}
		if _, err := e.Store.UpsertSource(s.Ctx, src); err == nil {
			fetched++
		}
		s.Progress(10+(i+1)*80/len(urls), fmt.Sprintf("已抓取 %d/%d", i+1, len(urls)))
	}
	return map[string]any{"fetched": fetched, "requested": len(urls)}, 0, nil
}

// fetchBatch 并发抓取一批资料，返回成功/失败计数。
func (e *Engine) fetchBatch(s *Session, srcs []*domain.Source, query string) (int, int) {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		fetched int
		failed  int
		sem     = make(chan struct{}, e.Cfg.FetchConcurrency)
	)
	for _, src := range srcs {
		wg.Add(1)
		go func(src *domain.Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			doc, err := e.Fetcher.Fetch(s.Ctx, src.URL)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || doc == nil {
				failed++
				src.Status = "fetch_failed"
				if err != nil {
					src.Error = truncate(err.Error(), 300)
				}
				if _, uerr := e.Store.UpsertSource(s.Ctx, src); uerr != nil {
					slog.Warn("mark source failed", "err", uerr)
				}
				return
			}
			if doc.RobotsBlock {
				src.Status = "robots_blocked"
				src.Error = "robots.txt 禁止抓取"
				_, _ = e.Store.UpsertSource(s.Ctx, src)
				return
			}
			fetched++
			src.Status = "ok"
			src.HTTPStatus = doc.HTTPStatus
			src.ContentHash = doc.Hash
			if doc.Title != "" && doc.Title != "（无标题）" {
				src.Title = doc.Title
			}
			src.Excerpt = textutil.Excerpt(doc.Text, textutil.Terms(query), 4000)
			src.FetchedAt = &doc.FetchedAt
			src.Snippet = ""
			if s2, err := e.Store.UpsertSource(s.Ctx, src); err == nil && s2 != nil {
				src.Excerpt = s2.Excerpt
			}
			s.Emit(domain.EvtToolCompleted, map[string]any{
				"tool": "fetch", "url": src.URL, "http_status": doc.HTTPStatus, "bytes": doc.Bytes,
			})
		}(src)
	}
	wg.Wait()
	return fetched, failed
}

// pickDiverse 选出待抓取集合：同一站点最多 2 条，避免集中在单一域名。
func pickDiverse(srcs []*domain.Source, max int) []*domain.Source {
	if max <= 0 {
		max = len(srcs)
	}
	out := make([]*domain.Source, 0, max)
	// 候选充足时把同站上限收到 1，优先保证站点多样性；
	// 只有候选不足时才允许同站 2 条，避免抓取名额被单一站点吃掉。
	perSiteCap := 2
	if len(srcs) >= max*2 {
		perSiteCap = 1
	}
	perSite := map[string]int{}
	for _, src := range srcs {
		if len(out) >= max {
			break
		}
		if perSite[src.Site] >= perSiteCap {
			continue
		}
		perSite[src.Site]++
		out = append(out, src)
	}
	return out
}

func remaining(srcs, picked []*domain.Source) []*domain.Source {
	seen := map[string]bool{}
	for _, p := range picked {
		seen[p.ID] = true
	}
	out := make([]*domain.Source, 0, len(srcs))
	for _, src := range srcs {
		if !seen[src.ID] {
			out = append(out, src)
		}
	}
	return out
}

func fetchCapFor(depth string) int {
	switch depth {
	case "quick":
		return 4
	case "deep":
		return 12
	default:
		return 8
	}
}

func providerRank(p string) int {
	switch p {
	case "wikipedia":
		return 0
	case "github":
		return 1
	case "tavily", "serpapi", "brave":
		return 2
	case "bing":
		return 3
	case "hackernews":
		return 4
	default:
		return 5
	}
}

func resultsPerProvider(depth string) int {
	switch depth {
	case "quick":
		return 3
	case "deep":
		return 8
	default:
		return 5
	}
}

func fetchTopNFor(depth string) int {
	switch depth {
	case "quick":
		return 2
	case "deep":
		return 8
	default:
		return 4
	}
}

func credibilityFor(provider string, fields map[string]any) string {
	switch provider {
	case "github":
		if stars, ok := fields["stars"].(int); ok && stars > 1000 {
			return "high"
		}
		return "medium"
	case "wikipedia":
		return "high"
	case "tavily", "serpapi", "brave":
		return "medium"
	case "hackernews":
		return "low"
	default:
		return "unknown"
	}
}

func stringInput(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func stringSliceInput(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
