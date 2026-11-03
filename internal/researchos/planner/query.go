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

// aspectsCN 是中文维度词，用途与 aspects 相同。
//
// 没有它时，纯中文目标的所有检索分支会生成完全相同的检索式，多个 search
// 节点召回同一批资料，等于把一个检索分支复制了 N 份，覆盖度并没有提升。
var aspectsCN = []string{
	"现状",
	"方案对比",
	"风险与限制",
	"趋势",
	"评测基准",
}

// SearchQueryFor 依据研究目标与节点序号生成检索式。
//
// 规则：
//   - 目标中含英文技术词（如 "AI Agent"）时，构造 "<技术词> <维度>" 的英文检索式，
//     因为 GitHub / Hacker News / Wikipedia 对英文关键词召回质量显著更高；
//   - 纯中文目标保留原文，并追加中文维度词，交由支持中文的 Provider
//     （Bing / 中文维基）处理；
//   - 结果始终可解释：检索式取自目标本身 + 固定维度词，不做隐式扩写。
func SearchQueryFor(goal string, index int) string {
	goal = strings.TrimSpace(goal)
	core := asciiCore(goal)
	if core == "" {
		q := condenseCJK(goal)
		return q + " " + aspectsCN[index%len(aspectsCN)]
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

// 中文检索式的清洗原则是「不破坏语义」：
//   - 标点换成空格；
//   - 只剥掉位于目标首尾的研究动作词（研究/分析/调研…），因为那描述的是
//     「要做什么事」，不是「研究对象」；
//   - 不删除任何其它汉字。曾经把「检索增强生成」里的「生成」当动词删掉，
//     检索式退化成「向量数据库在检索增强 中 召回质量对比」；也曾经把「对比」
//     的「对」当介词删掉，剩一个「比」。这类删字都是语料损坏，交给检索引擎
//     自己分词更可靠。
var (
	cjkPunct = strings.NewReplacer(
		"，", " ", "。", " ", "、", " ", "：", " ", "；", " ",
		"？", " ", "！", " ", ",", " ", ".", " ", ":", " ", ";", " ",
		"（", " ", "）", " ", "(", " ", ")", " ", "「", " ", "」", " ",
		"\"", " ", "'", " ", "·", " ", "/", " ", "\\", " ",
	)
	cjkLeadVerb  = regexp.MustCompile(`^(?:请|帮我|帮忙|我们|我想)?(?:研究|调研|分析|梳理|总结|综述|评估|撰写|整理|比较|对比)+`)
	cjkTrailVerb = regexp.MustCompile(`(?:研究|调研|分析|梳理|总结|综述|评估|撰写|整理)+$`)
)

// condenseCJK 把中文目标压成较短的关键词串。
func condenseCJK(goal string) string {
	s := cjkPunct.Replace(goal)
	s = cjkLeadVerb.ReplaceAllString(s, " ")
	s = cjkTrailVerb.ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return strings.TrimSpace(goal)
	}
	out := strings.Join(fields, " ")
	if r := []rune(out); len(r) > 60 {
		out = string(r[:60])
	}
	return out
}

// FallbackQuery 在首选检索式无结果时使用的简化检索式。
//
// 中文目标截断时回退到最近的空格边界，避免把词切成半截。
func FallbackQuery(goal string) string {
	if core := asciiCore(goal); core != "" {
		return core
	}
	q := condenseCJK(goal)
	r := []rune(q)
	if len(r) <= 20 {
		return q
	}
	cut := 20
	for i := cut; i > 8; i-- {
		if r[i-1] == ' ' {
			cut = i - 1
			break
		}
	}
	return strings.TrimSpace(string(r[:cut]))
}
