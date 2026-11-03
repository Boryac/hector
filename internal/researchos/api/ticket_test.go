package api

import (
	"crypto/sha256"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestStore(ttl time.Duration) (*ticketStore, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	s := newTicketStore(ttl)
	s.now = clk.Now
	return s, clk
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestTicketSingleUse(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	tk, _, err := s.Issue("task-a")
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	if err := s.Consume(tk, "task-a"); err != nil {
		t.Fatalf("首次使用应通过：%v", err)
	}
	if err := s.Consume(tk, "task-a"); err != ErrTicketInvalid {
		t.Fatalf("二次使用应失败，实际 %v", err)
	}
	if s.Len() != 0 {
		t.Fatalf("消费后表应为空，实际 %d", s.Len())
	}
}

func TestTicketExpires(t *testing.T) {
	s, clk := newTestStore(60 * time.Second)
	tk, exp, _ := s.Issue("task-a")
	if got := exp.Sub(clk.Now()); got != 60*time.Second {
		t.Fatalf("有效期应为 60s，实际 %v", got)
	}
	clk.Advance(61 * time.Second)
	if err := s.Consume(tk, "task-a"); err != ErrTicketExpired {
		t.Fatalf("过期 ticket 应返回过期错误，实际 %v", err)
	}
	// 过期项应被清掉，不留内存垃圾
	if s.Len() != 0 {
		t.Fatalf("过期项应被清除，实际剩余 %d", s.Len())
	}
}

func TestTicketScopeMismatch(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	tk, _, _ := s.Issue("task-a")
	if err := s.Consume(tk, "task-b"); err != ErrTicketScope {
		t.Fatalf("跨任务应返回 scope 错误，实际 %v", err)
	}
	// 跨任务的失败不应销毁 ticket 本身（否则构成对合法持有者的拒绝服务）
	if err := s.Consume(tk, "task-a"); err != nil {
		t.Fatalf("原任务仍应可用：%v", err)
	}
}

// 随机猜测：任何未签发的字符串都必须被拒绝。
func TestTicketRejectsGuesses(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	_, _, _ = s.Issue("task-a")
	candidates := []string{
		"", " ", "0", strings.Repeat("A", 43), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0000000000000000000000000000000000000000000",
	}
	for _, c := range candidates {
		if err := s.Consume(c, "task-a"); err == nil {
			t.Fatalf("猜测值 %q 不应通过", c)
		}
	}
}

// 篡改：改动任意一个字符都不应命中，且不影响原 ticket。
func TestTicketRejectsTampered(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	tk, _, _ := s.Issue("task-a")
	b := []byte(tk)
	b[len(b)-1] ^= 0x01 // 翻转最后一字节的末位
	tampered := string(b)
	if tampered == tk {
		t.Fatal("篡改构造失败")
	}
	if err := s.Consume(tampered, "task-a"); err != ErrTicketInvalid {
		t.Fatalf("篡改 ticket 应被拒，实际 %v", err)
	}
	if err := s.Consume(tk, "task-a"); err != nil {
		t.Fatalf("原 ticket 不应受影响：%v", err)
	}
}

// 超长输入不应造成异常或拒绝服务。
func TestTicketRejectsOversizedInput(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	huge := strings.Repeat("x", 1<<16)
	if err := s.Consume(huge, "task-a"); err != ErrTicketInvalid {
		t.Fatalf("超长输入应快速拒绝，实际 %v", err)
	}
}

// 熵与长度：32 字节随机源、base64url 43 字符，且两次签发不同。
func TestTicketEntropy(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tk, err := randomTicket()
		if err != nil {
			t.Fatalf("随机源失败：%v", err)
		}
		if len(tk) != 43 {
			t.Fatalf("ticket 长度应为 43，实际 %d（%q）", len(tk), tk)
		}
		if strings.ContainsAny(tk, "+/=") {
			t.Fatalf("应为 base64url 无填充，实际含非法字符：%q", tk)
		}
		if seen[tk] {
			t.Fatalf("出现重复 ticket：%q", tk)
		}
		seen[tk] = true
	}
}

// 内存里不存明文：表 key 是 sha256 摘要。
func TestTicketStoredHashed(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	tk, _, _ := s.Issue("task-a")
	want := sha256.Sum256([]byte(tk))
	s.mu.Lock()
	_, ok := s.entries[want]
	s.mu.Unlock()
	if !ok {
		t.Fatal("ticket 应以 sha256 摘要存储")
	}
	for k := range s.entries {
		if strings.Contains(string(k[:]), tk) {
			t.Fatal("内存中发现明文 ticket")
		}
	}
}

// 并发消费：同一张 ticket 只能有一个赢家。
func TestTicketConcurrentConsume(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	tk, _, _ := s.Issue("task-a")
	const n = 64
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		okCnt int
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := s.Consume(tk, "task-a"); err == nil {
				mu.Lock()
				okCnt++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if okCnt != 1 {
		t.Fatalf("并发下应恰好一个成功，实际 %d", okCnt)
	}
}

// 单任务未消费上限：超过上限时淘汰最早的一张，内存不无限增长。
func TestTicketCapsOutstandingPerTask(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	for i := 0; i < ticketMaxPerTask+10; i++ {
		if _, _, err := s.Issue("task-a"); err != nil {
			t.Fatalf("签发失败：%v", err)
		}
	}
	if s.Len() > ticketMaxPerTask {
		t.Fatalf("单任务未消费 ticket 应不超过 %d，实际 %d", ticketMaxPerTask, s.Len())
	}
	// 其他任务不受影响
	if _, _, err := s.Issue("task-b"); err != nil {
		t.Fatalf("签发失败：%v", err)
	}
}

// TTL 兜底：非法值回落到 60s，避免误配置签发永久 ticket。
func TestTicketTTLFallback(t *testing.T) {
	for _, ttl := range []time.Duration{0, -1} {
		s := newTicketStore(ttl)
		if s.ttl != 60*time.Second {
			t.Fatalf("ttl=%v 应回落到 60s，实际 %v", ttl, s.ttl)
		}
	}
}

func TestIssueRejectsEmptyTask(t *testing.T) {
	s, _ := newTestStore(time.Minute)
	if _, _, err := s.Issue(""); err == nil {
		t.Fatal("空 task_id 不应签发")
	}
}
