package exec

import (
	"strings"
	"testing"

	"github.com/verikod/hector/internal/researchos/domain"
)

func claim(id, statement string, val *float64) *domain.Claim {
	return &domain.Claim{ID: id, Statement: statement, NumericValue: val, Status: domain.ClaimUnverified}
}

func TestJaccardIdenticalAndDisjoint(t *testing.T) {
	a := tokenSet("LangGraph 支持有状态图式编排")
	b := tokenSet("LangGraph 支持有状态图式编排")
	if got := jaccard(a, b); got != 1 {
		t.Fatalf("相同文本 Jaccard 应为 1，实际 %v", got)
	}
	c := tokenSet("CrewAI 提供角色化多智能体协作")
	if got := jaccard(a, c); got > 0.2 {
		t.Fatalf("无关文本 Jaccard 应接近 0，实际 %v", got)
	}
	if got := jaccard(tokenSet(""), tokenSet("x y")); got != 0 {
		t.Fatalf("空集合应为 0，实际 %v", got)
	}
}

func TestClusterClaimsGroupsSimilar(t *testing.T) {
	claims := []*domain.Claim{
		claim("c1", "LangGraph 支持有状态图式编排，适合多 Agent 工作流", nil),
		claim("c2", "LangGraph 支持有状态图式编排，适合多 Agent 工作流场景", nil),
		claim("c3", "向量数据库用于检索增强生成的语义召回", nil),
	}
	groups := clusterClaims(claims)
	if len(groups) != 2 {
		t.Fatalf("应聚成 2 组，实际 %d 组：%+v", len(groups), groups)
	}
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	if total != 3 {
		t.Fatalf("聚类不得丢结论，实际 %d 条", total)
	}
	// 相似的两条必须在同一组。
	var same bool
	for _, g := range groups {
		ids := map[string]bool{}
		for _, c := range g {
			ids[c.ID] = true
		}
		if ids["c1"] && ids["c2"] {
			same = true
		}
		if ids["c3"] && ids["c1"] {
			t.Fatal("无关结论不应与相似结论同组")
		}
	}
	if !same {
		t.Fatal("相似结论未被聚到同一组")
	}
}

func TestDetectConflictNumericSpread(t *testing.T) {
	conflict, note, group := detectConflict([]float64{100, 160})
	if !conflict {
		t.Fatalf("相差 60%% 的数值应判为冲突，note=%q", note)
	}
	if group == "" {
		t.Fatal("冲突应带有分组标识")
	}
	if !strings.Contains(note, "100") && !strings.Contains(note, "160") {
		t.Fatalf("说明应包含具体数值，实际 %q", note)
	}
}

func TestDetectConflictWithinTolerance(t *testing.T) {
	conflict, _, _ := detectConflict([]float64{100, 110})
	if conflict {
		t.Fatal("相差 10% 的数值不应判为冲突")
	}
	conflict, _, _ = detectConflict([]float64{100})
	if conflict {
		t.Fatal("单值不应判为冲突")
	}
}

func TestQuoteGrounded(t *testing.T) {
	excerpt := "LangGraph 是 LangChain 团队推出的有状态编排框架，支持多 Agent 协作。"
	if !quoteGrounded("有状态编排框架", excerpt) {
		t.Fatal("原文子串应判定为有据")
	}
	if quoteGrounded("完全捏造的一段话", excerpt) {
		t.Fatal("原文中不存在的引文必须判为无据")
	}
	if quoteGrounded("", excerpt) {
		t.Fatal("空引文应判为无据")
	}
	if quoteGrounded("有状态", "短") {
		t.Fatal("过短的原文不足以支撑引用")
	}
}

func TestExtractNumber(t *testing.T) {
	cases := map[string]float64{
		"周下载量为 120 万":  1200000,
		"增长了 25%":      25,
		"star 数 3.2k":  3200,
		"仅有 1,200 次提交": 1200,
		"没有任何数字":       0,
	}
	for in, want := range cases {
		got := extractNumber(in)
		if got == nil {
			if want == 0 {
				continue
			}
			t.Fatalf("应从 %q 中解析出数值", in)
		}
		if *got != want {
			t.Fatalf("解析 %q 得到 %v，期望 %v", in, *got, want)
		}
	}
}

func TestDowngradeCredibility(t *testing.T) {
	if downgrade("high") == "high" {
		t.Fatal("降级应降低可信度")
	}
	if credibilityWeight("high") <= credibilityWeight("low") {
		t.Fatal("可信度权重应满足 high > low")
	}
}

func TestRuleBasedClaimsRespectsLimitAndGrounding(t *testing.T) {
	sources := []*domain.Source{
		{ID: "s1", Title: "LangGraph", Excerpt: "LangGraph 支持有状态图式编排，用于构建多 Agent 工作流。", Credibility: "high", URL: "https://a.com/1"},
		{ID: "s2", Title: "AutoGen", Excerpt: "AutoGen 提供多 Agent 对话框架，支持工具调用与代码执行。", Credibility: "high", URL: "https://b.com/2"},
	}
	claims := ruleBasedClaims(sources, "多 Agent 框架", 5)
	if len(claims) == 0 {
		t.Fatal("规则抽取应至少产出 1 条结论")
	}
	if len(claims) > 5 {
		t.Fatalf("应遵守上限，实际 %d", len(claims))
	}
	for _, c := range claims {
		if strings.TrimSpace(c.Statement) == "" {
			t.Fatal("结论陈述不能为空")
		}
		if strings.TrimSpace(c.SourceID) == "" {
			t.Fatalf("结论 %q 缺少来源（规则抽取必须带 SourceID）", c.Statement)
		}
		if strings.TrimSpace(c.Quote) == "" {
			t.Fatalf("结论 %q 缺少引文", c.Statement)
		}
	}
}
