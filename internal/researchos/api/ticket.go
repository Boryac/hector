package api

// SSE 事件流的凭据方案。
//
// 背景：浏览器原生 EventSource 不能设置请求头，只能把凭据放进 URL 查询串。
// 早期实现直接把长期 API Token 放进 `?token=`，代价是：
//   - Token 落入浏览器历史（用户复制/分享页面即可泄露）、Referer、反代的 access log；
//   - Token 长期有效，一旦泄露窗口极长，且能调用全部接口（读写任务、导出报告）。
//
// 现方案：前端先用 Bearer Token 调一次签发接口换 ticket，再用 ticket 建立事件流。
// ticket 的性质：
//   - 短时：默认 60s（RESEARCHOS_SSE_TICKET_TTL，强制上限 10 分钟）；
//   - 受限：绑定单一 task_id，仅 GET /api/v1/research/{id}/events 接受，不能当 API Token 用；
//   - 一次性：校验通过即销毁，无法重放；
//   - 不可猜：32 字节 crypto/rand，base64url 43 字符；
//   - 内存不存明文：只存 sha256 摘要作 key。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// ticketBytes 是随机源长度（32 字节 = 256 bit）。
	ticketBytes = 32
	// ticketMaxLen 限制入参长度，避免超长字符串打满哈希与内存。
	ticketMaxLen = 128
	// ticketMaxPerTask 限制单任务未消费的 ticket 数量，防止签发接口被刷爆内存。
	ticketMaxPerTask = 16
)

var (
	// ErrTicketInvalid 表示 ticket 不存在、已被使用或格式非法。
	ErrTicketInvalid = errors.New("sse ticket 无效或已被使用")
	// ErrTicketExpired 表示 ticket 超过有效期。
	ErrTicketExpired = errors.New("sse ticket 已过期")
	// ErrTicketScope 表示 ticket 与其绑定的任务不一致。
	ErrTicketScope = errors.New("sse ticket 与请求的任务不匹配")
)

type ticketEntry struct {
	taskID    string
	expiresAt time.Time
}

// ticketStore 是进程内的 ticket 表。
//
// 只在 API 进程内签发与校验：同一进程既签发又校验，多副本部署时需要在前面做会话粘滞
// 或换成共享存储（本机单用户模式不涉及，已在文档写明限制）。
type ticketStore struct {
	mu      sync.Mutex
	entries map[[32]byte]ticketEntry
	ttl     time.Duration

	// now 与 newToken 可在测试中替换，避免依赖真实时钟与随机源。
	now      func() time.Time
	newToken func() (string, error)
}

func newTicketStore(ttl time.Duration) *ticketStore {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &ticketStore{
		entries:  make(map[[32]byte]ticketEntry),
		ttl:      ttl,
		now:      time.Now,
		newToken: randomTicket,
	}
}

func randomTicket() (string, error) {
	buf := make([]byte, ticketBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Issue 为指定任务签发一张一次性 ticket，返回 ticket 与过期时间。
func (t *ticketStore) Issue(taskID string) (string, time.Time, error) {
	if taskID == "" {
		return "", time.Time{}, errors.New("task_id 不能为空")
	}
	tok, err := t.newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := t.now()
	exp := now.Add(t.ttl)
	key := sha256.Sum256([]byte(tok))

	t.mu.Lock()
	t.sweepLocked(now)
	t.evictForTaskLocked(taskID)
	t.entries[key] = ticketEntry{taskID: taskID, expiresAt: exp}
	t.mu.Unlock()
	return tok, exp, nil
}

// Consume 校验并销毁一张 ticket。taskID 必须与签发时绑定的一致。
//
// 校验通过即删除，保证一次性：即使 URL 被日志记录，也无法二次使用。
func (t *ticketStore) Consume(token, taskID string) error {
	if token == "" || len(token) > ticketMaxLen {
		return ErrTicketInvalid
	}
	key := sha256.Sum256([]byte(token))
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[key]
	if !ok {
		return ErrTicketInvalid
	}
	if now.After(e.expiresAt) {
		delete(t.entries, key)
		return ErrTicketExpired
	}
	if e.taskID != taskID {
		return ErrTicketScope
	}
	delete(t.entries, key)
	return nil
}

// Len 返回当前未消费的 ticket 数（测试与可观测性用）。
func (t *ticketStore) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

func (t *ticketStore) sweepLocked(now time.Time) {
	for k, e := range t.entries {
		if now.After(e.expiresAt) {
			delete(t.entries, k)
		}
	}
}

// evictForTaskLocked 在单任务 ticket 数达到上限时淘汰最早过期的一张。
func (t *ticketStore) evictForTaskLocked(taskID string) {
	var (
		count  int
		oldest [32]byte
		oldExp time.Time
		found  bool
	)
	for k, e := range t.entries {
		if e.taskID != taskID {
			continue
		}
		count++
		if !found || e.expiresAt.Before(oldExp) {
			oldest, oldExp, found = k, e.expiresAt, true
		}
	}
	if count >= ticketMaxPerTask && found {
		delete(t.entries, oldest)
	}
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

// handleIssueEventTicket 用 Bearer Token 换取一张 SSE ticket。
//
// 之所以单独开一个端点而不是在 events 响应里下发：签发必须走正常的 Authorization
// 头，这样「长期凭据」永远不会出现在 URL 里。
func (s *Server) handleIssueEventTicket(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r, 5*time.Second)
	defer cancel()
	task, err := s.Store.GetTaskRow(ctx, r.PathValue("id"))
	if err != nil || task.CreatedBy != s.owner(r) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	store := s.tstore()
	tok, exp, err := store.Issue(task.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ticket_failed", "签发 ticket 失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     tok,
		"task_id":    task.ID,
		"expires_at": exp.UTC().Format(time.RFC3339),
		"expires_in": int(store.ttl.Seconds()),
		"scope":      "仅限 GET /api/v1/research/" + task.ID + "/events，一次性",
	})
}

// tstore 惰性初始化 ticket 表（测试里直接构造 Server 字面量时也能用）。
func (s *Server) tstore() *ticketStore {
	s.ticketOnce.Do(func() {
		ttl := 60 * time.Second
		if s.Cfg != nil && s.Cfg.SSETicketTTL > 0 {
			ttl = s.Cfg.SSETicketTTL
		}
		if s.tickets == nil {
			s.tickets = newTicketStore(ttl)
		}
	})
	return s.tickets
}

// eventsTaskID 从 /api/v1/research/{id}/events 中取出任务 id。
// 路径不匹配时返回 false，调用方应继续走 Bearer 校验。
func eventsTaskID(path string) (string, bool) {
	const prefix = "/api/v1/research/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "events" || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}
