package exec

// 交叉验证的集成测试：真的写库、真的跑 verify 节点、真的读回状态。
//
// 为什么需要它：单元测试只能证明「聚类函数」和「状态映射」正确，证明不了
// 这两段在真实数据流上被正确地接起来——真实网页数据里几乎不会出现两条字面
// 相近的结论，于是 supported / conflicting 两条分支长期没有实际到达过。
//
// 运行方式（需要真实 PostgreSQL）：
//   RESEARCHOS_TEST_DSN='postgres://postgres@127.0.0.1:55432/researchos?sslmode=disable' \
//     go test ./internal/researchos/exec/ -run TestVerifyIntegration -v
// 未设置该变量时跳过，不影响常规 go test。

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/store"
)

func TestVerifyIntegration(t *testing.T) {
	dsn := os.Getenv("RESEARCHOS_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 RESEARCHOS_TEST_DSN，跳过集成测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("连接数据库失败：%v", err)
	}
	defer pool.Close()
	st := store.New(pool)

	taskID := uuid.NewString()
	task := &domain.Task{
		ID: taskID, Title: "[itest] 交叉验证", Goal: "[itest] 交叉验证",
		Depth: "quick", Status: domain.TaskRunning, CreatedBy: "local-user",
		PlannerMode: "heuristic", MaxAgents: 1, MaxDurationSeconds: 60,
	}
	if err := st.CreateTask(ctx, task); err != nil {
		t.Fatalf("建任务失败：%v", err)
	}
	t.Cleanup(func() {
		// 外键是 ON DELETE CASCADE，删任务即清干净。
		_, _ = pool.Exec(context.Background(), "DELETE FROM research_tasks WHERE id=$1", taskID)
	})

	nodeID := uuid.NewString()
	if err := st.CreateNode(ctx, &domain.Node{
		ID: nodeID, TaskID: taskID, Title: "交叉验证", Type: domain.NodeTypeVerify,
		AgentRole: domain.RoleVerifier, Position: 0, MaxAttempts: 1,
	}); err != nil {
		t.Fatalf("建节点失败：%v", err)
	}

	// 四个来源，分属四个独立站点。
	mkSource := func(site, url string) string {
		src := &domain.Source{
			ID: uuid.NewString(),
			TaskID: taskID, NodeID: nodeID, URL: url, NormalizedURL: url,
			Title: site, Site: site, Provider: "test", Status: "ok",
			Credibility: "medium", Excerpt: "测试正文",
		}
		saved, err := st.UpsertSource(ctx, src)
		if err != nil {
			t.Fatalf("建来源失败：%v", err)
		}
		return saved.ID
	}
	sA := mkSource("alpha.example", "https://alpha.example/a")
	sB := mkSource("beta.example", "https://beta.example/b")
	sC := mkSource("gamma.example", "https://gamma.example/c")
	sD := mkSource("delta.example", "https://delta.example/d")

	// 结论 + 证据：前两条是同一事实的两个独立来源（应 supported）；
	// 中间两条是同一指标的冲突数值（应 conflicting）；最后一条没有证据（应 unverified）。
	mkClaim := func(statement string, value *float64, key string, sources ...string) *domain.Claim {
		c := &domain.Claim{
			ID: uuid.NewString(),
			TaskID: taskID, NodeID: nodeID, Statement: statement,
			NormalizedKey: key, Status: domain.ClaimUnverified, Confidence: 0.3,
			NumericValue: value,
		}
		saved, err := st.InsertClaim(ctx, c)
		if err != nil {
			t.Fatalf("建结论失败：%v", err)
		}
		for _, sid := range sources {
			if _, err := st.InsertEvidence(ctx, &domain.Evidence{
				ID: uuid.NewString(),
				TaskID: taskID, ClaimID: saved.ID, SourceID: sid, NodeID: nodeID,
				Quote: statement,
			}); err != nil {
				t.Fatalf("建证据失败：%v", err)
			}
		}
		return saved
	}
	f := func(v float64) *float64 { return &v }
	supported1 := mkClaim("Qdrant 在百万向量规模下的召回率为 0.95", f(0.95), "k1", sA)
	supported2 := mkClaim("Qdrant 在百万向量规模下的召回率达到 0.95", f(0.95), "k2", sB)
	conflict1 := mkClaim("pgvector 的写入吞吐为每秒 1200 条", f(1200), "k3", sC)
	conflict2 := mkClaim("pgvector 的写入吞吐为每秒 400 条", f(400), "k4", sD)
	noEvidence := mkClaim("Milvus 在大规模集群下的运维成本更低", nil, "k5")

	eng := New(st, nil, nil, nil, Config{})
	// Session 的 store 字段未导出：同包测试直接注入真实 store，
	// 这样 Progress/Emit 会真的写库，验证链路而不是绕过它。
	sess := &Session{
		Ctx: ctx, Task: task, store: st, engine: eng,
		Node: &domain.Node{ID: nodeID, TaskID: taskID, Type: domain.NodeTypeVerify},
	}
	out, _, err := eng.runVerify(sess)
	if err != nil {
		t.Fatalf("verify 执行失败：%v", err)
	}
	t.Logf("节点输出：%v", out)

	get := func(id string) *domain.Claim {
		c, err := st.GetClaim(ctx, taskID, id)
		if err != nil {
			t.Fatalf("读回结论失败：%v", err)
		}
		return c
	}
	if c := get(supported1.ID); c.Status != domain.ClaimSupported || c.SourceCount < 2 {
		t.Fatalf("双来源结论应 supported(sites>=2)，实际 %s sites=%d", c.Status, c.SourceCount)
	}
	if c := get(supported2.ID); c.Status != domain.ClaimSupported {
		t.Fatalf("双来源结论应 supported，实际 %s", c.Status)
	}
	if c := get(conflict1.ID); c.Status != domain.ClaimConflicting {
		t.Fatalf("数值冲突结论应 conflicting，实际 %s（notes=%q）", c.Status, c.VerifierNotes)
	}
	if c := get(conflict2.ID); c.Status != domain.ClaimConflicting {
		t.Fatalf("数值冲突结论应 conflicting，实际 %s", c.Status)
	}
	if c := get(noEvidence.ID); c.Status != domain.ClaimUnverified {
		t.Fatalf("无证据结论应 unverified，实际 %s", c.Status)
	}
	// 冲突对必须落在同一个冲突组，供报告「争议与不确定性」章节引用。
	if g1, g2 := get(conflict1.ID).ConflictGroup, get(conflict2.ID).ConflictGroup; g1 == "" || g1 != g2 {
		t.Fatalf("冲突双方应共享冲突组，实际 %q / %q", g1, g2)
	}
	t.Logf("PASS：supported=%v conflicting=%v unverified=%v 冲突组=%q",
		out["supported"], out["conflicting"], out["unverified"], get(conflict1.ID).ConflictGroup)
}
