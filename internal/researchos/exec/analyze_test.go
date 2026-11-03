package exec

import (
	"strings"
	"testing"

	"github.com/verikod/hector/internal/researchos/domain"
)

// 单段落的长正文必须被判为可用。
//
// 回归用例：维基百科正文抽取出来常常只有 1–2 个换行，曾经因为死守
// 「至少 3 行、每行 ≥40 字」把 3999 rune 的完整条目判成不可用正文，
// analyze 因此拿到 0 条资料，整条任务在 synthesize 阶段失败。
func TestExcerptUsableSingleParagraph(t *testing.T) {
	sentence := "检索增强生成把外部知识检索与语言模型解码结合起来，从而降低事实性幻觉的比例。"
	para := strings.Repeat(sentence, 12) // 单段落，无换行
	if n := len([]rune(para)); n < 400 {
		t.Fatalf("用例正文过短：%d", n)
	}
	src := &domain.Source{Status: "ok", Excerpt: para}
	if !excerptUsable(src) {
		t.Fatal("单段落长正文应判为可用")
	}
}

func TestExcerptUsableMultiLine(t *testing.T) {
	line := strings.Repeat("这是一段长度足够的正文行，用来验证多行判定逻辑。", 2)
	src := &domain.Source{Status: "ok", Excerpt: strings.Join([]string{line, line, line, line}, "\n")}
	if !excerptUsable(src) {
		t.Fatal("多行长正文应判为可用")
	}
}

func TestExcerptUsableRejectsFragments(t *testing.T) {
	cases := []string{
		"",
		"短句。",
		strings.Repeat("导航 首页 登录 注册 ", 20), // 无句末标点
	}
	for _, c := range cases {
		if excerptUsable(&domain.Source{Status: "ok", Excerpt: c}) {
			t.Fatalf("碎片正文不应判为可用：%q", c)
		}
	}
}

func TestSnippetUsableGuards(t *testing.T) {
	long := strings.Repeat("向量数据库的召回质量取决于索引结构与嵌入模型。", 6)
	if !snippetUsable(long) {
		t.Fatal("带句读的长摘要应判为可用")
	}
	if snippetUsable("首页 登录 注册 关于 联系") {
		t.Fatal("标签串不应判为可用")
	}
}

func TestCountSentenceEnds(t *testing.T) {
	if got := countSentenceEnds("甲。乙！丙？"); got != 3 {
		t.Fatalf("应统计到 3 个句末标点，实际 %d", got)
	}
}

// 书单式罗列不能被当成结论。
//
// 回归用例：某公共仓库 README 的书籍目录被抽成「一句话结论」写进报告，
// 报告核心发现里出现的是十几个书名拼接。
func TestUsableStatementRejectsListLikeText(t *testing.T) {
	bookList := "配套学习资源等)PDF下载_唯美映像,《数据分析之图算法 基于Spark和Neo4j》PDF下载_" +
		"[英]马克·尼达姆,《云计算SaaS服务演化》PDF下载_何俊,《Word/Excel/PPT办公应用教程》PDF下载_谢力"
	if usableStatement(bookList) {
		t.Fatal("书名罗列不应通过结论校验")
	}
	long := strings.Repeat("这是一段很长的说明文字，用来验证长度上限。", 10)
	if usableStatement(long) {
		t.Fatal("超过 120 字的整段文字不应通过结论校验")
	}
	good := "截至 2025 年，pgvector 在 100 万条向量规模下的召回率约为 0.95。"
	if !usableStatement(good) {
		t.Fatal("正常陈述句应通过结论校验")
	}
}

// 与问题词面完全不重叠的资料必须被剔除。
func TestRelevantEnough(t *testing.T) {
	q := "分析：向量数据库在检索增强生成中的召回质量对比 的现状"
	off := &domain.Source{Title: "个人收藏书籍列表", Excerpt: strings.Repeat("本书单收录了若干计算机教材，供个人学习使用。", 20)}
	if relevantEnough(q, off) {
		t.Fatal("无关资料不应判为相关")
	}
	on := &domain.Source{Title: "检索增强生成 - 维基百科", Excerpt: strings.Repeat("检索增强生成通过检索外部知识来提升答案质量。", 20)}
	if !relevantEnough(q, on) {
		t.Fatal("同主题资料应判为相关")
	}
}

func TestClipRunes(t *testing.T) {
	if got := clipRunes("向量数据库召回质量", 4); got != "向量数据" {
		t.Fatalf("按 rune 截断结果错误：%q", got)
	}
	if got := clipRunes("abc", 10); got != "abc" {
		t.Fatalf("未超长时应原样返回：%q", got)
	}
}

// 以半个单词开头的碎片不能作为结论。
//
// 回归用例：抓取正文按窗口截取，窗口首尾落在词中间，产出
// 「s AI agent deleted a database despite explicit instructions not to make any changes。」
func TestUsableStatementRejectsMidWordFragment(t *testing.T) {
	for _, s := range []string{
		"s AI agent deleted a database despite explicit instructions not to make changes。",
		"of AI on workplace and productivity, executives projected AI to increase productivity by 1.",
	} {
		if usableStatement(s) {
			t.Fatalf("半词碎片不应通过结论校验：%q", s)
		}
	}
	if !startsWithSentenceCase("AI agent 生态在 2025 年快速扩张。") {
		t.Fatal("正常句子应通过首字符校验")
	}
	if startsWithSentenceCase("s AI agent") {
		t.Fatal("小写字母开头应被拒绝")
	}
}

// GitHub 界面文案与维基编者提示不能进结论。
func TestUsableStatementRejectsUIText(t *testing.T) {
	for _, s := range []string{
		"500-AI-Agents-Projects Notifications You must be signed in to change notification settings Star 39.",
		"Period of rapid progress in AI This article is about the acceleration of technological advancements.",
	} {
		if usableStatement(s) {
			t.Fatalf("界面文案不应通过结论校验：%q", s)
		}
	}
}
