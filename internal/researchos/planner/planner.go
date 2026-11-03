// Package planner 把自然语言研究目标转成结构化研究计划（DATA_MODEL.md 的 Plan Schema）。
//
// 两种模式，绝不混淆：
//   - llm：调用真实大模型，输出必须通过 Schema 与 DAG 校验，失败重试一次后降级；
//   - heuristic：未配置 LLM 时使用的确定性模板规划器，输出在报告中明确标注。
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/verikod/hector/internal/researchos/dag"
	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/llm"
)

// Options 是规划参数。
type Options struct {
	Depth              string
	EnableVerification bool
	MaxQuestions       int
	Scope              map[string]any
}

// Planner 生成研究计划。
type Planner interface {
	Plan(ctx context.Context, goal string, opts Options) (*domain.Plan, llm.Usage, error)
	Mode() string
}

// New 构造规划器：LLM 可用则用 LLM（带回退），否则用启发式。
func New(client llm.Client) Planner {
	heur := &Heuristic{}
	if client != nil && client.Available() {
		return &Fallback{primary: &LLM{client: client}, fallback: heur, client: client}
	}
	return heur
}

// ---------------------------------------------------------------------------
// 启发式规划器
// ---------------------------------------------------------------------------

// Heuristic 是确定性模板规划器（无 LLM 时使用）。
type Heuristic struct{}

// Mode 返回模式标识。
func (h *Heuristic) Mode() string { return "heuristic" }

// Plan 生成模板化 DAG：N 个检索 → N 个分析 → 交叉验证 → 综述。
func (h *Heuristic) Plan(_ context.Context, goal string, opts Options) (*domain.Plan, llm.Usage, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return nil, llm.Usage{}, fmt.Errorf("研究目标为空")
	}
	n := opts.MaxQuestions
	if n <= 0 {
		n = 3
	}
	templates := []string{
		"%s 的现状、主要参与者与代表性项目",
		"%s 的关键技术路线与能力差异",
		"%s 的生态成熟度、落地案例与已知风险",
		"%s 的发展趋势与尚未解决的问题",
	}
	questions := make([]domain.Question, 0, n)
	subtasks := make([]domain.Subtask, 0, n*2+2)
	for i := 0; i < n; i++ {
		q := fmt.Sprintf(templates[i%len(templates)], goal)
		qid := fmt.Sprintf("q%d", i+1)
		priority := "high"
		if i >= 2 {
			priority = "medium"
		}
		questions = append(questions, domain.Question{ID: qid, Question: q, Priority: priority})
		searchID := fmt.Sprintf("search_%d", i+1)
		analyzeID := fmt.Sprintf("analyze_%d", i+1)
		subtasks = append(subtasks,
			domain.Subtask{
				ID: searchID, Title: "检索：" + q, AgentRole: domain.RoleResearcher,
				Inputs: []string{qid}, Outputs: []string{"sources"}, NodeType: domain.NodeTypeSearch,
			},
			domain.Subtask{
				ID: analyzeID, Title: "分析：" + q, AgentRole: domain.RoleAnalyst,
				Inputs: []string{searchID}, Outputs: []string{"claims"}, NodeType: domain.NodeTypeAnalyze,
				DependsOn: []string{searchID},
			},
		)
	}

	lastIDs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lastIDs = append(lastIDs, fmt.Sprintf("analyze_%d", i+1))
	}
	if opts.EnableVerification {
		subtasks = append(subtasks, domain.Subtask{
			ID: "verify", Title: "交叉验证：来源冲突识别与置信度计算", AgentRole: domain.RoleVerifier,
			Inputs: lastIDs, Outputs: []string{"verified_claims"}, NodeType: domain.NodeTypeVerify,
			DependsOn: lastIDs,
		})
		lastIDs = []string{"verify"}
	}
	subtasks = append(subtasks, domain.Subtask{
		ID: "synthesize", Title: "综述：生成带引用的研究报告", AgentRole: domain.RoleSynthesizer,
		Inputs: lastIDs, Outputs: []string{"report"}, NodeType: domain.NodeTypeSynthesize,
		DependsOn: lastIDs,
	})

	plan := &domain.Plan{
		Goal:              goal,
		ResearchQuestions: questions,
		Subtasks:          subtasks,
		RisksAndGaps: []string{
			"当前为启发式规划模式（未配置 LLM），研究问题由模板生成，覆盖面有限。",
			"检索质量取决于可用搜索 Provider，缺失凭据时仅使用公开 API。",
		},
	}
	plan.Workflow = BuildWorkflow(plan)
	return plan, llm.Usage{}, nil
}

// ---------------------------------------------------------------------------
// LLM 规划器
// ---------------------------------------------------------------------------

// LLM 用大模型生成计划。
type LLM struct{ client llm.Client }

// Mode 返回模式标识。
func (l *LLM) Mode() string { return "llm" }

const plannerSystemPrompt = `你是 Hector ResearchOS 的研究规划器（Planner）。
你的任务是把用户的研究目标拆解为可执行的研究计划，并且只输出 JSON，不要输出任何解释文字。

输出 JSON Schema：
{
  "goal": "string",
  "research_questions": [{"id":"q1","question":"string","priority":"high|medium|low"}],
  "subtasks": [{
     "id":"search_1","title":"string","agent_role":"researcher|analyst|verifier|synthesizer",
     "node_type":"search|analyze|verify|synthesize",
     "inputs":["string"],"outputs":["string"],"depends_on":["string"]
  }],
  "risks_and_gaps": ["string"]
}

硬性要求：
1. 必须有 1 个 node_type=search 的节点作为每个研究问题的入口，depends_on 为空。
2. 每个 search 节点后必须有 1 个 node_type=analyze 节点，depends_on 指向对应 search。
3. 若开启验证，必须有且仅有 1 个 node_type=verify 节点，依赖全部 analyze 节点。
4. 必须有且仅有 1 个 node_type=synthesize 节点，位于最后。
5. 节点 id 只能由字母、数字、下划线组成，且全图唯一；依赖必须构成无环图。
6. 研究问题控制在 2-6 个，每个问题对应一组 search+analyze。`

// Plan 调用模型生成计划并校验。
func (l *LLM) Plan(ctx context.Context, goal string, opts Options) (*domain.Plan, llm.Usage, error) {
	scope, _ := json.Marshal(opts.Scope)
	user := fmt.Sprintf("研究目标：%s\n研究深度：%s\n是否开启交叉验证：%v\n范围限制：%s\n请输出计划 JSON。",
		goal, opts.Depth, opts.EnableVerification, string(scope))
	raw, usage, err := l.client.CompleteJSON(ctx, plannerSystemPrompt, user)
	if err != nil {
		return nil, usage, err
	}
	var plan domain.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, usage, fmt.Errorf("计划 JSON 解析失败: %w", err)
	}
	if plan.Goal == "" {
		plan.Goal = goal
	}
	if err := Normalize(&plan, opts); err != nil {
		return nil, usage, err
	}
	return &plan, usage, nil
}

// Fallback 先试 LLM，失败则用启发式，并把原因带回给调用方记录。
type Fallback struct {
	primary  Planner
	fallback Planner
	client   llm.Client
}

// Mode 返回模式标识。
func (f *Fallback) Mode() string { return "llm" }

// Plan 执行带降级的规划。
func (f *Fallback) Plan(ctx context.Context, goal string, opts Options) (*domain.Plan, llm.Usage, error) {
	plan, usage, err := f.primary.Plan(ctx, goal, opts)
	if err == nil {
		return plan, usage, nil
	}
	plan, usage2, err2 := f.fallback.Plan(ctx, goal, opts)
	if err2 != nil {
		return nil, usage, fmt.Errorf("LLM 规划失败(%v)，启发式规划亦失败(%v)", err, err2)
	}
	plan.RisksAndGaps = append(plan.RisksAndGaps,
		fmt.Sprintf("LLM 规划失败并已降级为启发式规划：%v", err))
	usage.TotalTokens += usage2.TotalTokens
	return plan, usage, nil
}

// ---------------------------------------------------------------------------
// 校验与规范化
// ---------------------------------------------------------------------------

var idRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)

// Normalize 校验并规范化计划：补 node_type、修正角色映射、校验 DAG。
func Normalize(plan *domain.Plan, opts Options) error {
	if len(plan.Subtasks) == 0 {
		return fmt.Errorf("计划缺少 subtasks")
	}
	seen := map[string]bool{}
	for i := range plan.Subtasks {
		st := &plan.Subtasks[i]
		st.ID = strings.TrimSpace(st.ID)
		if !idRe.MatchString(st.ID) {
			return fmt.Errorf("子任务 ID 非法：%q", st.ID)
		}
		if seen[st.ID] {
			return fmt.Errorf("子任务 ID 重复：%s", st.ID)
		}
		seen[st.ID] = true
		if st.NodeType == "" {
			st.NodeType = nodeTypeForRole(st.AgentRole)
		}
		if !validNodeType(st.NodeType) {
			return fmt.Errorf("子任务 %s 的 node_type 非法：%q", st.ID, st.NodeType)
		}
		if st.AgentRole == "" {
			st.AgentRole = roleForNodeType(st.NodeType)
		}
		if st.Title == "" {
			st.Title = st.ID
		}
		// 去重依赖，剔除自依赖。
		deps := make([]string, 0, len(st.DependsOn))
		dedupe := map[string]bool{}
		for _, d := range st.DependsOn {
			d = strings.TrimSpace(d)
			if d == "" || d == st.ID || dedupe[d] {
				continue
			}
			dedupe[d] = true
			deps = append(deps, d)
		}
		st.DependsOn = deps
	}

	nodes := NodesFromPlan(plan)
	if err := dag.Validate(nodes); err != nil {
		return err
	}

	// 结构约束：至少一个 search、一个 synthesize，且最后一个节点必须是 synthesize。
	counts := map[string]int{}
	for _, n := range nodes {
		counts[n.Type]++
	}
	if counts[domain.NodeTypeSearch] == 0 {
		return fmt.Errorf("计划缺少 search 节点")
	}
	if counts[domain.NodeTypeSynthesize] != 1 {
		return fmt.Errorf("计划必须有且仅有 1 个 synthesize 节点，实际 %d 个", counts[domain.NodeTypeSynthesize])
	}
	if opts.EnableVerification && counts[domain.NodeTypeVerify] == 0 {
		return fmt.Errorf("开启交叉验证但计划缺少 verify 节点")
	}
	order, err := dag.TopoSort(nodes)
	if err != nil {
		return err
	}
	last := order[len(order)-1]
	for _, n := range nodes {
		if n.ID == last && n.Type != domain.NodeTypeSynthesize {
			return fmt.Errorf("拓扑序最后一个节点 %s 不是 synthesize", n.ID)
		}
	}
	if len(plan.ResearchQuestions) == 0 {
		plan.ResearchQuestions = []domain.Question{{ID: "q1", Question: plan.Goal, Priority: "high"}}
	}
	return nil
}

// NodesFromPlan 把计划转成工作流节点（含位置序）。
func NodesFromPlan(plan *domain.Plan) []domain.WorkflowNode {
	nodes := make([]domain.WorkflowNode, 0, len(plan.Subtasks))
	for i, st := range plan.Subtasks {
		nodes = append(nodes, domain.WorkflowNode{
			ID:        st.ID,
			Type:      st.NodeType,
			Title:     st.Title,
			AgentRole: st.AgentRole,
			DependsOn: st.DependsOn,
		})
		_ = i
	}
	return nodes
}

// BuildWorkflow 由计划构造定义层工作流（nodes + edges）。
func BuildWorkflow(plan *domain.Plan) domain.Workflow {
	nodes := NodesFromPlan(plan)
	return domain.Workflow{Nodes: nodes, Edges: dag.BuildEdges(nodes)}
}

func nodeTypeForRole(role string) string {
	switch role {
	case domain.RoleResearcher:
		return domain.NodeTypeSearch
	case domain.RoleAnalyst:
		return domain.NodeTypeAnalyze
	case domain.RoleVerifier:
		return domain.NodeTypeVerify
	case domain.RoleSynthesizer, domain.RoleReporter:
		return domain.NodeTypeSynthesize
	default:
		return domain.NodeTypeAnalyze
	}
}

func roleForNodeType(t string) string {
	switch t {
	case domain.NodeTypeSearch:
		return domain.RoleResearcher
	case domain.NodeTypeAnalyze:
		return domain.RoleAnalyst
	case domain.NodeTypeVerify:
		return domain.RoleVerifier
	case domain.NodeTypeSynthesize:
		return domain.RoleSynthesizer
	default:
		return domain.RoleAnalyst
	}
}

func validNodeType(t string) bool {
	switch t {
	case domain.NodeTypeSearch, domain.NodeTypeAnalyze, domain.NodeTypeVerify, domain.NodeTypeSynthesize, domain.NodeTypeFetch:
		return true
	}
	return false
}
