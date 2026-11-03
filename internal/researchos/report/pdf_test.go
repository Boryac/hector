package report

import (
	"bytes"
	"strings"

	"github.com/signintech/gopdf"
	"testing"
)

// 长文档样例：中文 + 英文长 URL + 表格 + 列表，足以跨多页。
func longMarkdown() string {
	var b strings.Builder
	b.WriteString("# pgvector 与 Qdrant 向量检索对比研究\n\n")
	b.WriteString("## 摘要\n\n本轮研究对比了 pgvector 0.7 与 Qdrant 1.8 在千万级向量检索场景下的召回率、写入吞吐与运维成本。结论：pgvector 在 1kw 数据、HNSW 索引下召回率 0.98 [1]，Qdrant 量化后内存占用低 42% [2]，两者差距在工程可接受范围内。\n\n")
	for i := 0; i < 12; i++ {
		b.WriteString("## 主题：技术架构对比\n\npgvector 以扩展形式运行在 PostgreSQL 事务边界内，适合已有 PG 技术栈的团队 [1]。Qdrant 是 Rust 实现的专用向量数据库，原生支持分布式分片与标量过滤 [2]。\n\n")
		b.WriteString("| 指标 | pgvector | Qdrant |\n|---|---|---|\n| 召回率@10 | 0.98 | 0.97 |\n| 写入吞吐 | 8k/s | 15k/s |\n\n")
		b.WriteString("- 要点一：召回率受 ef_search 与 m 参数影响显著，实测 m=16、ef_search=200 时两者的差距收敛到 1% 以内 [1]。\n- 要点二：过滤条件复杂时 Qdrant 的 payload index 更快，延迟低 30% [2]。\n\n")
	}
	b.WriteString("## 信息来源\n\n")
	b.WriteString("1. [pgvector: Open-source vector similarity search for Postgres](https://github.com/pgvector/pgvector) — github.com\n")
	b.WriteString("2. [Qdrant - Vector Database](https://qdrant.tech/) — qdrant.tech\n")
	b.WriteString("3. [超长无空格链接测试](https://example.com/very/long/path/without/any/space/segment-one/segment-two/segment-three/segment-four) — example.com\n")
	return b.String()
}

func TestRenderPDFProducesValidFile(t *testing.T) {
	data, err := RenderPDF("标题", longMarkdown())
	if err != nil {
		t.Fatalf("RenderPDF: %v", err)
	}
	if len(data) < 1000 {
		t.Fatalf("输出过小: %d 字节", len(data))
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("缺少 PDF 头: %q", data[:min(8, len(data))])
	}
	if !bytes.Contains(data, []byte("%%EOF")) {
		t.Fatal("缺少 PDF 结尾标记") // 内容为两个百分号加 EOF，此处不能写成格式化指令
	}
}

func TestRenderPDFDeterministic(t *testing.T) {
	a, err := RenderPDF("标题", longMarkdown())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := RenderPDF("标题", longMarkdown())
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("同一输入两次渲染结果不一致（页脚或排版含随机因素）")
	}
}

// 内容完整性：所有章节标题、来源编号、URL 都必须出现在最终 PDF 里。
// 通过内部两次渲染一致性 + 覆盖清洗计数为 0 来保证；文本级验证走
// scripts/verify_pdf_content.py（端到端，见 docs/USAGE.md）。
func TestRenderPDFFontCoverageClean(t *testing.T) {
	md := longMarkdown()
	sanitized, replaced := sanitizeForFont(md)
	if replaced != 0 {
		t.Fatalf("常规报告文本不应有字体覆盖缺口，实际替换 %d 处", replaced)
	}
	if sanitized != md {
		t.Fatal("覆盖清洗不应改动常规文本")
	}
}

func TestSanitizeReplacesUncoveredRunes(t *testing.T) {
	// U+2A6A2（CJK 扩展B，字体未收录）应替换为 □ 且计数 1
	in := "正常文字𪚢结尾"
	out, n := sanitizeForFont(in)
	if n != 1 {
		t.Fatalf("期望替换 1 处，实际 %d", n)
	}
	if !strings.Contains(out, "□") {
		t.Fatalf("替换结果缺 □: %q", out)
	}
	if strings.ContainsRune(out, '𪚢') {
		t.Fatal("未收录字符未被替换")
	}
}

func TestWrapBreaksCJKAnywhereButKeepsLatinWords(t *testing.T) {
	r := testRenderer(t)
	const maxW = 200.0
	// 能放进一行的英文单词必须整体换行，不能拆散
	word := strings.Repeat("word", 5) // 20 字符，远短于一行
	lines := r.wrap("中文开头 "+word+" 中文结尾", styleBody.size, maxW)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, word) {
		t.Fatalf("能放进行宽的英文单词被拆散: %q", lines)
	}
	// 纯中文按宽度断行：每行都不超宽
	cjk := strings.Repeat("向", 100)
	for _, l := range r.wrap(cjk, styleBody.size, maxW) {
		if w, _ := r.pdf.MeasureTextWidth(l); w > maxW+1 {
			t.Fatalf("中文断行超宽 %f: %q", w, l)
		}
	}
}

func TestWrapHardSplitsLongURL(t *testing.T) {
	r := testRenderer(t)
	const maxW = 200.0
	url := "https://example.com/" + strings.Repeat("segment/", 20)
	lines := r.wrap(url, styleBody.size, maxW)
	if len(lines) < 3 {
		t.Fatalf("长 URL 应硬拆成多行，实际 %d 行", len(lines))
	}
	for _, l := range lines {
		if w, _ := r.pdf.MeasureTextWidth(l); w > maxW+1 {
			t.Fatalf("硬拆后仍超宽 %f: %q", w, l)
		}
	}
}

func TestTableColumnWidthsFitPage(t *testing.T) {
	r := testRenderer(t)
	blk := ParseBlocks("| 指标 | 极长列名向量数据库召回率基准测试与写入吞吐对比说明 | 短 |\n|---|---|---|\n| a | " + strings.Repeat("数据", 40) + " | c |\n")[0]
	if err := r.table(blk); err != nil {
		t.Fatalf("table: %v", err)
	}
	// 内容底部不能越过正文区（允许 1pt 浮点误差）
	if r.y > pdfContentBottom+1 {
		t.Fatalf("表格渲染越界: y=%f > %f", r.y, pdfContentBottom)
	}
}

func TestRenderPDFPaginates(t *testing.T) {
	// 直接用内部渲染拿到页数，长文必须跨页且页脚两次渲染页数一致
	first, err := renderPDFOnce(longMarkdown(), false, 0)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.pages < 2 {
		t.Fatalf("长文档应产生多页，实际 %d 页", first.pages)
	}
	second, err := renderPDFOnce(longMarkdown(), true, first.pages)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.pages != first.pages {
		t.Fatalf("两遍渲染页数不一致: %d vs %d", first.pages, second.pages)
	}
}

func testRenderer(t *testing.T) *pdfRenderer {
	t.Helper()
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFontData("cjk", embeddedFont); err != nil {
		t.Fatalf("加载内嵌字体: %v", err)
	}
	pdf.AddPage()
	r := &pdfRenderer{pdf: pdf, y: pdfMarginT}
	r.setFontSize(styleBody.size) // MeasureTextWidth 依赖当前字体
	return r
}
