package search

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeProvider struct {
	name  string
	ok    bool
	items []Result
	err   error
	calls *int32
}

func (f fakeProvider) Name() string    { return f.name }
func (f fakeProvider) Available() bool { return f.ok }
func (f fakeProvider) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if f.calls != nil {
		atomic.AddInt32(f.calls, 1)
	}
	if f.err != nil {
		return nil, f.err
	}
	out := make([]Result, 0, len(f.items))
	for _, it := range f.items {
		it.Provider = f.name
		out = append(out, it)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func TestDedupeByNormalizedURL(t *testing.T) {
	in := []Result{
		{Title: "A", URL: "https://example.com/a/?utm_source=x"},
		{Title: "A 副本", URL: "https://example.com/a"},
		{Title: "B", URL: "https://example.com/b"},
	}
	out := Dedupe(in)
	if len(out) != 2 {
		t.Fatalf("URL 归一化去重后应为 2 条，实际 %d：%+v", len(out), out)
	}
	if out[0].Title != "A" {
		t.Fatalf("应保留先出现的条目，实际 %q", out[0].Title)
	}
}

func TestDedupeKeepsDifferentHosts(t *testing.T) {
	in := []Result{
		{Title: "A", URL: "https://a.com/x"},
		{Title: "B", URL: "https://b.com/x"},
	}
	if out := Dedupe(in); len(out) != 2 {
		t.Fatalf("不同站点不应被合并，实际 %d", len(out))
	}
}

func TestDedupeSkipsInvalidURL(t *testing.T) {
	in := []Result{
		{Title: "坏", URL: "javascript:alert(1)"},
		{Title: "好", URL: "https://ok.com/x"},
	}
	out := Dedupe(in)
	if len(out) != 1 || out[0].Title != "好" {
		t.Fatalf("非法 URL 应被剔除，实际 %+v", out)
	}
}

// 注：Result 没有 Site 字段时 Dedupe 依赖 NormalizeURL/Dedupe 的内部实现，
// 这里只验证行为契约（非法剔除、重复合并、跨站保留）。

func TestSearchAllGracefulDegradation(t *testing.T) {
	var calls int32
	eng := New(Options{
		Timeout: 2 * time.Second,
		Providers: []Provider{
			fakeProvider{name: "good", ok: true, items: []Result{{Title: "G", URL: "https://g.com/1"}}, calls: &calls},
			fakeProvider{name: "bad", ok: true, err: errors.New("quota exceeded")},
			fakeProvider{name: "off", ok: false},
		},
	})
	res, traces := eng.SearchAll(context.Background(), "ai agent", 5)
	if len(res) != 1 {
		t.Fatalf("健康 Provider 的结果应保留，实际 %d", len(res))
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("不可用 Provider 不应被调用，实际调用 %d 次", calls)
	}
	if len(traces) != 3 {
		t.Fatalf("每个声明 Provider 都应留下 trace（含跳过），实际 %d", len(traces))
	}
	var hasErr, hasSkip bool
	for _, tr := range traces {
		if tr.Error != "" {
			hasErr = true
			if !strings.Contains(tr.Error, "quota") {
				t.Fatalf("trace 应保留原始错误，实际 %q", tr.Error)
			}
		}
		if tr.Skipped != "" {
			hasSkip = true
		}
	}
	if !hasErr || !hasSkip {
		t.Fatalf("应同时记录失败与跳过，traces=%+v", traces)
	}
}

func TestSearchAllRespectsLimit(t *testing.T) {
	eng := New(Options{
		Timeout: 2 * time.Second,
		Providers: []Provider{
			fakeProvider{name: "p", ok: true, items: []Result{
				{Title: "1", URL: "https://a.com/1"},
				{Title: "2", URL: "https://a.com/2"},
				{Title: "3", URL: "https://a.com/3"},
				{Title: "4", URL: "https://a.com/4"},
			}},
		},
	})
	res, _ := eng.SearchAll(context.Background(), "q", 2)
	if len(res) > 2 {
		t.Fatalf("应遵守 perProvider 上限，实际 %d", len(res))
	}
}

func TestSearchAllEmptyProviders(t *testing.T) {
	eng := New(Options{Timeout: time.Second})
	res, traces := eng.SearchAll(context.Background(), "q", 3)
	if len(res) != 0 || len(traces) != 0 {
		t.Fatalf("无 Provider 时应返回空结果，实际 res=%d traces=%d", len(res), len(traces))
	}
}

func TestNamesVsAvailableNames(t *testing.T) {
	eng := New(Options{Providers: []Provider{
		fakeProvider{name: "a", ok: true},
		fakeProvider{name: "b", ok: false},
	}})
	if names := eng.Names(); len(names) != 2 {
		t.Fatalf("Names 应列出全部声明 Provider，实际 %v", names)
	}
	avail := eng.AvailableNames()
	if len(avail) != 1 || avail[0] != "a" {
		t.Fatalf("AvailableNames 应只列出可用 Provider，实际 %v", avail)
	}
}

// Bing 的市场参数必须跟随检索式语种。
//
// 回归用例：此前固定 mkt=en-US，中文检索式在英文市场里被弱化，
// 实测返回与查询完全无关的本地化页面（从而拖垮下游 analyze）。
func TestBingMarketFollowsQueryLanguage(t *testing.T) {
	if got := bingMarket("向量数据库召回质量"); !strings.Contains(got, "zh-CN") {
		t.Fatalf("中文检索式应使用中文市场，实际 %q", got)
	}
	if got := bingMarket("vector database recall benchmark"); !strings.Contains(got, "en-US") {
		t.Fatalf("英文检索式应使用英文市场，实际 %q", got)
	}
	if got := bingMarket("MCP 协议 vs LangGraph"); !strings.Contains(got, "zh-CN") {
		t.Fatalf("中英混排应优先中文市场，实际 %q", got)
	}
}

// 纯中文检索式不应打到英文技术索引上。
//
// 回归用例：GitHub 用「向量数据库在检索增强生成中的召回质量对比」检索，
// 召回到一个个人书单仓库（README 里恰好有这些汉字），其目录被当成研究结论。
func TestLanguageMismatchSkipsEnglishIndexes(t *testing.T) {
	if why := languageMismatch("github", "向量数据库召回质量"); why == "" {
		t.Fatal("纯中文检索式应跳过 github")
	}
	if why := languageMismatch("hackernews", "向量数据库召回质量"); why == "" {
		t.Fatal("纯中文检索式应跳过 hackernews")
	}
	if why := languageMismatch("wikipedia", "向量数据库召回质量"); why != "" {
		t.Fatal("中文维基不应被跳过")
	}
	if why := languageMismatch("github", "LangGraph 状态图编排"); why != "" {
		t.Fatal("含英文技术词的检索式不应跳过 github")
	}
}
