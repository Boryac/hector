package report

import (
	"regexp"
	"strings"
)

// Markdown 块解析：报告生成器实际产出的子集（标题/段落/无序与有序列表/表格/引用行）。
// html.go 与 pdf.go 共用这份解析，保证两种导出的内容与结构一致——
// 不共用的话，「HTML 里有、PDF 里没有」这类内容丢失只会被抓包时发现。

type blockType int

const (
	blockH1 blockType = iota
	blockH2
	blockH3
	blockPara
	blockUL
	blockOL
	blockTable
	blockQuote
)

type block struct {
	Type   blockType
	Text   string     // 行内原始文本（仍含 **、_、[n](url) 等标记，由各渲染器自行处理）
	Header []string   // 表头单元格（仅 blockTable）
	Rows   [][]string // 表格数据行（仅 blockTable）
}

var (
	orderedRe = regexp.MustCompile(`^\d+\.\s+`)
	linkRe    = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRe    = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe  = regexp.MustCompile(`_([^_]+)_`)
)

// ParseBlocks 把报告 Markdown 拆成块序列。空行只负责分隔，不产生块；
// 连续的非空行各自成块（与既有 HTML 渲染行为一致，不合并段落）。
func ParseBlocks(md string) []block {
	lines := strings.Split(md, "\n")
	blocks := make([]block, 0, len(lines)/2+4)

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}

		// 表格：当前行以 | 开头且下一行是分隔行
		if strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && isTableSeparator(lines[i+1]) {
			b := block{Type: blockTable, Header: splitRow(trimmed)}
			for j := i + 2; j < len(lines); j++ {
				row := strings.TrimSpace(lines[j])
				if !strings.HasPrefix(row, "|") {
					break
				}
				b.Rows = append(b.Rows, splitRow(row))
			}
			blocks = append(blocks, b)
			i += 1 + len(b.Rows) // 跳过分隔行与数据行
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "### "):
			blocks = append(blocks, block{Type: blockH3, Text: strings.TrimPrefix(trimmed, "### ")})
		case strings.HasPrefix(trimmed, "## "):
			blocks = append(blocks, block{Type: blockH2, Text: strings.TrimPrefix(trimmed, "## ")})
		case strings.HasPrefix(trimmed, "# "):
			blocks = append(blocks, block{Type: blockH1, Text: strings.TrimPrefix(trimmed, "# ")})
		case strings.HasPrefix(trimmed, "> "):
			blocks = append(blocks, block{Type: blockQuote, Text: strings.TrimPrefix(trimmed, "> ")})
		case strings.HasPrefix(trimmed, "- "):
			blocks = append(blocks, block{Type: blockUL, Text: strings.TrimPrefix(trimmed, "- ")})
		case orderedRe.MatchString(trimmed):
			blocks = append(blocks, block{Type: blockOL, Text: orderedRe.ReplaceAllString(trimmed, "")})
		default:
			blocks = append(blocks, block{Type: blockPara, Text: trimmed})
		}
	}
	return blocks
}

func isTableSeparator(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "|") {
		return false
	}
	for _, r := range t {
		if r != '|' && r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return strings.Contains(t, "-")
}

func splitRow(line string) []string {
	t := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(t, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// plainText 去掉行内标记，供 PDF 排版使用（PDF 无加粗/斜体字重）。
// 链接保留为「文本（URL）」：PDF 没有可悬停的 href，URL 必须可见，
// 否则来源列表在纸面上不可回溯——这是端到端内容比对抓出来的回归点。
func plainText(s string) string {
	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := linkRe.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		text, target := sub[1], sub[2]
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			return text // 与 HTML 渲染一致：非 http(s) 目标不保留
		}
		return text + "（" + target + "）"
	})
	s = boldRe.ReplaceAllString(s, "$1")
	s = italicRe.ReplaceAllString(s, "$1")
	return s
}

// linkParts 提取行内第一个链接（用于来源列表的可点击 URL）。
func firstLink(s string) (text, url string, rest string) {
	loc := linkRe.FindStringSubmatchIndex(s)
	if loc == nil {
		return "", "", s
	}
	text = s[loc[2]:loc[3]]
	url = s[loc[4]:loc[5]]
	rest = s[:loc[0]] + s[loc[1]:]
	return text, url, rest
}
