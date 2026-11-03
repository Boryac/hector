package exec

import (
	"math"
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

// 相对差异不能出现负数或天文数字：这两处都曾经算错，且错值会直接印进报告。
func TestRelativeSpreadEdgeCases(t *testing.T) {
	cases := []struct {
		min, max float64
		want     float64
	}{
		{100, 100, 0},   // 完全相同
		{0, 0, 0},       // 全为 0：旧写法得 -1（印成 -100%）
		{0, 23.6, 1},    // 最小值为 0：旧写法得 2.36e10（印成 2359999999900%）
		{100, 110, 0.1}, // 常规
		{100, 160, 0.6}, // 常规
		{-5, 10, 3},     // 负最小值走绝对值分母
	}
	for _, c := range cases {
		got := relativeSpread(c.min, c.max)
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("relativeSpread(%v,%v)=%v，期望 %v", c.min, c.max, got, c.want)
		}
	}

	// 印到报告里的百分比不应带负号，也不应超过 100%（用 0 作基线时最大差异就是 100%）。
	for _, v := range [][2]float64{{0, 0}, {0, 23.6}, {0, 9999}} {
		_, note, _ := detectConflict([]float64{v[0], v[1]}, 2)
		if strings.Contains(note, "-") {
			t.Fatalf("差异说明出现负号：%q", note)
		}
	}
}

func TestDetectConflictNumericSpread(t *testing.T) {
	conflict, note, group := detectConflict([]float64{100, 160}, 2)
	if !conflict {
		t.Fatalf("相差 60%% 的数值应判为冲突，note=%q", note)
	}
	if group == "" {
		t.Fatal("冲突应带有分组标识")
	}
	if !strings.Contains(note, "100") && !strings.Contains(note, "160") {
		t.Fatalf("说明应包含具体数值，实际 %q", note)
	}
	if !strings.Contains(note, "2 个来源页面") {
		t.Fatalf("冲突说明应写明涉及页面数，实际 %q", note)
	}
}

func TestDetectConflictWithinTolerance(t *testing.T) {
	conflict, _, _ := detectConflict([]float64{100, 110}, 2)
	if conflict {
		t.Fatal("相差 10% 的数值不应判为冲突")
	}
	conflict, _, _ = detectConflict([]float64{100}, 1)
	if conflict {
		t.Fatal("单值不应判为冲突")
	}
}

// 只有 1 个来源页面时，不能把「同一页内多处数值相同」说成「多来源印证」；
// 也不能把同一页内的数值分歧说成「不同来源冲突」。
func TestDetectConflictSingleSourceWording(t *testing.T) {
	_, note, _ := detectConflict([]float64{16000, 16000}, 1)
	if strings.Contains(note, "多来源") || strings.Contains(note, "数值一致") {
		t.Fatalf("单页面不得声称多来源数值一致，实际 %q", note)
	}
	if !strings.Contains(note, "仅 1 个来源页面") {
		t.Fatalf("应明确说明只有单页面，实际 %q", note)
	}

	_, note, _ = detectConflict([]float64{16000, 16000}, 2)
	if !strings.Contains(note, "2 个来源页面数值一致") {
		t.Fatalf("两页面应写明页面数，实际 %q", note)
	}

	conflict, note, _ := detectConflict([]float64{100, 160}, 1)
	if !conflict {
		t.Fatal("同页内的显著数值分歧仍应被识别")
	}
	if strings.Contains(note, "不同来源冲突") || !strings.Contains(note, "同一来源页面内") {
		t.Fatalf("单页面的分歧不得说成不同来源冲突，实际 %q", note)
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

// 四条验证分支都必须可达。此前这段判定内联在循环里，只有 partially_supported
// 在真实数据上出现过，其余三条无人验证过是否真的能落到。
func TestVerificationStatusBranches(t *testing.T) {
	cases := []struct {
		name     string
		conflict bool
		sites    int
		evidence int
		want     string
		minConf  float64
		maxConf  float64
	}{
		{"冲突优先于来源数", true, 3, 3, domain.ClaimConflicting, 0.4, 0.4},
		{"双来源支持", false, 2, 2, domain.ClaimSupported, 0.6, 0.7},
		{"三来源支持", false, 3, 3, domain.ClaimSupported, 0.7, 0.8},
		{"来源极多仍封顶 0.95", false, 10, 10, domain.ClaimSupported, 0.9, 0.95},
		{"单来源有证据", false, 1, 2, domain.ClaimPartial, 0.5, 0.55},
		{"无来源无证据", false, 0, 0, domain.ClaimUnverified, 0.3, 0.3},
		{"单站点但零证据算未验证", false, 1, 0, domain.ClaimUnverified, 0.3, 0.3},
	}
	for _, c := range cases {
		got, conf := verificationStatus(c.conflict, c.sites, c.evidence)
		if got != c.want {
			t.Fatalf("%s：状态应为 %s，实际 %s", c.name, c.want, got)
		}
		if conf < c.minConf || conf > c.maxConf {
			t.Fatalf("%s：置信度应在 [%.2f,%.2f]，实际 %.2f", c.name, c.minConf, c.maxConf, conf)
		}
	}
}
