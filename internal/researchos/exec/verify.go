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
		return nil, 0, fmt.Errorf("没有可验证的结论")
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
		values := make([]float64, 0, len(cluster))
		for _, c := range cluster {
			for _, ev := range c.Evidence {
				if src := loadSource(ev.SourceID); src != nil {
					sites[src.Site] = true
				}
			}
			if c.NumericValue != nil {
				values = append(values, *c.NumericValue)
			}
		}
		conflict, note, group := detectConflict(values)
		siteCount := len(sites)

		for _, c := range cluster {
			status := domain.ClaimUnverified
			conf := 0.3
			switch {
			case conflict:
				status = domain.ClaimConflicting
				conf = 0.4
			case siteCount >= 2:
				status = domain.ClaimSupported
				conf = math.Min(0.95, 0.6+0.1*float64(siteCount-1))
			case siteCount == 1 && len(c.Evidence) > 0:
				status = domain.ClaimPartial
				conf = math.Min(0.55, 0.5)
			}
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

// detectConflict 在同一簇内检测数值冲突。
func detectConflict(values []float64) (conflict bool, note, group string) {
	if len(values) < 2 {
		return false, "", ""
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	minV, maxV := sorted[0], sorted[len(sorted)-1]
	if minV == 0 {
		minV = 1e-9
	}
	ratio := (maxV - minV) / math.Abs(minV)
	if ratio > 0.25 {
		return true,
			fmt.Sprintf("同一结论在不同来源中的数值差异显著（最小 %.4g / 最大 %.4g，相对差异 %.0f%%）",
				sorted[0], sorted[len(sorted)-1], ratio*100),
			"conflict:" + textutil.Hash(fmt.Sprintf("%.4g-%.4g", sorted[0], sorted[len(sorted)-1]))[:10]
	}
	return false, fmt.Sprintf("多来源数值一致（相对差异 %.0f%%）", ratio*100), ""
}
