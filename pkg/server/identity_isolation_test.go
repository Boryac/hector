package server

// Phase 0 security review — A2A Issue #2 identity isolation evidence.
//
// These tests document the trust boundary of `user_id` as of commit
// abe472b6deb12b3dbba290dbec44cd3be27e163d (main).
//
// Finding: toInvocationMeta() first binds userID to validated JWT claims,
// then UNCONDITIONALLY lets client-supplied Message.Metadata["user_id"]
// override it. A caller can therefore impersonate any user, and session /
// task stores keyed on (app_name, user_id, ...) become cross-user accessible
// together with a known/guessable context_id.
//
// The tests below PROVE the override behavior. They are expected to PASS
// against the vulnerable code (i.e. they demonstrate the vulnerability).
// After a fix (server-side identity binding: metadata must never override
// claims), TestToInvocationMeta_MetadataMustNotOverrideClaims must be
// updated to assert claims win, and this file renamed/split accordingly.

import (
	"context"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/verikod/hector/pkg/auth"
)

// TestToInvocationMeta_ForgedUserIDOverridesClaims demonstrates Issue #2:
// an authenticated user "alice" can assert an arbitrary user_id ("bob")
// via A2A message metadata, and the server accepts it.
func TestToInvocationMeta_ForgedUserIDOverridesClaims(t *testing.T) {
	claims := &auth.Claims{Subject: "alice"}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	reqCtx := &a2asrv.RequestContext{
		ContextID: "session-abc",
		TaskID:    "task-1",
		Message: &a2a.Message{
			Metadata: map[string]any{
				// Attacker-controlled field:
				"user_id": "bob",
			},
		},
	}

	meta := toInvocationMeta(ctx, reqCtx)

	if meta.userID != "bob" {
		t.Fatalf("forged user_id not honored — got %q (vulnerability may be fixed; update this test)", meta.userID)
	}
	// Session key components are both client-influenced:
	if meta.sessionID != "session-abc" {
		t.Fatalf("expected client-provided context_id as sessionID, got %q", meta.sessionID)
	}
	t.Logf("CONFIRMED: authenticated user %q is replaced by forged metadata user_id %q",
		claims.Subject, meta.userID)
}

// TestToInvocationMeta_UnauthenticatedCallerPicksAnyUserID shows that without
// auth the metadata field alone decides identity — full impersonation.
func TestToInvocationMeta_UnauthenticatedCallerPicksAnyUserID(t *testing.T) {
	reqCtx := &a2asrv.RequestContext{
		ContextID: "session-xyz",
		Message: &a2a.Message{
			Metadata: map[string]any{"user_id": "victim"},
		},
	}

	meta := toInvocationMeta(context.Background(), reqCtx)
	if meta.userID != "victim" {
		t.Fatalf("expected forged user_id %q, got %q", "victim", meta.userID)
	}
	t.Logf("CONFIRMED: unauthenticated request adopts arbitrary user_id %q", meta.userID)
}

// TestToInvocationMeta_ClaimsUsedWhenNoMetadata documents the intended path
// (claims.Subject) so the contrast with the forged case is explicit.
func TestToInvocationMeta_ClaimsUsedWhenNoMetadata(t *testing.T) {
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Subject: "alice"})
	reqCtx := &a2asrv.RequestContext{ContextID: "s1"}

	meta := toInvocationMeta(ctx, reqCtx)
	if meta.userID != "alice" {
		t.Fatalf("expected claims subject, got %q", meta.userID)
	}
}

// TestToInvocationMeta_DefaultUserFallback documents the anonymous fallback.
func TestToInvocationMeta_DefaultUserFallback(t *testing.T) {
	meta := toInvocationMeta(context.Background(), &a2asrv.RequestContext{})
	if meta.userID != "default" {
		t.Fatalf("expected default user, got %q", meta.userID)
	}
}
