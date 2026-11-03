// Package domain 定义 ResearchOS 的领域模型。
//
// 模型与 DATA_MODEL.md 一一对应；任何字段变更必须同步更新 DATA_MODEL.md 与迁移脚本。
package domain

import "time"

// 任务生命周期（DATA_MODEL.md）：Created → Planning → AwaitingApproval → Running →
// Synthesizing → Completed；异常分支 Paused / Failed / Cancelled。
const (
	TaskCreated          = "created"
	TaskPlanning         = "planning"
	TaskAwaitingApproval = "awaiting_approval"
	TaskRunning          = "running"
	TaskPaused           = "paused"
	TaskSynthesizing     = "synthesizing"
	TaskCompleted        = "completed"
	TaskFailed           = "failed"
	TaskCancelled        = "cancelled"
)

// 节点状态。
const (
	NodePending   = "pending"
	NodeQueued    = "queued"
	NodeRunning   = "running"
	NodeCompleted = "completed"
	NodeFailed    = "failed"
	NodeSkipped   = "skipped"
	NodeCancelled = "cancelled"
)

// 节点类型（决定由哪个执行器处理）。
const (
	NodeTypeSearch     = "search"
	NodeTypeFetch      = "fetch"
	NodeTypeAnalyze    = "analyze"
	NodeTypeVerify     = "verify"
	NodeTypeSynthesize = "synthesize"
)

// Agent 角色（DATA_MODEL.md AgentExecution.role）。
const (
	RolePlanner     = "planner"
	RoleResearcher  = "researcher"
	RoleAnalyst     = "analyst"
	RoleVerifier    = "verifier"
	RoleSynthesizer = "synthesizer"
	RoleReporter    = "reporter"
)

// Claim 状态。
const (
	ClaimUnverified  = "unverified"
	ClaimSupported   = "supported"
	ClaimPartial     = "partially_supported"
	ClaimConflicting = "conflicting"
	ClaimRefuted     = "refuted"
)

// Task 是一次研究任务。
type Task struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Goal               string         `json:"goal"`
	Depth              string         `json:"depth"`
	Scope              map[string]any `json:"scope"`
	MaxAgents          int            `json:"max_agents"`
	MaxDurationSeconds int            `json:"max_duration_seconds"`
	EnableVerification bool           `json:"enable_verification"`
	Status             string         `json:"status"`
	CreatedBy          string         `json:"created_by"`
	PlannerMode        string         `json:"planner_mode"`
	Plan               *Plan          `json:"plan,omitempty"`
	PlanError          string         `json:"plan_error,omitempty"`
	Workflow           *Workflow      `json:"workflow,omitempty"`
	Stats              map[string]any `json:"stats"`
	Error              string         `json:"error,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	StartedAt          *time.Time     `json:"started_at,omitempty"`
	FinishedAt         *time.Time     `json:"finished_at,omitempty"`
	Nodes              []*Node        `json:"nodes,omitempty"`
	LatestSequence     int64          `json:"latest_sequence"`
}

// Node 是工作流节点（定义 + 当前执行状态）。
type Node struct {
	ID           string         `json:"id"`
	TaskID       string         `json:"task_id"`
	Title        string         `json:"title"`
	Type         string         `json:"type"`
	AgentRole    string         `json:"agent_role"`
	AgentName    string         `json:"agent_name"`
	DependsOn    []string       `json:"depends_on"`
	Position     int            `json:"position"`
	Status       string         `json:"status"`
	Attempts     int            `json:"attempts"`
	MaxAttempts  int            `json:"max_attempts"`
	Input        map[string]any `json:"input,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
	Error        string         `json:"error,omitempty"`
	Progress     int            `json:"progress"`
	ProgressText string         `json:"progress_text,omitempty"`
	TokensTotal  int            `json:"tokens_total"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
}

// Workflow 是工作流定义（纯结构，无执行状态）。
type Workflow struct {
	Nodes []WorkflowNode `json:"nodes"`
	Edges []Edge         `json:"edges"`
}

// WorkflowNode 是定义层节点。
type WorkflowNode struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Title     string   `json:"title"`
	AgentRole string   `json:"agent_role"`
	DependsOn []string `json:"depends_on"`
}

// Edge 是定义层边。
type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Plan 是 Planner 的结构化输出（DATA_MODEL.md 规定的 Schema）。
type Plan struct {
	Goal              string     `json:"goal"`
	ResearchQuestions []Question `json:"research_questions"`
	Subtasks          []Subtask  `json:"subtasks"`
	Workflow          Workflow   `json:"workflow"`
	RisksAndGaps      []string   `json:"risks_and_gaps"`
}

// Question 是研究问题。
type Question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Priority string `json:"priority"`
}

// Subtask 是 Planner 拆解出的子任务。
type Subtask struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	AgentRole string   `json:"agent_role"`
	Inputs    []string `json:"inputs"`
	Outputs   []string `json:"outputs"`
	DependsOn []string `json:"depends_on"`
	NodeType  string   `json:"node_type,omitempty"`
	// Query 是 search 节点提交给搜索引擎的检索式。
	//
	// 由 LLM 规划器填写时优先使用：模型知道每个子问题该查什么词，比按
	// 「目标 + 固定维度词」套模板更贴近问题。启发式规划器留空，此时回退到
	// SearchQueryFor 的模板检索式。
	Query string `json:"query,omitempty"`
}

// Source 是一条检索/抓取到的资料。
type Source struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"task_id"`
	NodeID        string     `json:"node_id"`
	URL           string     `json:"url"`
	NormalizedURL string     `json:"normalized_url"`
	Title         string     `json:"title"`
	Site          string     `json:"site"`
	Provider      string     `json:"provider"`
	Snippet       string     `json:"snippet"`
	Excerpt       string     `json:"excerpt"`
	ContentHash   string     `json:"content_hash"`
	HTTPStatus    int        `json:"http_status"`
	Credibility   string     `json:"credibility"`
	Status        string     `json:"status"`
	Error         string     `json:"error,omitempty"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	FetchedAt     *time.Time `json:"fetched_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	EvidenceCount int        `json:"evidence_count"`
}

// Claim 是一条结论。
type Claim struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"task_id"`
	NodeID        string     `json:"node_id"`
	Statement     string     `json:"statement"`
	NormalizedKey string     `json:"normalized_key"`
	Status        string     `json:"status"`
	Confidence    float64    `json:"confidence"`
	ConflictGroup string     `json:"conflict_group,omitempty"`
	VerifierNotes string     `json:"verifier_notes,omitempty"`
	SourceCount   int        `json:"source_count"`
	NumericValue  *float64   `json:"numeric_value,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Evidence      []Evidence `json:"evidence,omitempty"`
}

// Evidence 是结论与资料之间的证据边。
type Evidence struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	ClaimID    string    `json:"claim_id"`
	SourceID   string    `json:"source_id"`
	NodeID     string    `json:"node_id"`
	Quote      string    `json:"quote"`
	Stance     string    `json:"stance"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
	Source     *Source   `json:"source,omitempty"`
}

// Report 是研究报告（多版本）。
type Report struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"task_id"`
	Version       int        `json:"version"`
	Title         string     `json:"title"`
	Summary       string     `json:"summary"`
	Markdown      string     `json:"markdown"`
	Sections      []Section  `json:"sections"`
	Citations     []Citation `json:"citations"`
	CitationCount int        `json:"citation_count"`
	WordCount     int        `json:"word_count"`
	Model         string     `json:"model"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Section 是报告章节。
type Section struct {
	Heading  string   `json:"heading"`
	Body     string   `json:"body"`
	Citation []string `json:"citation_ids,omitempty"`
}

// Citation 是引用映射（报告编号 → 证据 → 资料）。
type Citation struct {
	Index    int    `json:"index"`
	ClaimID  string `json:"claim_id"`
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Site     string `json:"site"`
	Quote    string `json:"quote,omitempty"`
}

// Event 是统一事件协议中的事件。
type Event struct {
	EventID   string         `json:"event_id"`
	TaskID    string         `json:"task_id"`
	RunID     string         `json:"run_id"`
	NodeID    string         `json:"node_id"`
	Type      string         `json:"event_type"`
	Timestamp time.Time      `json:"timestamp"`
	Sequence  int64          `json:"sequence"`
	Payload   map[string]any `json:"payload"`
}

// AgentExecution 记录一次 Agent 角色执行。
type AgentExecution struct {
	ID         string         `json:"id"`
	TaskID     string         `json:"task_id"`
	RunID      string         `json:"run_id"`
	NodeID     string         `json:"node_id"`
	Role       string         `json:"role"`
	Name       string         `json:"name"`
	Model      string         `json:"model"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	DurationMS int            `json:"duration_ms"`
	TokenUsage map[string]any `json:"token_usage"`
	ToolCalls  []any          `json:"tool_calls"`
	Error      string         `json:"error,omitempty"`
}

// 事件类型常量（EVENT_PROTOCOL.md，不得擅自重命名）。
const (
	EvtResearchCreated      = "research.created"
	EvtResearchPlanning     = "research.planning"
	EvtResearchPlanned      = "research.planned"
	EvtResearchStarted      = "research.started"
	EvtResearchPaused       = "research.paused"
	EvtResearchResumed      = "research.resumed"
	EvtResearchCompleted    = "research.completed"
	EvtResearchFailed       = "research.failed"
	EvtResearchSynthesizing = "research.synthesizing"
	EvtResearchCancelled    = "research.cancelled"
	// EvtResearchTransitionRejected 记录被状态机拒绝的非法跃迁（审计用，不改状态）。
	EvtResearchTransitionRejected = "research.transition_rejected"
	EvtWorkflowCreated            = "workflow.created"
	EvtWorkflowUpdated            = "workflow.updated"
	EvtNodeQueued                 = "node.queued"
	EvtNodeStarted                = "node.started"
	EvtNodeProgress               = "node.progress"
	EvtNodeCompleted              = "node.completed"
	EvtNodeFailed                 = "node.failed"
	EvtNodeRetrying               = "node.retrying"
	EvtAgentCreated               = "agent.created"
	EvtAgentStarted               = "agent.started"
	EvtAgentCompleted             = "agent.completed"
	EvtAgentFailed                = "agent.failed"
	EvtToolInvoked                = "tool.invoked"
	EvtToolCompleted              = "tool.completed"
	EvtToolFailed                 = "tool.failed"
	EvtEvidenceCreated            = "evidence.created"
	EvtEvidenceVerified           = "evidence.verified"
	EvtReportStarted              = "report.started"
	EvtReportCompleted            = "report.completed"
)
