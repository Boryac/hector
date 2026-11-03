// Package settings 实现「可被 Web 界面管理的运行配置」。
//
// 三层模型（见 DECISIONS.md 第 29 条）：
//   - deploy：部署级。只从环境变量读取，界面只读展示，**不接受写入**。
//     数据库连接串、监听地址、API Token 属于这一类——它们是部署者的决定，
//     不该被一个能打开网页的人改掉。
//   - hot：即时生效。保存后重建 LLM / 搜索客户端并原子替换快照，
//     正在跑的任务用旧客户端跑完，新任务用新配置。
//   - restart：需要重启。进程启动时就把这些参数固化进了 goroutine 数量、
//     Fetcher 之类的地方，热改只会造成「界面显示 A、实际跑 B」，所以明确标注。
//
// 优先级：Web 保存值 > 环境变量 > 内置默认。
// 环境变量是首次启动的种子与部署级唯一来源；一旦用户在界面保存过，
// 就以界面值为准，否则「重启后仍然有效」这条要求无法成立。
// 想回到环境变量，把该字段清空即可（界面提供「清除覆盖」操作）。
package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
)

// Scope 表示配置的生效方式。
type Scope string

const (
	ScopeHot     Scope = "hot"
	ScopeRestart Scope = "restart"
	ScopeDeploy  Scope = "deploy"
)

// Kind 表示配置值类型，决定前端控件与后端校验。
type Kind string

const (
	KindString   Kind = "string"
	KindSecret   Kind = "secret"
	KindInt      Kind = "int"
	KindBool     Kind = "bool"
	KindDuration Kind = "duration"
	KindEnum     Kind = "enum"
	KindList     Kind = "list"
	KindReadOnly Kind = "readonly"
)

// Field 是一个可配置项。
type Field struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Help    string   `json:"help"`
	Kind    Kind     `json:"kind"`
	Scope   Scope    `json:"scope"`
	Env     string   `json:"env"`
	Default any      `json:"default,omitempty"`
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
	Options []string `json:"options,omitempty"`
	Unit    string   `json:"unit,omitempty"`

	// Read 读取 deploy 字段的当前值（来自环境变量）。
	Read func(config.Config) any `json:"-"`
	// Get 从配置读出当前值，用于判断「环境变量给了什么」。
	Get func(config.Config) any `json:"-"`
	// Set 把值写进配置。
	Set func(*config.Config, any) `json:"-"`
}

// Section 是一组相关的配置项。
type Section struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
}

func strField(key, label string, scope Scope, env string, def string, help string, set func(*config.Config, string), get func(config.Config) string) Field {
	return Field{
		Key: key, Label: label, Help: help, Kind: KindString, Scope: scope, Env: env, Default: def,
		Set: func(c *config.Config, v any) { set(c, v.(string)) },
		Get: func(c config.Config) any { return get(c) },
	}
}

func secretField(key, label, env, help string, set func(*config.Config, string), get func(config.Config) string) Field {
	return Field{
		Key: key, Label: label, Help: help, Kind: KindSecret, Scope: ScopeHot, Env: env,
		Set: func(c *config.Config, v any) { set(c, v.(string)) },
		Get: func(c config.Config) any { return get(c) },
	}
}

func intField(key, label string, scope Scope, env string, def, min, max int, help string, set func(*config.Config, int), get func(config.Config) int) Field {
	return Field{
		Key: key, Label: label, Help: help, Kind: KindInt, Scope: scope, Env: env,
		Default: def, Min: min, Max: max,
		Set: func(c *config.Config, v any) { set(c, v.(int)) },
		Get: func(c config.Config) any { return get(c) },
	}
}

func boolField(key, label string, scope Scope, env string, def bool, help string, set func(*config.Config, bool), get func(config.Config) bool) Field {
	return Field{
		Key: key, Label: label, Help: help, Kind: KindBool, Scope: scope, Env: env, Default: def,
		Set: func(c *config.Config, v any) { set(c, v.(bool)) },
		Get: func(c config.Config) any { return get(c) },
	}
}

func durField(key, label string, scope Scope, env string, def time.Duration, help string, set func(*config.Config, time.Duration), get func(config.Config) time.Duration) Field {
	return Field{
		Key: key, Label: label, Help: help, Kind: KindDuration, Scope: scope, Env: env,
		Default: def.String(), Unit: "duration",
		Set: func(c *config.Config, v any) { set(c, v.(time.Duration)) },
		Get: func(c config.Config) any { return get(c).String() },
	}
}

// Sections 返回全部配置分组。顺序即前端导航顺序。
func Sections() []Section {
	return []Section{
		{
			ID: "model", Title: "模型设置", Description: "规划与抽取使用的 LLM。保存后立即生效，不需要重启。",
			Fields: []Field{
				{
					Key: "model.provider", Label: "Provider", Kind: KindEnum, Scope: ScopeHot,
					Env: "RESEARCHOS_LLM_PROVIDER", Default: "none",
					Options: []string{"none", "openai", "deepseek", "ollama"},
					Help:    "选择 none 表示不使用大模型，规划与抽取降级为本地确定性流程。",
					Set:     func(c *config.Config, v any) { c.LLMProvider = config.LLMProviderKind(v.(string)) },
					Get:     func(c config.Config) any { return string(c.LLMProvider) },
				},
				secretField("model.api_key", "API Key", "RESEARCHOS_LLM_API_KEY",
					"加密保存；读取接口只返回掩码。留空表示不修改已保存的值。",
					func(c *config.Config, v string) { c.LLMAPIKey = v },
					func(c config.Config) string { return c.LLMAPIKey }),
				strField("model.base_url", "Base URL", ScopeHot, "RESEARCHOS_LLM_BASE_URL", "",
					"留空使用 Provider 默认地址；Ollama 等自建服务在这里填完整地址（含 /v1）。",
					func(c *config.Config, v string) { c.LLMBaseURL = v },
					func(c config.Config) string { return c.LLMBaseURL }),
				strField("model.model", "模型名", ScopeHot, "RESEARCHOS_LLM_MODEL", "",
					"留空使用 Provider 默认模型。",
					func(c *config.Config, v string) { c.LLMModel = v },
					func(c config.Config) string { return c.LLMModel }),
				durField("model.timeout", "请求超时", ScopeHot, "RESEARCHOS_LLM_TIMEOUT", 90*time.Second,
					"单次补全调用的超时时间，例如 90s、2m。",
					func(c *config.Config, v time.Duration) { c.LLMTimeout = v },
					func(c config.Config) time.Duration { return c.LLMTimeout }),
			},
		},
		{
			ID: "search", Title: "搜索设置", Description: "检索 Provider 与密钥。保存后立即生效，下一个检索节点即生效。",
			Fields: []Field{
				{
					Key: "search.providers", Label: "启用的 Provider", Kind: KindList, Scope: ScopeHot,
					Env: "RESEARCHOS_SEARCH_PROVIDERS", Default: []string{"bing", "github", "wikipedia"},
					Options: []string{"bing", "github", "wikipedia", "hackernews", "tavily", "serpapi", "brave"},
					Help:    "多选。缺少密钥的 Provider 会自动跳过并在检索 trace 里标注。",
					Set:     func(c *config.Config, v any) { c.SearchProviders = v.([]string) },
					Get:     func(c config.Config) any { return c.SearchProviders },
				},
				intField("search.results", "每 Provider 结果数", ScopeHot, "RESEARCHOS_SEARCH_RESULTS", 6, 1, 20,
					"单个 Provider 每次检索返回的条数上限。",
					func(c *config.Config, v int) { c.SearchResults = v },
					func(c config.Config) int { return c.SearchResults }),
				durField("search.timeout", "检索超时", ScopeHot, "RESEARCHOS_SEARCH_TIMEOUT", 30*time.Second,
					"单个检索请求的超时时间。",
					func(c *config.Config, v time.Duration) { c.SearchTimeout = v },
					func(c config.Config) time.Duration { return c.SearchTimeout }),
				secretField("search.tavily_api_key", "Tavily API Key", "RESEARCHOS_TAVILY_API_KEY",
					"启用 tavily 时必填。",
					func(c *config.Config, v string) { c.TavilyAPIKey = v },
					func(c config.Config) string { return c.TavilyAPIKey }),
				secretField("search.serpapi_key", "SerpAPI Key", "RESEARCHOS_SERPAPI_KEY",
					"启用 serpapi 时必填。",
					func(c *config.Config, v string) { c.SerpAPIKey = v },
					func(c config.Config) string { return c.SerpAPIKey }),
				secretField("search.brave_api_key", "Brave Search Key", "RESEARCHOS_BRAVE_API_KEY",
					"启用 brave 时必填。",
					func(c *config.Config, v string) { c.BraveAPIKey = v },
					func(c config.Config) string { return c.BraveAPIKey }),
				secretField("search.github_token", "GitHub Token", "RESEARCHOS_GITHUB_TOKEN",
					"可选。填了能提高 GitHub 检索的配额上限。",
					func(c *config.Config, v string) { c.GitHubToken = v },
					func(c config.Config) string { return c.GitHubToken }),
			},
		},
		{
			ID: "execution", Title: "执行设置", Description: "并发、超时、重试与审批策略。标注「需重启」的项改了要重启服务才生效。",
			Fields: []Field{
				intField("execution.max_agents", "最大并发 Agent", ScopeHot, "RESEARCHOS_MAX_AGENTS", 4, 1, 8,
					"单个任务同时展开的搜索/分析节点上限。",
					func(c *config.Config, v int) { c.MaxAgents = v },
					func(c config.Config) int { return c.MaxAgents }),
				intField("execution.workers", "Worker 数量", ScopeRestart, "RESEARCHOS_WORKERS", 4, 1, 16,
					"进程启动时创建固定数量的 Worker goroutine，改这里需要重启。",
					func(c *config.Config, v int) { c.Workers = v },
					func(c config.Config) int { return c.Workers }),
				durField("execution.node_timeout", "单节点超时", ScopeHot, "RESEARCHOS_NODE_TIMEOUT", 180*time.Second,
					"单个搜索/分析节点的执行上限。",
					func(c *config.Config, v time.Duration) { c.NodeTimeout = v },
					func(c config.Config) time.Duration { return c.NodeTimeout }),
				durField("execution.task_timeout", "任务超时", ScopeHot, "RESEARCHOS_TASK_TIMEOUT", 30*time.Minute,
					"整个研究任务的执行上限。",
					func(c *config.Config, v time.Duration) { c.TaskTimeout = v },
					func(c config.Config) time.Duration { return c.TaskTimeout }),
				intField("execution.node_max_attempts", "节点最大重试次数", ScopeRestart, "RESEARCHOS_NODE_MAX_ATTEMPTS", 3, 1, 5,
					"确定性失败不会重试；这里只约束可重试的瞬时失败。Worker 在启动时读取，需重启。",
					func(c *config.Config, v int) { c.NodeMaxAttempts = v },
					func(c config.Config) int { return c.NodeMaxAttempts }),
				boolField("execution.require_approval", "新建任务需人工批准", ScopeHot, "RESEARCHOS_REQUIRE_APPROVAL", true,
					"开启后任务先停在规划结果，点批准才开始执行。",
					func(c *config.Config, v bool) { c.RequireApproval = v },
					func(c config.Config) bool { return c.RequireApproval }),
				{
					Key: "execution.default_depth", Label: "默认研究深度", Kind: KindEnum, Scope: ScopeHot,
					Env: "RESEARCHOS_DEFAULT_DEPTH", Default: "standard",
					Options: []string{"quick", "standard", "deep"},
					Help:    "新建任务时预选的深度。",
					Set:     func(c *config.Config, v any) { c.DefaultDepth = v.(string) },
					Get:     func(c config.Config) any { return c.DefaultDepth },
				},
				boolField("execution.respect_robots", "遵守 robots.txt", ScopeRestart, "RESEARCHOS_RESPECT_ROBOTS", true,
					"Fetcher 在启动时构造，改这里需要重启。关掉它意味着抓取不再受站点声明约束。",
					func(c *config.Config, v bool) { c.RespectRobots = v },
					func(c config.Config) bool { return c.RespectRobots }),
				durField("execution.fetch_timeout", "抓取超时", ScopeRestart, "RESEARCHOS_FETCH_TIMEOUT", 25*time.Second,
					"单页抓取的超时时间。Fetcher 启动时构造，需重启。",
					func(c *config.Config, v time.Duration) { c.FetchTimeout = v },
					func(c config.Config) time.Duration { return c.FetchTimeout }),
			},
		},
		{
			ID: "system", Title: "系统设置", Description: "部署级参数。只能通过环境变量设置，界面只读——防止网页端误改把服务改到起不来。",
			Fields: []Field{
				{
					Key: "system.addr", Label: "监听地址", Kind: KindReadOnly, Scope: ScopeDeploy,
					Env: "RESEARCHOS_ADDR", Help: "改它需要改环境变量并重启服务。",
					Read: func(c config.Config) any { return c.Addr },
					Get:  func(c config.Config) any { return c.Addr },
				},
				{
					Key: "system.database", Label: "数据库连接", Kind: KindReadOnly, Scope: ScopeDeploy,
					Env: "RESEARCHOS_DATABASE_URL", Help: "连接串按脱敏形式展示，密码不会出现在界面上。",
					Read: func(c config.Config) any { return config.RedactDSN(c.DatabaseURL) },
					Get:  func(c config.Config) any { return config.RedactDSN(c.DatabaseURL) },
				},
				{
					Key: "system.api_token", Label: "API Token", Kind: KindReadOnly, Scope: ScopeDeploy,
					Env: "RESEARCHOS_API_TOKEN", Help: "只显示是否已配置，不返回任何字符。",
					Read: func(c config.Config) any { return map[string]any{"configured": c.APIToken != ""} },
					Get:  func(c config.Config) any { return map[string]any{"configured": c.APIToken != ""} },
				},
				{
					Key: "system.data_dir", Label: "数据目录", Kind: KindReadOnly, Scope: ScopeDeploy,
					Env: "RESEARCHOS_DATA_DIR", Help: "原始抓取快照落盘位置。",
					Read: func(c config.Config) any { return c.DataDir },
					Get:  func(c config.Config) any { return c.DataDir },
				},
				{
					Key: "system.web_dir", Label: "前端目录", Kind: KindReadOnly, Scope: ScopeDeploy,
					Env: "RESEARCHOS_WEB_DIR", Help: "静态产物目录，由服务同源托管。",
					Read: func(c config.Config) any { return c.WebDir },
					Get:  func(c config.Config) any { return c.WebDir },
				},
				{
					Key: "system.local_user", Label: "身份", Kind: KindReadOnly, Scope: ScopeDeploy,
					Env: "RESEARCHOS_LOCAL_USER", Help: "单用户本地模式下的服务端身份。",
					Read: func(c config.Config) any { return c.LocalUser },
					Get:  func(c config.Config) any { return c.LocalUser },
				},
			},
		},
	}
}

// Find 按 key 查字段。
func Find(key string) (Field, bool) {
	for _, s := range Sections() {
		for _, f := range s.Fields {
			if f.Key == key {
				return f, true
			}
		}
	}
	return Field{}, false
}

// SectionByID 按分组 ID 查分组。
func SectionByID(id string) (Section, bool) {
	for _, s := range Sections() {
		if s.ID == id {
			return s, true
		}
	}
	return Section{}, false
}

// EnvLookup 返回某字段对应的环境变量名。
func EnvLookup(f Field) string { return f.Env }

// Validate 校验并归一化一个待写入的值。
//
// 返回的 error 一律是给人看的中文说明——前端直接展示，不再二次翻译。
func Validate(f Field, raw any) (any, error) {
	switch f.Kind {
	case KindSecret, KindString:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 需要一个字符串", f.Label)
		}
		s = strings.TrimSpace(s)
		if f.Kind == KindString && f.Key == "model.base_url" && s != "" {
			if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
				return nil, fmt.Errorf("Base URL 需要以 http:// 或 https:// 开头")
			}
		}
		return s, nil
	case KindEnum:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 需要一个字符串", f.Label)
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if len(f.Options) > 0 && !contains(f.Options, s) {
			return nil, fmt.Errorf("%s 只能是 %s 之一", f.Label, strings.Join(f.Options, " / "))
		}
		return s, nil
	case KindInt:
		n, ok := toInt(raw)
		if !ok {
			return nil, fmt.Errorf("%s 需要一个整数", f.Label)
		}
		if n < f.Min || n > f.Max {
			return nil, fmt.Errorf("%s 必须在 %d 到 %d 之间", f.Label, f.Min, f.Max)
		}
		return n, nil
	case KindBool:
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("%s 需要 true 或 false", f.Label)
		}
		return b, nil
	case KindDuration:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s 需要一个时长字符串，例如 90s", f.Label)
		}
		d, err := time.ParseDuration(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("%s 不是合法时长（示例：30s、5m、1h）", f.Label)
		}
		if d < time.Second || d > 24*time.Hour {
			return nil, fmt.Errorf("%s 需要在 1s 到 24h 之间", f.Label)
		}
		return d, nil
	case KindList:
		list, ok := toStringSlice(raw)
		if !ok {
			return nil, fmt.Errorf("%s 需要一个字符串数组", f.Label)
		}
		out := make([]string, 0, len(list))
		for _, item := range list {
			item = strings.ToLower(strings.TrimSpace(item))
			if item == "" {
				continue
			}
			if len(f.Options) > 0 && !contains(f.Options, item) {
				return nil, fmt.Errorf("%s 含未知选项 %q", f.Label, item)
			}
			if !contains(out, item) {
				out = append(out, item)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s 至少要选一项", f.Label)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s 不支持写入", f.Label)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

func toStringSlice(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		return list, true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}
