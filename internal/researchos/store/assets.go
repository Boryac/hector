package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/verikod/hector/internal/researchos/db"
	"github.com/verikod/hector/internal/researchos/domain"
	"github.com/verikod/hector/internal/researchos/settings"
)

// ---------------------------------------------------------------------------
// 资料
// ---------------------------------------------------------------------------

// UpsertSource 写入或更新资料（按 task_id + normalized_url 去重）。
func (s *Store) UpsertSource(ctx context.Context, src *domain.Source) (*domain.Source, error) {
	row, err := s.Q.UpsertSource(ctx, db.UpsertSourceParams{
		ID:            src.ID,
		TaskID:        src.TaskID,
		NodeID:        src.NodeID,
		Url:           src.URL,
		NormalizedUrl: src.NormalizedURL,
		Title:         src.Title,
		Site:          src.Site,
		Provider:      src.Provider,
		Snippet:       src.Snippet,
		Excerpt:       truncate(src.Excerpt, 20000),
		ContentHash:   src.ContentHash,
		HttpStatus:    int32(src.HTTPStatus),
		Credibility:   defaultStr(src.Credibility, "unknown"),
		Status:        defaultStr(src.Status, "ok"),
		Error:         src.Error,
		PublishedAt:   tsOrNull(src.PublishedAt),
		FetchedAt:     tsOrNull(src.FetchedAt),
	})
	if err != nil {
		return nil, err
	}
	return sourceFromRow(row), nil
}

// ListSources 查询任务资料列表。
func (s *Store) ListSources(ctx context.Context, taskID, site, credibility, query string, limit, offset int) ([]*domain.Source, error) {
	p := db.ListSourcesParams{TaskID: taskID, Limit: int32(limit), Offset: int32(offset)}
	if site != "" {
		p.Site = pgtype.Text{String: site, Valid: true}
	}
	if credibility != "" {
		p.Credibility = pgtype.Text{String: credibility, Valid: true}
	}
	if query != "" {
		p.Query = pgtype.Text{String: query, Valid: true}
	}
	rows, err := s.Q.ListSources(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Source, 0, len(rows))
	for _, r := range rows {
		src := sourceFromRow(db.Source{
			ID: r.ID, TaskID: r.TaskID, NodeID: r.NodeID, Url: r.Url, NormalizedUrl: r.NormalizedUrl,
			Title: r.Title, Site: r.Site, Provider: r.Provider, Snippet: r.Snippet, Excerpt: r.Excerpt,
			ContentHash: r.ContentHash, HttpStatus: r.HttpStatus, Credibility: r.Credibility,
			Status: r.Status, Error: r.Error, PublishedAt: r.PublishedAt, FetchedAt: r.FetchedAt,
			CreatedAt: r.CreatedAt,
		})
		src.EvidenceCount = int(r.EvidenceCount)
		out = append(out, src)
	}
	return out, nil
}

// GetSource 读取单条资料。
func (s *Store) GetSource(ctx context.Context, taskID, sourceID string) (*domain.Source, error) {
	row, err := s.Q.GetSource(ctx, db.GetSourceParams{TaskID: taskID, ID: sourceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sourceFromRow(row), nil
}

// CountSources 统计资料数。
func (s *Store) CountSources(ctx context.Context, taskID string) (int64, error) {
	return s.Q.CountSources(ctx, taskID)
}

func sourceFromRow(r db.Source) *domain.Source {
	return &domain.Source{
		ID:            r.ID,
		TaskID:        r.TaskID,
		NodeID:        r.NodeID,
		URL:           r.Url,
		NormalizedURL: r.NormalizedUrl,
		Title:         r.Title,
		Site:          r.Site,
		Provider:      r.Provider,
		Snippet:       r.Snippet,
		Excerpt:       r.Excerpt,
		ContentHash:   r.ContentHash,
		HTTPStatus:    int(r.HttpStatus),
		Credibility:   r.Credibility,
		Status:        r.Status,
		Error:         r.Error,
		PublishedAt:   tsPtr(r.PublishedAt),
		FetchedAt:     tsPtr(r.FetchedAt),
		CreatedAt:     r.CreatedAt,
	}
}

// ---------------------------------------------------------------------------
// 结论与证据
// ---------------------------------------------------------------------------

// InsertClaim 写入结论。
func (s *Store) InsertClaim(ctx context.Context, c *domain.Claim) (*domain.Claim, error) {
	var numeric pgtype.Float8
	if c.NumericValue != nil {
		numeric = pgtype.Float8{Float64: *c.NumericValue, Valid: true}
	}
	row, err := s.Q.InsertClaim(ctx, db.InsertClaimParams{
		ID:            c.ID,
		TaskID:        c.TaskID,
		NodeID:        c.NodeID,
		Statement:     c.Statement,
		NormalizedKey: c.NormalizedKey,
		Status:        defaultStr(c.Status, domain.ClaimUnverified),
		Confidence:    float32(c.Confidence),
		ConflictGroup: c.ConflictGroup,
		VerifierNotes: c.VerifierNotes,
		SourceCount:   int32(c.SourceCount),
		NumericValue:  numeric,
	})
	if err != nil {
		return nil, err
	}
	return claimFromRow(row), nil
}

// SetClaimSourceCount 更新结论的独立来源计数（多源印证时使用）。
func (s *Store) SetClaimSourceCount(ctx context.Context, claimID string, count int) error {
	return s.Q.SetClaimSourceCount(ctx, db.SetClaimSourceCountParams{
		SourceCount: int32(count),
		ID:          claimID,
	})
}

// UpdateClaimVerification 更新核验结果。
func (s *Store) UpdateClaimVerification(ctx context.Context, c *domain.Claim) error {
	return s.Q.UpdateClaimVerification(ctx, db.UpdateClaimVerificationParams{
		ID:            c.ID,
		Status:        c.Status,
		Confidence:    float32(c.Confidence),
		ConflictGroup: c.ConflictGroup,
		VerifierNotes: c.VerifierNotes,
		SourceCount:   int32(c.SourceCount),
		TaskID:        c.TaskID,
	})
}

// ListClaims 查询结论（含证据）。
func (s *Store) ListClaims(ctx context.Context, taskID, status string, limit, offset int) ([]*domain.Claim, error) {
	p := db.ListClaimsParams{TaskID: taskID, Limit: int32(limit), Offset: int32(offset)}
	if status != "" {
		p.Status = pgtype.Text{String: status, Valid: true}
	}
	rows, err := s.Q.ListClaims(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Claim, 0, len(rows))
	for _, r := range rows {
		c := claimFromRow(r)
		ev, err := s.evidenceForClaim(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		c.Evidence = ev
		out = append(out, c)
	}
	return out, nil
}

// GetClaim 读取结论及证据。
func (s *Store) GetClaim(ctx context.Context, taskID, claimID string) (*domain.Claim, error) {
	row, err := s.Q.GetClaim(ctx, db.GetClaimParams{TaskID: taskID, ID: claimID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c := claimFromRow(row)
	ev, err := s.evidenceForClaim(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	c.Evidence = ev
	return c, nil
}

// CountClaims 统计结论数。
func (s *Store) CountClaims(ctx context.Context, taskID string) (int64, error) {
	return s.Q.CountClaims(ctx, taskID)
}

// InsertEvidence 写入证据边。
func (s *Store) InsertEvidence(ctx context.Context, e *domain.Evidence) (*domain.Evidence, error) {
	row, err := s.Q.InsertEvidence(ctx, db.InsertEvidenceParams{
		ID:         e.ID,
		TaskID:     e.TaskID,
		ClaimID:    e.ClaimID,
		SourceID:   e.SourceID,
		NodeID:     e.NodeID,
		Quote:      truncate(e.Quote, 2000),
		Stance:     defaultStr(e.Stance, "support"),
		Confidence: float32(e.Confidence),
	})
	if err != nil {
		return nil, err
	}
	return evidenceFromRow(row), nil
}

// ListEvidence 查询任务下全部证据。
func (s *Store) ListEvidence(ctx context.Context, taskID string) ([]*domain.Evidence, error) {
	rows, err := s.Q.ListEvidenceByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Evidence, 0, len(rows))
	for _, r := range rows {
		out = append(out, evidenceFromRow(r))
	}
	return out, nil
}

func (s *Store) evidenceForClaim(ctx context.Context, claimID string) ([]domain.Evidence, error) {
	rows, err := s.Q.ListEvidenceByClaim(ctx, claimID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Evidence, 0, len(rows))
	for _, r := range rows {
		out = append(out, *evidenceFromRow(r))
	}
	return out, nil
}

func claimFromRow(r db.Claim) *domain.Claim {
	c := &domain.Claim{
		ID:            r.ID,
		TaskID:        r.TaskID,
		NodeID:        r.NodeID,
		Statement:     r.Statement,
		NormalizedKey: r.NormalizedKey,
		Status:        r.Status,
		Confidence:    float64(r.Confidence),
		ConflictGroup: r.ConflictGroup,
		VerifierNotes: r.VerifierNotes,
		SourceCount:   int(r.SourceCount),
		CreatedAt:     r.CreatedAt,
	}
	if r.NumericValue.Valid {
		v := r.NumericValue.Float64
		c.NumericValue = &v
	}
	return c
}

func evidenceFromRow(r db.Evidence) *domain.Evidence {
	return &domain.Evidence{
		ID:         r.ID,
		TaskID:     r.TaskID,
		ClaimID:    r.ClaimID,
		SourceID:   r.SourceID,
		NodeID:     r.NodeID,
		Quote:      r.Quote,
		Stance:     r.Stance,
		Confidence: float64(r.Confidence),
		CreatedAt:  r.CreatedAt,
	}
}

// ---------------------------------------------------------------------------
// 报告
// ---------------------------------------------------------------------------

// InsertReport 写入新版本报告。
func (s *Store) InsertReport(ctx context.Context, r *domain.Report) (*domain.Report, error) {
	sections, _ := json.Marshal(orEmptySlice(r.Sections))
	citations, _ := json.Marshal(orEmptyCitations(r.Citations))
	row, err := s.Q.InsertReport(ctx, db.InsertReportParams{
		ID:            r.ID,
		TaskID:        r.TaskID,
		Title:         r.Title,
		Summary:       r.Summary,
		Markdown:      r.Markdown,
		Sections:      sections,
		Citations:     citations,
		CitationCount: int32(r.CitationCount),
		WordCount:     int32(r.WordCount),
		Model:         r.Model,
	})
	if err != nil {
		return nil, err
	}
	return reportFromRow(row), nil
}

// GetLatestReport 读取最新版本报告。
func (s *Store) GetLatestReport(ctx context.Context, taskID string) (*domain.Report, error) {
	row, err := s.Q.GetLatestReport(ctx, taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return reportFromRow(row), nil
}

// GetReportVersion 读取指定版本。
func (s *Store) GetReportVersion(ctx context.Context, taskID string, version int) (*domain.Report, error) {
	row, err := s.Q.GetReportVersion(ctx, db.GetReportVersionParams{TaskID: taskID, Version: int32(version)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return reportFromRow(row), nil
}

func reportFromRow(r db.Report) *domain.Report {
	rep := &domain.Report{
		ID:            r.ID,
		TaskID:        r.TaskID,
		Version:       int(r.Version),
		Title:         r.Title,
		Summary:       r.Summary,
		Markdown:      r.Markdown,
		CitationCount: int(r.CitationCount),
		WordCount:     int(r.WordCount),
		Model:         r.Model,
		CreatedAt:     r.CreatedAt,
	}
	if len(r.Sections) > 0 {
		_ = json.Unmarshal(r.Sections, &rep.Sections)
	}
	if len(r.Citations) > 0 {
		_ = json.Unmarshal(r.Citations, &rep.Citations)
	}
	return rep
}

// ---------------------------------------------------------------------------
// Agent 执行与工具调用
// ---------------------------------------------------------------------------

// InsertAgentExecution 记录 Agent 开始执行。
func (s *Store) InsertAgentExecution(ctx context.Context, a *domain.AgentExecution) error {
	usage, _ := json.Marshal(orEmptyMap(a.TokenUsage))
	return s.Q.InsertAgentExecution(ctx, db.InsertAgentExecutionParams{
		ID:         a.ID,
		TaskID:     a.TaskID,
		RunID:      a.RunID,
		NodeID:     a.NodeID,
		Role:       a.Role,
		Name:       a.Name,
		Model:      a.Model,
		TokenUsage: usage,
	})
}

// FinishAgentExecution 记录 Agent 结束。
func (s *Store) FinishAgentExecution(ctx context.Context, id, status, errMsg string, usage map[string]any) error {
	raw, _ := json.Marshal(orEmptyMap(usage))
	return s.Q.FinishAgentExecution(ctx, db.FinishAgentExecutionParams{
		ID: id, Status: status, Error: errMsg, TokenUsage: raw,
	})
}

// ListAgentExecutions 查询 Agent 执行记录。
func (s *Store) ListAgentExecutions(ctx context.Context, taskID string) ([]*domain.AgentExecution, error) {
	rows, err := s.Q.ListAgentExecutions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.AgentExecution, 0, len(rows))
	for _, r := range rows {
		a := &domain.AgentExecution{
			ID: r.ID, TaskID: r.TaskID, RunID: r.RunID, NodeID: r.NodeID, Role: r.Role,
			Name: r.Name, Model: r.Model, Status: r.Status, StartedAt: r.StartedAt,
			DurationMS: int(r.DurationMs), Error: r.Error,
		}
		if len(r.TokenUsage) > 0 {
			_ = json.Unmarshal(r.TokenUsage, &a.TokenUsage)
		}
		if len(r.ToolCalls) > 0 {
			_ = json.Unmarshal(r.ToolCalls, &a.ToolCalls)
		}
		out = append(out, a)
	}
	return out, nil
}

// InsertToolInvocation 记录工具调用开始。
func (s *Store) InsertToolInvocation(ctx context.Context, id, taskID, nodeID, toolName, serverID string, args map[string]any) error {
	raw, _ := json.Marshal(orEmptyMap(args))
	return s.Q.InsertToolInvocation(ctx, db.InsertToolInvocationParams{
		ID: id, TaskID: taskID, NodeID: nodeID, ToolName: toolName, ServerID: serverID, Args: raw,
	})
}

// FinishToolInvocation 记录工具调用结束。
func (s *Store) FinishToolInvocation(ctx context.Context, id, status string, result map[string]any, errMsg string) error {
	var raw []byte
	if result != nil {
		raw, _ = json.Marshal(result)
	}
	return s.Q.FinishToolInvocation(ctx, db.FinishToolInvocationParams{
		ID: id, Status: status, Result: raw, Error: errMsg,
	})
}

func orEmptySlice(s []domain.Section) []domain.Section {
	if s == nil {
		return []domain.Section{}
	}
	return s
}

func orEmptyCitations(c []domain.Citation) []domain.Citation {
	if c == nil {
		return []domain.Citation{}
	}
	return c
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func tsOrNull(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// ListSourcesByNode 读取某节点产出的资料。
func (s *Store) ListSourcesByNode(ctx context.Context, taskID, nodeID string) ([]*domain.Source, error) {
	rows, err := s.Q.ListSourcesByNode(ctx, db.ListSourcesByNodeParams{TaskID: taskID, NodeID: nodeID})
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Source, 0, len(rows))
	for _, r := range rows {
		out = append(out, sourceFromRow(r))
	}
	return out, nil
}

// ListSourcesByIDs 按 ID 批量读取资料。
func (s *Store) ListSourcesByIDs(ctx context.Context, taskID string, ids []string) ([]*domain.Source, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.Q.ListSourcesByIDs(ctx, db.ListSourcesByIDsParams{TaskID: taskID, Ids: ids})
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Source, 0, len(rows))
	for _, r := range rows {
		out = append(out, sourceFromRow(r))
	}
	return out, nil
}

// ListAllClaims 读取任务全部结论（不做分页，用于验证与报告）。
func (s *Store) ListAllClaims(ctx context.Context, taskID string) ([]*domain.Claim, error) {
	rows, err := s.Q.ListClaimsFull(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Claim, 0, len(rows))
	for _, r := range rows {
		c := claimFromRow(r)
		ev, err := s.evidenceForClaim(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		c.Evidence = ev
		out = append(out, c)
	}
	return out, nil
}

// InsertRun 创建执行轮次。
func (s *Store) InsertRun(ctx context.Context, id, taskID, trigger string) error {
	_, err := s.Q.CreateRun(ctx, db.CreateRunParams{ID: id, TaskID: taskID, Trigger: trigger})
	return err
}

// GetOrCreateRun 返回当前 run，若不存在则创建。
func (s *Store) GetOrCreateRun(ctx context.Context, taskID string) (string, error) {
	row, err := s.Q.GetCurrentRun(ctx, taskID)
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	id := uuid.NewString()
	if err := s.InsertRun(ctx, id, taskID, "initial"); err != nil {
		return "", err
	}
	return id, nil
}

// NodeExecution 记录节点执行历史开始。
func (s *Store) StartNodeExecution(ctx context.Context, id, taskID, runID, nodeID string, attempt int, workerID string) error {
	return s.Q.InsertNodeExecution(ctx, db.InsertNodeExecutionParams{
		ID: id, TaskID: taskID, RunID: runID, NodeID: nodeID, Attempt: int32(attempt), WorkerID: workerID,
	})
}

// FinishNodeExecution 记录节点执行历史结束。
func (s *Store) FinishNodeExecution(ctx context.Context, id, status, errMsg string, output map[string]any) error {
	var raw []byte
	if output != nil {
		raw, _ = json.Marshal(output)
	}
	return s.Q.FinishNodeExecution(ctx, db.FinishNodeExecutionParams{
		ID: id, Status: status, Error: errMsg, Output: raw,
	})
}

// GetSetting 读取设置项（不存在返回 nil）。
func (s *Store) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	raw, err := s.Q.GetSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return raw, err
}

// SetSetting 写入设置项。
func (s *Store) SetSetting(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.Q.SetSetting(ctx, db.SetSettingParams{Key: key, Value: raw})
}

// ListMCPServers 列出已注册的 MCP 服务。
func (s *Store) ListMCPServers(ctx context.Context) ([]db.McpServer, error) {
	return s.Q.ListMCPServers(ctx)
}

// UpsertMCPServer 注册/更新 MCP 服务。
func (s *Store) UpsertMCPServer(ctx context.Context, id, name, command string, args []string, env map[string]string, status string, approved bool) (db.McpServer, error) {
	argsJSON, _ := json.Marshal(args)
	envJSON, _ := json.Marshal(env)
	return s.Q.UpsertMCPServer(ctx, db.UpsertMCPServerParams{
		ID: id, Name: name, Command: command, Args: argsJSON, Env: envJSON,
		Status: status, Approved: approved,
	})
}

// SetMCPServerStatus 更新 MCP 服务状态。
func (s *Store) SetMCPServerStatus(ctx context.Context, id, status string) error {
	return s.Q.SetMCPServerStatus(ctx, db.SetMCPServerStatusParams{ID: id, Status: status})
}

// ListToolCalls 查询工具调用记录。
func (s *Store) ListToolCalls(ctx context.Context, taskID string, limit int) ([]db.ToolInvocation, error) {
	p := db.ListToolInvocationsParams{Limit: int32(limit)}
	if taskID != "" {
		p.TaskID = pgtype.Text{String: taskID, Valid: true}
	}
	return s.Q.ListToolInvocations(ctx, p)
}

// LoadSettings 读取全部设置行（含 section / encrypted 元数据）。
func (s *Store) LoadSettings(ctx context.Context) (map[string]settings.StoredValue, error) {
	rows, err := s.Q.ListSettingRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]settings.StoredValue, len(rows))
	for _, r := range rows {
		var v any
		if err := json.Unmarshal(r.Value, &v); err != nil {
			// 解析不了就跳过：坏数据不该让整个设置中心打不开。
			continue
		}
		out[r.Key] = settings.StoredValue{Value: v, Encrypted: r.Encrypted}
	}
	return out, nil
}

// SaveSettingRow 写入设置行（携带分组与加密标记）。
func (s *Store) SaveSettingRow(ctx context.Context, key, section string, value any, encrypted bool, updatedBy string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.Q.UpsertSettingRow(ctx, db.UpsertSettingRowParams{
		Key: key, Value: raw, Section: section, Encrypted: encrypted, UpdatedBy: updatedBy,
	})
}

// DeleteSettingRow 删除设置行。
func (s *Store) DeleteSettingRow(ctx context.Context, key string) error {
	return s.Q.DeleteSettingRow(ctx, key)
}

// SetMCPServerApproved 设置审批状态并同步运行状态。
func (s *Store) SetMCPServerApproved(ctx context.Context, id string, approved bool, status string) error {
	return s.Q.SetMCPServerApproved(ctx, db.SetMCPServerApprovedParams{ID: id, Approved: approved, Status: status})
}

// UpdateMCPServerTools 写入工具发现结果。
func (s *Store) UpdateMCPServerTools(ctx context.Context, id string, tools json.RawMessage, status, lastErr string) error {
	return s.Q.UpdateMCPServerTools(ctx, db.UpdateMCPServerToolsParams{
		ID: id, Tools: tools, Status: status, LastError: lastErr,
	})
}

// DeleteMCPServer 删除 MCP 服务登记。
func (s *Store) DeleteMCPServer(ctx context.Context, id string) error {
	return s.Q.DeleteMCPServer(ctx, id)
}
