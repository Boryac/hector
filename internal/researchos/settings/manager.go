package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/llm"
	"github.com/verikod/hector/internal/researchos/planner"
	"github.com/verikod/hector/internal/researchos/search"
	"github.com/verikod/hector/internal/researchos/secrets"
)

var (
	// ErrDeployField 表示试图通过 Web 修改部署级参数。
	ErrDeployField = errors.New("部署级参数只能通过环境变量设置，界面不提供修改")
	// ErrUnknownSetting 表示提交了未定义的配置键。
	ErrUnknownSetting = errors.New("未知的配置项")
)

// StoredValue 是数据库里的一行设置（已解码为 Go 值）。
type StoredValue struct {
	Value     any
	Encrypted bool
}

// Backend 是设置持久化的最小依赖，便于用假实现跑单测而不连数据库。
type Backend interface {
	LoadSettings(ctx context.Context) (map[string]StoredValue, error)
	SaveSetting(ctx context.Context, key, section string, value any, encrypted bool, updatedBy string) error
	DeleteSetting(ctx context.Context, key string) error
}

// Runtime 是「当前真正生效」的配置快照。
//
// Cfg 是值拷贝：调用方拿走一份就不会读到别人正在改的中间态。
// LLM / Search / Planner 构造后不再变更，可以安全共享。
type Runtime struct {
	Cfg           config.Config
	LLM           llm.Client
	Search        *search.Engine
	Planner       planner.Planner
	SearchUnknown []string
	LoadedAt      time.Time
	Revision      int64
}

// Manager 负责设置读取、校验、持久化与生效。
type Manager struct {
	mu       sync.Mutex
	backend  Backend
	box      *secrets.Box
	keySrc   string
	envBase  config.Config // 环境变量基线（部署级唯一来源）
	applied  config.Config // 已生效基线（env + 需要重启的 Web 值）
	overlay  map[string]any
	rev      int64
	lastErr  string
	runtimeV atomic.Pointer[Runtime]
}

// NewManager 创建管理器。box 允许为 nil：此时敏感项不可写，但页面仍能打开
// （敏感项显示为「未配置」），不会因为缺少主密钥把整个设置中心拖死。
func NewManager(backend Backend, box *secrets.Box, keySource string, envBase config.Config) *Manager {
	m := &Manager{
		backend: backend, box: box, keySrc: keySource,
		envBase: envBase, applied: envBase, overlay: map[string]any{},
	}
	m.rebuildRuntime()
	return m
}

// KeySource 返回主密钥来源，供系统设置页展示。
func (m *Manager) KeySource() string { return m.keySrc }

// SecretsAvailable 表示敏感配置是否可写。
func (m *Manager) SecretsAvailable() bool { return m.box != nil }

// Runtime 返回当前生效快照（永不为 nil）。
func (m *Manager) Runtime() *Runtime { return m.runtimeV.Load() }

// Reload 从数据库重新装载设置并重建运行时。
//
// 服务启动时调用一次。「重启后生效」的验证就是再调一次它，
// 因此这里必须完整走 overlay 解析与客户端重建，不能抄近路。
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored, err := m.backend.LoadSettings(ctx)
	if err != nil {
		m.lastErr = err.Error()
		return fmt.Errorf("读取设置失败: %w", err)
	}
	overlay := make(map[string]any, len(stored))
	var problems []string
	for key, sv := range stored {
		f, ok := Find(key)
		if !ok {
			// 历史遗留键（旧版 max_agents 之类）不再映射到任何字段：忽略，但不删。
			continue
		}
		v := sv.Value
		if f.Kind == KindSecret {
			if m.box == nil {
				problems = append(problems, "没有主密钥，无法解密 "+key)
				continue
			}
			s, _ := v.(string)
			plain, err := m.box.Open(s)
			if err != nil {
				problems = append(problems, fmt.Sprintf("解密 %s 失败: %v", key, err))
				continue
			}
			v = plain
		}
		coerced, err := coerceStored(f, v)
		if err != nil {
			problems = append(problems, fmt.Sprintf("解析 %s 失败: %v", key, err))
			continue
		}
		overlay[key] = coerced
	}
	if len(problems) > 0 {
		m.lastErr = strings.Join(problems, "；")
	} else {
		m.lastErr = ""
	}
	m.overlay = overlay
	m.applied = m.composeConfig(true)
	m.rebuildRuntime()
	return nil
}

// composeConfig 叠加配置。includeRestart=false 时只叠加即时生效项，
// 需要重启的项保持上次启动时的值——否则界面会显示「已改」而实际没生效。
func (m *Manager) composeConfig(includeRestart bool) config.Config {
	cfg := m.envBase
	for key, v := range m.overlay {
		f, ok := Find(key)
		if !ok || f.Set == nil {
			continue
		}
		if !includeRestart && f.Scope == ScopeRestart {
			continue
		}
		f.Set(&cfg, v)
	}
	return cfg
}

func (m *Manager) rebuildRuntime() {
	cfg := m.applied
	engine, unknown := search.NewDefault(cfg.SearchProviders, search.ProviderOptions{
		UserAgent: cfg.UserAgent, Timeout: cfg.SearchTimeout,
		TavilyKey: cfg.TavilyAPIKey, SerpAPIKey: cfg.SerpAPIKey,
		BraveKey: cfg.BraveAPIKey, GitHubToken: cfg.GitHubToken,
	})
	client := llm.New(llm.Config{
		Provider: string(cfg.LLMProvider), APIKey: cfg.LLMAPIKey,
		Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL, Timeout: cfg.LLMTimeout,
	})
	m.rev++
	m.runtimeV.Store(&Runtime{
		Cfg: cfg, LLM: client, Search: engine, Planner: planner.New(client),
		SearchUnknown: unknown, LoadedAt: time.Now(), Revision: m.rev,
	})
}

// FieldView 是发给前端的一项配置。
type FieldView struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Help       string   `json:"help"`
	Kind       Kind     `json:"kind"`
	Scope      Scope    `json:"scope"`
	Env        string   `json:"env"`
	Value      any      `json:"value"`
	Source     string   `json:"source"`
	Configured bool     `json:"configured"`
	Default    any      `json:"default,omitempty"`
	Min        int      `json:"min,omitempty"`
	Max        int      `json:"max,omitempty"`
	Options    []string `json:"options,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Editable   bool     `json:"editable"`
}

// SectionView 是一组配置。
type SectionView struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Fields      []FieldView `json:"fields"`
}

// SecretsInfo 说明密钥保护状态。
type SecretsInfo struct {
	Available bool   `json:"available"`
	KeySource string `json:"key_source"`
	Cipher    string `json:"cipher"`
}

// SnapshotData 是 GET /settings 的返回体。
type SnapshotData struct {
	Sections       []SectionView `json:"sections"`
	Secrets        SecretsInfo   `json:"secrets"`
	Revision       int64         `json:"revision"`
	LastError      string        `json:"last_error,omitempty"`
	Precedence     string        `json:"precedence"`
	RestartNeeded  bool          `json:"restart_needed"`
	PendingRestart []string      `json:"pending_restart,omitempty"`
}

// Snapshot 组装当前设置视图。敏感项只返回掩码，绝不返回明文。
func (m *Manager) Snapshot() SnapshotData {
	m.mu.Lock()
	defer m.mu.Unlock()

	pending := m.pendingRestartKeys()
	sections := Sections()
	views := make([]SectionView, 0, len(sections))
	for _, s := range sections {
		fv := make([]FieldView, 0, len(s.Fields))
		for _, f := range s.Fields {
			fv = append(fv, m.viewOf(f))
		}
		views = append(views, SectionView{ID: s.ID, Title: s.Title, Description: s.Description, Fields: fv})
	}
	return SnapshotData{
		Sections: views,
		Secrets: SecretsInfo{
			Available: m.box != nil,
			KeySource: m.keySrc,
			Cipher:    "AES-256-GCM",
		},
		Revision:       m.rev,
		LastError:      m.lastErr,
		Precedence:     "Web 保存值 > 环境变量 > 内置默认；部署级参数只认环境变量",
		RestartNeeded:  len(pending) > 0,
		PendingRestart: pending,
	}
}

func (m *Manager) viewOf(f Field) FieldView {
	v := FieldView{
		Key: f.Key, Label: f.Label, Help: f.Help, Kind: f.Kind, Scope: f.Scope, Env: f.Env,
		Default: f.Default, Min: f.Min, Max: f.Max, Options: f.Options, Unit: f.Unit,
		Editable: f.Scope != ScopeDeploy && f.Kind != KindReadOnly,
	}

	// deploy：只从环境变量读，永远不可写。
	if f.Scope == ScopeDeploy {
		if f.Read != nil {
			v.Value = f.Read(m.envBase)
		}
		v.Source = "env"
		if mp, ok := v.Value.(map[string]any); ok {
			v.Configured = mp["configured"] == true
		}
		return v
	}

	webVal, hasWeb := m.overlay[f.Key]
	envVal := any(nil)
	if f.Get != nil {
		envVal = f.Get(m.envBase)
	}

	var effective any
	switch {
	case hasWeb:
		effective = webVal
		v.Source = "web"
	case envSet(envVal):
		effective = envVal
		v.Source = "env"
	default:
		effective = f.Default
		v.Source = "default"
	}

	if f.Kind == KindSecret {
		s, _ := effective.(string)
		v.Configured = strings.TrimSpace(s) != ""
		v.Value = secrets.Mask(s)
	} else {
		v.Value = effective
	}
	return v
}

// pendingRestartKeys 列出「Web 已改、但还在等重启」的键。
func (m *Manager) pendingRestartKeys() []string {
	out := make([]string, 0)
	for key, v := range m.overlay {
		f, ok := Find(key)
		if !ok || f.Scope != ScopeRestart || f.Set == nil || f.Get == nil {
			continue
		}
		applied := m.applied
		f.Set(&applied, v)
		if !sameValue(f.Get(applied), f.Get(m.applied)) {
			out = append(out, key)
		}
	}
	return out
}

// envSet 判断环境变量是否真的给了值（布尔/int 的零值也可能是显式设置）。
func envSet(envVal any) bool {
	switch v := envVal.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case nil:
		return false
	case []string:
		return len(v) > 0
	default:
		return true
	}
}

func sameValue(a, b any) bool { return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) }

// PatchResult 是保存后的反馈。
type PatchResult struct {
	Applied         map[string]any `json:"applied"`
	Cleared         []string       `json:"cleared,omitempty"`
	RequiresRestart []string       `json:"requires_restart,omitempty"`
	Warnings        []string       `json:"warnings,omitempty"`
	HotApplied      bool           `json:"hot_applied"`
	Revision        int64          `json:"revision"`
	Message         string         `json:"message"`
}

// Patch 校验并保存一组设置，随后重建运行时。
func (m *Manager) Patch(ctx context.Context, section string, patch map[string]any) (*PatchResult, error) {
	sec, ok := SectionByID(section)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownSetting, section)
	}
	allowed := map[string]Field{}
	for _, f := range sec.Fields {
		allowed[f.Key] = f
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	res := &PatchResult{Applied: map[string]any{}}
	// 先整体校验，避免「前三个字段已写库、第四个报错」的半成功状态。
	type pendingWrite struct {
		field     Field
		value     any
		encrypted bool
	}
	writes := make([]pendingWrite, 0, len(patch))
	deletes := make([]string, 0)

	for key, raw := range patch {
		f, ok := allowed[key]
		if !ok {
			if _, exists := Find(key); exists {
				return nil, fmt.Errorf("%w: %s 不属于「%s」分组", ErrUnknownSetting, key, sec.Title)
			}
			return nil, fmt.Errorf("%w: %s", ErrUnknownSetting, key)
		}
		if f.Scope == ScopeDeploy || f.Kind == KindReadOnly {
			return nil, fmt.Errorf("%w: %s", ErrDeployField, f.Label)
		}

		if s, _ := raw.(string); strings.TrimSpace(s) == "__clear__" {
			deletes = append(deletes, f.Key)
			continue
		}

		if f.Kind == KindSecret {
			s, _ := raw.(string)
			// 空串、纯星号（*** / ********）、带掩码格式（sk-a****z）都表示
			// 「用户没动这一项」——把它们当新密钥存进去，等于把占位符写成了真凭据。
			if strings.TrimSpace(s) == "" || strings.Contains(s, "****") || strings.Trim(s, "* ") == "" {
				continue
			}
			if m.box == nil {
				return nil, fmt.Errorf("没有可用主密钥，无法加密保存 %s", f.Label)
			}
		}

		v, err := Validate(f, raw)
		if err != nil {
			return nil, err
		}
		writes = append(writes, pendingWrite{field: f, value: v, encrypted: f.Kind == KindSecret})
	}

	for _, key := range deletes {
		if err := m.backend.DeleteSetting(ctx, key); err != nil {
			return nil, fmt.Errorf("清除设置失败: %w", err)
		}
		delete(m.overlay, key)
		res.Cleared = append(res.Cleared, key)
	}
	for _, w := range writes {
		saved := w.value
		if w.encrypted {
			sealed, err := m.box.Seal(w.value.(string))
			if err != nil {
				return nil, fmt.Errorf("加密失败: %w", err)
			}
			saved = sealed
		}
		if err := m.backend.SaveSetting(ctx, w.field.Key, sec.ID, saved, w.encrypted, "local-user"); err != nil {
			return nil, fmt.Errorf("保存失败: %w", err)
		}
		m.overlay[w.field.Key] = w.value
		if w.encrypted {
			// 保存响应同样不能回显明文：它会进浏览器控制台、进日志、进截图。
			res.Applied[w.field.Key] = secrets.Mask(w.value.(string))
		} else {
			res.Applied[w.field.Key] = w.value
		}
	}

	// 即时生效项重建运行时；需要重启项只记录待生效。
	// 清除同样是一次变更：删掉 Key 之后运行时必须立刻看不到它，
	// 否则会出现「界面显示已清除、模型却还在用旧 Key」的状态。
	hot := false
	for _, w := range writes {
		if w.field.Scope == ScopeHot {
			hot = true
		} else {
			res.RequiresRestart = append(res.RequiresRestart, w.field.Key)
		}
	}
	for _, key := range deletes {
		if f, ok := Find(key); ok && f.Scope == ScopeHot {
			hot = true
		}
	}
	if hot {
		m.applied = m.composeConfig(false)
		m.rebuildRuntime()
	}

	// 一致性提醒：不阻断保存，但要说清楚现在还不能用。
	if rt := m.runtimeV.Load(); rt != nil {
		if rt.Cfg.LLMProvider != config.LLMNone && strings.TrimSpace(rt.Cfg.LLMAPIKey) == "" {
			res.Warnings = append(res.Warnings, "已选择 LLM Provider 但未配置 API Key，规划与抽取仍会降级为本地流程")
		}
		if len(rt.Search.AvailableNames()) == 0 {
			res.Warnings = append(res.Warnings, "当前没有任何可用的搜索 Provider，检索会失败")
		}
	}

	res.HotApplied = hot
	res.Revision = m.rev
	switch {
	case len(res.RequiresRestart) > 0:
		res.Message = "已保存。含需要重启的项，重启服务后生效。"
	case hot:
		res.Message = "已保存并即时生效。"
	default:
		res.Message = "没有需要变更的项。"
	}
	return res, nil
}

func coerceStored(f Field, v any) (any, error) {
	switch f.Kind {
	case KindInt:
		switch n := v.(type) {
		case float64:
			return int(n), nil
		case int:
			return n, nil
		case json.Number:
			i, err := n.Int64()
			return int(i), err
		}
		return nil, fmt.Errorf("期望整数，实际 %T", v)
	case KindBool:
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("期望布尔值，实际 %T", v)
	case KindList:
		switch list := v.(type) {
		case []string:
			return list, nil
		case []any:
			out := make([]string, 0, len(list))
			for _, item := range list {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("期望字符串数组")
				}
				out = append(out, s)
			}
			return out, nil
		}
		return nil, fmt.Errorf("期望字符串数组，实际 %T", v)
	default:
		if s, ok := v.(string); ok {
			return s, nil
		}
		return nil, fmt.Errorf("期望字符串，实际 %T", v)
	}
}

// StoreBackend 用函数适配 *store.Store，避免 settings 直接依赖 store 包。
type StoreBackend struct {
	Load func(ctx context.Context) (map[string]StoredValue, error)
	Save func(ctx context.Context, key, section string, value any, encrypted bool, updatedBy string) error
	Del  func(ctx context.Context, key string) error
}

// LoadSettings 实现 Backend。
func (b StoreBackend) LoadSettings(ctx context.Context) (map[string]StoredValue, error) {
	return b.Load(ctx)
}

// SaveSetting 实现 Backend。
func (b StoreBackend) SaveSetting(ctx context.Context, key, section string, value any, encrypted bool, updatedBy string) error {
	return b.Save(ctx, key, section, value, encrypted, updatedBy)
}

// DeleteSetting 实现 Backend。
func (b StoreBackend) DeleteSetting(ctx context.Context, key string) error { return b.Del(ctx, key) }

var _ Backend = StoreBackend{}
