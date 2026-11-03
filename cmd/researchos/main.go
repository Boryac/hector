// Command researchos 是 Hector ResearchOS 的入口。
//
// 子命令：
//
//	researchos migrate   执行数据库迁移
//	researchos serve     启动 REST + SSE API
//	researchos worker    启动 DAG 执行引擎
//	researchos all       同时启动 API 与 Worker（本地单用户部署推荐）
//	researchos seed      创建一个演示研究任务（真实执行，不造假数据）
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/verikod/hector/internal/researchos/api"
	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/db"
	"github.com/verikod/hector/internal/researchos/exec"
	"github.com/verikod/hector/internal/researchos/fetch"
	"github.com/verikod/hector/internal/researchos/llm"
	"github.com/verikod/hector/internal/researchos/planner"
	"github.com/verikod/hector/internal/researchos/search"
	"github.com/verikod/hector/internal/researchos/secrets"
	"github.com/verikod/hector/internal/researchos/settings"
	"github.com/verikod/hector/internal/researchos/store"
	"github.com/verikod/hector/internal/researchos/worker"
)

// Version 由构建参数注入（-ldflags "-X main.Version=..."）。
var Version = "1.0.0-dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel()})))
	loadLocalEnv()
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "migrate":
		err = runMigrate(args)
	case "serve":
		err = runServe(args)
	case "worker":
		err = runWorker(args)
	case "all":
		err = runAll(args)
	case "seed":
		err = runSeed(args)
	case "version", "-v", "--version":
		fmt.Println("hector-researchos", Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令 %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("command failed", "cmd", cmd, "err", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `hector-researchos — 自主研究平台

用法:
  researchos migrate               执行数据库迁移
  researchos serve                 启动 API (:8080)
  researchos worker                启动执行引擎
  researchos all                   同时启动 API 与执行引擎
  researchos seed [--goal "..."]   创建演示研究任务
  researchos version               打印版本

环境变量见 .env.example（RESEARCHOS_DATABASE_URL / RESEARCHOS_API_TOKEN 必填）。
`)
}

func slogLevel() slog.Level {
	switch os.Getenv("RESEARCHOS_LOG_LEVEL") {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ---------------------------------------------------------------------------
// 装配
// ---------------------------------------------------------------------------

type app struct {
	cfg      *config.Config
	pool     *pgxpool.Pool
	store    *store.Store
	search   *search.Engine
	fetcher  *fetch.Fetcher
	llm      llm.Client
	planner  planner.Planner
	engine   *exec.Engine
	settings *settings.Manager
}

func bootstrap(ctx context.Context) (*app, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := db.EnsureDatabase(ctx, cfg.DatabaseURL); err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("解析数据库 DSN 失败: %w", err)
	}
	poolCfg.MaxConns = int32(cfg.Workers + 8)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("数据库不可达（%s）: %w", config.RedactDSN(cfg.DatabaseURL), err)
	}

	st := store.New(pool)

	// 设置中心：把数据库里保存的运行配置叠加到环境变量之上。
	// 环境变量是首次启动的种子与部署级唯一来源，Web 保存值优先（见 DECISIONS 第 29 条）。
	keyFile := filepath.Join(cfg.DataDir, secrets.KeyFileName)
	box, keySource, boxErr := secrets.FromEnvOrFile(os.Getenv(secrets.EnvKey), keyFile)
	if boxErr != nil {
		// 没有主密钥时敏感项不可写，但设置中心其余功能照常可用——不因此拒绝启动。
		slog.Warn("主密钥不可用，敏感配置将无法通过界面保存", "err", boxErr)
		box = nil
		keySource = "unavailable: " + boxErr.Error()
	}
	backend := settings.StoreBackend{
		Load: st.LoadSettings,
		Save: st.SaveSettingRow,
		Del:  st.DeleteSettingRow,
	}
	mgr := settings.NewManager(backend, box, keySource, *cfg)
	if err := mgr.Reload(ctx); err != nil {
		slog.Error("读取已保存设置失败，本次按环境变量基线运行", "err", err)
	}
	rt := mgr.Runtime()
	if len(rt.SearchUnknown) > 0 {
		slog.Warn("部分搜索 Provider 未识别，已忽略", "names", rt.SearchUnknown)
	}
	slog.Info("运行配置已装载",
		"llm_provider", string(rt.Cfg.LLMProvider), "llm_enabled", rt.Cfg.LLMEnabled(),
		"search_available", rt.Search.AvailableNames(), "settings_key_source", keySource)

	fetcher := fetch.New(fetch.Options{
		UserAgent: rt.Cfg.UserAgent, Timeout: rt.Cfg.FetchTimeout,
		MaxBytes: rt.Cfg.FetchMaxBytes, RespectRobots: rt.Cfg.RespectRobots, DataDir: rt.Cfg.DataDir,
	})

	execCfg := exec.Config{
		FetchConcurrency:     rt.Cfg.Workers,
		MaxSourcesPerAnalyze: 6,
		LeaseTTL:             rt.Cfg.StaleLease,
	}
	eng := exec.New(st, rt.Search, fetcher, rt.LLM, execCfg)
	// 节点执行时向设置中心要「当前」客户端：用户改了模型或搜索源，下一个节点即生效。
	eng.SetRuntime(func() (llm.Client, *search.Engine) {
		cur := mgr.Runtime()
		return cur.LLM, cur.Search
	})

	return &app{
		cfg: cfg, pool: pool, store: st, search: rt.Search, fetcher: fetcher,
		llm: rt.LLM, planner: rt.Planner, engine: eng, settings: mgr,
	}, nil
}

func (a *app) close() { a.pool.Close() }

// ---------------------------------------------------------------------------
// 子命令
// ---------------------------------------------------------------------------

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	a, err := bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.close()
	if err := db.Migrate(ctx, a.pool); err != nil {
		return err
	}
	applied, _ := db.AppliedMigrations(ctx, a.pool)
	slog.Info("迁移完成", "versions", applied)
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "", "监听地址（覆盖 RESEARCHOS_ADDR）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a, err := bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.close()
	if a.cfg.APIToken == "" {
		return errors.New("RESEARCHOS_API_TOKEN 未设置：ResearchOS 不允许无鉴权启动（见 SECURITY_REVIEW.md）")
	}
	if err := db.Migrate(ctx, a.pool); err != nil {
		return err
	}
	if *addr != "" {
		a.cfg.Addr = *addr
	}

	srv := api.NewServer(api.Options{
		Cfg: a.cfg, Store: a.store, Planner: a.planner, LLM: a.llm, Search: a.search,
		Pool: a.pool, RequireApproval: a.cfg.RequireApproval, Version: Version, Settings: a.settings,
	})
	httpSrv := &http.Server{
		Addr:              a.cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("ResearchOS API 启动", "addr", a.cfg.Addr, "version", Version)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, sc := context.WithTimeout(context.Background(), 15*time.Second)
	defer sc()
	_ = httpSrv.Shutdown(shutdownCtx)
	slog.Info("API 已停止")
	return nil
}

func runWorker(args []string) error {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	a, err := bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.close()
	if err := db.Migrate(ctx, a.pool); err != nil {
		return err
	}
	wcfg := a.cfg
	if rt := a.settings.Runtime(); rt != nil {
		wcfg = &rt.Cfg
	}
	w := worker.New(a.store, a.engine, worker.Config{
		Workers: wcfg.Workers, NodeTimeout: wcfg.NodeTimeout, TaskTimeout: wcfg.TaskTimeout,
		Lease: wcfg.StaleLease, Tick: wcfg.SchedulerTick, MaxAttempts: wcfg.NodeMaxAttempts,
	})
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func runAll(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a, err := bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.close()
	if a.cfg.APIToken == "" {
		return errors.New("RESEARCHOS_API_TOKEN 未设置：ResearchOS 不允许无鉴权启动")
	}
	if err := db.Migrate(ctx, a.pool); err != nil {
		return err
	}

	wcfg := a.cfg
	if rt := a.settings.Runtime(); rt != nil {
		wcfg = &rt.Cfg
	}
	w := worker.New(a.store, a.engine, worker.Config{
		Workers: wcfg.Workers, NodeTimeout: wcfg.NodeTimeout, TaskTimeout: wcfg.TaskTimeout,
		Lease: wcfg.StaleLease, Tick: wcfg.SchedulerTick, MaxAttempts: wcfg.NodeMaxAttempts,
	})
	go func() {
		if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("worker 退出", "err", err)
			cancel()
		}
	}()

	srv := api.NewServer(api.Options{
		Cfg: a.cfg, Store: a.store, Planner: a.planner, LLM: a.llm, Search: a.search,
		Pool: a.pool, RequireApproval: a.cfg.RequireApproval, Version: Version, Settings: a.settings,
	})
	httpSrv := &http.Server{
		Addr:              a.cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("ResearchOS 启动（API + Worker）", "addr", a.cfg.Addr, "workers", a.cfg.Workers, "version", Version)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, sc := context.WithTimeout(context.Background(), 15*time.Second)
	defer sc()
	_ = httpSrv.Shutdown(shutdownCtx)
	// 给正在执行的节点一点收尾时间。
	time.Sleep(1500 * time.Millisecond)
	return nil
}

// runSeed 创建一个演示研究任务，走真实检索与抓取。
func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	goal := fs.String("goal", "研究全球 AI Agent 开源生态，筛选 10 个代表性项目并生成竞争分析报告", "研究目标")
	depth := fs.String("depth", "standard", "研究深度 quick/standard/deep")
	planNow := fs.Bool("plan", true, "是否立即规划")
	autoApprove := fs.Bool("approve", true, "规划后自动批准执行")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	a, err := bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.close()
	if err := db.Migrate(ctx, a.pool); err != nil {
		return err
	}

	// seed 直接复用 API 的规划与状态机：通过本地 HTTP 调用，避免逻辑分叉。
	base := fmt.Sprintf("http://127.0.0.1%s", portOf(a.cfg.Addr))
	client := &http.Client{Timeout: 30 * time.Second}
	body, _ := json.Marshal(map[string]any{"goal": *goal, "depth": *depth, "plan_now": *planNow})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/research", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIToken)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("调用本地 API 失败（请先启动 researchos serve/all）：%w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if out.Task.ID == "" {
		return fmt.Errorf("创建任务失败: %v", out.Error)
	}
	fmt.Println("task_id:", out.Task.ID)

	if *planNow && *autoApprove {
		// 等待规划完成后再批准。
		deadline := time.Now().Add(4 * time.Minute)
		for time.Now().Before(deadline) {
			st := getStatus(ctx, client, base, a.cfg.APIToken, out.Task.ID)
			if st == "awaiting_approval" {
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
					base+"/api/v1/research/"+out.Task.ID+"/approve", nil)
				req.Header.Set("Authorization", "Bearer "+a.cfg.APIToken)
				if r2, err := client.Do(req); err == nil {
					r2.Body.Close()
				}
				fmt.Println("approved:", out.Task.ID)
				return nil
			}
			if st == "failed" || st == "completed" {
				return fmt.Errorf("任务状态为 %s，未批准执行", st)
			}
			time.Sleep(time.Second)
		}
		return errors.New("等待规划超时")
	}
	return nil
}

func getStatus(ctx context.Context, c *http.Client, base, token, id string) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/research/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var out struct {
		Task struct {
			Status string `json:"status"`
		} `json:"task"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Task.Status
}

func portOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i:]
		}
	}
	return ":8080"
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true" || v == "TRUE"
	}
	return def
}

// loadLocalEnv 加载本地开发配置。
//
// 查找顺序：$RESEARCHOS_ENV_FILE → 当前目录 .env.researchos → 上两级目录同名文件。
// 只补未设置的变量，因此 shell 里显式 export 的值优先；文件缺失静默跳过。
func loadLocalEnv() {
	candidates := make([]string, 0, 3)
	if p := os.Getenv("RESEARCHOS_ENV_FILE"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, ".env.researchos", "../.env.researchos", "../../.env.researchos")
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := config.LoadEnvFile(p); err != nil {
			slog.Warn("读取本地环境文件失败", "file", p, "err", err)
			continue
		}
		slog.Info("已加载本地环境文件", "file", p)
		return
	}
}
