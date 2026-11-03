package exec

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/textutil"
)

func newID() string { return uuid.NewString() }

const analyzeSystemPrompt = `你是 Hector ResearchOS 的研究分析师（Analyst）。
输入是一组已经抓取到的网页资料（含 source_id、URL、正文片段）。请严格基于资料内容抽取结论，只输出 JSON。

输出 Schema：
{"claims":[{"statement":"一句话结论","quote":"支撑该结论的原文片段","source_id":"资料 id","confidence":0.0,"metric":"指标名或空字符串","value":数字或null}]}

硬性要求：
1. quote 必须逐字来自给定资料正文，不得改写、不得跨资料拼接；找不到逐字依据就不要输出该结论。
2. source_id 必须是输入资料中真实存在的 id。
3. statement 必须是可验证的事实陈述，不要写建议、评价或推测语气。
4. 最多输出 8 条结论，优先输出含具体数字、版本、时间、规模的事实。`

// runAnalyze 从上游资料的正文中抽取结论与证据。
func (e *Engine) runAnalyze(s *Session) (map[string]any, int, error) {
	s.Progress(10, "汇聚资料")
	sources, err := e.collectSources(s)
	if err != nil {
		return nil, 0, err
	}
	if len(sources) == 0 {
		return nil, 0, fmt.Errorf("分析节点无可用资料（上游 search 未产出正文）")
	}
	question := stringInput(s.Node.Input, "question")
	if question == "" {
		question = s.Node.Title
	}

	var (
		claims []analyzedClaim
		tokens int
	)
	if e.LLM != nil && e.LLM.Available() {
		s.Progress(35, "调用大模型抽取结论")
		claims, tokens, err = e.llmAnalyze(s, sources, question)
		if err != nil || len(claims) == 0 {
			if err != nil {
				s.Emit(domain.EvtAgentFailed, map[string]any{
					"role": domain.RoleAnalyst, "error": truncate(err.Error(), 300),
					"fallback": "rule_based_extraction",
				})
			}
			claims = ruleBasedClaims(sources, question, e.Cfg.MaxClaimsPerNode)
		}
	} else {
		s.Progress(35, "规则抽取结论（未配置 LLM）")
		claims = ruleBasedClaims(sources, question, e.Cfg.MaxClaimsPerNode)
	}
	if len(claims) == 0 {
		return nil, tokens, fmt.Errorf("未从资料中抽取出可引用结论")
	}
	sort.SliceStable(claims, func(i, j int) bool { return claims[i].Confidence > claims[j].Confidence })
	if len(claims) > e.Cfg.MaxClaimsPerNode {
		claims = claims[:e.Cfg.MaxClaimsPerNode]
	}

	s.Progress(70, "写入证据链")
	stored := 0
	emitted := 0
	for _, c := range claims {
		claim := &domain.Claim{
			ID:            newID(),
			TaskID:        s.Task.ID,
			NodeID:        s.Node.ID,
			Statement:     c.Statement,
			NormalizedKey: topicKey(s.Node.ID, c.Statement),
			Status:        domain.ClaimUnverified,
			Confidence:    c.Confidence,
			SourceCount:   1,
			NumericValue:  c.Value,
		}
		saved, err := e.Store.InsertClaim(s.Ctx, claim)
		if err != nil {
			continue
		}
		ev, err := e.Store.InsertEvidence(s.Ctx, &domain.Evidence{
			ID: newID(), TaskID: s.Task.ID, ClaimID: saved.ID, SourceID: c.SourceID,
			NodeID: s.Node.ID, Quote: c.Quote, Stance: "support", Confidence: c.Confidence,
		})
		if err != nil {
			continue
		}
		// 同一句原文在别的来源中也出现时，一并挂证据边（多源印证）。
		added := 1
		for _, sid := range c.RelatedSources {
			if sid == c.SourceID {
				continue
			}
			if _, err := e.Store.InsertEvidence(s.Ctx, &domain.Evidence{
				ID: newID(), TaskID: s.Task.ID, ClaimID: saved.ID, SourceID: sid,
				NodeID: s.Node.ID, Quote: c.Quote, Stance: "support", Confidence: c.Confidence,
			}); err == nil {
				added++
			}
		}
		if added > 1 {
			_ = e.Store.SetClaimSourceCount(s.Ctx, saved.ID, added)
		}
		stored++
		if emitted < 8 {
			emitted++
			s.Emit(domain.EvtEvidenceCreated, map[string]any{
				"claim_id": saved.ID, "source_id": ev.SourceID,
				"statement": truncate(saved.Statement, 160), "confidence": saved.Confidence,
			})
		}
	}
	if stored == 0 {
		return nil, tokens, fmt.Errorf("结论写入失败")
	}
	s.Progress(92, fmt.Sprintf("产出 %d 条结论", stored))
	return map[string]any{
		"claims":   stored,
		"question": question,
		"llm_used": e.LLM != nil && e.LLM.Available(),
		"model":    modelName(e.LLM),
		"tokens":   tokens,
		"source_n": len(sources),
	}, tokens, nil
}

type analyzedClaim struct {
	Statement  string
	Quote      string
	SourceID   string
	Confidence float64
	Value      *float64
	// RelatedSources 记录同一句原文在其他来源中的出现，用于多源交叉验证。
	// 没有它就永远只有 1 个来源，结论最多只能是 partially_supported。
	RelatedSources []string
}

// collectSources 汇总上游节点的资料（去重，按可信度与正文长度排序）。
// excerptUsable 判断资料的正文是否值得拿去抽取结论。
//
// 只看 snippet 的搜索结果、以及只有导航块的页面（GitHub 仓库页、维基语言选择页）
// 会产出「Repository navigation Security and quality」这类垃圾结论，必须在入口挡住。
func excerptUsable(src *domain.Source) bool {
	text := strings.TrimSpace(src.Excerpt)
	if len([]rune(text)) < 150 {
		return false
	}
	lines := 0
	for _, l := range strings.Split(text, "\n") {
		if len([]rune(strings.TrimSpace(l))) >= 40 {
			lines++
		}
	}
	return lines >= 3
}

func (e *Engine) collectSources(s *Session) ([]*domain.Source, error) {
	seen := map[string]bool{}
	var out []*domain.Source
	for _, dep := range s.Node.DependsOn {
		list, err := e.Store.ListSourcesByNode(s.Ctx, s.Task.ID, dep)
		if err != nil {
			return nil, err
		}
		for _, src := range list {
			if seen[src.ID] {
				continue
			}
			switch {
			case src.Status == "ok" && excerptUsable(src):
				seen[src.ID] = true
				out = append(out, src)
			case (src.Status == "discovered" || src.Status == "fetch_failed") && len([]rune(src.Snippet)) >= 120:
				// 抓取失败但检索服务返回了足够长的真实摘要：降级使用摘要，可信度折半。
				cp := *src
				cp.Excerpt = src.Snippet
				cp.Credibility = downgrade(src.Credibility)
				if !excerptUsable(&cp) {
					continue
				}
				seen[src.ID] = true
				out = append(out, &cp)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		wi := credibilityWeight(out[i].Credibility)
		wj := credibilityWeight(out[j].Credibility)
		if wi != wj {
			return wi > wj
		}
		return len(out[i].Excerpt) > len(out[j].Excerpt)
	})
	if len(out) > e.Cfg.MaxSourcesPerAnalyze {
		out = out[:e.Cfg.MaxSourcesFor()]
	}
	return out, nil
}

// MaxSourcesFor 返回分析阶段最大资料数。
func (c Config) MaxSourcesFor() int { return c.MaxSourcesPerAnalyze }

// downgrade 把可信度降一档（用于仅有检索摘要、无正文的资料）。
func downgrade(c string) string {
	switch c {
	case "high":
		return "medium"
	case "medium":
		return "low"
	default:
		return "unknown"
	}
}

func credibilityWeight(c string) int {
	switch c {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func (e *Engine) llmAnalyze(s *Session, sources []*domain.Source, question string) ([]analyzedClaim, int, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "研究目标：%s\n当前研究问题：%s\n\n资料列表：\n", s.Task.Goal, question)
	valid := map[string]*domain.Source{}
	for _, src := range sources {
		valid[src.ID] = src
		excerpt := src.Excerpt
		if r := []rune(excerpt); len(r) > e.Cfg.ExcerptChars {
			excerpt = string(r[:e.Cfg.ExcerptChars])
		}
		fmt.Fprintf(&b, "\n[source_id=%s]\n标题：%s\nURL：%s\n正文：%s\n", src.ID, src.Title, src.URL, excerpt)
	}
	b.WriteString("\n请输出结论 JSON。")

	raw, usage, err := e.LLM.CompleteJSON(s.Ctx, analyzeSystemPrompt, b.String())
	if err != nil {
		return nil, usage.TotalTokens, err
	}
	var parsed struct {
		Claims []struct {
			Statement  string   `json:"statement"`
			Quote      string   `json:"quote"`
			SourceID   string   `json:"source_id"`
			Confidence float64  `json:"confidence"`
			Metric     string   `json:"metric"`
			Value      *float64 `json:"value"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, usage.TotalTokens, fmt.Errorf("分析输出解析失败: %w", err)
	}

	out := make([]analyzedClaim, 0, len(parsed.Claims))
	dropped := 0
	for _, c := range parsed.Claims {
		src, ok := valid[c.SourceID]
		if !ok {
			dropped++
			continue
		}
		quote := textutil.CleanText(c.Quote)
		if len([]rune(quote)) < 10 || !quoteGrounded(quote, src.Excerpt) {
			// 拒绝无法在原文中逐字定位的引文，避免模型编造依据。
			dropped++
			continue
		}
		conf := c.Confidence
		if conf <= 0 || conf > 1 {
			conf = 0.6
		}
		value := c.Value
		if value == nil {
			value = extractNumber(c.Statement)
		}
		out = append(out, analyzedClaim{
			Statement: textutil.CleanText(c.Statement), Quote: quote,
			SourceID: src.ID, Confidence: conf, Value: value,
		})
	}
	if dropped > 0 {
		s.Emit(domain.EvtToolCompleted, map[string]any{
			"tool": "llm_claim_extraction", "kept": len(out), "dropped_ungrounded": dropped,
		})
	}
	return out, usage.TotalTokens, nil
}

// quoteGrounded 检查引文是否能在原文中逐字定位（容忍空白差异）。
// quoteGrounded 判断引文是否能在原文中逐字找到。
//
// 空引文、过短引文（<6 字符）一律判为无据：既防止 strings.Contains 的空串恒真，
// 也避免「是」「and」这类碎片被当成证据。
func quoteGrounded(quote, excerpt string) bool {
	norm := func(s string) string {
		return strings.Join(strings.Fields(strings.ToLower(s)), " ")
	}
	q := norm(quote)
	if len([]rune(q)) < 6 {
		return false
	}
	return strings.Contains(norm(excerpt), q)
}

// ruleBasedClaims 是不依赖大模型的抽取：按关键词命中句抽取，证据即原句。
//
// 两条质量约束：
//  1. usableStatement 过滤导航残渣与维护标记，避免把「(December 2025)」这类框架文本
//     当成结论；
//  2. 同一句原文若出现在多个来源正文中，全部挂到同一条结论下（RelatedSources），
//     这样交叉验证才有真实的独立来源可数。
func ruleBasedClaims(sources []*domain.Source, question string, limit int) []analyzedClaim {
	terms := textutil.Terms(question)
	if len(terms) == 0 {
		terms = textutil.Terms(question + " " + strings.Join(terms, " "))
	}
	if len(terms) == 0 {
		return nil
	}

	// 句级索引：规范化句子 → 出现该句的来源列表（用于多源印证）。
	index := map[string][]string{}
	for _, src := range sources {
		for _, line := range strings.Split(src.Excerpt, "\n") {
			for _, sentence := range splitSentences(line) {
				key := normalizeForIndex(sentence)
				if key == "" {
					continue
				}
				index[key] = appendUnique(index[key], src.ID)
			}
		}
	}

	out := make([]analyzedClaim, 0, limit)
	seen := map[string]bool{}
	for _, src := range sources {
		perSource := 0
		for _, term := range terms {
			sentence := textutil.Sentence(src.Excerpt, []string{term}, 280)
			if sentence == "" || !usableStatement(sentence) {
				continue
			}
			key := normalizeForIndex(sentence)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			conf := 0.45
			switch src.Credibility {
			case "high":
				conf = 0.7
			case "medium":
				conf = 0.6
			case "low":
				conf = 0.45
			}
			out = append(out, analyzedClaim{
				Statement:      sentence,
				Quote:          sentence,
				SourceID:       src.ID,
				Confidence:     conf,
				Value:          extractNumber(sentence),
				RelatedSources: index[key],
			})
			perSource++
			if perSource >= 3 || len(out) >= limit {
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

// usableStatement 判断一句话能否作为结论。
func usableStatement(s string) bool {
	t := strings.TrimSpace(s)
	if len([]rune(t)) < 15 {
		return false
	}
	// 括号开头的多为维护标记或补充说明。
	if strings.HasPrefix(t, "(") || strings.HasPrefix(t, "[") {
		return false
	}
	// 引用编号噪声。
	if strings.Count(t, "[") >= 2 {
		return false
	}
	letters := 0
	for _, r := range t {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= 0x4e00 && r <= 0x9fff {
			letters++
		}
	}
	if float64(letters)/float64(len([]rune(t))) < 0.55 {
		return false
	}
	lower := strings.ToLower(t)
	for _, kw := range []string{"learn how and when to", "switch to legacy parser", "cookie", "subscribe", "sign in",
		"repository navigation", "security and quality", "skip to content"} {
		if strings.Contains(lower, kw) {
			return false
		}
	}
	if informativeTokens(t) < 6 {
		return false
	}
	// 标签串：既没有句末标点也没有逗号，却堆了 8 个以上 token（如仓库 topics 列表）。
	if !strings.ContainsAny(t, "。，,.;；!！?？:：") && len(strings.Fields(t)) >= 8 {
		return false
	}
	return true
}

// informativeTokens 统计「像内容」的 token 数：长度 ≥4 的英文词（排除 UI 词）
// 加上中文 bigram 数量。用于把标签堆与导航残渣挡在结论之外。
func informativeTokens(s string) int {
	uiWords := map[string]bool{
		"with": true, "from": true, "this": true, "that": true, "have": true, "will": true,
		"your": true, "more": true, "than": true, "into": true, "when": true, "they": true,
		"their": true, "there": true, "which": true, "about": true, "also": true, "been": true,
		"repository": true, "navigation": true, "quality": true, "security": true,
	}
	n := 0
	lower := strings.ToLower(s)
	for _, w := range strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(w) >= 4 && !uiWords[w] {
			n++
		}
	}
	runes := []rune(lower)
	for i := 0; i+1 < len(runes); i++ {
		if runes[i] >= 0x4e00 && runes[i] <= 0x9fff && runes[i+1] >= 0x4e00 && runes[i+1] <= 0x9fff {
			n++
		}
	}
	return n
}

// normalizeForIndex 生成用于跨源比对的句子键（去空白、去大小写、去尾部标点）。
func normalizeForIndex(s string) string {
	t := strings.ToLower(textutil.CleanText(s))
	t = strings.TrimRight(t, "。．.!！?？;；:：")
	if len([]rune(t)) < 15 {
		return ""
	}
	return t
}

// splitSentences 按中英文句末标点切句。
func splitSentences(line string) []string {
	parts := strings.FieldsFunc(line, func(r rune) bool {
		switch r {
		case '。', '！', '？', '；', '.', '!', '?', ';':
			return true
		}
		return false
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

var numberRe = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*(万|亿|k|K|m|M|%)?`)

// extractNumber 抽取陈述中的首个数量指标，用于冲突比对与对比表。
func extractNumber(s string) *float64 {
	m := numberRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return nil
	}
	raw := strings.ReplaceAll(m[1], ",", "")
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	switch m[2] {
	case "万":
		v *= 10_000
	case "亿":
		v *= 100_000_000
	case "k", "K":
		v *= 1_000
	case "m", "M":
		v *= 1_000_000
	}
	return &v
}

// topicKey 生成结论归组键：节点（主题）+ 归一化陈述前缀。
func topicKey(nodeID, statement string) string {
	norm := strings.ToLower(textutil.CleanText(statement))
	norm = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r >= 0x4e00 && r <= 0x9fff {
			return r
		}
		return ' '
	}, norm)
	fields := strings.Fields(norm)
	if len(fields) > 6 {
		fields = fields[:6]
	}
	return nodeID + ":" + textutil.Hash(strings.Join(fields, " "))[:12]
}

func modelName(c interface{ Model() string }) string {
	if c == nil {
		return ""
	}
	if m, ok := c.(interface{ Model() string }); ok {
		return m.Model()
	}
	return ""
}
