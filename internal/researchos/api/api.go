// Package api 暴露 ResearchOS 的 REST + SSE 接口。
//
// 安全基线（对应 SECURITY_REVIEW.md 第 1.6 节修复项）：
//   - 身份只来自服务端签发的 Bearer Token，请求体/查询参数/事件元数据中的 user_id 一律忽略；
//   - 所有任务读写都按 created_by 过滤，越权访问返回 404（不泄露存在性）；
//   - 错误响应统一为 {code, message, details}，不回显凭据。
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/llm"
	"github.com/verikod/hector/internal/researchos/planner"
	"github.com/verikod/hector/internal/researchos/search"
	"github.com/verikod/hector/internal/researchos/settings"
	"github.com/verikod/hector/internal/researchos/store"
)

// Server 是 HTTP 服务。
type Server struct {
	Cfg       *config.Config
	Store     *store.Store
	Planner   planner.Planner
	LLM       llm.Client
	Search    *search.Engine
	Pool      *pgxpool.Pool
	StartedAt time.Time
	Version   string

	requireApproval bool

	// Settings 是设置中心。非 nil 时，Cfg / LLM / Search / Planner 一律以它的
	// 运行时快照为准——用户改了模型或搜索配置，下一个请求就生效。
	// 为 nil 时回退到构造时注入的静态值（便于单测直接构造 Server）。
	Settings *settings.Manager

	// Shutdown 非 nil 时注册 POST /api/v1/admin/shutdown（鉴权保护），
	// 供托盘管理程序等受控方优雅停止服务；nil 时该端点返回 501。
	Shutdown func()

	// tickets 存放 SSE 一次性 ticket（短时、绑定任务、用完即销毁）。
	tickets    *ticketStore
	ticketOnce sync.Once
}

// Options 是 Server 构造参数。
type Options struct {
	Cfg             *config.Config
	Store           *store.Store
	Planner         planner.Planner
	LLM             llm.Client
	Search          *search.Engine
	Pool            *pgxpool.Pool
	RequireApproval bool
	Version         string
	Settings        *settings.Manager
	Shutdown        func()
}

// NewServer 创建服务。
func NewServer(opts Options) *Server {
	return &Server{
		Cfg: opts.Cfg, Store: opts.Store, Planner: opts.Planner, LLM: opts.LLM,
		Search: opts.Search, Pool: opts.Pool, StartedAt: time.Now(),
		Version: opts.Version, requireApproval: opts.RequireApproval,
		Settings: opts.Settings, Shutdown: opts.Shutdown,
	}
}

// runtime 返回当前生效快照；未接入设置中心时为 nil。
func (s *Server) runtime() *settings.Runtime {
	if s.Settings == nil {
		return nil
	}
	return s.Settings.Runtime()
}

// cfg 返回当前生效配置。
//
// 返回副本而不是指针：设置中心可能正在替换配置，把指针递出去会让调用方
// 读到一半新一半旧的字段。
func (s *Server) cfg() config.Config {
	if rt := s.runtime(); rt != nil {
		return rt.Cfg
	}
	if s.Cfg == nil {
		return config.Config{}
	}
	return *s.Cfg
}

func (s *Server) llmClient() llm.Client {
	if rt := s.runtime(); rt != nil && rt.LLM != nil {
		return rt.LLM
	}
	return s.LLM
}

func (s *Server) searchEngine() *search.Engine {
	if rt := s.runtime(); rt != nil && rt.Search != nil {
		return rt.Search
	}
	return s.Search
}

func (s *Server) plannerNow() planner.Planner {
	if rt := s.runtime(); rt != nil && rt.Planner != nil {
		return rt.Planner
	}
	return s.Planner
}

// approvalRequired 返回当前审批策略（设置中心可热改）。
func (s *Server) approvalRequired() bool {
	if rt := s.runtime(); rt != nil {
		return rt.Cfg.RequireApproval
	}
	return s.approvalRequired()
}

// Handler 返回带中间件的路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/v1/settings/{section}", s.handlePutSection)
	mux.HandleFunc("DELETE /api/v1/settings/{section}/{key}", s.handleClearSetting)
	mux.HandleFunc("POST /api/v1/settings/llm/test", s.handleTestLLM)
	mux.HandleFunc("POST /api/v1/settings/search/test", s.handleTestSearch)
	mux.HandleFunc("GET /api/v1/system", s.handleSystemInfo)
	mux.HandleFunc("GET /api/v1/stats", s.handleStats)
	mux.HandleFunc("POST /api/v1/admin/shutdown", s.handleAdminShutdown)

	mux.HandleFunc("POST /api/v1/research", s.handleCreateResearch)
	mux.HandleFunc("GET /api/v1/research", s.handleListResearch)
	mux.HandleFunc("GET /api/v1/research/{id}", s.handleGetResearch)
	mux.HandleFunc("DELETE /api/v1/research/{id}", s.handleDeleteResearch)
	mux.HandleFunc("POST /api/v1/research/{id}/approve", s.handleApprove)
	mux.HandleFunc("POST /api/v1/research/{id}/pause", s.handlePause)
	mux.HandleFunc("POST /api/v1/research/{id}/resume", s.handleResume)
	mux.HandleFunc("POST /api/v1/research/{id}/cancel", s.handleCancel)
	mux.HandleFunc("POST /api/v1/research/{id}/plan", s.handleReplan)
	mux.HandleFunc("POST /api/v1/research/{id}/nodes/{node_id}/retry", s.handleRetryNode)

	mux.HandleFunc("GET /api/v1/research/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /api/v1/research/{id}/events/ticket", s.handleIssueEventTicket)
	mux.HandleFunc("GET /api/v1/research/{id}/agents", s.handleListAgents)
	mux.HandleFunc("GET /api/v1/research/{id}/agents/{agent_id}", s.handleGetAgent)
	mux.HandleFunc("GET /api/v1/research/{id}/sources", s.handleListSources)
	mux.HandleFunc("GET /api/v1/research/{id}/sources/{source_id}", s.handleGetSource)
	mux.HandleFunc("GET /api/v1/research/{id}/evidence", s.handleListEvidence)
	mux.HandleFunc("GET /api/v1/research/{id}/evidence/{claim_id}", s.handleGetClaim)
	mux.HandleFunc("GET /api/v1/research/{id}/report", s.handleGetReport)
	mux.HandleFunc("GET /api/v1/research/{id}/report.md", s.handleExportMarkdown)
	mux.HandleFunc("GET /api/v1/research/{id}/report.html", s.handleExportHTML)
	mux.HandleFunc("GET /api/v1/research/{id}/report.pdf", s.handleExportPDF)

	mux.HandleFunc("GET /api/v1/tools", s.handleListTools)
	mux.HandleFunc("GET /api/v1/tools/calls", s.handleListToolCalls)
	mux.HandleFunc("GET /api/v1/tools/servers", s.handleListMCPServers)
	mux.HandleFunc("POST /api/v1/tools/servers", s.handleRegisterMCP)
	mux.HandleFunc("POST /api/v1/tools/servers/{id}/discover", s.handleDiscoverMCPTools)
	mux.HandleFunc("POST /api/v1/tools/servers/{id}/approve", s.handleApproveMCP)
	mux.HandleFunc("DELETE /api/v1/tools/servers/{id}", s.handleDeleteMCP)
	mux.HandleFunc("POST /api/v1/tools/servers/{id}/{action}", s.handleStartMCPServer)

	mux.HandleFunc("GET /api/v1/sources", s.handleListAllSources)
	mux.HandleFunc("GET /api/v1/reports", s.handleListAllReports)
	mux.HandleFunc("GET /api/v1/agents", s.handleListRunningAgents)

	if s.cfg().WebDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(s.cfg().WebDir)))
	}

	return withRecover(withCORS(s.cfg().Addr, s.withAuth(mux)))
}

// ---------------------------------------------------------------------------
// 中间件
// ---------------------------------------------------------------------------

// withAuth 校验 Bearer Token。身份固定为服务端配置的本地用户，绝不读取客户端传入的 user_id。
//
// 例外：
//   - `/api/v1/health` 免鉴权（探活）；
//   - 非 `/api/` 前缀的静态资源免鉴权（浏览器无法为页面附加请求头，静态产物不含任务数据）；
//   - `GET /api/v1/research/{id}/events` 接受一次性 SSE ticket（查询参数 `ticket`）。
//     ticket 由 `POST /api/v1/research/{id}/events/ticket` 用 Bearer Token 换取，
//     短时、绑定单一任务、校验通过即销毁，**不能**用于其他任何接口。
//
// 查询串里不再接受长期 API Token：`?token=` 会把凭据写进浏览器历史、Referer 与反代日志。
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// SSE：ticket 通道。只有这一个路由接受 ticket，且 ticket 不能替代 Bearer Token。
		if taskID, ok := eventsTaskID(r.URL.Path); ok {
			if tk := strings.TrimSpace(r.URL.Query().Get("ticket")); tk != "" {
				switch err := s.tstore().Consume(tk, taskID); {
				case err == nil:
					next.ServeHTTP(w, r)
				case errors.Is(err, ErrTicketExpired):
					writeErr(w, http.StatusUnauthorized, "sse_ticket_expired", "SSE ticket 已过期，请重新签发")
				case errors.Is(err, ErrTicketScope):
					// 403 而非 404：ticket 本身有效，只是不能用于这个任务。
					writeErr(w, http.StatusForbidden, "sse_ticket_scope", "SSE ticket 与任务不匹配")
				default:
					writeErr(w, http.StatusUnauthorized, "sse_ticket_invalid", "SSE ticket 无效或已被使用")
				}
				return
			}
		}

		got := bearerToken(r)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg().APIToken)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "缺少或无效的 API Token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken 只从请求头取凭据。查询参数一律不接受：
// 长期凭据一旦进 URL，就会沉淀在浏览器历史、Referer 和各级代理日志里。
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if h := r.Header.Get("X-API-Token"); h != "" {
		return strings.TrimSpace(h)
	}
	return ""
}

// withCORS 仅对「同源请求」回显 CORS 头。
//
// RC1 安全复查（2026-11-04）发现旧实现把任意 Origin 原样反射进
// Access-Control-Allow-Origin，等于对全互联网开放跨域读取。
// 实际可利用性低（鉴权走 Authorization 头且未开启 Allow-Credentials，
// 攻击者页面拿不到 Token），但 V1 本地单用户工具的合法用法只有
// 「前端与 API 同源部署」一种，直接把放行条件收紧为 Origin 的 host
// 与请求 Host 一致；其余 Origin 不回显，浏览器会自行拦截跨域读取。
func withCORS(addr string, next http.Handler) http.Handler {
	_ = addr
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && sameHost(origin, r.Host) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Token,Last-Event-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameHost 判断 Origin 头的 host:port 是否与请求自身的 Host 一致。
// 解析失败一律视为不同源（宁可错杀，不做宽松回退）。
func sameHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic in handler", "path", r.URL.Path, "panic", rec)
				writeErr(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// 响应辅助
// ---------------------------------------------------------------------------

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeErr(w http.ResponseWriter, status int, code, msg string, details ...any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	payload := apiError{Code: code, Message: msg}
	if len(details) > 0 {
		payload.Details = details[0]
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": payload})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response failed", "err", err)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// owner 返回当前请求的服务端身份（单用户本地模式下等价于本地用户）。
func (s *Server) owner(*http.Request) string { return s.cfg().LocalUser }

// withTimeout 为请求派生带超时的上下文。
func withTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
