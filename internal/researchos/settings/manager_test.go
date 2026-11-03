package settings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/secrets"
)

// fakeBackend 用内存 map 模拟 app_settings，记录写入内容以便断言「密文入库」。
type fakeBackend struct {
	mu     sync.Mutex
	rows   map[string]StoredValue
	saves  []string // 每次 SaveSetting 收到的原始值（用于验证是否密文）
	failOn string
}

func newFakeBackend() *fakeBackend { return &fakeBackend{rows: map[string]StoredValue{}} }

func (f *fakeBackend) LoadSettings(ctx context.Context) (map[string]StoredValue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]StoredValue, len(f.rows))
	for k, v := range f.rows {
		out[k] = v
	}
	return out, nil
}

func (f *fakeBackend) SaveSetting(ctx context.Context, key, section string, value any, encrypted bool, updatedBy string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == key {
		return errors.New("模拟写库失败")
	}
	f.rows[key] = StoredValue{Value: value, Encrypted: encrypted}
	if s, ok := value.(string); ok {
		f.saves = append(f.saves, s)
	}
	return nil
}

func (f *fakeBackend) DeleteSetting(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, key)
	return nil
}

func (f *fakeBackend) raw(key string) StoredValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows[key]
}

func testBox(t *testing.T) *secrets.Box {
	t.Helper()
	key := make([]byte, secrets.KeySize)
	for i := range key {
		key[i] = byte(i * 7)
	}
	box, err := secrets.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// envBase 构造一份确定的「环境变量基线」。不用 config.Load()：
// 它会因为缺少 DATABASE_URL 直接报错，而设置逻辑本身与数据库无关。
func envBase() config.Config {
	return config.Config{
		Addr:            "127.0.0.1:8080",
		DatabaseURL:     "postgres://postgres@127.0.0.1:55432/researchos?sslmode=disable",
		APIToken:        "test-token",
		LocalUser:       "local-user",
		LLMProvider:     config.LLMNone,
		LLMTimeout:      90 * time.Second,
		SearchProviders: []string{"bing", "github", "wikipedia"},
		SearchTimeout:   30 * time.Second,
		SearchResults:   6,
		FetchTimeout:    25 * time.Second,
		FetchMaxBytes:   3_000_000,
		RespectRobots:   true,
		UserAgent:       "HectorResearchOS/test",
		DefaultDepth:    "standard",
		RequireApproval: true,
		Workers:         4,
		MaxAgents:       4,
		NodeTimeout:     180 * time.Second,
		TaskTimeout:     1800 * time.Second,
		NodeMaxAttempts: 3,
		DataDir:         "data",
		WebDir:          "",
	}
}

func newTestManager(t *testing.T) (*Manager, *fakeBackend) {
	t.Helper()
	be := newFakeBackend()
	base := envBase()
	// 固定基线，避免测试受本机环境变量影响。
	base.MaxAgents = 4
	base.Workers = 4
	base.LLMProvider = config.LLMNone
	base.LLMAPIKey = ""
	base.LLMModel = ""
	base.LLMBaseURL = ""
	base.SearchProviders = []string{"bing", "github", "wikipedia"}
	base.SearchResults = 6
	base.RequireApproval = true
	base.DefaultDepth = "standard"
	base.RespectRobots = true
	m := NewManager(be, testBox(t), "test", base)
	if err := m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m, be
}

func toIntLoose(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	}
	return -1
}

func fieldOf(t *testing.T, snap SnapshotData, key string) FieldView {
	t.Helper()
	for _, s := range snap.Sections {
		for _, f := range s.Fields {
			if f.Key == key {
				return f
			}
		}
	}
	t.Fatalf("未找到字段 %s", key)
	return FieldView{}
}

// 未保存任何 Web 值时，来源应标注为 env 或 default，而不是含糊的「已配置」。
func TestSnapshotReportsSource(t *testing.T) {
	m, _ := newTestManager(t)
	snap := m.Snapshot()

	if f := fieldOf(t, snap, "execution.max_agents"); f.Source != "env" {
		t.Fatalf("max_agents 应来自 env，实际 %s", f.Source)
	}
	if f := fieldOf(t, snap, "model.api_key"); f.Source != "default" || f.Configured {
		t.Fatalf("未配置的 Key 应为 default 且 configured=false，实际 %s/%v", f.Source, f.Configured)
	}
	if f := fieldOf(t, snap, "system.addr"); f.Editable || f.Scope != ScopeDeploy {
		t.Fatal("部署级参数必须不可编辑")
	}
	if !strings.Contains(snap.Precedence, "环境变量") {
		t.Fatal("返回体必须说明优先级")
	}
}

// Web 值必须覆盖环境变量，否则「界面保存了但重启还是老配置」。
func TestWebValueOverridesEnv(t *testing.T) {
	m, _ := newTestManager(t)
	base := envBase()
	base.MaxAgents = 4
	m2 := NewManager(newFakeBackend(), testBox(t), "test", base)
	if err := m2.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m2.Runtime().Cfg.MaxAgents != 4 {
		t.Fatalf("基线应是 4，实际 %d", m2.Runtime().Cfg.MaxAgents)
	}
	if _, err := m2.Patch(context.Background(), "execution", map[string]any{"execution.max_agents": 6}); err != nil {
		t.Fatal(err)
	}
	if got := m2.Runtime().Cfg.MaxAgents; got != 6 {
		t.Fatalf("Web 值应覆盖环境变量，实际 %d", got)
	}
	if f := fieldOf(t, m2.Snapshot(), "execution.max_agents"); f.Source != "web" {
		t.Fatalf("保存后来源应为 web，实际 %s", f.Source)
	}
	_ = m
}

// 敏感值：入库必须是密文，接口只回掩码，明文一次都不能出现。
func TestSecretIsEncryptedAndMasked(t *testing.T) {
	m, be := newTestManager(t)
	plain := "sk-proj-verysecretvalue-0123456789"
	if _, err := m.Patch(context.Background(), "model", map[string]any{"model.api_key": plain}); err != nil {
		t.Fatal(err)
	}

	row := be.raw("model.api_key")
	if !row.Encrypted {
		t.Fatal("敏感项应标记 encrypted")
	}
	stored, _ := row.Value.(string)
	if !secrets.IsEncrypted(stored) {
		t.Fatalf("入库值应是密文，实际 %q", stored)
	}
	if strings.Contains(stored, plain) {
		t.Fatal("入库值不得包含明文")
	}

	snap := m.Snapshot()
	f := fieldOf(t, snap, "model.api_key")
	if !f.Configured {
		t.Fatal("保存后 configured 应为 true")
	}
	masked, _ := f.Value.(string)
	if masked == plain || strings.Contains(masked, "verysecretvalue") {
		t.Fatalf("接口返回了明文或可推断的掩码: %q", masked)
	}
	// 整个响应序列化后也不能出现明文。
	blob, _ := json.Marshal(snap)
	if strings.Contains(string(blob), plain) || strings.Contains(string(blob), "verysecretvalue") {
		t.Fatal("GET /settings 响应泄露了密钥")
	}
	// 但运行时应拿到明文（否则调不通模型）。
	if m.Runtime().Cfg.LLMAPIKey != plain {
		t.Fatal("运行时必须拿到明文 Key")
	}
}

// 前端把掩码原样回传表示「没改」，不能把掩码当成新 Key 存进去。
func TestMaskedValueKeepsExistingSecret(t *testing.T) {
	m, be := newTestManager(t)
	plain := "sk-abcdefghijklmnop"
	if _, err := m.Patch(context.Background(), "model", map[string]any{"model.api_key": plain}); err != nil {
		t.Fatal(err)
	}
	before := be.raw("model.api_key").Value

	for _, echo := range []string{"***", "********", "   "} {
		if _, err := m.Patch(context.Background(), "model", map[string]any{"model.api_key": echo}); err != nil {
			t.Fatalf("回传 %q 不应报错: %v", echo, err)
		}
	}
	if after := be.raw("model.api_key").Value; after != before {
		t.Fatalf("回传掩码不应改写密文：%v → %v", before, after)
	}
	if m.Runtime().Cfg.LLMAPIKey != plain {
		t.Fatal("掩码回传后运行时 Key 不应变化")
	}
}

func TestSecretCanBeCleared(t *testing.T) {
	m, be := newTestManager(t)
	if _, err := m.Patch(context.Background(), "model", map[string]any{"model.api_key": "sk-abcdefghijklmnop"}); err != nil {
		t.Fatal(err)
	}
	res, err := m.Patch(context.Background(), "model", map[string]any{"model.api_key": "__clear__"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Cleared) != 1 {
		t.Fatalf("应记录清除项，实际 %v", res.Cleared)
	}
	if _, ok := be.rows["model.api_key"]; ok {
		t.Fatal("清除后数据库不应还有该键")
	}
	if m.Runtime().Cfg.LLMAPIKey != "" {
		t.Fatal("清除后运行时 Key 应为空")
	}
}

// 部署级参数任何写入都必须被拒绝，且不能落库。
func TestDeployFieldRejected(t *testing.T) {
	m, be := newTestManager(t)
	cases := []struct {
		section string
		key     string
		value   any
	}{
		{"system", "system.addr", ":9999"},
		{"system", "system.database", "postgres://evil"},
		{"system", "system.api_token", "x"},
		{"system", "system.data_dir", "/tmp"},
	}
	for _, c := range cases {
		_, err := m.Patch(context.Background(), c.section, map[string]any{c.key: c.value})
		if !errors.Is(err, ErrDeployField) {
			t.Fatalf("%s 应被拒绝，实际 err=%v", c.key, err)
		}
		if _, exists := be.rows[c.key]; exists {
			t.Fatalf("%s 不应落库", c.key)
		}
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	m, _ := newTestManager(t)
	if _, err := m.Patch(context.Background(), "model", map[string]any{"model.not_a_field": 1}); !errors.Is(err, ErrUnknownSetting) {
		t.Fatalf("未知键应报 ErrUnknownSetting，实际 %v", err)
	}
	if _, err := m.Patch(context.Background(), "nope", map[string]any{}); !errors.Is(err, ErrUnknownSetting) {
		t.Fatalf("未知分组应报 ErrUnknownSetting，实际 %v", err)
	}
	// 键存在但放错分组，也要拒绝，避免前端串组提交。
	if _, err := m.Patch(context.Background(), "model", map[string]any{"execution.max_agents": 5}); !errors.Is(err, ErrUnknownSetting) {
		t.Fatalf("跨分组键应被拒绝，实际 %v", err)
	}
}

func TestValidationMessagesAreActionable(t *testing.T) {
	m, _ := newTestManager(t)
	cases := []struct {
		name    string
		section string
		patch   map[string]any
		expect  string
	}{
		{"整数越界", "execution", map[string]any{"execution.max_agents": 99}, "1 到 8"},
		{"类型错误", "execution", map[string]any{"execution.max_agents": "abc"}, "整数"},
		{"时长格式", "execution", map[string]any{"execution.node_timeout": "3 分钟"}, "合法时长"},
		{"时长越界", "execution", map[string]any{"execution.task_timeout": "48h"}, "1s 到 24h"},
		{"枚举非法", "model", map[string]any{"model.provider": "gemini"}, "只能是"},
		{"列表含未知项", "search", map[string]any{"search.providers": []any{"bing", "duckduckgo"}}, "未知选项"},
		{"列表为空", "search", map[string]any{"search.providers": []any{}}, "至少要选一项"},
		{"布尔类型", "execution", map[string]any{"execution.require_approval": "yes"}, "true 或 false"},
		{"URL 协议", "model", map[string]any{"model.base_url": "ftp://x"}, "http"},
	}
	for _, c := range cases {
		_, err := m.Patch(context.Background(), c.section, c.patch)
		if err == nil {
			t.Fatalf("%s：应报错", c.name)
		}
		if !strings.Contains(err.Error(), c.expect) {
			t.Fatalf("%s：错误信息应包含 %q，实际 %q", c.name, c.expect, err.Error())
		}
	}
}

// 一批里只要有一项非法，整批都不落库——避免半成功的配置。
func TestPatchIsAllOrNothing(t *testing.T) {
	m, be := newTestManager(t)
	_, err := m.Patch(context.Background(), "execution", map[string]any{
		"execution.max_agents":   5,
		"execution.node_timeout": "120s",
		"execution.task_timeout": "not-a-duration",
	})
	if err == nil {
		t.Fatal("含非法项应整批失败")
	}
	if len(be.rows) != 0 {
		t.Fatalf("失败时不应写入任何行，实际 %v", be.rows)
	}
	if m.Runtime().Cfg.MaxAgents != 4 {
		t.Fatal("失败时运行时不应被改动")
	}
}

// 即时生效项改完立刻反映到 Runtime；这是「不用重启」的硬证据。
func TestHotSettingsApplyImmediately(t *testing.T) {
	m, _ := newTestManager(t)
	res, err := m.Patch(context.Background(), "execution", map[string]any{
		"execution.max_agents":       7,
		"execution.require_approval": false,
		"execution.default_depth":    "deep",
		"execution.node_timeout":     "45s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HotApplied || len(res.RequiresRestart) != 0 {
		t.Fatalf("这批应全部即时生效，实际 %+v", res)
	}
	rt := m.Runtime()
	if rt.Cfg.MaxAgents != 7 || rt.Cfg.RequireApproval || rt.Cfg.DefaultDepth != "deep" {
		t.Fatalf("热更新未生效: %+v", rt.Cfg)
	}
	if rt.Cfg.NodeTimeout != 45*time.Second {
		t.Fatalf("时长热更新未生效: %v", rt.Cfg.NodeTimeout)
	}
	if !strings.Contains(res.Message, "即时生效") {
		t.Fatalf("应提示已即时生效，实际 %q", res.Message)
	}
}

// 需要重启的项：保存后写库，但运行时保持不变；Reload 之后才生效。
// 这条测试就是「重启行为」的验收依据。
func TestRestartScopedSettingWaitsForReload(t *testing.T) {
	m, be := newTestManager(t)

	res, err := m.Patch(context.Background(), "execution", map[string]any{"execution.workers": 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RequiresRestart) != 1 || res.RequiresRestart[0] != "execution.workers" {
		t.Fatalf("应标注需重启，实际 %+v", res.RequiresRestart)
	}
	if res.HotApplied {
		t.Fatal("restart 作用域不应声称即时生效")
	}
	if !strings.Contains(res.Message, "重启") {
		t.Fatalf("应提示需要重启，实际 %q", res.Message)
	}
	if m.Runtime().Cfg.Workers != 4 {
		t.Fatalf("重启前运行时应保持 4，实际 %d", m.Runtime().Cfg.Workers)
	}
	if got := toIntLoose(be.raw("execution.workers").Value); got != 12 {
		t.Fatalf("数据库应已保存 12，实际 %v", be.raw("execution.workers").Value)
	}

	snap := m.Snapshot()
	if !snap.RestartNeeded || len(snap.PendingRestart) != 1 {
		t.Fatalf("快照应提示待重启，实际 %+v", snap.PendingRestart)
	}
	if f := fieldOf(t, snap, "execution.workers"); f.Source != "web" {
		t.Fatalf("待生效项的来源仍应显示 web，实际 %s", f.Source)
	}

	// 模拟重启：新建 Manager 并 Reload。
	m2 := NewManager(be, testBox(t), "test", m.envBase)
	if err := m2.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m2.Runtime().Cfg.Workers != 12 {
		t.Fatalf("重启后应生效为 12，实际 %d", m2.Runtime().Cfg.Workers)
	}
	if m2.Snapshot().RestartNeeded {
		t.Fatal("重启后不应再有待生效项")
	}
}

// 持久化：新建 Manager（等价于进程重启）后，Web 保存的值仍在。
func TestSettingsPersistAcrossReload(t *testing.T) {
	m, be := newTestManager(t)
	if _, err := m.Patch(context.Background(), "model", map[string]any{
		"model.provider": "openai",
		"model.model":    "gpt-4o-mini",
		"model.api_key":  "sk-persisted-key-123456",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Patch(context.Background(), "search", map[string]any{
		"search.providers": []any{"wikipedia", "bing"},
		"search.results":   9,
	}); err != nil {
		t.Fatal(err)
	}

	m2 := NewManager(be, testBox(t), "test", m.envBase)
	if err := m2.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt := m2.Runtime()
	if rt.Cfg.LLMProvider != config.LLMOpenAI || rt.Cfg.LLMModel != "gpt-4o-mini" {
		t.Fatalf("LLM 设置未持久化: %+v", rt.Cfg)
	}
	if rt.Cfg.LLMAPIKey != "sk-persisted-key-123456" {
		t.Fatal("加密的 Key 重启后应能解回明文")
	}
	if rt.Cfg.SearchResults != 9 || len(rt.Cfg.SearchProviders) != 2 {
		t.Fatalf("搜索设置未持久化: %+v", rt.Cfg)
	}
	if !rt.LLM.Available() {
		t.Fatal("有 Key 且 provider=openai 时客户端应可用")
	}
}

// 没有主密钥时不能静默把明文写库，必须明确拒绝。
func TestSecretWriteRefusedWithoutBox(t *testing.T) {
	be := newFakeBackend()
	m := NewManager(be, nil, "", envBase())
	if err := m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := m.Patch(context.Background(), "model", map[string]any{"model.api_key": "sk-abcdefghij"})
	if err == nil || !strings.Contains(err.Error(), "主密钥") {
		t.Fatalf("应拒绝并说明缺少主密钥，实际 %v", err)
	}
	if len(be.rows) != 0 {
		t.Fatal("不得降级成明文入库")
	}
}

// 数据库里的历史明文（改造前写入）仍要能读出来，不能升级即丢配置。
func TestLegacyPlaintextSettingStillLoads(t *testing.T) {
	be := newFakeBackend()
	be.rows["model.api_key"] = StoredValue{Value: "legacy-plain-key-123456", Encrypted: false}
	m := NewManager(be, testBox(t), "test", envBase())
	if err := m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Runtime().Cfg.LLMAPIKey != "legacy-plain-key-123456" {
		t.Fatal("历史明文值应能继续使用")
	}
}

// 选了 Provider 却没 Key、且没有任何可用搜索源时，保存要给出提醒而不是假装成功。
func TestPatchWarnsAboutIncompleteConfig(t *testing.T) {
	be := newFakeBackend()
	base := envBase()
	base.SearchProviders = []string{}
	base.LLMProvider = config.LLMNone
	base.LLMAPIKey = ""
	m := NewManager(be, testBox(t), "test", base)
	if err := m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := m.Patch(context.Background(), "model", map[string]any{"model.provider": "openai"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.Warnings, "；")
	if !strings.Contains(joined, "API Key") {
		t.Fatalf("应提醒缺少 API Key，实际 %v", res.Warnings)
	}
	if !strings.Contains(joined, "搜索 Provider") {
		t.Fatalf("应提醒没有可用搜索源，实际 %v", res.Warnings)
	}
}
