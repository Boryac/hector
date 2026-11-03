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
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/verikod/hector/internal/researchos/config"
	"github.com/verikod/hector/internal/researchos/llm"
	"github.com/verikod/hector/internal/researchos/planner"
	"github.com/verikod/hector/internal/researchos/search"
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
}

// NewServer 创建服务。
func NewServer(opts Options) *Server {
	return &Server{
		Cfg: opts.Cfg, Store: opts.Store, Planner: opts.Planner, LLM: opts.LLM,
		Search: opts.Search, Pool: opts.Pool, StartedAt: time.Now(),
		Version: opts.Version, requireApproval: opts.RequireApproval,
	}
}

// Handler 返回带中间件的路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/v1/settings", s.handlePutSettings)

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

	mux.HandleFunc("GET /api/v1/tools", s.handleListTools)
	mux.HandleFunc("GET /api/v1/tools/calls", s.handleListToolCalls)
	mux.HandleFunc("POST /api/v1/tools/servers", s.handleRegisterMCP)
	mux.HandleFunc("POST /api/v1/tools/servers/{id}/{action}", s.handleMCPServerAction)

	if s.Cfg.WebDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(s.Cfg.WebDir)))
	}

	return withRecover(withCORS(s.Cfg.Addr, s.withAuth(mux)))
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
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.Cfg.APIToken)) != 1 {
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

func withCORS(addr string, next http.Handler) http.Handler {
	_ = addr
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
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
func (s *Server) owner(*http.Request) string { return s.Cfg.LocalUser }

// withTimeout 为请求派生带超时的上下文。
func withTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
