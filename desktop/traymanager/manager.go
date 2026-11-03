package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// manager.go — 后台服务进程管理（与托盘生命周期严格分离）。
//
// 身份验证: 数据目录 run\backend.pid 记录 {pid, created(UTC)}；
// 判定进程"就是我们的后端"必须同时满足:
//   1. 进程存在；
//   2. 可执行文件名是 researchos.exe（QueryFullProcessImageName，防 PID 复用误杀）；
//   3. 进程创建时间与 pidfile 记录一致（防 PID 重放）。
// 绝不按进程名模糊匹配误杀其它 Go 进程。

type BackendStatus struct {
	Running    bool   `json:"running"`
	PID        int    `json:"pid,omitempty"`
	Healthy    bool   `json:"healthy"` // HTTP /api/v1/health 200
	Endpoint   string `json:"endpoint,omitempty"`
	Version    string `json:"version,omitempty"`
	StartedFor string `json:"startedFor,omitempty"` // human 可读启动时长
}

type pidRecord struct {
	PID     int       `json:"pid"`
	Created time.Time `json:"created"` // UTC，来自 GetProcessTimes
}

type Manager struct {
	cfg *Config // 明文凭据已还原

	mu      sync.Mutex
	pidPath string
}

func NewManager(c *Config) *Manager {
	return &Manager{cfg: c, pidPath: filepath.Join(c.DataDir, "run", "backend.pid")}
}

// backendExe 定位后端可执行文件：
// 1) 环境变量 RESEARCHOS_BACKEND_EXE（开发/测试覆盖）；
// 2) 托盘同目录 bin\researchos.exe（便携布局）。
func backendExe() (string, error) {
	if p := os.Getenv("RESEARCHOS_BACKEND_EXE"); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("RESEARCHOS_BACKEND_EXE 指向的程序不存在: %w", err)
		}
		return p, nil
	}
	self, err := osExecutable()
	if err != nil {
		return "", err
	}
	p := filepath.Join(filepath.Dir(self), "bin", "researchos.exe")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("未找到后端程序 %s: %w", p, err)
	}
	return p, nil
}

func (m *Manager) writePID(rec pidRecord) error {
	raw, _ := json.Marshal(rec)
	return os.WriteFile(m.pidPath, raw, 0o644)
}

func (m *Manager) readPID() (pidRecord, bool) {
	raw, err := os.ReadFile(m.pidPath)
	if err != nil {
		return pidRecord{}, false
	}
	var rec pidRecord
	if json.Unmarshal(raw, &rec) != nil || rec.PID <= 0 {
		return pidRecord{}, false
	}
	return rec, true
}

func (m *Manager) clearPID() { _ = os.Remove(m.pidPath) }

// verifyIdentity 校验 pid 指向的进程确为我们的后端。
func verifyIdentity(pid int, created time.Time) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var buf [windows.MAX_LONG_PATH]uint16
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return false
	}
	exe := strings.ToLower(windows.UTF16ToString(buf[:size]))
	if !strings.HasSuffix(exe, "researchos.exe") {
		return false
	}
	var ct, et, kt, ut windows.Filetime
	if err := windows.GetProcessTimes(h, &ct, &et, &kt, &ut); err != nil {
		return false
	}
	return time.Unix(0, ct.Nanoseconds()).UTC().Equal(created.UTC())
}

// Running 返回（进程存活, 身份确认, 记录的 PID）。
func (m *Manager) Running() (bool, bool, int) {
	rec, ok := m.readPID()
	if !ok {
		return false, false, 0
	}
	if verifyIdentity(rec.PID, rec.Created) {
		return true, true, rec.PID
	}
	// pidfile 存在但身份不符：残留记录，清理。
	m.clearPID()
	return false, false, rec.PID
}

// Start 启动后台服务。已运行则幂等返回。
func (m *Manager) Start() (*BackendStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if alive, _, pid := m.Running(); alive {
		return m.probe(pid, true), nil
	}
	exe, err := backendExe()
	if err != nil {
		return nil, err
	}
	if err := EnsureSubdirs(m.cfg.DataDir); err != nil {
		return nil, err
	}
	if m.cfg.Addr == "" {
		m.cfg.Addr = "127.0.0.1:8080"
	}

	logFile, err := os.OpenFile(filepath.Join(m.cfg.DataDir, "logs", fmt.Sprintf("backend-%s.log", time.Now().Format("20060102-150405"))),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "all")
	cmd.Dir = filepath.Dir(exe) // 资源（web/.build、迁移、字体）相对后端 exe 定位
	cmd.Env = append(os.Environ(),
		"RESEARCHOS_ADDR="+m.cfg.Addr,
		"RESEARCHOS_DATABASE_URL="+m.cfg.PgDSN,
		"RESEARCHOS_API_TOKEN="+m.cfg.APIToken,
		"RESEARCHOS_SECRET_KEY="+m.cfg.SecretKey,
		"RESEARCHOS_WEB_DIR="+filepath.Join(filepath.Dir(exe), "web", ".build"),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// 注意：不使用 Job Object 绑定托盘进程——托盘崩溃不得终止后台研究任务。
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动后台服务失败: %w", err)
	}
	rec := pidRecord{PID: cmd.Process.Pid, Created: time.Now().UTC()}
	// 进程创建时间以后端真实 GetProcessTimes 为准：稍后校准时若不一致会自我修复
	// （verifyIdentity 不匹配则清 pidfile）。这里先写入启动时刻，误差容忍由
	// identity 校验的 created 精确匹配保证——因此改为立刻读取真实创建时间。
	if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(rec.PID)); err == nil {
		var ct, et, kt, ut windows.Filetime
		if windows.GetProcessTimes(h, &ct, &et, &kt, &ut) == nil {
			rec.Created = time.Unix(0, ct.Nanoseconds()).UTC()
		}
		windows.CloseHandle(h)
	}
	_ = cmd.Process.Release()
	if err := m.writePID(rec); err != nil {
		return nil, err
	}
	return m.probe(rec.PID, false), nil
}

// Stop 优雅停止：先 shutdown 端点，超时且身份确认后受控强制终止。
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	alive, trusted, pid := m.Running()
	if !alive {
		m.clearPID()
		return nil
	}
	if m.cfg.Addr != "" {
		_ = postShutdown(m.cfg.Addr, m.cfg.APIToken)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if a, _, _ := m.Running(); !a {
			m.clearPID()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	// 超时：仅在身份可信时允许强制终止。
	if trusted {
		if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run(); err != nil {
			return fmt.Errorf("强制终止失败: %w", err)
		}
	}
	m.clearPID()
	return nil
}

// Restart = Stop + Start。
func (m *Manager) Restart() (*BackendStatus, error) {
	if err := m.Stop(); err != nil {
		return nil, err
	}
	return m.Start()
}

// Status 综合状态（进程 + HTTP 健康）。
func (m *Manager) Status() *BackendStatus {
	alive, _, pid := m.Running()
	if !alive {
		return &BackendStatus{Running: false}
	}
	return m.probe(pid, true)
}

func (m *Manager) probe(pid int, waitHealth bool) *BackendStatus {
	st := &BackendStatus{Running: true, PID: pid, Endpoint: "http://" + m.cfg.Addr}
	url := "http://" + m.cfg.Addr + "/api/v1/health"
	tries := 1
	if waitHealth {
		tries = 20 // 最多等 10s
	}
	for i := 0; i < tries; i++ {
		resp, err := http.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				st.Healthy = true
				break
			}
		}
		if !waitHealth {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return st
}

func postShutdown(addr, token string) error {
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/v1/admin/shutdown", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown 端点返回 %d", resp.StatusCode)
	}
	return nil
}

// TestAndMigratePG 用后端二进制验证 DSN 并执行幂等迁移。
// 不直接连接数据库：连接/权限/版本/迁移检查全部由 researchos.exe migrate 完成。
func TestAndMigratePG(dsn string) error {
	exe, err := backendExe()
	if err != nil {
		return err
	}
	if strings.TrimSpace(dsn) == "" {
		return errors.New("数据库连接串为空")
	}
	cmd := exec.Command(exe, "migrate")
	cmd.Dir = filepath.Dir(exe)
	cmd.Env = append(os.Environ(), "RESEARCHOS_DATABASE_URL="+dsn)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return fmt.Errorf("迁移失败: %s", msg)
	}
	return nil
}
