package exec

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/textutil"
)

// runVerify 执行交叉验证：近似结论聚类 + 独立站点计数 + 数值冲突检测。
//
// 判定规则（可解释、可复现，不依赖模型主观判断）：
//   - 独立站点数 ≥ 2              → supported（置信度 0.6 + 0.1×(站点数-1)，上限 0.95）
//   - 独立站点数 = 1              → partially_supported（置信度上限 0.55）
//   - 无可用证据                  → unverified
//   - 同簇内数值相对差异 > 25%    → conflicting（置信度 0.4，记录冲突双方）
func (e *Engine) runVerify(s *Session) (map[string]any, int, error) {
	s.Progress(15, "加载结论")
	claims, err := e.Store.ListAllClaims(s.Ctx, s.Task.ID)
	if err != nil {
		return nil, 0, err
	}
	if len(claims) == 0 {
		// 没有任何结论时核验无事可做；由 synthesize 判断是否整体失败。
		return map[string]any{"verified": 0, "clusters": 0, "reason": "没有可验证的结论"}, 0, nil
	}

	// 证据 → 站点
	srcCache := map[string]*domain.Source{}
	loadSource := func(id string) *domain.Source {
		if src, ok := srcCache[id]; ok {
			return src
		}
		src, err := e.Store.GetSource(s.Ctx, s.Task.ID, id)
		if err != nil {
			return nil
		}
		srcCache[id] = src
		return src
	}

	s.Progress(35, "近义结论聚类")
	clusters := clusterClaims(claims)

	stats := map[string]int{}
	updated := 0
	emitted := 0
	for _, cluster := range clusters {
		// 汇总簇内独立站点与数值
		sites := map[string]bool{}
		srcIDs := map[string]bool{}
		values := make([]float64, 0, len(cluster))
		for _, c := range cluster {
			for _, ev := range c.Evidence {
				if src := loadSource(ev.SourceID); src != nil {
					sites[src.Site] = true
					srcIDs[ev.SourceID] = true
				}
			}
			if c.NumericValue != nil {
				values = append(values, *c.NumericValue)
			}
		}
		conflict, note, group := detectConflict(values, len(srcIDs))
		siteCount := len(sites)

		for _, c := range cluster {
			status, conf := verificationStatus(conflict, siteCount, len(c.Evidence))
			c.Status = status
			c.Confidence = conf
			c.SourceCount = siteCount
			c.ConflictGroup = group
			c.VerifierNotes = note
			if err := e.Store.UpdateClaimVerification(s.Ctx, c); err != nil {
				continue
			}
			updated++
			stats[status]++
			if emitted < 10 {
				emitted++
				s.Emit(domain.EvtEvidenceVerified, map[string]any{
					"claim_id": c.ID, "status": status, "confidence": conf,
					"sites": siteCount, "conflict_group": group,
				})
			}
		}
	}

	s.Progress(85, fmt.Sprintf("已验证 %d 条结论", updated))
	return map[string]any{
		"verified":    updated,
		"clusters":    len(clusters),
		"supported":   stats[domain.ClaimSupported],
		"partial":     stats[domain.ClaimPartial],
		"conflicting": stats[domain.ClaimConflicting],
		"unverified":  stats[domain.ClaimUnverified],
	}, 0, nil
}

// clusterClaims 用 token Jaccard 相似度把近似结论聚成一簇（并查集式贪心）。
//
// 判定条件为双重门槛：Jaccard ≥ 0.45 且共享内容 token ≥ 4。
// 只看 Jaccard 会把「同主题但结论不同」的短句误并（短句 token 少，分母小），
// 加上共享 token 数下限后，改写/翻译造成的重复仍能合并，语义不同的不会被并。
func clusterClaims(claims []*domain.Claim) [][]*domain.Claim {
	tokens := make([]map[string]bool, len(claims))
	for i, c := range claims {
		tokens[i] = tokenSet(c.Statement)
	}
	assigned := make([]bool, len(claims))
	clusters := make([][]*domain.Claim, 0, len(claims))
	for i := range claims {
		if assigned[i] {
			continue
		}
		cluster := []*domain.Claim{claims[i]}
		assigned[i] = true
		for j := i + 1; j < len(claims); j++ {
			if assigned[j] {
				continue
			}
			if similar(tokens[i], tokens[j]) {
				cluster = append(cluster, claims[j])
				assigned[j] = true
			}
		}
		clusters = append(clusters, cluster)
	}
	return clusters
}

// similarityThreshold / minSharedTokens 是聚类的双门槛。
const (
	similarityThreshold = 0.45
	minSharedTokens     = 4
)

// similar 判断两组 token 是否指向同一结论。
func similar(a, b map[string]bool) bool {
	shared := 0
	for k := range a {
		if b[k] {
			shared++
		}
	}
	if shared < minSharedTokens {
		return false
	}
	return jaccard(a, b) >= similarityThreshold
}

func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	lower := strings.ToLower(s)
	for _, w := range strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && !(r >= 0x4e00 && r <= 0x9fff)
	}) {
		if len([]rune(w)) >= 2 {
			out[w] = true
		}
	}
	// 中文按 bigram 切分，避免整句当成一个 token。
	runes := []rune(lower)
	for i := 0; i+1 < len(runes); i++ {
		if runes[i] >= 0x4e00 && runes[i] <= 0x9fff && runes[i+1] >= 0x4e00 && runes[i+1] <= 0x9fff {
			out[string(runes[i:i+2])] = true
		}
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// relativeSpread 计算相对差异，返回 0~1 的比例（调用方乘 100 转百分比）。
//
// 以最小值为分母是原本的阈值语义（25% 判定冲突），但最小值可能是 0：
//   - 全部为 0 时旧写法把分母替换成 1e-9，算出 -100%，符号都是错的；
//   - 最小值 0 与 23.6 相比时算出 2.36e12%，这种数字没法读。
//
// 因此：最小值非零仍以它作分母；最小值近似 0 时改用最大值作分母（此时最大差异即 100%）。
func relativeSpread(minV, maxV float64) float64 {
	if minV == maxV {
		return 0
	}
	denom := math.Abs(minV)
	if denom < 1e-9 {
		denom = math.Abs(maxV)
	}
	if denom < 1e-9 {
		return 0
	}
	return (maxV - minV) / denom
}

// detectConflict 在同一簇内检测数值冲突。
//
// distinctPages 是簇内证据涉及的独立来源页面数，用来把提示写准：
//   - 站点数（SourceCount）与页面数不是一回事，github.com 下一个域名可以挂多个仓库页面；
//   - 只有 1 个页面时，比较的只是同一页面内的多处数值，不能说成「不同来源冲突」。
func detectConflict(values []float64, distinctPages int) (conflict bool, note, group string) {
	if len(values) < 2 {
		return false, "", ""
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	minV, maxV := sorted[0], sorted[len(sorted)-1]
	ratio := relativeSpread(minV, maxV)
	if ratio > 0.25 {
		scope := "不同来源页面上"
		if distinctPages < 2 {
			scope = "同一来源页面内"
		}
		return true,
			fmt.Sprintf("%s的数值差异显著（最小 %.4g / 最大 %.4g，相对差异 %.0f%%，涉及 %d 个来源页面）",
				scope, sorted[0], sorted[len(sorted)-1], ratio*100, distinctPages),
			"conflict:" + textutil.Hash(fmt.Sprintf("%.4g-%.4g", sorted[0], sorted[len(sorted)-1]))[:10]
	}
	if distinctPages < 2 {
		// 只有 1 个页面时没有可比对的独立来源，不能对外说成「数值一致、已印证」。
		return false,
			fmt.Sprintf("仅 1 个来源页面，数值无跨来源可比（相对差异 %.0f%%）", ratio*100), ""
	}
	return false, fmt.Sprintf("%d 个来源页面数值一致（相对差异 %.0f%%）", distinctPages, ratio*100), ""
}

// verificationStatus 把「冲突 / 独立站点数 / 证据条数」映射为结论状态与置信度。
//
// 抽成纯函数是为了能逐分支验证：冲突识别、双来源支持、单来源部分支持、无证据未验证
// 四条路径都必须可达，不能只靠真实网页数据碰运气。
//   - 同一指标出现显著分歧（数值相对差 > 25%） → conflicting（置信度 0.4）
//   - 独立站点 ≥ 2                            → supported（0.6 起，每多一站 +0.1，上限 0.95）
//   - 单一站点且有证据                        → partially_supported（0.5，上限 0.55）
//   - 其余（无证据）                          → unverified（0.3）
func verificationStatus(conflict bool, siteCount, evidenceCount int) (string, float64) {
	switch {
	case conflict:
		return domain.ClaimConflicting, 0.4
	case siteCount >= 2:
		return domain.ClaimSupported, math.Min(0.95, 0.6+0.1*float64(siteCount-1))
	case siteCount == 1 && evidenceCount > 0:
		return domain.ClaimPartial, math.Min(0.55, 0.5)
	default:
		return domain.ClaimUnverified, 0.3
	}
}
