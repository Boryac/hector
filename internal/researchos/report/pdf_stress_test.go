package report

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// RC1 阶段6 压力测试：真实报告形态的边界输入，全部走 RenderPDF 全流程。

// stressMarkdown 生成一份带指定数量章节/表格行/长URL的「恶意」报告。
func stressMarkdown(sections, tableRows, longURLs int) string {
	var b strings.Builder
	b.WriteString("# RC1 压力测试报告\n\n")
	b.WriteString("中英文混排 English mixed with 中文，带符号：± °C μm § ① ≠ «» — …\n\n")
	for i := 0; i < longURLs; i++ {
		fmt.Fprintf(&b, "- [超长链接 %d](https://example.com/very/long/path/%s?query=%d)\n", i, strings.Repeat("segment-", 12), i)
	}
	b.WriteString("\n")
	for s := 0; s < sections; s++ {
		fmt.Fprintf(&b, "## 章节 %d 一级标题 Heading\n\n", s)
		fmt.Fprintf(&b, "### 小节 %d.%d 二级标题\n\n", s, 1)
		long := strings.Repeat("这一段是很长的中文正文，用来测试 CJK 逐字断行在满行时不溢出。", 20)
		fmt.Fprintf(&b, "%s\n\n", long)
		latin := strings.Repeat("Supercalifragilisticexpialidocious-antidisestablishmentarianism ", 10)
		fmt.Fprintf(&b, "%s\n\n", latin)
		b.WriteString("| 指标 | 数值 | 说明列内容较长的情况也会出现 |\n|---|---:|---|\n")
		for r := 0; r < tableRows; r++ {
			fmt.Fprintf(&b, "| 行 %d-%d | %d.%02d | 表格单元格中文内容较长时列宽等比缩放并不越界 |\n", s, r, r*13, r%100)
		}
		b.WriteString("\n> 引用块：块引用中的中文与 English 混排也要正常断行。\n\n")
		b.WriteString("- 无序列表项甲\n- 无序列表项乙\n1. 有序列表项一\n2. 有序列表项二\n\n")
	}
	b.WriteString("## 结论\n\n含缺字码点测试：🚀𝔘𝔫𝔦𝔠𝔬𝔡𝔢✅罕见符号应计为缺字并替换为占位符。\n")
	return b.String()
}

func assertPDFStructure(t *testing.T, pdf []byte) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("缺少 PDF 头")
	}
	if !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("缺少 PDF 结尾标记")
	}
	if len(pdf) < 1000 {
		t.Fatalf("输出过小: %d 字节", len(pdf))
	}
}

func TestRenderPDFStressLargeReport(t *testing.T) {
	md := stressMarkdown(12, 30, 15) // 大报告：12 章 × 30 行表格 + 15 条超长 URL
	pdf, err := RenderPDF("RC1 压力测试", md)
	if err != nil {
		t.Fatalf("大报告渲染失败: %v", err)
	}
	assertPDFStructure(t, pdf)
	// 两遍渲染页数一致（页码页脚依赖该一致性）；文本级页脚校验走
	// scripts/verify_pdf_content.py（pymupdf 解压后比对）。
	first, err := renderPDFOnce(md, false, 0)
	if err != nil {
		t.Fatalf("renderPDFOnce first pass: %v", err)
	}
	second, err := renderPDFOnce(md, true, first.pages)
	if err != nil {
		t.Fatalf("renderPDFOnce second pass: %v", err)
	}
	if second.pages != first.pages {
		t.Fatalf("两遍渲染页数不一致: %d vs %d", first.pages, second.pages)
	}
	if first.pages < 5 {
		t.Fatalf("大报告应产生多页，实际 %d 页", first.pages)
	}
}

func TestRenderPDFStressEdgeTables(t *testing.T) {
	// 空表格 + 单列多行 + 超宽单元格
	md := "## 空表头表格\n\n| | |\n|---|---|\n\n## 单列多行\n\n| 只有说明 |\n|---|\n"
	for i := 0; i < 25; i++ {
		md += fmt.Sprintf("| 第 %d 行内容较长的单元格文本确保换行处理正确不会越界 |\n", i)
	}
	pdf, err := RenderPDF("边界表格", md)
	if err != nil {
		t.Fatalf("边界表格渲染失败: %v", err)
	}
	assertPDFStructure(t, pdf)
}

func TestRenderPDFStressConcurrency(t *testing.T) {
	md := stressMarkdown(4, 10, 5)
	const n = 8
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		first []byte
		errs  []error
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pdf, err := RenderPDF("并发测试", md)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if first == nil {
				first = pdf
			} else if !bytes.Equal(first, pdf) {
				errs = append(errs, fmt.Errorf("并发渲染输出不一致"))
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("并发渲染 %d 次，出现 %d 个问题，第一个: %v", n, len(errs), errs[0])
	}
}

func TestRenderPDFFontFallbackRenders(t *testing.T) {
	// 缺字码点必须替换为 □ 而非报错或静默丢字；渲染流程必须完整。
	md := "正常中文前后🚀夹着缺字符号𝔘与✅。"
	pdf, err := RenderPDF("缺字测试", md)
	if err != nil {
		t.Fatalf("缺字文档渲染失败: %v", err)
	}
	assertPDFStructure(t, pdf)
	sanitized, n := sanitizeForFont(md)
	if n < 3 {
		t.Fatalf("期望至少替换 3 处缺字（🚀𝔘✅），实际 %d", n)
	}
	if strings.ContainsAny(sanitized, "🚀𝔘✅") {
		t.Fatal("缺字码点未被替换")
	}
}

func TestRenderPDFFontOverrideRejectsNonTTF(t *testing.T) {
	// 外部字体路径指向非 TTF 内容时必须报错（而不是产出坏 PDF）。
	t.Setenv("RESEARCHOS_PDF_FONT", "OFL.txt") // 存在但非 TTF
	if _, _, err := fontBytes(); err == nil {
		t.Fatal("非 TTF 字体应导致 fontBytes 报错")
	}
	if _, err := RenderPDF("标题", "# 标题\n\n正文"); err == nil {
		t.Fatal("外部字体非法时 RenderPDF 应报错而不是产出坏文件")
	}
}
