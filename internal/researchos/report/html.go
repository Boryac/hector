package report

import (
	"fmt"
	"html"
	"strings"
)

// RenderHTML 把报告 Markdown 渲染为独立 HTML 页面。
//
// 块解析与 PDF 导出共用 ParseBlocks（见 blocks.go），保证两种导出内容一致；
// 所有文本先做 HTML 转义，链接只允许 http(s)，避免注入。
func RenderHTML(title, markdown string) string {
	body := renderBlocks(markdown)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
:root{--fg:#1a1a1a;--muted:#5b6270;--line:#e5e7eb;--accent:#1f6feb;--bg:#ffffff}
*{box-sizing:border-box}
body{margin:0;background:#f6f7f9;color:var(--fg);font:16px/1.75 -apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
article{max-width:860px;margin:32px auto;background:var(--bg);padding:48px 56px;border:1px solid var(--line);border-radius:12px}
h1{font-size:30px;margin:0 0 24px;letter-spacing:-.4px}
h2{font-size:22px;margin:36px 0 12px;padding-bottom:8px;border-bottom:1px solid var(--line)}
h3{font-size:17px;margin:24px 0 8px}
p{margin:12px 0}
ul,ol{padding-left:22px}
li{margin:6px 0}
a{color:var(--accent);text-decoration:none}
a:hover{text-decoration:underline}
table{border-collapse:collapse;width:100%%;margin:16px 0;font-size:14px}
th,td{border:1px solid var(--line);padding:8px 10px;text-align:left}
th{background:#f3f4f6}
code{background:#f3f4f6;padding:1px 5px;border-radius:4px;font-size:13px}
blockquote{margin:12px 0;padding:8px 14px;border-left:3px solid var(--line);color:var(--muted)}
footer{margin-top:40px;color:var(--muted);font-size:12px}
</style>
</head>
<body>
<article>
%s
<footer>由 Hector ResearchOS 生成 · 引用编号对应文末「信息来源」</footer>
</article>
</body>
</html>`, html.EscapeString(title), body)
}

func renderBlocks(md string) string {
	var b strings.Builder
	inList := false
	inOrdered := false
	inTable := false

	closeList := func() {
		if inList {
			if inOrdered {
				b.WriteString("</ol>\n")
			} else {
				b.WriteString("</ul>\n")
			}
			inList = false
			inOrdered = false
		}
	}
	closeTable := func() {
		if inTable {
			b.WriteString("</table>\n")
			inTable = false
		}
	}

	for _, blk := range ParseBlocks(md) {
		// 列表与表格的闭合：除连续列表项外都先闭合
		isListItem := blk.Type == blockUL || blk.Type == blockOL
		if !isListItem {
			closeList()
		}
		if blk.Type != blockTable {
			closeTable()
		}

		switch blk.Type {
		case blockH1:
			b.WriteString("<h1>" + inline(blk.Text) + "</h1>\n")
		case blockH2:
			b.WriteString("<h2>" + inline(blk.Text) + "</h2>\n")
		case blockH3:
			b.WriteString("<h3>" + inline(blk.Text) + "</h3>\n")
		case blockPara:
			b.WriteString("<p>" + inline(blk.Text) + "</p>\n")
		case blockQuote:
			b.WriteString("<blockquote>" + inline(blk.Text) + "</blockquote>\n")
		case blockUL:
			if !inList || inOrdered {
				closeList()
				b.WriteString("<ul>\n")
				inList, inOrdered = true, false
			}
			b.WriteString("<li>" + inline(blk.Text) + "</li>\n")
		case blockOL:
			if !inList || !inOrdered {
				closeList()
				b.WriteString("<ol>\n")
				inList, inOrdered = true, true
			}
			b.WriteString("<li>" + inline(blk.Text) + "</li>\n")
		case blockTable:
			b.WriteString("<table>\n<thead><tr>")
			for _, cell := range blk.Header {
				b.WriteString("<th>" + inline(cell) + "</th>")
			}
			b.WriteString("</tr></thead>\n<tbody>\n")
			for _, row := range blk.Rows {
				b.WriteString("<tr>")
				for _, cell := range row {
					b.WriteString("<td>" + inline(cell) + "</td>")
				}
				b.WriteString("</tr>\n")
			}
			b.WriteString("</tbody>\n</table>\n")
			inTable = true
		}
	}
	closeList()
	closeTable()
	return b.String()
}

func inline(s string) string {
	// 先转义，再处理标记，防止 HTML 注入。
	escaped := html.EscapeString(s)
	escaped = linkRe.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := linkRe.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		target := sub[2]
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			return sub[1]
		}
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, target, sub[1])
	})
	escaped = boldRe.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = italicRe.ReplaceAllString(escaped, "<em>$1</em>")
	return escaped
}
