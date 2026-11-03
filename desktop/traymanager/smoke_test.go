package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestSmokeInitAndLifecycle 端到端冒烟（需本地环境，env 守卫）：
//   RESEARCHOS_SMOKE_DSN=postgres://... go test -run TestSmokeInitAndLifecycle ./...
// 覆盖：配置 DPAPI 加密落盘 → 重载解密 → 幂等迁移 → 后台启动 → 健康检查 → 优雅停止。
func TestSmokeInitAndLifecycle(t *testing.T) {
	dsn := os.Getenv("RESEARCHOS_SMOKE_DSN")
	if dsn == "" {
		t.Skip("未设置 RESEARCHOS_SMOKE_DSN，跳过（CI/无本地库时不跑）")
	}
	dataDir := t.TempDir()

	token, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	secret, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		Version: 1, Initialized: true, DataDir: dataDir, Addr: "127.0.0.1:18091",
		PgDSN: dsn, APIToken: token, SecretKey: secret,
	}
	if err := ProtectConfig(cfg); err != nil {
		t.Fatalf("DPAPI 加密失败: %v", err)
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}
	// 重载 + 解密（模拟托盘重启路径）
	loaded, err := LoadConfig()
	if err != nil || loaded == nil {
		t.Fatalf("重载配置失败: %v", err)
	}
	if err := UnprotectConfig(loaded); err != nil {
		t.Fatalf("DPAPI 解密失败: %v", err)
	}
	if loaded.PgDSN != dsn || loaded.APIToken != token {
		t.Fatal("解密后的字段与明文不一致")
	}
	if loaded.PgDSN == cfg.PgDSN {
		t.Fatal("密文字段意外与明文相同（DPAPI 未生效）")
	}

	// 幂等迁移
	if err := TestAndMigratePG(dsn); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 启动 → 健康 → 优雅停止
	m := NewManager(loaded)
	st, err := m.Start()
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for !st.Healthy && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		st = m.Status()
	}
	if !st.Healthy {
		t.Fatalf("30s 内健康检查未通过: %+v", st)
	}
	if !verifyIdentity(st.PID, func() time.Time { rec, _ := m.readPID(); return rec.Created }()) {
		t.Fatal("进程身份校验失败")
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("优雅停止失败: %v", err)
	}
	if alive, _, _ := m.Running(); alive {
		t.Fatal("停止后进程仍存活")
	}
	// 停止后端口应释放（连接被拒即视为成功）
	if resp, err := http.Get("http://" + loaded.Addr + "/api/v1/health"); err == nil {
		resp.Body.Close()
		t.Fatal("停止后健康端点仍可达")
	}
	_ = context.Background
}
