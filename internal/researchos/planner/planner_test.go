package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/verikod/hector/internal/researchos/domain"
)

func validPlan() *domain.Plan {
	return &domain.Plan{
		Goal: "AI Agent 开源生态",
		Subtasks: []domain.Subtask{
			{ID: "search-1", Title: "检索", AgentRole: domain.RoleResearcher, NodeType: domain.NodeTypeSearch},
			{ID: "analyze-1", Title: "分析", AgentRole: domain.RoleAnalyst, NodeType: domain.NodeTypeAnalyze, DependsOn: []string{"search-1"}},
			{ID: "verify", Title: "验证", AgentRole: domain.RoleVerifier, NodeType: domain.NodeTypeVerify, DependsOn: []string{"analyze-1"}},
			{ID: "synthesize", Title: "综述", AgentRole: domain.RoleSynthesizer, NodeType: domain.NodeTypeSynthesize, DependsOn: []string{"verify"}},
		},
	}
}

func TestNormalizeAcceptsValidPlan(t *testing.T) {
	p := validPlan()
	if err := Normalize(p, Options{EnableVerification: true}); err != nil {
		t.Fatalf("合法计划不应报错：%v", err)
	}
	if len(p.ResearchQuestions) == 0 {
		t.Fatal("缺少研究问题时应自动补一条")
	}
}

func TestNormalizeEmptySubtasks(t *testing.T) {
	err := Normalize(&domain.Plan{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "subtasks") {
		t.Fatalf("空计划应被拒绝，实际：%v", err)
	}
}

func TestNormalizeRejectsIllegalID(t *testing.T) {
	p := validPlan()
	p.Subtasks[0].ID = "search 1/../etc"
	if err := Normalize(p, Options{}); err == nil {
		t.Fatal("非法 ID 应被拒绝")
	}
}

func TestNormalizeRejectsDuplicateID(t *testing.T) {
	p := validPlan()
	p.Subtasks[1].ID = "search-1"
	if err := Normalize(p, Options{}); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("重复 ID 应被拒绝，实际：%v", err)
	}
}

func TestNormalizeRejectsIllegalNodeType(t *testing.T) {
	p := validPlan()
	p.Subtasks[1].NodeType = "hack"
	if err := Normalize(p, Options{}); err == nil {
		t.Fatal("非法 node_type 应被拒绝")
	}
}

func TestNormalizeRequiresSynthesize(t *testing.T) {
	p := validPlan()
	p.Subtasks = p.Subtasks[:3] // 去掉 synthesize
	if err := Normalize(p, Options{}); err == nil || !strings.Contains(err.Error(), "synthesize") {
		t.Fatalf("缺少 synthesize 应被拒绝，实际：%v", err)
	}
}

func TestNormalizeRequiresSingleSynthesize(t *testing.T) {
	p := validPlan()
	p.Subtasks = append(p.Subtasks, domain.Subtask{
		ID: "synthesize-2", NodeType: domain.NodeTypeSynthesize, AgentRole: domain.RoleSynthesizer,
		DependsOn: []string{"verify"},
	})
	if err := Normalize(p, Options{}); err == nil || !strings.Contains(err.Error(), "仅") {
		t.Fatalf("多个 synthesize 应被拒绝，实际：%v", err)
	}
}

func TestNormalizeRejectsCycle(t *testing.T) {
	p := validPlan()
	// search-1 反向依赖 synthesize → 成环
	p.Subtasks[0].DependsOn = []string{"synthesize"}
	if err := Normalize(p, Options{}); err == nil {
		t.Fatal("有环计划应被拒绝")
	}
}

func TestNormalizeRequiresVerifyWhenEnabled(t *testing.T) {
	p := validPlan()
	p.Subtasks = []domain.Subtask{
		{ID: "search-1", NodeType: domain.NodeTypeSearch},
		{ID: "analyze-1", NodeType: domain.NodeTypeAnalyze, DependsOn: []string{"search-1"}},
		{ID: "synthesize", NodeType: domain.NodeTypeSynthesize, DependsOn: []string{"analyze-1"}},
	}
	if err := Normalize(p, Options{EnableVerification: true}); err == nil || !strings.Contains(err.Error(), "verify") {
		t.Fatalf("开启验证但缺少 verify 节点应被拒绝，实际：%v", err)
	}
}

func TestNormalizeDropsSelfAndDuplicateDeps(t *testing.T) {
	p := validPlan()
	p.Subtasks[1].DependsOn = []string{"search-1", "search-1", "analyze-1", " "}
	if err := Normalize(p, Options{}); err != nil {
		t.Fatalf("应容忍可清理的依赖：%v", err)
	}
	deps := p.Subtasks[1].DependsOn
	if len(deps) != 1 || deps[0] != "search-1" {
		t.Fatalf("依赖未去重/未剔除自依赖：%v", deps)
	}
}

func TestHeuristicPlanIsValid(t *testing.T) {
	for _, depth := range []string{"quick", "standard", "deep"} {
		p, usage, err := New(nil).Plan(context.Background(), "AI Agent 开源生态现状", Options{
			Depth: depth, EnableVerification: true, MaxQuestions: 3,
		})
		if err != nil {
			t.Fatalf("depth=%s 规划失败：%v", depth, err)
		}
		if err := Normalize(p, Options{EnableVerification: true}); err != nil {
			t.Fatalf("depth=%s 规划结果未通过校验：%v", depth, err)
		}
		if len(p.Subtasks) == 0 {
			t.Fatalf("depth=%s 规划为空", depth)
		}
		if usage.TotalTokens < 0 {
			t.Fatalf("token 用量不应为负")
		}
	}
}

func TestHeuristicAlwaysEndsWithSynthesize(t *testing.T) {
	p, _, err := New(nil).Plan(context.Background(), "联邦学习在 Wi-Fi CSI 行为识别中的应用", Options{
		Depth: "standard", MaxQuestions: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	last := p.Subtasks[len(p.Subtasks)-1]
	if last.NodeType != domain.NodeTypeSynthesize {
		t.Fatalf("末尾节点应为 synthesize，实际 %q", last.NodeType)
	}
}

func TestSearchQueryForProducesDistinctQueries(t *testing.T) {
	goal := "AI Agent 开源生态现状"
	a := SearchQueryFor(goal, 0)
	b := SearchQueryFor(goal, 1)
	if a == "" || b == "" {
		t.Fatal("检索式不应为空")
	}
	if a == b {
		t.Fatalf("不同维度的检索式不应相同：%q", a)
	}
	if len([]rune(a)) > 120 {
		t.Fatalf("检索式过长：%d", len([]rune(a)))
	}
}

func TestFallbackQueryNonEmpty(t *testing.T) {
	if q := FallbackQuery("量子计算商业化"); q == "" {
		t.Fatal("兜底检索式不应为空")
	}
}

// 中文检索式不能从词中间删字。
//
// 回归用例：曾把「检索增强生成」中的「生成」当动词剥掉，检索式退化成
// 「向量数据库在检索增强 中 召回质量对比」，Bing 直接返回无关页面，
// 进而导致整条任务的 analyze 阶段拿不到可用正文而失败。
func TestCondenseCJKPreservesContentWords(t *testing.T) {
	goal := "向量数据库在检索增强生成中的召回质量对比"
	got := SearchQueryFor(goal, 0)
	for _, must := range []string{"向量数据库", "检索增强生成", "召回质量"} {
		if !strings.Contains(got, must) {
			t.Fatalf("检索式 %q 丢失了内容词 %q", got, must)
		}
	}
	if strings.Contains(got, "检索增强 中") {
		t.Fatalf("检索式 %q 仍在固定搭配中间插入了空格", got)
	}
}

func TestCondenseCJKStripsBoundaryVerbsOnly(t *testing.T) {
	if got := condenseCJK("分析 MCP(协议) 现状"); strings.HasPrefix(got, "分析") {
		t.Fatalf("句首研究动作词应被剥离：%q", got)
	}
	if got := condenseCJK("向量数据库召回质量研究"); strings.HasSuffix(got, "研究") {
		t.Fatalf("句末研究动作词应被剥离：%q", got)
	}
}

// 纯中文目标的不同检索分支必须生成不同检索式。
//
// 回归用例：此前中文分支直接返回 condenseCJK(goal)，index 被忽略，
// 多个 search 节点召回同一批资料，覆盖度并没有随分支数增加。
func TestSearchQueryForCJKDiffersByIndex(t *testing.T) {
	goal := "向量数据库在检索增强生成中的召回质量对比"
	a := SearchQueryFor(goal, 0)
	b := SearchQueryFor(goal, 1)
	c := SearchQueryFor(goal, 2)
	if a == b || b == c || a == c {
		t.Fatalf("中文目标的不同维度检索式不应相同：%q / %q / %q", a, b, c)
	}
	for _, q := range []string{a, b, c} {
		if !strings.Contains(q, "检索增强生成") {
			t.Fatalf("检索式 %q 丢失了内容词", q)
		}
		if n := len([]rune(q)); n > 45 {
			t.Fatalf("中文检索式过长（%d rune）：%q", n, q)
		}
	}
}

func TestFallbackQueryCutsAtWordBoundary(t *testing.T) {
	goal := "向量数据库在检索增强生成中的召回质量对比与技术选型建议"
	fb := FallbackQuery(goal)
	if fb == "" {
		t.Fatal("兜底检索式不应为空")
	}
	if r := []rune(fb); len(r) > 20 {
		t.Fatalf("兜底检索式应截断到 20 rune，实际 %d：%q", len(r), fb)
	}
	if strings.HasSuffix(fb, " ") {
		t.Fatalf("兜底检索式不应以空格结尾：%q", fb)
	}
	if !strings.HasPrefix(condenseCJK(goal), fb) {
		t.Fatalf("兜底检索式应为压缩检索式的前缀，实际 %q", fb)
	}
}
