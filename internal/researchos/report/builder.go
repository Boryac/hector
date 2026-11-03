// Package report 由结论与证据生成结构化研究报告（Markdown + 章节 + 引用映射）。
//
// 硬性规则（DATA_MODEL.md / PRD）：
//   - 报告中的每个 [n] 引用都必须能映射到真实 source（URL 可点击）；
//   - 没有证据的结论只能出现在「未验证」章节，不得写成已证实结论；
//   - 冲突必须显式披露，不得粉饰。
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/textutil"
)

// Input 是报告生成入参。
type Input struct {
	Task        *domain.Task
	Claims      []*domain.Claim
	Sources     map[string]*domain.Source
	PlannerMode string
	ModelNote   string
	GeneratedAt string
	Narrative   map[string]string // 章节标题 → LLM 生成的正文（可选）
}

// Build 生成报告。
func Build(in Input) (*domain.Report, error) {
	if in.Task == nil {
		return nil, fmt.Errorf("缺少任务")
	}
	claims := in.Claims
	sort.SliceStable(claims, func(i, j int) bool {
		return claims[i].Confidence > claims[j].Confidence
	})

	supported, partial, conflicting, unverified := classify(claims)

	// 引用编号：按证据首次出现顺序分配，保证编号稳定且可回溯。
	citations := make([]domain.Citation, 0, len(claims)*2)
	index := map[string]int{}
	addCitation := func(c *domain.Claim, e domain.Evidence) int {
		key := c.ID + "|" + e.SourceID
		if n, ok := index[key]; ok {
			return n
		}
		src := in.Sources[e.SourceID]
		if src == nil {
			return 0
		}
		n := len(citations) + 1
		index[key] = n
		citations = append(citations, domain.Citation{
			Index: n, ClaimID: c.ID, SourceID: e.SourceID,
			Title: src.Title, URL: src.URL, Site: src.Site, Quote: e.Quote,
		})
		return n
	}
	// 先为「已验证」结论分配编号，让正文引用稳定。
	for _, c := range append(append([]*domain.Claim{}, supported...), partial...) {
		for _, e := range c.Evidence {
			addCitation(c, e)
		}
	}
	for _, c := range conflicting {
		for _, e := range c.Evidence {
			addCitation(c, e)
		}
	}

	var b strings.Builder
	sections := make([]domain.Section, 0, 8)
	title := in.Task.Title
	if title == "" {
		title = truncate(in.Task.Goal, 60)
	}
	b.WriteString("# " + title + "\n\n")

	// 摘要
	summary := buildSummary(in, supported, partial, conflicting, unverified, len(citations), index)
	b.WriteString("## 摘要\n\n" + summary + "\n\n")
	sections = append(sections, domain.Section{Heading: "摘要", Body: summary})

	// 目标与范围
	scopeBody := "**研究目标**：" + in.Task.Goal + "\n\n"
	if len(in.Task.Scope) > 0 {
		parts := make([]string, 0, len(in.Task.Scope))
		for k, v := range in.Task.Scope {
			parts = append(parts, fmt.Sprintf("- %s：%v", k, v))
		}
		sort.Strings(parts)
		scopeBody += "**范围约束**：\n" + strings.Join(parts, "\n") + "\n\n"
	}
	scopeBody += fmt.Sprintf("**研究深度**：%s　**最大并发 Agent**：%d　**交叉验证**：%v\n",
		in.Task.Depth, in.Task.MaxAgents, in.Task.EnableVerification)
	b.WriteString("## 目标与范围\n\n" + scopeBody + "\n")
	sections = append(sections, domain.Section{Heading: "目标与范围", Body: scopeBody})

	// 核心发现
	keyFindings := buildFindings(supported, partial, index)
	if in.Narrative["key_findings"] != "" {
		keyFindings = sanitizeMarkers(in.Narrative["key_findings"], len(citations))
	}
	b.WriteString("## 核心发现\n\n" + keyFindings + "\n\n")
	sections = append(sections, domain.Section{Heading: "核心发现", Body: keyFindings})

	// 分主题分析
	topicBody := buildTopicAnalysis(claims, in.Sources, index)
	if topicBody == "" {
		topicBody = "_本次执行未产出可归类的结论。_"
	}
	b.WriteString("## 分主题分析\n\n" + topicBody + "\n\n")
	sections = append(sections, domain.Section{Heading: "分主题分析", Body: topicBody})

	// 对比表
	table := buildComparisonTable(claims)
	if table != "" {
		b.WriteString("## 关键数据对比\n\n" + table + "\n\n")
		sections = append(sections, domain.Section{Heading: "关键数据对比", Body: table})
	}

	// 争议与不确定性
	disputeBody := buildDisputes(conflicting, unverified, in.Sources)
	b.WriteString("## 争议与不确定性\n\n" + disputeBody + "\n\n")
	sections = append(sections, domain.Section{Heading: "争议与不确定性", Body: disputeBody})

	// 来源
	sourceBody := buildSourceList(citations)
	b.WriteString("## 信息来源\n\n" + sourceBody + "\n\n")
	sections = append(sections, domain.Section{Heading: "信息来源", Body: sourceBody})

	// 方法与局限
	methodBody := buildMethod(in, len(in.Sources), len(claims), len(citations))
	b.WriteString("## 方法与局限性\n\n" + methodBody + "\n")
	sections = append(sections, domain.Section{Heading: "方法与局限性", Body: methodBody})

	md := b.String()
	rep := &domain.Report{
		TaskID:        in.Task.ID,
		Title:         title,
		Summary:       summary,
		Markdown:      md,
		Sections:      sections,
		Citations:     citations,
		CitationCount: len(citations),
		WordCount:     textutil.WordCount(md),
		Model:         in.ModelNote,
	}
	return rep, nil
}

func classify(claims []*domain.Claim) (supported, partial, conflicting, unverified []*domain.Claim) {
	for _, c := range claims {
		switch c.Status {
		case domain.ClaimSupported:
			supported = append(supported, c)
		case domain.ClaimPartial:
			partial = append(partial, c)
		case domain.ClaimConflicting:
			conflicting = append(conflicting, c)
		default:
			unverified = append(unverified, c)
		}
	}
	return
}

func buildSummary(in Input, sup, par, con, unv []*domain.Claim, citations int, index map[string]int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "本次研究围绕「%s」执行，共采集资料 %d 条，产出结论 %d 条（已验证 %d 条、部分支持 %d 条、存在冲突 %d 条、未验证 %d 条），"+
		"生成可追溯引用 %d 条。\n\n",
		truncate(in.Task.Goal, 120), len(in.Sources), len(sup)+len(par)+len(con)+len(unv),
		len(sup), len(par), len(con), len(unv), citations)
	if len(sup) > 0 {
		fmt.Fprintf(&b, "最重要的已验证结论：%s", truncate(sup[0].Statement, 160))
		if n := claimCitation(sup[0], index); n > 0 {
			fmt.Fprintf(&b, "[%d]", n)
		}
		b.WriteString("。")
	} else {
		b.WriteString("**注意**：本次执行未产出多来源交叉验证通过的结论，结论强度有限。")
	}
	if len(con) > 0 {
		fmt.Fprintf(&b, "另有 %d 条结论存在来源冲突，已在「争议与不确定性」章节列出。", len(con))
	}
	return b.String()
}

func claimCitation(c *domain.Claim, index map[string]int) int {
	for _, e := range c.Evidence {
		if n, ok := index[c.ID+"|"+e.SourceID]; ok {
			return n
		}
	}
	return 0
}

func buildFindings(sup, par []*domain.Claim, index map[string]int) string {
	if len(sup)+len(par) == 0 {
		return "_本次执行未产出可引用的结论。_"
	}
	var b strings.Builder
	i := 0
	emit := func(c *domain.Claim, tag string) {
		i++
		fmt.Fprintf(&b, "%d. %s（%s，置信度 %.2f）", i, c.Statement, tag, c.Confidence)
		for _, e := range c.Evidence {
			if n, ok := index[c.ID+"|"+e.SourceID]; ok {
				fmt.Fprintf(&b, "[%d]", n)
			}
		}
		b.WriteString("\n")
	}
	for _, c := range sup {
		if i >= 8 {
			break
		}
		emit(c, "多来源支持")
	}
	for _, c := range par {
		if i >= 10 {
			break
		}
		emit(c, "单一来源")
	}
	return b.String()
}

func buildTopicAnalysis(claims []*domain.Claim, sources map[string]*domain.Source, index map[string]int) string {
	byTopic := map[string][]*domain.Claim{}
	order := []string{}
	for _, c := range claims {
		if c.Status == domain.ClaimUnverified {
			continue
		}
		key := c.NodeID
		if key == "" {
			key = "other"
		}
		if _, ok := byTopic[key]; !ok {
			order = append(order, key)
		}
		byTopic[key] = append(byTopic[key], c)
	}
	sort.Strings(order)
	var b strings.Builder
	for _, key := range order {
		fmt.Fprintf(&b, "### 主题 %s\n\n", key)
		for _, c := range byTopic[key] {
			b.WriteString("- " + c.Statement)
			for _, e := range c.Evidence {
				if n, ok := index[c.ID+"|"+e.SourceID]; ok {
					fmt.Fprintf(&b, "[%d]", n)
				}
			}
			if c.VerifierNotes != "" {
				b.WriteString("（" + c.VerifierNotes + "）")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func buildComparisonTable(claims []*domain.Claim) string {
	type row struct {
		statement string
		value     float64
		source    string
	}
	var rows []row
	for _, c := range claims {
		if c.NumericValue == nil {
			continue
		}
		src := ""
		if len(c.Evidence) > 0 {
			src = c.Evidence[0].SourceID
		}
		rows = append(rows, row{statement: c.Statement, value: *c.NumericValue, source: src})
	}
	if len(rows) < 2 {
		return ""
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].value > rows[j].value })
	if len(rows) > 12 {
		rows = rows[:12]
	}
	var b strings.Builder
	b.WriteString("| 结论 | 数值 | 状态 |\n|---|---:|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %.4g | 见来源 %s |\n", truncate(r.statement, 80), r.value, r.source)
	}
	return b.String()
}

func buildDisputes(conflicting, unverified []*domain.Claim, sources map[string]*domain.Source) string {
	var b strings.Builder
	if len(conflicting) == 0 {
		b.WriteString("本次执行未检测到同一指标上的来源冲突。\n\n")
	} else {
		b.WriteString("以下结论在不同来源之间存在冲突，已标注为「存在冲突」，请勿直接采信：\n\n")
		for _, c := range conflicting {
			fmt.Fprintf(&b, "- **%s**：%s\n", truncate(c.Statement, 120), nonEmpty(c.VerifierNotes, "来源数值不一致"))
			for _, e := range c.Evidence {
				src := sources[e.SourceID]
				if src == nil {
					continue
				}
				fmt.Fprintf(&b, "  - [%s](%s) — %s\n", src.Site, src.URL, truncate(e.Quote, 160))
			}
		}
		b.WriteString("\n")
	}
	if len(unverified) > 0 {
		fmt.Fprintf(&b, "另有 %d 条结论因缺少可引用证据，仅作为线索保留，未计入已验证结论：\n\n", len(unverified))
		for i, c := range unverified {
			if i >= 12 {
				fmt.Fprintf(&b, "- 其余 %d 条见证据面板。\n", len(unverified)-12)
				break
			}
			fmt.Fprintf(&b, "- %s\n", truncate(c.Statement, 160))
		}
	}
	return b.String()
}

func buildSourceList(citations []domain.Citation) string {
	if len(citations) == 0 {
		return "_无可引用来源。_"
	}
	var b strings.Builder
	for _, c := range citations {
		fmt.Fprintf(&b, "%d. [%s](%s) — %s\n", c.Index, nonEmpty(c.Title, c.URL), c.URL, c.Site)
	}
	return b.String()
}

func buildMethod(in Input, sourceCount, claimCount, citationCount int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- 规划模式：%s；执行模型：%s。\n", in.PlannerMode, nonEmpty(in.ModelNote, "未使用大模型（本地确定性流程）")))
	b.WriteString("- 检索与抓取均走真实外部服务；抓取遵守 robots.txt，正文经过去标签与指纹去重。\n")
	b.WriteString("- 结论由来源正文片段抽取，交叉验证按「独立站点数 + 数值一致性」判定，未使用模型主观判断替换证据。\n")
	b.WriteString(fmt.Sprintf("- 本次统计：资料 %d 条、结论 %d 条、引用 %d 条。\n", sourceCount, claimCount, citationCount))
	b.WriteString("- 局限：检索覆盖受 Provider 配额与反爬限制；未做全文语义相似度聚类，冲突检测聚焦显式数值指标。\n")
	return b.String()
}

// sanitizeMarkers 移除模型输出中不存在的引用编号，防止伪造引用。
func sanitizeMarkers(text string, max int) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '[' {
			j := strings.IndexByte(text[i:], ']')
			if j > 0 {
				numStr := text[i+1 : i+j]
				n := 0
				ok := numStr != ""
				for _, r := range numStr {
					if r < '0' || r > '9' {
						ok = false
						break
					}
					n = n*10 + int(r-'0')
				}
				if ok && n >= 1 && n <= max {
					b.WriteString(text[i : i+j+1])
					i += j
					continue
				}
				if ok {
					// 越界引用直接丢弃。
					i += j
					continue
				}
			}
		}
		b.WriteByte(text[i])
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
