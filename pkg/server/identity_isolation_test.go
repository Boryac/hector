package server

// A2A Issue #2 身份隔离回归测试。
//
// 背景：上游 commit abe472b6（main）中 toInvocationMeta() 先用已验证的 JWT claims
// 绑定 userID，随后又无条件允许客户端可控的 Message.Metadata["user_id"] 覆盖它。
// 攻击者因此可以冒充任意用户；会话与任务存储以 (app_name, user_id, ...) 为键，
// 配合可猜的 context_id 即可跨用户读写。
//
// 现状：已修复（2026-11-04）。身份只认服务端校验结果；metadata 中的 user_id 被忽略
// 并记录审计日志；未认证请求统一归入常量匿名身份 "anonymous"。
// 下面的用例锁定修复后的行为，任何回退都会让它们失败。

import (
	"context"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/verikod/hector/pkg/auth"
)

// 已认证请求：metadata 里的 user_id 不得覆盖 claims.Subject。
func TestToInvocationMeta_MetadataMustNotOverrideClaims(t *testing.T) {
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Subject: "alice"})
	reqCtx := &a2asrv.RequestContext{
		ContextID: "session-abc",
		TaskID:    "task-1",
		Message:   &a2a.Message{Metadata: map[string]any{"user_id": "bob"}},
	}

	meta := toInvocationMeta(ctx, reqCtx)

	if meta.userID != "alice" {
		t.Fatalf("身份必须来自已验证 claims：期望 alice，实际 %q（身份伪造漏洞回归）", meta.userID)
	}
	if meta.sessionID != "session-abc" {
		t.Fatalf("context_id 仍应作为会话标识，实际 %q", meta.sessionID)
	}
}

// 未认证请求：metadata 里的 user_id 不得被采纳。
func TestToInvocationMeta_UnauthenticatedCannotPickUserID(t *testing.T) {
	reqCtx := &a2asrv.RequestContext{
		ContextID: "session-xyz",
		Message:   &a2a.Message{Metadata: map[string]any{"user_id": "victim"}},
	}

	meta := toInvocationMeta(context.Background(), reqCtx)

	if meta.userID == "victim" {
		t.Fatal("未认证请求采纳了客户端提供的 user_id（身份伪造漏洞回归）")
	}
	if meta.userID != anonymousUserID {
		t.Fatalf("未认证请求应归入 %q，实际 %q", anonymousUserID, meta.userID)
	}
}

// 没有 metadata 时使用 claims.Subject（正常路径不能被改坏）。
func TestToInvocationMeta_ClaimsUsedWhenNoMetadata(t *testing.T) {
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Subject: "alice"})
	meta := toInvocationMeta(ctx, &a2asrv.RequestContext{ContextID: "s1"})
	if meta.userID != "alice" {
		t.Fatalf("期望 claims subject，实际 %q", meta.userID)
	}
}

// 无 claims 且无 metadata：匿名常量。
func TestToInvocationMeta_AnonymousFallback(t *testing.T) {
	meta := toInvocationMeta(context.Background(), &a2asrv.RequestContext{})
	if meta.userID != anonymousUserID {
		t.Fatalf("期望 %q，实际 %q", anonymousUserID, meta.userID)
	}
}

// metadata 里非字符串的 user_id 不应导致 panic 或被字符串化后使用。
func TestToInvocationMeta_NonStringUserIDIgnored(t *testing.T) {
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Subject: "carol"})
	reqCtx := &a2asrv.RequestContext{
		Message: &a2a.Message{Metadata: map[string]any{"user_id": 12345}},
	}
	meta := toInvocationMeta(ctx, reqCtx)
	if meta.userID != "carol" {
		t.Fatalf("期望 carol，实际 %q", meta.userID)
	}
}

// 大小写/空白变体也不能绕过（历史实现直接取字符串，这里锁定不接受任何变体）。
func TestToInvocationMeta_MetadataVariantsIgnored(t *testing.T) {
	for _, key := range []string{"user_id", "userId", "UserID", "user"} {
		reqCtx := &a2asrv.RequestContext{
			Message: &a2a.Message{Metadata: map[string]any{key: "attacker"}},
		}
		meta := toInvocationMeta(context.Background(), reqCtx)
		if meta.userID != anonymousUserID {
			t.Fatalf("metadata key %q 影响了身份：%q", key, meta.userID)
		}
	}
}
