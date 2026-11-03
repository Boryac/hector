package report

import (
	"strings"
	"testing"

	"github.com/verikod/hector/internal/researchos/domain"
)

func fixture() Input {
	task := &domain.Task{ID: "task-1", Title: "AI Agent 开源生态", Goal: "梳理 AI Agent 开源生态现状与主要项目", Depth: "standard"}
	sources := map[string]*domain.Source{
		"s1": {ID: "s1", Title: "LangGraph 文档", URL: "https://langchain-ai.github.io/langgraph/", Site: "langchain-ai.github.io", Credibility: "high"},
		"s2": {ID: "s2", Title: "AutoGen 仓库", URL: "https://github.com/microsoft/autogen", Site: "github.com", Credibility: "high"},
		"s3": {ID: "s3", Title: "CrewAI 仓库", URL: "https://github.com/crewAIInc/crewAI", Site: "github.com", Credibility: "medium"},
	}
	claims := []*domain.Claim{
		{
			ID: "c1", Statement: "LangGraph 支持有状态的图式编排，可用于多 Agent 工作流。",
			Status: domain.ClaimSupported, Confidence: 0.9, SourceCount: 2,
			Evidence: []domain.Evidence{
				{ID: "e1", ClaimID: "c1", SourceID: "s1", Quote: "LangGraph 支持有状态的图式编排", Stance: "support"},
				{ID: "e2", ClaimID: "c1", SourceID: "s2", Quote: "多 Agent 工作流", Stance: "support"},
			},
		},
		{
			ID: "c2", Statement: "AutoGen 提供多 Agent 对话框架。",
			Status: domain.ClaimPartial, Confidence: 0.6, SourceCount: 1,
			Evidence: []domain.Evidence{{ID: "e3", ClaimID: "c2", SourceID: "s2", Quote: "多 Agent 对话", Stance: "support"}},
		},
		{
			ID: "c3", Statement: "CrewAI 的周下载量为 120 万。",
			Status: domain.ClaimConflicting, Confidence: 0.4, SourceCount: 1, ConflictGroup: "downloads",
			Evidence: []domain.Evidence{{ID: "e4", ClaimID: "c3", SourceID: "s3", Quote: "周下载量 120 万", Stance: "unknown"}},
		},
		{
			ID: "c4", Statement: "某未公开项目将取代所有框架。",
			Status: domain.ClaimUnverified, Confidence: 0.1,
		},
	}
	return Input{
		Task: task, Claims: claims, Sources: sources,
		PlannerMode: "heuristic", GeneratedAt: "2026-11-04T00:00:00Z",
	}
}

func TestBuildProducesSections(t *testing.T) {
	rep, err := Build(fixture())
	if err != nil {
		t.Fatalf("Build 失败：%v", err)
	}
	if rep.Title == "" || rep.Markdown == "" {
		t.Fatal("报告标题与正文不能为空")
	}
	if len(rep.Sections) < 5 {
		t.Fatalf("章节数过少：%d", len(rep.Sections))
	}
	headings := make([]string, 0, len(rep.Sections))
	for _, s := range rep.Sections {
		headings = append(headings, s.Heading)
	}
	joined := strings.Join(headings, "|")
	for _, want := range []string{"摘要", "核心发现", "信息来源", "方法"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("缺少章节「%s」，实际：%v", want, headings)
		}
	}
}

func TestBuildCitationsPointToRealSources(t *testing.T) {
	in := fixture()
	rep, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Citations) == 0 {
		t.Fatal("应生成引用映射")
	}
	for i, c := range rep.Citations {
		if c.Index != i+1 {
			t.Fatalf("引用编号应连续：位置 %d 的编号为 %d", i, c.Index)
		}
		src := in.Sources[c.SourceID]
		if src == nil {
			t.Fatalf("引用 %d 指向不存在的资料 %q", c.Index, c.SourceID)
		}
		if c.URL == "" || !strings.HasPrefix(c.URL, "http") {
			t.Fatalf("引用 %d 的 URL 不可点击：%q", c.Index, c.URL)
		}
	}
	if rep.CitationCount != len(rep.Citations) {
		t.Fatalf("CitationCount 与实际不一致：%d vs %d", rep.CitationCount, len(rep.Citations))
	}
}

func TestBuildDropsOutOfRangeMarkers(t *testing.T) {
	in := fixture()
	in.Narrative = map[string]string{
		"核心发现": "这段正文引用了不存在的来源 [99]，以及合法来源 [1]。",
	}
	rep, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rep.Markdown, "[99]") {
		t.Fatal("越界引用标记应被剔除")
	}
	if !strings.Contains(rep.Markdown, "[1]") {
		t.Fatal("合法引用标记应保留")
	}
}

func TestBuildUnverifiedNotPresentedAsFact(t *testing.T) {
	rep, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range rep.Sections {
		if strings.Contains(s.Heading, "争议") || strings.Contains(s.Heading, "不确定") {
			found = true
			if !strings.Contains(s.Body, "取代所有框架") {
				t.Fatal("未验证结论应出现在不确定性章节")
			}
		}
	}
	if !found {
		t.Fatal("缺少争议与不确定性章节")
	}
}

func TestBuildRequiresTask(t *testing.T) {
	if _, err := Build(Input{}); err == nil {
		t.Fatal("缺少任务应报错")
	}
}

func TestBuildConflictDisclosed(t *testing.T) {
	rep, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rep.Markdown, "冲突") && !strings.Contains(rep.Markdown, "争议") {
		t.Fatal("存在冲突结论时报告必须显式披露")
	}
}

func TestRenderHTMLEscapesAndRestrictsLinks(t *testing.T) {
	rep, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	html := RenderHTML(rep.Title, rep.Markdown)
	if !strings.Contains(html, "<html") {
		t.Fatal("HTML 输出结构缺失")
	}
	if strings.Contains(html, "javascript:") {
		t.Fatal("HTML 不应包含 javascript: 链接")
	}
	if strings.Contains(html, "<script>alert") {
		t.Fatal("HTML 未转义脚本内容")
	}
	if !strings.Contains(html, "langchain-ai.github.io") {
		t.Fatal("HTML 应包含来源链接")
	}
}

func TestSanitizeMarkersKeepsValid(t *testing.T) {
	got := sanitizeMarkers("见 [1] 与 [2]，以及 [5]。", 2)
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[2]") {
		t.Fatalf("合法标记被误删：%q", got)
	}
	if strings.Contains(got, "[5]") {
		t.Fatalf("越界标记未被删除：%q", got)
	}
}
