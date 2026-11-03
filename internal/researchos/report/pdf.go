package report

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/signintech/gopdf"
)

// RenderPDF 把报告 Markdown 排版为 A4 PDF。
//
// 排版规则：
//   - 中英混排换行：CJK 逐字可断，拉丁单词整体不断行（长 URL 等无空格长串硬拆）；
//   - 表格按内容等比分配列宽，跨页时重复表头；
//   - 每页页脚带「第 N 页 / 共 M 页」（两遍渲染：第一遍数页数，第二遍补页脚）；
//   - 字体内嵌（见 font.go），字体覆盖不到的码点在排版前替换为 '□' 并如实计数。
//
// 与 HTML 导出共用 ParseBlocks，内容结构一致；同一输入永远产出字节级相同的 PDF
// （页脚刻意不含时间戳），便于回归测试与内容比对。
func RenderPDF(title, markdown string) ([]byte, error) {
	// 第一遍：确定总页数。页脚画在预留的底部边距里，不影响分页，两遍分页必然一致。
	first, err := renderPDFOnce(markdown, false, 0)
	if err != nil {
		return nil, err
	}
	final, err := renderPDFOnce(markdown, true, first.pages)
	if err != nil {
		return nil, err
	}
	_ = title // 标题由正文首行 `# ...` 承载，与 HTML 版式一致
	return final.buf.Bytes(), nil
}

type pdfResult struct {
	buf          *bytes.Buffer
	pages        int
	fallbackRuns int // 字体覆盖不到而被替换为 '□' 的字符数
}

// A4 版心（单位 pt）。页脚画在底部边距内，正文永远不会与它重叠。
const (
	pdfPageW         = 595.28
	pdfPageH         = 841.89
	pdfMarginL       = 52.0
	pdfMarginR       = 52.0
	pdfMarginT       = 56.0
	pdfMarginB       = 64.0
	pdfContentW      = pdfPageW - pdfMarginL - pdfMarginR
	pdfContentBottom = pdfPageH - pdfMarginB
	pdfFooterY       = pdfPageH - 34.0
)

type pdfTextStyle struct {
	size  float64
	lineH float64 // 行高倍数
	color [3]uint8
}

var (
	styleH1   = pdfTextStyle{size: 19, lineH: 1.45, color: [3]uint8{17, 24, 39}}
	styleH2   = pdfTextStyle{size: 14, lineH: 1.5, color: [3]uint8{17, 24, 39}}
	styleH3   = pdfTextStyle{size: 12, lineH: 1.5, color: [3]uint8{31, 41, 55}}
	styleBody = pdfTextStyle{size: 10.5, lineH: 1.62, color: [3]uint8{31, 41, 55}}
	styleMute = pdfTextStyle{size: 9, lineH: 1.5, color: [3]uint8{107, 114, 128}}
	styleCell = pdfTextStyle{size: 9.5, lineH: 1.45, color: [3]uint8{31, 41, 55}}
)

const (
	pdfSpaceBeforeH2 = 16.0
	pdfSpaceBeforeH3 = 12.0
	pdfGapPara       = 7.0
	pdfGapListItem   = 4.0
	pdfMinColW       = 56.0
)

type pdfRenderer struct {
	pdf        *gopdf.GoPdf
	y          float64
	pages      int
	totalPages int // 第二遍渲染时的总页数；0 = 第一遍
	withFooter bool
}

func renderPDFOnce(markdown string, withFooter bool, totalPages int) (*pdfResult, error) {
	fontData, fontSrc, err := fontBytes()
	if err != nil {
		return nil, err
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFontData("cjk", fontData); err != nil {
		return nil, fmt.Errorf("加载 PDF 字体失败（来源 %s）: %w", fontSrc, err)
	}

	res := &pdfResult{}
	r := &pdfRenderer{pdf: pdf, y: pdfMarginT, withFooter: withFooter, totalPages: totalPages}
	r.newPage()
	res.pages = 1

	body, replaced := sanitizeForFont(markdown)
	res.fallbackRuns = replaced
	blocks := ParseBlocks(body)

	olSeq := 0
	for _, blk := range blocks {
		switch blk.Type {
		case blockH1:
			r.heading(blk.Text, styleH1, 0, false)
			r.y += 8 // 文档标题之后留出明显间距，拉开与首个章节的层级
		case blockH2:
			olSeq = 0
			r.heading(blk.Text, styleH2, pdfSpaceBeforeH2, true)
		case blockH3:
			r.heading(blk.Text, styleH3, pdfSpaceBeforeH3, false)
		case blockPara:
			r.paragraph(blk.Text, styleBody, 0)
		case blockQuote:
			r.paragraph(blk.Text, styleMute, pdfGapPara)
		case blockUL:
			olSeq = 0
			r.listItem("•", blk.Text)
		case blockOL:
			olSeq++
			r.listItem(fmt.Sprintf("%d.", olSeq), blk.Text)
		case blockTable:
			olSeq = 0
			if err := r.table(blk); err != nil {
				return nil, err
			}
		}
	}

	if withFooter {
		r.drawFooter(r.pages)
	}

	res.pages = r.pages
	out := &bytes.Buffer{}
	if err := pdf.Write(out); err != nil {
		return nil, fmt.Errorf("写出 PDF 失败: %w", err)
	}
	res.buf = out
	return res, nil
}

func (r *pdfRenderer) newPage() {
	r.pdf.AddPage()
	r.pages++
	r.y = pdfMarginT
}

func (r *pdfRenderer) ensureSpace(h float64) {
	if r.y+h > pdfContentBottom && r.y > pdfMarginT {
		if r.withFooter {
			r.drawFooter(r.pages)
		}
		r.newPage()
	}
}

// drawFooter 在当前页画页脚：左侧产品名，右侧「第 N 页 / 共 M 页」。
func (r *pdfRenderer) drawFooter(pageNo int) {
	r.pdf.SetTextColor(styleMute.color[0], styleMute.color[1], styleMute.color[2])
	r.setFontSize(styleMute.size)
	r.pdf.SetXY(pdfMarginL, pdfFooterY)
	_ = r.pdf.Cell(nil, "Hector ResearchOS")
	right := fmt.Sprintf("第 %d 页 / 共 %d 页", pageNo, r.totalPages)
	w, err := r.pdf.MeasureTextWidth(right)
	if err != nil {
		return
	}
	r.pdf.SetXY(pdfPageW-pdfMarginR-w, pdfFooterY)
	_ = r.pdf.Cell(nil, right)
}

// pdfToken 是断行最小单元：拉丁词整体，CJK 与标点逐字。
type pdfToken struct {
	s string
	w float64
}

// wrap 按可用宽度断行。measure 用当前字体；文本必须已 sanitize（否则测量会报错）。
func (r *pdfRenderer) wrap(text string, size float64, maxW float64) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	measure := func(s string) float64 {
		w, err := r.pdf.MeasureTextWidth(s)
		if err != nil {
			// sanitize 之后不该发生；兜底按字数估，保证排版不中断
			return float64(len([]rune(s))) * size
		}
		return w
	}

	var toks []pdfToken
	var word []rune
	flushWord := func() {
		if len(word) > 0 {
			s := string(word)
			toks = append(toks, pdfToken{s, measure(s)})
			word = word[:0]
		}
	}
	for _, rn := range text {
		switch {
		case rn == ' ':
			flushWord()
			toks = append(toks, pdfToken{" ", measure(" ")})
		case rn > 0x2E7F: // CJK 统一表意文字及其标点：逐字可断
			flushWord()
			s := string(rn)
			toks = append(toks, pdfToken{s, measure(s)})
		default:
			word = append(word, rn)
		}
	}
	flushWord()

	var lines []string
	var cur []pdfToken
	curW := 0.0
	appendLine := func() {
		var sb strings.Builder
		for _, t := range cur {
			sb.WriteString(t.s)
		}
		lines = append(lines, strings.TrimSpace(sb.String()))
		cur = nil
		curW = 0
	}
	for _, t := range toks {
		if curW+t.w <= maxW || len(cur) == 0 {
			cur = append(cur, t)
			curW += t.w
			continue
		}
		if cur[len(cur)-1].s == " " { // 行尾空格抖掉再断
			curW -= cur[len(cur)-1].w
			cur = cur[:len(cur)-1]
		}
		appendLine()
		if t.s != " " {
			cur = append(cur, t)
			curW = t.w
		}
	}
	if len(cur) > 0 {
		appendLine()
	}

	// 无空格长串（长 URL）超宽时逐字符二分硬拆
	hardSplit := make([]string, 0, len(lines))
	for _, line := range lines {
		for measure(line) > maxW && len([]rune(line)) > 1 {
			runes := []rune(line)
			lo, hi := 1, len(runes)
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if measure(string(runes[:mid])) <= maxW {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			hardSplit = append(hardSplit, string(runes[:lo]))
			line = strings.TrimLeft(string(runes[lo:]), " ")
			if line == "" {
				break
			}
		}
		if line != "" {
			hardSplit = append(hardSplit, line)
		}
	}
	return hardSplit
}

func (r *pdfRenderer) setFontSize(size float64) {
	_ = r.pdf.SetFont("cjk", "", size)
}

func (r *pdfRenderer) heading(text string, st pdfTextStyle, spaceBefore float64, rule bool) {
	plain := plainText(text)
	if plain == "" {
		return
	}
	r.y += spaceBefore
	r.pdf.SetTextColor(st.color[0], st.color[1], st.color[2])
	r.setFontSize(st.size)
	for _, line := range r.wrap(plain, st.size, pdfContentW) {
		r.ensureSpace(st.size * st.lineH)
		r.pdf.SetXY(pdfMarginL, r.y)
		_ = r.pdf.Cell(nil, line)
		r.y += st.size * st.lineH
	}
	if rule { // H2 下加一条浅分隔线，与 HTML 版式对应
		r.y += 5
		r.ensureSpace(1)
		r.pdf.SetLineType("solid")
		r.pdf.SetLineWidth(0.6)
		r.pdf.SetStrokeColor(229, 231, 235)
		r.pdf.Line(pdfMarginL, r.y, pdfPageW-pdfMarginR, r.y)
		r.y += 9
	}
}

func (r *pdfRenderer) paragraph(text string, st pdfTextStyle, spaceBefore float64) {
	plain := plainText(text)
	if plain == "" {
		return
	}
	r.y += spaceBefore
	r.pdf.SetTextColor(st.color[0], st.color[1], st.color[2])
	r.setFontSize(st.size)
	for _, line := range r.wrap(plain, st.size, pdfContentW) {
		r.ensureSpace(st.size * st.lineH)
		r.pdf.SetXY(pdfMarginL, r.y)
		_ = r.pdf.Cell(nil, line)
		r.y += st.size * st.lineH
	}
	r.y += pdfGapPara
}

func (r *pdfRenderer) listItem(marker, text string) {
	st := styleBody
	plain := plainText(text)
	if plain == "" {
		return
	}
	const indent = 16.0
	r.pdf.SetTextColor(st.color[0], st.color[1], st.color[2])
	r.setFontSize(st.size)
	lines := r.wrap(plain, st.size, pdfContentW-indent)
	for i, line := range lines {
		r.ensureSpace(st.size * st.lineH)
		if i == 0 {
			r.pdf.SetXY(pdfMarginL, r.y)
			_ = r.pdf.Cell(nil, marker)
			mw, _ := r.pdf.MeasureTextWidth(marker)
			r.pdf.SetXY(pdfMarginL+mw+5, r.y)
		} else {
			r.pdf.SetXY(pdfMarginL+indent, r.y)
		}
		_ = r.pdf.Cell(nil, line)
		r.y += st.size * st.lineH
	}
	r.y += pdfGapListItem
}

// table 渲染表格：列宽按内容等比缩放到页宽、表头带底色、跨页重复表头。
func (r *pdfRenderer) table(b block) error {
	n := len(b.Header)
	if n == 0 {
		return nil
	}
	pad := 4.0
	r.setFontSize(styleCell.size) // 后续测量依赖当前字体

	measure := func(s string) float64 {
		w, err := r.pdf.MeasureTextWidth(s)
		if err != nil {
			return float64(len([]rune(s))) * styleCell.size
		}
		return w
	}

	// 自然列宽 = 各列最宽单元格；先保证不窄于 minCol，再等比压到页宽
	nat := make([]float64, n)
	maxOf := func(cells []string) {
		for i, c := range cells {
			if i >= n {
				break
			}
			if w := measure(plainText(c)) + 2*pad; w > nat[i] {
				nat[i] = w
			}
		}
	}
	maxOf(b.Header)
	for _, row := range b.Rows {
		maxOf(row)
	}

	colW := make([]float64, n)
	sum := 0.0
	for _, w := range nat {
		sum += w
	}
	if sum <= pdfContentW {
		extra := (pdfContentW - sum) / float64(n)
		for i, w := range nat {
			colW[i] = w + extra
		}
	} else {
		scale := pdfContentW / sum
		over := 0.0
		for i, w := range nat {
			colW[i] = w * scale
			if colW[i] < pdfMinColW {
				colW[i] = pdfMinColW
			}
			over += colW[i]
		}
		// 保底列抬价后可能又超宽，按比例压回去
		if over > pdfContentW {
			k := pdfContentW / over
			for i := range colW {
				colW[i] *= k
			}
		}
	}

	// rowLayout 计算一行的换行与高度。
	rowLayout := func(cells []string) ([][]string, float64) {
		linesPer := make([][]string, n)
		h := 0.0
		for i := 0; i < n; i++ {
			cell := ""
			if i < len(cells) {
				cell = plainText(cells[i])
			}
			linesPer[i] = r.wrap(cell, styleCell.size, colW[i]-2*pad)
			if len(linesPer[i]) == 0 {
				linesPer[i] = []string{""}
			}
			if hh := float64(len(linesPer[i]))*styleCell.size*styleCell.lineH + 2*pad; hh > h {
				h = hh
			}
		}
		return linesPer, h
	}

	// drawRow 绘制一行（cells 对应 linesPer），含单元格边框与表头底色。
	drawRow := func(cells []string, linesPer [][]string, h float64, header bool) {
		if header {
			r.pdf.SetFillColor(243, 244, 246)
			r.pdf.RectFromUpperLeftWithStyle(pdfMarginL, r.y, colSum(colW), h, "F")
		}
		x := pdfMarginL
		for i := 0; i < n; i++ {
			r.pdf.SetTextColor(styleCell.color[0], styleCell.color[1], styleCell.color[2])
			r.setFontSize(styleCell.size)
			ty := r.y + pad
			for _, line := range linesPer[i] {
				r.pdf.SetXY(x+pad, ty)
				_ = r.pdf.Cell(nil, line)
				ty += styleCell.size * styleCell.lineH
			}
			r.pdf.SetLineType("solid")
			r.pdf.SetLineWidth(0.5)
			r.pdf.SetStrokeColor(217, 220, 225)
			r.pdf.RectFromUpperLeftWithStyle(x, r.y, colW[i], h, "D")
			x += colW[i]
		}
		r.y += h
	}

	// drawOne 放一行，放不下就换页并重画表头。
	drawOne := func(cells []string, header bool) error {
		linesPer, h := rowLayout(cells)
		if r.y+h > pdfContentBottom && r.y > pdfMarginT {
			if r.withFooter {
				r.drawFooter(r.pages)
			}
			r.newPage()
			if !header { // 数据行跨页：先在新页重复表头
				hLines, hh := rowLayout(b.Header)
				drawRow(b.Header, hLines, hh, true)
			}
		}
		drawRow(cells, linesPer, h, header)
		return nil
	}

	if err := drawOne(b.Header, true); err != nil {
		return err
	}
	for _, row := range b.Rows {
		if err := drawOne(row, false); err != nil {
			return err
		}
	}
	r.y += pdfGapPara
	return nil
}

func colSum(cols []float64) float64 {
	s := 0.0
	for _, c := range cols {
		s += c
	}
	return s
}
