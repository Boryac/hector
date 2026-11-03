package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// app.go — Wails 绑定层：首次启动向导、服务管理入口、设置、退出确认。
// Wails 只承担桌面 UI 与生命周期控制；Agent Runtime / 研究任务全部在
// 独立的 researchos.exe 后端进程中，托盘崩溃或窗口关闭都不影响后台任务。

type App struct {
	ctx context.Context

	cfg *Config
	mgr *Manager

	trayQuit    func()
	exitPending bool // 托盘「退出…」触发的确认窗口模式
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := LoadConfig()
	if err != nil {
		runtime.LogErrorf(ctx, "读取配置失败: %v", err)
	}
	a.cfg = cfg
	if cfg != nil && cfg.Initialized {
		if err := UnprotectConfig(cfg); err != nil {
			runtime.LogErrorf(ctx, "解密配置失败: %v", err)
			cfg.Initialized = false // 凭据不可解密 → 重新走向导（不静默失败）
		} else {
			a.mgr = NewManager(cfg)
		}
	}

	// 启动托盘（独立消息循环 goroutine）。
	tray := &tray{}
	a.trayQuit = tray.QuitTray
	go func() {
		defer func() {
			if r := recover(); r != nil {
				runtime.LogErrorf(ctx, "托盘崩溃（后台服务不受影响）: %v\n%s", r, debug.Stack())
			}
		}()
		if err := RunTray(TrayCallbacks{
			Status: func() (bool, bool) {
				st := a.Status()
				return st.Running, st.Healthy
			},
			On: a.onTrayCommand,
		}); err != nil {
			runtime.LogErrorf(ctx, "托盘初始化失败: %v", err)
		}
	}()

	// 已初始化且后台未运行 → 自动启动（幂等）。
	if a.mgr != nil {
		if alive, _, _ := a.mgr.Running(); !alive {
			go func() {
				if _, err := a.mgr.Start(); err != nil {
					runtime.LogErrorf(ctx, "后台服务自动启动失败: %v", err)
				}
			}()
		}
	}
}

// onTrayCommand 处理托盘菜单命令。
func (a *App) onTrayCommand(c TrayCommand) {
	switch c {
	case CmdOpenUI:
		_ = a.OpenUI()
	case CmdStart:
		go func() { _, _ = a.mgr.Start() }()
	case CmdStop:
		go func() { _ = a.mgr.Stop() }()
	case CmdRestart:
		go func() { _, _ = a.mgr.Restart() }()
	case CmdStatus:
		st := a.Status()
		var detail string
		if st.Running {
			health := "健康检查未通过（服务可能仍在启动）"
			if st.Healthy {
				health = "健康检查通过"
			}
			detail = fmt.Sprintf("运行中 (PID %d)\n%s\n地址: %s", st.PID, health, st.Endpoint)
		} else {
			detail = "已停止"
		}
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type: runtime.InfoDialog, Title: "Hector ResearchOS 服务状态", Message: detail,
		})
	case CmdOpenLogs:
		_ = a.openDir(filepath.Join(a.cfg.DataDir, "logs"))
	case CmdOpenData:
		_ = a.openDir(a.cfg.DataDir)
	case CmdSettings:
		a.exitPending = false
		runtime.WindowShow(a.ctx)
	case CmdExit:
		a.exitPending = true
		runtime.WindowShow(a.ctx)
		runtime.EventsEmit(a.ctx, "exit-confirm")
	}
}

// ---------- 绑定（前端可调用） ----------

// InitState 向导/主界面初始状态。
type InitState struct {
	Initialized    bool           `json:"initialized"`
	DefaultDataDir string         `json:"defaultDataDir"`
	DataDir        string         `json:"dataDir"`
	Addr           string         `json:"addr"`
	PgConfigured   bool           `json:"pgConfigured"`
	Autostart      bool           `json:"autostart"`
	ExitPending    bool           `json:"exitPending"`
	BackendStatus  *BackendStatus `json:"backendStatus,omitempty"`
}

func (a *App) GetState() InitState {
	def, _ := DefaultDataDir()
	st := InitState{
		DefaultDataDir: def,
		Addr:           "127.0.0.1:8080",
		ExitPending:    a.exitPending,
	}
	if a.cfg != nil {
		st.Initialized = a.cfg.Initialized
		st.DataDir = a.cfg.DataDir
		st.Addr = a.cfg.Addr
		st.Autostart = a.cfg.Autostart
		st.PgConfigured = a.cfg.PgDSN != ""
		if a.mgr != nil {
			st.BackendStatus = a.Status()
		}
	}
	return st
}

// ChooseDataDir 弹出目录选择框。
func (a *App) ChooseDataDir() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 ResearchOS 数据目录",
	})
	if err != nil {
		return ""
	}
	return dir
}

// ValidateDir 校验数据目录，无错误返回空串。
func (a *App) ValidateDir(dir string) string {
	if err := ValidateDataDir(dir); err != nil {
		return err.Error()
	}
	return ""
}

// CompleteInit 完成首次初始化：写配置 → DPAPI 加密 → 迁移 → 启动后台 → 自启动。
func (a *App) CompleteInit(dataDir, addr, pgDSN string, autostart bool) string {
	if err := ValidateDataDir(dataDir); err != nil {
		return err.Error()
	}
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	token, err := RandomToken()
	if err != nil {
		return "生成 API Token 失败: " + err.Error()
	}
	secret, err := RandomToken()
	if err != nil {
		return "生成 SECRET_KEY 失败: " + err.Error()
	}
	if err := EnsureSubdirs(dataDir); err != nil {
		return err.Error()
	}
	cfg := &Config{
		Version: 1, Initialized: true,
		DataDir: dataDir, Addr: addr, Autostart: autostart,
		PgDSN: pgDSN, APIToken: token, SecretKey: secret,
	}
	if err := ProtectConfig(cfg); err != nil {
		return "凭据加密失败: " + err.Error()
	}
	if err := SaveConfig(cfg); err != nil {
		return "保存配置失败: " + err.Error()
	}

	// 迁移验证（幂等，不覆盖不删除任何已有数据）。
	if err := TestAndMigratePG(pgDSN); err != nil {
		return err.Error()
	}
	if err := SetAutostart(autostart); err != nil {
		return "设置开机启动失败: " + err.Error()
	}

	a.cfg = cfg
	a.mgr = NewManager(cfg)
	if _, err := a.mgr.Start(); err != nil {
		return "后台服务启动失败: " + err.Error()
	}
	return ""
}

// Status 后台服务综合状态。
func (a *App) Status() *BackendStatus {
	if a.mgr == nil {
		return &BackendStatus{Running: false}
	}
	return a.mgr.Status()
}

// StartBackend / StopBackend / RestartBackend 由窗口按钮触发。
func (a *App) StartBackend() string {
	if a.mgr == nil {
		return "尚未完成初始化"
	}
	if _, err := a.mgr.Start(); err != nil {
		return err.Error()
	}
	return ""
}
func (a *App) StopBackend() string {
	if a.mgr == nil {
		return "尚未完成初始化"
	}
	if err := a.mgr.Stop(); err != nil {
		return err.Error()
	}
	return ""
}
func (a *App) RestartBackend() string {
	if a.mgr == nil {
		return "尚未完成初始化"
	}
	if _, err := a.mgr.Restart(); err != nil {
		return err.Error()
	}
	return ""
}

// SetAutostartPref 开机启动偏好。
func (a *App) SetAutostartPref(enable bool) string {
	if a.cfg == nil {
		return "尚未完成初始化"
	}
	if err := SetAutostart(enable); err != nil {
		return err.Error()
	}
	a.cfg.Autostart = enable
	if err := ProtectConfig(a.cfg); err != nil {
		return err.Error()
	}
	if err := SaveConfig(a.cfg); err != nil {
		return err.Error()
	}
	return ""
}

// ConfigureLLM 经后端设置中心 API 写入模型配置（密钥由后端加密落库，不经过托盘存储）。
func (a *App) ConfigureLLM(baseURL, model, apiKey string) string {
	return a.putSettings("model", map[string]string{
		"model.provider": "openai",
		"model.base_url": baseURL,
		"model.model":    model,
		"model.api_key":  apiKey,
	})
}

// ConfigureSearch 写入搜索 Provider 配置。
func (a *App) ConfigureSearch(provider string) string {
	return a.putSettings("search", map[string]string{"search.provider": provider})
}

func (a *App) putSettings(section string, values map[string]string) string {
	if a.mgr == nil || a.cfg == nil {
		return "后台服务未运行"
	}
	body := fmt.Sprintf(`{"values":{%s}}`, joinJSON(values))
	req, err := http.NewRequest(http.MethodPut, "http://"+a.cfg.Addr+"/api/v1/settings/"+section, strings.NewReader(body))
	if err != nil {
		return err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "连接后台失败: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "保存失败: HTTP " + fmt.Sprint(resp.StatusCode)
	}
	return ""
}

func joinJSON(values map[string]string) string {
	parts := make([]string, 0, len(values))
	for k, v := range values {
		parts = append(parts, fmt.Sprintf(`%q:%q`, k, v))
	}
	return strings.Join(parts, ",")
}

// OpenUI 用默认浏览器打开 Web UI（Token 经 #fragment 一次性注入）。
func (a *App) OpenUI() error {
	if a.cfg == nil {
		return fmt.Errorf("尚未完成初始化")
	}
	url := "http://" + a.cfg.Addr + "/#token=" + a.cfg.APIToken
	return openBrowser(url)
}

func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func (a *App) openDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return exec.Command("explorer", dir).Start()
}

// TestAndMigratePGOnly 仅测试 DSN 连接并执行幂等迁移，不写任何配置（向导预检用）。
func (a *App) TestAndMigratePGOnly(dsn string) string {
	if err := TestAndMigratePG(dsn); err != nil {
		return err.Error()
	}
	return ""
}

// OpenLogsDir / OpenDataDir 打开对应目录。
func (a *App) OpenLogsDir() string {
	if a.cfg == nil {
		return "尚未完成初始化"
	}
	if err := a.openDir(filepath.Join(a.cfg.DataDir, "logs")); err != nil {
		return err.Error()
	}
	return ""
}
func (a *App) OpenDataDir() string {
	if a.cfg == nil {
		return "尚未完成初始化"
	}
	if err := a.openDir(a.cfg.DataDir); err != nil {
		return err.Error()
	}
	return ""
}

// ExitApp 退出确认页三选一。
// mode: "stop_and_exit"（停止后台并退出）| "tray_only"（仅退出托盘，后台继续）| "cancel"
func (a *App) ExitApp(mode string) {
	switch mode {
	case "stop_and_exit":
		if a.mgr != nil {
			_ = a.mgr.Stop()
		}
		fallthrough
	case "tray_only":
		if a.trayQuit != nil {
			a.trayQuit()
		}
		runtime.Quit(a.ctx)
	default:
		// 取消：只隐藏窗口
		a.exitPending = false
		runtime.WindowHide(a.ctx)
	}
}

// osExecutable 当前托盘 exe 路径。
func osExecutable() (string, error) {
	return os.Executable()
}
