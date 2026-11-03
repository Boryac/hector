package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	if got := Mask(""); got != "" {
		t.Fatalf("空值应返回空串，实际 %q", got)
	}
	if got := Mask("short"); got != "****" {
		t.Fatalf("短密钥应完全遮蔽，实际 %q", got)
	}
	long := "sk-abcdefghijklmnop"
	got := Mask(long)
	if got == long {
		t.Fatal("长密钥不应原样返回")
	}
	if !strings.Contains(got, "****") {
		t.Fatalf("应包含遮蔽标记，实际 %q", got)
	}
	if strings.Contains(got, "defghij") {
		t.Fatalf("中段不应泄露，实际 %q", got)
	}
}

func TestRedactDSN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"postgres://user:secret@host:5432/db", "postgres://user:****@host:5432/db"},
		{"postgres://user@host/db", "postgres://user@host/db"},
		{"not-a-dsn", "not-a-dsn"},
	}
	for _, c := range cases {
		if got := RedactDSN(c.in); got != c.want {
			t.Fatalf("RedactDSN(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
	if strings.Contains(RedactDSN("postgres://u:topsecret@h/d"), "topsecret") {
		t.Fatal("DSN 密码不得出现在脱敏结果中")
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	c := &Config{
		Addr:         "127.0.0.1:8080",
		DatabaseURL:  "postgres://u:dbsecret@127.0.0.1:5432/x",
		LLMAPIKey:    "sk-llmsecretvalue1234",
		TavilyAPIKey: "tvly-secret-value",
		SerpAPIKey:   "serp-secret",
		GitHubToken:  "ghp_supersecrettoken",
		Workers:      4,
	}
	out := c.Redacted()
	blob := ""
	for k, v := range out {
		blob += k + "=" + toString(v) + ";"
	}
	for _, secret := range []string{"dbsecret", "llmsecretvalue", "tvly-secret-value", "serp-secret", "ghp_supersecrettoken"} {
		if strings.Contains(blob, secret) {
			t.Fatalf("脱敏结果泄露 %q：%s", secret, blob)
		}
	}
	if !strings.Contains(blob, "127.0.0.1:8080") {
		t.Fatal("非敏感字段应保留")
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return string(rune('0' + x%10))
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []string:
		return strings.Join(x, ",")
	default:
		return ""
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("RESEARCHOS_DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("缺少 DATABASE_URL 应报错")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("RESEARCHOS_DATABASE_URL", "postgres://u@h/db")
	t.Setenv("RESEARCHOS_WORKERS", "0")
	t.Setenv("RESEARCHOS_MAX_AGENTS", "-1")
	t.Setenv("RESEARCHOS_SEARCH_RESULTS", "0")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Workers < 1 || c.MaxAgents < 1 || c.SearchResults < 1 {
		t.Fatalf("非法数值应回落到安全默认值：workers=%d agents=%d results=%d", c.Workers, c.MaxAgents, c.SearchResults)
	}
}

func TestLLMEnabled(t *testing.T) {
	c := &Config{LLMProvider: LLMNone}
	if c.LLMEnabled() {
		t.Fatal("provider=none 时不应视为可用")
	}
	c = &Config{LLMProvider: "openai"}
	if c.LLMEnabled() {
		t.Fatal("缺少 API Key 时不应视为可用")
	}
	c = &Config{LLMProvider: "openai", LLMAPIKey: "k"}
	if !c.LLMEnabled() {
		t.Fatal("provider 与 key 齐备时应可用")
	}
}

func TestPlannerMode(t *testing.T) {
	withLLM := &Config{LLMProvider: "openai", LLMAPIKey: "k"}
	if withLLM.PlannerMode() != "llm" {
		t.Fatalf("应返回 llm，实际 %q", withLLM.PlannerMode())
	}
	noLLM := &Config{LLMProvider: LLMNone}
	if noLLM.PlannerMode() != "heuristic" {
		t.Fatalf("应返回 heuristic，实际 %q", noLLM.PlannerMode())
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.test")
	content := "# 注释\nRESEARCHOS_TEST_A=value1\nexport RESEARCHOS_TEST_B=\"带 空格 的值\"\nRESEARCHOS_TEST_EMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("RESEARCHOS_TEST_A"); got != "value1" {
		t.Fatalf("A 解析错误：%q", got)
	}
	if got := os.Getenv("RESEARCHOS_TEST_B"); got != "带 空格 的值" {
		t.Fatalf("B 引号未剥离：%q", got)
	}
	if v, ok := os.LookupEnv("RESEARCHOS_TEST_EMPTY"); !ok || v != "" {
		t.Fatalf("空值应被设置，实际 %q %v", v, ok)
	}
}

func TestLoadEnvFileDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.test")
	if err := os.WriteFile(path, []byte("RESEARCHOS_TEST_C=fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RESEARCHOS_TEST_C", "fromenv")
	if err := LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("RESEARCHOS_TEST_C"); got != "fromenv" {
		t.Fatalf("环境变量应优先，实际 %q", got)
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Fatal("缺失文件应返回错误")
	}
}
