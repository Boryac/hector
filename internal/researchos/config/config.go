// Package config 提供 ResearchOS 的运行时配置。
//
// 设计约束（见 DECISIONS.md 第 9 条与 SECURITY.md）：
//   - 所有凭据只从环境变量读取，不落库、不进日志、不进 Git。
//   - 缺少 LLM / 搜索凭据时不得阻断服务启动，而是降级到明确标注的本地模式。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// LLMProviderKind 标识 LLM 接入方式。
type LLMProviderKind string

const (
	LLMNone     LLMProviderKind = "none"
	LLMOpenAI   LLMProviderKind = "openai"
	LLMDeepSeek LLMProviderKind = "deepseek"
	LLMOllama   LLMProviderKind = "ollama"
)

// Config 是 ResearchOS 的完整配置。
type Config struct {
	// HTTP
	Addr string

	// 数据库
	DatabaseURL string

	// 身份（单用户本地模式：服务端唯一身份来源，绝不接受请求体/元数据传入的 user_id）
	APIToken  string
	LocalUser string

	// LLM
	LLMProvider LLMProviderKind
	LLMAPIKey   string
	LLMModel    string
	LLMBaseURL  string
	LLMTimeout  time.Duration

	// 搜索
	SearchProviders []string
	TavilyAPIKey    string
	SerpAPIKey      string
	BraveAPIKey     string
	GitHubToken     string
	SearchTimeout   time.Duration
	SearchResults   int

	// 抓取
	FetchTimeout  time.Duration
	FetchMaxBytes int64
	RespectRobots bool
	UserAgent     string

	// 默认研究深度（quick/standard/deep）
	DefaultDepth string

	// 执行
	Workers         int
	MaxAgents       int
	NodeTimeout     time.Duration
	TaskTimeout     time.Duration
	NodeMaxAttempts int
	StaleLease      time.Duration
	SchedulerTick   time.Duration

	// 数据目录（原始快照）
	DataDir string

	// 前端静态资源目录（可选）
	WebDir string

	// SSE 一次性 ticket 的有效期。EventSource 无法自定义请求头，浏览器端只能用查询参数
	// 传递凭据；为避免长期 API Token 出现在 URL（浏览器历史、Referer、反代访问日志），
	// 改为由前端先用 Bearer Token 换取短时一次性 ticket，再用 ticket 建立事件流。
	SSETicketTTL time.Duration
}

// Load 从环境变量装载配置，并在缺失可选项时应用安全默认值。
func Load() (*Config, error) {
	c := &Config{
		Addr:            env("RESEARCHOS_ADDR", ":8080"),
		DatabaseURL:     env("RESEARCHOS_DATABASE_URL", ""),
		APIToken:        env("RESEARCHOS_API_TOKEN", ""),
		LocalUser:       env("RESEARCHOS_LOCAL_USER", "local-user"),
		LLMProvider:     LLMProviderKind(strings.ToLower(env("RESEARCHOS_LLM_PROVIDER", "none"))),
		LLMAPIKey:       env("RESEARCHOS_LLM_API_KEY", ""),
		LLMModel:        env("RESEARCHOS_LLM_MODEL", ""),
		LLMBaseURL:      env("RESEARCHOS_LLM_BASE_URL", ""),
		LLMTimeout:      envDuration("RESEARCHOS_LLM_TIMEOUT", 90*time.Second),
		SearchProviders: envList("RESEARCHOS_SEARCH_PROVIDERS", []string{"bing", "github", "wikipedia"}),
		TavilyAPIKey:    env("RESEARCHOS_TAVILY_API_KEY", ""),
		SerpAPIKey:      env("RESEARCHOS_SERPAPI_KEY", ""),
		BraveAPIKey:     env("RESEARCHOS_BRAVE_API_KEY", ""),
		GitHubToken:     env("RESEARCHOS_GITHUB_TOKEN", ""),
		SearchTimeout:   envDuration("RESEARCHOS_SEARCH_TIMEOUT", 30*time.Second),
		SearchResults:   envInt("RESEARCHOS_SEARCH_RESULTS", 6),
		FetchTimeout:    envDuration("RESEARCHOS_FETCH_TIMEOUT", 25*time.Second),
		FetchMaxBytes:   int64(envInt("RESEARCHOS_FETCH_MAX_BYTES", 3_000_000)),
		RespectRobots:   envBool("RESEARCHOS_RESPECT_ROBOTS", true),
		UserAgent:       env("RESEARCHOS_USER_AGENT", "HectorResearchOS/1.0 (+local research agent)"),
		DefaultDepth:    env("RESEARCHOS_DEFAULT_DEPTH", "standard"),
		Workers:         envInt("RESEARCHOS_WORKERS", 4),
		MaxAgents:       envInt("RESEARCHOS_MAX_AGENTS", 4),
		NodeTimeout:     envDuration("RESEARCHOS_NODE_TIMEOUT", 180*time.Second),
		TaskTimeout:     envDuration("RESEARCHOS_TASK_TIMEOUT", 1800*time.Second),
		NodeMaxAttempts: envInt("RESEARCHOS_NODE_MAX_ATTEMPTS", 3),
		StaleLease:      envDuration("RESEARCHOS_STALE_LEASE", 3*time.Minute),
		SchedulerTick:   envDuration("RESEARCHOS_SCHEDULER_TICK", 500*time.Millisecond),
		DataDir:         env("RESEARCHOS_DATA_DIR", "data"),
		WebDir:          env("RESEARCHOS_WEB_DIR", ""),
		SSETicketTTL:    envDuration("RESEARCHOS_SSE_TICKET_TTL", 60*time.Second),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("RESEARCHOS_DATABASE_URL 未设置（示例：postgres://postgres@127.0.0.1:55432/researchos?sslmode=disable）")
	}
	if c.Workers <= 0 {
		c.Workers = 1
	}
	if c.MaxAgents <= 0 {
		c.MaxAgents = 1
	}
	if c.NodeMaxAttempts <= 0 {
		c.NodeMaxAttempts = 1
	}
	if c.SearchResults <= 0 {
		c.SearchResults = 5
	}
	if c.SSETicketTTL <= 0 {
		c.SSETicketTTL = 60 * time.Second
	}
	// 上限 10 分钟：再长就失去「短时」的意义，等同于把长期凭据写进 URL。
	if c.SSETicketTTL > 10*time.Minute {
		c.SSETicketTTL = 10 * time.Minute
	}
	return c, nil
}

// LLMEnabled 表示是否具备真实 LLM 调用条件。
func (c *Config) LLMEnabled() bool {
	return c.LLMProvider != "" && c.LLMProvider != LLMNone && c.LLMAPIKey != ""
}

// PlannerMode 返回规划器工作模式的机器可读标识，用于在报告与 UI 中如实标注。
func (c *Config) PlannerMode() string {
	if c.LLMEnabled() {
		return "llm"
	}
	return "heuristic"
}

// Redacted 返回可安全写入日志的配置副本（凭据只留掩码）。
func (c *Config) Redacted() map[string]any {
	return map[string]any{
		"addr":             c.Addr,
		"database":         RedactDSN(c.DatabaseURL),
		"llm_provider":     string(c.LLMProvider),
		"llm_model":        c.LLMModel,
		"llm_api_key":      Mask(c.LLMAPIKey),
		"search_providers": c.SearchProviders,
		"tavily_api_key":   Mask(c.TavilyAPIKey),
		"serpapi_key":      Mask(c.SerpAPIKey),
		"github_token":     Mask(c.GitHubToken),
		"workers":          c.Workers,
		"max_agents":       c.MaxAgents,
		"node_timeout":     c.NodeTimeout.String(),
		"respect_robots":   c.RespectRobots,
	}
}

// Mask 将凭据变为 sk-****abcd 形式。
func Mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:3] + "****" + s[len(s)-4:]
}

// RedactDSN 去掉 DSN 中的密码。
func RedactDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return dsn
	}
	scheme := strings.Index(dsn, "://")
	if scheme < 0 {
		return dsn
	}
	cred := dsn[scheme+3 : at]
	if i := strings.Index(cred, ":"); i >= 0 {
		return dsn[:scheme+3] + cred[:i] + ":****" + dsn[at:]
	}
	return dsn
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
