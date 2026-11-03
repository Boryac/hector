package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// GitHub 仓库检索（真实 API，未配置 Token 时使用匿名配额）
// ---------------------------------------------------------------------------

// GitHub 检索公开仓库。
type GitHub struct {
	opt ProviderOptions
}

// NewGitHub 创建 GitHub Provider。
func NewGitHub(opt ProviderOptions) *GitHub { return &GitHub{opt: opt} }

// Name 返回 provider 名。
func (g *GitHub) Name() string { return "github" }

// Available GitHub 匿名可用。
func (g *GitHub) Available() bool { return true }

type ghSearchResp struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		FullName    string    `json:"full_name"`
		HTMLURL     string    `json:"html_url"`
		Description string    `json:"description"`
		Stars       int       `json:"stargazers_count"`
		Forks       int       `json:"forks_count"`
		OpenIssues  int       `json:"open_issues_count"`
		Language    string    `json:"language"`
		Topics      []string  `json:"topics"`
		PushedAt    time.Time `json:"pushed_at"`
		CreatedAt   time.Time `json:"created_at"`
		License     *struct {
			SPDXID string `json:"spdx_id"`
		} `json:"license"`
		Archived bool `json:"archived"`
	} `json:"items"`
}

// Search 调用 GitHub 搜索 API。
func (g *GitHub) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("sort", "stars")
	q.Set("order", "desc")
	q.Set("per_page", strconv.Itoa(limit))
	req, err := NewRequest(ctx, http.MethodGet,
		"https://api.github.com/search/repositories?"+q.Encode(), g.opt.ua())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.opt.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.opt.GitHubToken)
	}
	resp, err := g.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github status %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}
	var out ghSearchResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("github decode: %w", err)
	}
	results := make([]Result, 0, len(out.Items))
	for _, it := range out.Items {
		license := ""
		if it.License != nil {
			license = it.License.SPDXID
		}
		snippet := fmt.Sprintf("%s · ★%d · fork %d · %s · 最近推送 %s",
			nonEmpty(it.Description, "（无描述）"), it.Stars, it.Forks,
			nonEmpty(it.Language, "未知语言"), it.PushedAt.Format("2006-01-02"))
		if license != "" {
			snippet += " · " + license
		}
		pushed := it.PushedAt
		results = append(results, Result{
			Title:       it.FullName,
			URL:         it.HTMLURL,
			Snippet:     snippet,
			Provider:    "github",
			PublishedAt: &pushed,
			Fields: map[string]any{
				"stars":     it.Stars,
				"forks":     it.Forks,
				"language":  it.Language,
				"topics":    it.Topics,
				"license":   license,
				"pushed_at": it.PushedAt.Format(time.RFC3339),
				"archived":  it.Archived,
				"kind":      "repository",
			},
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Hacker News（Algolia API，公开无密钥）
// ---------------------------------------------------------------------------

// HackerNews 检索 HN 讨论。
type HackerNews struct{ opt ProviderOptions }

// NewHackerNews 创建 HN Provider。
func NewHackerNews(opt ProviderOptions) *HackerNews { return &HackerNews{opt: opt} }

// Name 返回 provider 名。
func (h *HackerNews) Name() string { return "hackernews" }

// Available HN 无需凭据。
func (h *HackerNews) Available() bool { return true }

// Search 调用 HN Algolia 搜索。
func (h *HackerNews) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=%d",
		queryEscape(query), limit)
	req, err := NewRequest(ctx, http.MethodGet, u, h.opt.ua())
	if err != nil {
		return nil, err
	}
	resp, err := h.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("hn request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hn status %d", resp.StatusCode)
	}
	var out struct {
		Hits []struct {
			Title       string    `json:"title"`
			URL         string    `json:"url"`
			ObjectID    string    `json:"objectID"`
			Points      int       `json:"points"`
			NumComments int       `json:"num_comments"`
			CreatedAt   time.Time `json:"created_at"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("hn decode: %w", err)
	}
	results := make([]Result, 0, len(out.Hits))
	for _, hit := range out.Hits {
		if hit.Title == "" {
			continue
		}
		link := hit.URL
		if link == "" {
			link = "https://news.ycombinator.com/item?id=" + hit.ObjectID
		}
		created := hit.CreatedAt
		results = append(results, Result{
			Title:       hit.Title,
			URL:         link,
			Snippet:     fmt.Sprintf("Hacker News 讨论 · %d 分 · %d 评论 · %s", hit.Points, hit.NumComments, hit.CreatedAt.Format("2006-01-02")),
			Provider:    "hackernews",
			PublishedAt: &created,
			Fields: map[string]any{
				"points":       hit.Points,
				"num_comments": hit.NumComments,
				"hn_item":      "https://news.ycombinator.com/item?id=" + hit.ObjectID,
				"kind":         "hn_story",
			},
		})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Wikipedia（MediaWiki API，公开无密钥）
// ---------------------------------------------------------------------------

// Wikipedia 检索百科条目。
type Wikipedia struct{ opt ProviderOptions }

// NewWikipedia 创建 Wikipedia Provider。
func NewWikipedia(opt ProviderOptions) *Wikipedia { return &Wikipedia{opt: opt} }

// Name 返回 provider 名。
func (w *Wikipedia) Name() string { return "wikipedia" }

// Available 无需凭据。
func (w *Wikipedia) Available() bool { return true }

// Search 调用 MediaWiki 搜索 + REST 摘要。
func (w *Wikipedia) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	lang := "en"
	if hasCJK(query) {
		lang = "zh"
	}
	u := fmt.Sprintf("https://%s.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=%d&srprop=snippet",
		lang, queryEscape(query), limit)
	req, err := NewRequest(ctx, http.MethodGet, u, w.opt.ua())
	if err != nil {
		return nil, err
	}
	resp, err := w.opt.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikipedia request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikipedia status %d", resp.StatusCode)
	}
	var out struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("wikipedia decode: %w", err)
	}
	results := make([]Result, 0, len(out.Query.Search))
	for _, s := range out.Query.Search {
		title := s.Title
		results = append(results, Result{
			Title:    title,
			URL:      fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(strings.ReplaceAll(title, " ", "_"))),
			Snippet:  stripTags(s.Snippet),
			Provider: "wikipedia",
			Fields:   map[string]any{"kind": "encyclopedia"},
		})
	}
	return results, nil
}

// hasCJK 判断查询是否包含中日韩字符，用于选择维基语言站点。
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
