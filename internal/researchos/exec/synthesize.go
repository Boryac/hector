package exec

import (
	"fmt"
	"time"

	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/report"
)

// runSynthesize 生成结构化研究报告（含真实引用）。
func (e *Engine) runSynthesize(s *Session) (map[string]any, int, error) {
	s.Emit(domain.EvtReportStarted, map[string]any{"node_id": s.Node.ID})
	s.Progress(20, "汇总结论与证据")

	claims, err := e.Store.ListAllClaims(s.Ctx, s.Task.ID)
	if err != nil {
		return nil, 0, err
	}
	if len(claims) == 0 {
		// 确定性失败：没有结论这件事不会因为重试而改变，直接判定永久失败。
		return nil, 0, Permanent(fmt.Errorf("没有结论可生成报告"))
	}

	srcMap := map[string]*domain.Source{}
	for _, c := range claims {
		for _, ev := range c.Evidence {
			if _, ok := srcMap[ev.SourceID]; ok {
				continue
			}
			src, err := e.Store.GetSource(s.Ctx, s.Task.ID, ev.SourceID)
			if err != nil {
				continue
			}
			srcMap[ev.SourceID] = src
		}
	}

	s.Progress(55, "构建报告结构")
	modelNote := "本地确定性流程（未使用大模型）"
	if e.LLM != nil && e.LLM.Available() {
		modelNote = "LLM 规划 + 规则/LLM 混合抽取（" + e.LLM.Model() + "）"
	}
	rep, err := report.Build(report.Input{
		Task:        s.Task,
		Claims:      claims,
		Sources:     srcMap,
		PlannerMode: s.Task.PlannerMode,
		ModelNote:   modelNote,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, 0, err
	}
	rep.ID = newID()

	saved, err := e.Store.InsertReport(s.Ctx, rep)
	if err != nil {
		return nil, 0, fmt.Errorf("报告落库失败: %w", err)
	}
	s.Progress(95, "报告已生成")
	s.Emit(domain.EvtReportCompleted, map[string]any{
		"report_id": saved.ID, "version": saved.Version,
		"citation_count": saved.CitationCount, "word_count": saved.WordCount,
	})
	return map[string]any{
		"report_id":      saved.ID,
		"version":        saved.Version,
		"citation_count": saved.CitationCount,
		"word_count":     saved.WordCount,
		"claims":         len(claims),
		"sources":        len(srcMap),
	}, 0, nil
}
