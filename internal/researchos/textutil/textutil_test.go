package textutil

import (
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://Example.com/a/?utm_source=x&utm_medium=y#frag": "https://example.com/a",
		"http://example.com/a/":                                 "http://example.com/a",
		"https://example.com/a?q=1&utm_campaign=z":              "https://example.com/a?q=1",
		"HTTPS://EXAMPLE.COM/Path":                              "https://example.com/Path",
	}
	for in, want := range cases {
		got, err := NormalizeURL(in)
		if err != nil {
			t.Fatalf("NormalizeURL(%q) 报错：%v", in, err)
		}
		if got != want {
			t.Fatalf("NormalizeURL(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestNormalizeURLRejectsBadScheme(t *testing.T) {
	for _, in := range []string{"ftp://x.com/a", "javascript:alert(1)", "file:///c:/x"} {
		if _, err := NormalizeURL(in); err == nil {
			t.Fatalf("%q 应被拒绝", in)
		}
	}
}

func TestHost(t *testing.T) {
	if got := Host("https://news.ycombinator.com/item?id=1"); got != "news.ycombinator.com" {
		t.Fatalf("Host 解析错误：%q", got)
	}
	if got := Host("not a url"); got != "" {
		t.Fatalf("非法 URL 应返回空字符串，实际 %q", got)
	}
}

func TestHashStable(t *testing.T) {
	a := Hash("同一段文本")
	b := Hash("同一段文本")
	if a != b {
		t.Fatal("相同输入哈希应一致")
	}
	if a == Hash("不同文本") {
		t.Fatal("不同输入哈希不应相同")
	}
	if len(a) != 64 {
		t.Fatalf("应为 SHA256 十六进制（64 字符），实际 %d", len(a))
	}
}

func TestExtractHTMLCleansBoilerplate(t *testing.T) {
	doc := `<html><head><title>测试 &amp; 标题</title></head><body>
	<nav>首页 | 登录 | 注册</nav>
	<main><p>这是第一段正文，讲述了 AI Agent 的开源生态现状与主要参与者。</p>
	<p>这是第二段正文，包含更多细节说明与引用数据。</p></main>
	<footer>版权所有 隐私政策</footer></body></html>`
	title, text := ExtractHTML(doc)
	if !strings.Contains(title, "测试") {
		t.Fatalf("标题解析失败：%q", title)
	}
	if !strings.Contains(text, "第一段正文") || !strings.Contains(text, "第二段正文") {
		t.Fatalf("正文抽取丢失内容：%q", text)
	}
	if strings.Contains(text, "登录") || strings.Contains(text, "版权") {
		t.Fatalf("正文不应包含导航/页脚样板：%q", text)
	}
}

func TestExtractHTMLStripsWikiMarkup(t *testing.T) {
	doc := `<main><p>{{flatlist|* [[Time (magazine)|''Time'']]}} 正文内容在这里，描述该领域的发展。</p></main>`
	_, text := ExtractHTML(doc)
	if strings.Contains(text, "[[") || strings.Contains(text, "''") || strings.Contains(text, "{{") {
		t.Fatalf("维基标记未清理：%q", text)
	}
}

func TestCleanText(t *testing.T) {
	got := CleanText("  多个   空格\t制表\n\n换行   ")
	if strings.Contains(got, "  ") || strings.Contains(got, "\t") {
		t.Fatalf("空白未压缩：%q", got)
	}
}

func TestExcerptPreferTerms(t *testing.T) {
	text := strings.Repeat("无关铺垫内容。", 20) + "这里提到 vector database 与检索增强生成。" + strings.Repeat("后续无关内容。", 20)
	ex := Excerpt(text, []string{"vector database"}, 80)
	if !strings.Contains(ex, "vector database") {
		t.Fatalf("摘要应命中关键词，实际：%q", ex)
	}
	if len([]rune(ex)) > 80 {
		t.Fatalf("摘要超长：%d", len([]rune(ex)))
	}
}

func TestSentenceStopsAtCJKPeriod(t *testing.T) {
	text := "第一句话。第二句话。第三句话。"
	s := Sentence(text, []string{"第二句"}, 100)
	if !strings.Contains(s, "第二句") {
		t.Fatalf("应返回含关键词的句子，实际：%q", s)
	}
	if strings.Contains(s, "第三句") {
		t.Fatalf("不应跨越句号拼接：%q", s)
	}
}

func TestTermsSplitsCJKAndASCII(t *testing.T) {
	terms := Terms("AI Agent 开源生态与 Federated Learning")
	joined := strings.Join(terms, "|")
	for _, want := range []string{"agent", "federated", "learning"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("术语缺失 %q：%v", want, terms)
		}
	}
	if len(terms) == 0 {
		t.Fatal("术语列表不应为空")
	}
}

func TestWordCount(t *testing.T) {
	if n := WordCount("hello world"); n < 2 {
		t.Fatalf("英文词数应 ≥2，实际 %d", n)
	}
	if n := WordCount("中文内容也有长度"); n < 4 {
		t.Fatalf("中文按字数计应 ≥4，实际 %d", n)
	}
	if n := WordCount(""); n != 0 {
		t.Fatalf("空串应为 0，实际 %d", n)
	}
}

// 抽出的句子必须是原文的逐字子串。
//
// 回归用例：Sentence 在文本末尾补了一个「。」，导致证据引文不再等于原文，
// 验收脚本逐字比对时判为「引文未在原文找到」（Codex 词条、Generative AI 词条各中一条）。
func TestSentenceIsVerbatimSubstring(t *testing.T) {
	cases := []struct {
		text  string
		terms []string
	}{
		{"Codex is an AI coding agent developed by Open AI for software engineering tasks", []string{"codex"}},
		{"向量数据库通过近似最近邻索引加速检索，召回率与延迟需要权衡", []string{"向量数据库"}},
		{strings.Repeat("这是一段很长的正文，用于验证截断路径不会追加省略号。", 30), []string{"正文"}},
	}
	for _, c := range cases {
		got := Sentence(c.text, c.terms, 120)
		if got == "" {
			t.Fatalf("未抽出句子：%q", c.text[:20])
		}
		if !strings.Contains(c.text, got) {
			t.Fatalf("句子不是原文子串：%q 不在 %q 中", got, c.text[:40])
		}
	}
}
