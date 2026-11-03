package planner

import (
	"regexp"
	"strings"
)

// ASCII 技术词识别：用于把中文研究目标转成对英文索引（GitHub / HN / Wikipedia）友好的检索式。
var asciiWordRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+#._-]{1,30}`)

var queryStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true, "into": true,
	"research": true, "study": true, "analyze": true, "analysis": true, "report": true,
	"global": true, "world": true, "overview": true, "about": true, "using": true,
	"of": true, "in": true, "on": true, "to": true, "a": true, "an": true,
}

// aspects 是检索维度模板（与启发式规划器的研究问题一一对应）。
var aspects = []string{
	"open source projects",
	"framework comparison",
	"ecosystem adoption risks",
	"roadmap and trends",
	"benchmark evaluation",
}

// SearchQueryFor 依据研究目标与节点序号生成检索式。
//
// 规则：
//   - 目标中含英文技术词（如 "AI Agent"）时，构造 "<技术词> <维度>" 的英文检索式，
//     因为 GitHub / Hacker News / Wikipedia 对英文关键词召回质量显著更高；
//   - 纯中文目标保留原文，交由支持中文的 Provider（Bing / 中文维基）处理；
//   - 结果始终可解释：检索式取自目标本身 + 固定维度词，不做隐式扩写。
func SearchQueryFor(goal string, index int) string {
	goal = strings.TrimSpace(goal)
	core := asciiCore(goal)
	if core == "" {
		return condenseCJK(goal)
	}
	aspect := aspects[index%len(aspects)]
	return core + " " + aspect
}

// asciiCore 抽取目标中的英文技术词并去停用词。
func asciiCore(goal string) string {
	words := asciiWordRe.FindAllString(goal, -1)
	out := make([]string, 0, len(words))
	seen := map[string]bool{}
	for _, w := range words {
		lw := strings.ToLower(w)
		if queryStopwords[lw] || seen[lw] || len(lw) < 2 {
			continue
		}
		seen[lw] = true
		out = append(out, w)
		if len(out) >= 3 {
			break
		}
	}
	return strings.Join(out, " ")
}

var cjkTrim = strings.NewReplacer(
	"，", " ", "。", " ", "、", " ", "：", " ", "；", " ",
	"研究", " ", "分析", " ", "生成", " ", "筛选", " ", "报告", " ",
	"的", " ", "与", " ", "和", " ",
)

// condenseCJK 把中文目标压成较短的关键词串。
func condenseCJK(goal string) string {
	s := cjkTrim.Replace(goal)
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return goal
	}
	out := strings.Join(fields, " ")
	if r := []rune(out); len(r) > 60 {
		out = string(r[:60])
	}
	return out
}

// FallbackQuery 在首选检索式无结果时使用的简化检索式。
func FallbackQuery(goal string) string {
	if core := asciiCore(goal); core != "" {
		return core
	}
	r := []rune(condenseCJK(goal))
	if len(r) > 20 {
		return string(r[:20])
	}
	return string(r)
}
