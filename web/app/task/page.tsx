'use client';

import React, { Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { apiGet, apiSend, downloadReport } from '@/lib/api';
import type {
  AgentExecution, Claim, EventItem, Node, Report, Source, TaskDetail,
} from '@/lib/types';
import {
  cx, errorText, formatDateTime, formatDurationMs, formatRelative, num, pct,
  DEPTH_TEXT, PLANNER_TEXT, NODE_TYPE_TEXT, taskStatusText,
} from '@/lib/format';
import { useEventStream, usePolling, useTaskEvents } from '@/lib/hooks';
import { eventMeta, type EventCategory } from '@/lib/event-map';
import { Icon } from '@/components/Icon';
import {
  Button, EmptyState, ErrorState, Modal, Skeleton, StatusBadge, Tabs, Progress,
} from '@/components/ui';
import { useToast } from '@/components/Toast';
import { WorkflowCanvas } from '@/components/WorkflowCanvas';
import { SourceDetail, SourceTable } from '@/components/SourceTable';
import { EvidenceList, EvidenceSummary } from '@/components/EvidenceList';
import { ReportReader } from '@/components/ReportReader';
import { AgentTable } from '@/components/AgentTable';

type Tab = 'workflow' | 'sources' | 'evidence' | 'report' | 'agents';
type EventFilter = 'all' | EventCategory;

const EVENT_FILTERS: Array<{ value: EventFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'research', label: '研究' },
  { value: 'node', label: '节点' },
  { value: 'agent', label: 'Agent' },
  { value: 'tool', label: '工具' },
  { value: 'evidence', label: '证据' },
  { value: 'report', label: '报告' },
];

function outputValue(node: Node, key: string): unknown {
  const output = node.output as Record<string, unknown> | undefined;
  return output?.[key];
}

function inputValue(node: Node, key: string): unknown {
  const input = node.input as Record<string, unknown> | undefined;
  return input?.[key];
}

function TaskWorkspace() {
  const params = useSearchParams();
  const id = params.get('id') ?? '';
  const { push } = useToast();

  const detailQ = usePolling<TaskDetail>(
    () => apiGet<TaskDetail>(`/api/v1/research/${id}`),
    { intervalMs: 4000, key: id, enabled: Boolean(id) },
  );
  const detail = detailQ.data;
  const task = detail?.task;
  const nodes = useMemo(() => detail?.nodes ?? [], [detail]);

  const [tab, setTab] = useState<Tab>('workflow');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [liveEvents, setLiveEvents] = useState<EventItem[]>([]);
  const [eventFilter, setEventFilter] = useState<EventFilter>('all');
  const [outlineOpen, setOutlineOpen] = useState(false);
  const [sourceQuery, setSourceQuery] = useState('');
  const [sourceOpen, setSourceOpen] = useState<Source | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [confirmCancel, setConfirmCancel] = useState(false);

  const initialEvents = useTaskEvents(id);
  const { live } = useEventStream(id, {
    onEvent: (event) => {
      setLiveEvents((prev) => (prev.some((e) => e.sequence === event.sequence) ? prev : [...prev, event].slice(-400)));
    },
    onRefresh: () => { void detailQ.refresh(); },
  });

  const events = useMemo(() => {
    const map = new Map<number, EventItem>();
    (initialEvents.data?.items ?? []).forEach((event) => map.set(event.sequence, event));
    liveEvents.forEach((event) => map.set(event.sequence, event));
    return Array.from(map.values()).sort((a, b) => b.sequence - a.sequence);
  }, [initialEvents.data, liveEvents]);

  const sourcesQ = usePolling<{ items: Source[] }>(
    () => apiGet<{ items: Source[] }>(`/api/v1/research/${id}/sources?limit=200`),
    { key: `${id}-sources`, enabled: Boolean(id) && tab === 'sources' },
  );
  const claimsQ = usePolling<{ items: Claim[] }>(
    () => apiGet<{ items: Claim[] }>(`/api/v1/research/${id}/evidence?limit=200`),
    { key: `${id}-claims`, enabled: Boolean(id) && tab === 'evidence' },
  );
  const agentsQ = usePolling<{ items: AgentExecution[] }>(
    () => apiGet<{ items: AgentExecution[] }>(`/api/v1/research/${id}/agents`),
    { key: `${id}-agents`, enabled: Boolean(id) && tab === 'agents' },
  );
  const reportQ = usePolling<{ report: Report; versions?: unknown[] }>(
    () => apiGet<{ report: Report; versions?: unknown[] }>(`/api/v1/research/${id}/report`),
    { key: `${id}-report`, enabled: Boolean(id) && tab === 'report' },
  );

  useEffect(() => {
    // The canvas is the primary surface; only very wide screens keep the outline docked.
    setOutlineOpen(window.innerWidth > 1440);
  }, []);

  useEffect(() => {
    if (!nodes.length) return;
    if (!selectedId || !nodes.some((n) => n.id === selectedId)) {
      const running = nodes.find((n) => n.status === 'running') ?? nodes[0];
      setSelectedId(running.id);
    }
  }, [nodes, selectedId]);

  const selectedNode = nodes.find((n) => n.id === selectedId) ?? null;

  const counts = detail?.node_status ?? {};
  const totalNodes = Object.values(counts).reduce((sum, value) => sum + value, 0);
  const doneNodes = (counts.completed ?? 0) + (counts.failed ?? 0) + (counts.cancelled ?? 0) + (counts.skipped ?? 0);

  const elapsed = task
    ? formatDurationMs(
      new Date(task.finished_at || new Date().toISOString()).getTime() - new Date(task.created_at).getTime(),
    )
    : '—';

  const filteredEvents = useMemo(
    () => (eventFilter === 'all' ? events : events.filter((event) => eventMeta(event.event_type).category === eventFilter)),
    [events, eventFilter],
  );

  const eventCounts = useMemo(() => {
    const map: Record<string, number> = {};
    events.forEach((event) => {
      const category = eventMeta(event.event_type).category;
      map[category] = (map[category] ?? 0) + 1;
    });
    return map;
  }, [events]);

  const filteredSources = useMemo(() => {
    const sources = sourcesQ.data?.items ?? [];
    const q = sourceQuery.trim().toLowerCase();
    if (!q) return sources;
    return sources.filter((source) => `${source.title} ${source.url}`.toLowerCase().includes(q));
  }, [sourcesQ.data, sourceQuery]);

  const act = async (action: 'approve' | 'pause' | 'resume' | 'cancel' | 'plan') => {
    setBusy(action);
    try {
      await apiSend('POST', `/api/v1/research/${id}/${action}`);
      await detailQ.refresh();
      push(
        action === 'approve' ? '已批准，执行开始'
          : action === 'pause' ? '已请求暂停'
            : action === 'resume' ? '已恢复执行'
              : action === 'cancel' ? '任务已取消' : '已重新规划',
        action === 'cancel' ? 'warn' : 'ok',
      );
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusy(null);
      setConfirmCancel(false);
    }
  };

  const retryNode = async (nodeId: string) => {
    setBusy(`node-${nodeId}`);
    try {
      await apiSend('POST', `/api/v1/research/${id}/nodes/${nodeId}/retry`);
      await detailQ.refresh();
      push('节点已重新入队', 'ok');
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusy(null);
    }
  };

  const exportReport = async (format: 'md' | 'html') => {
    setBusy(`export-${format}`);
    try {
      await downloadReport(id, format);
      push(`已导出 .${format}`, 'ok');
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusy(null);
    }
  };

  if (!id) {
    return (
      <div className="page">
        <EmptyState title="缺少任务 ID" description="请从研究任务列表打开一个任务。" action={<Link href="/tasks" className="btn btn-primary btn-sm">返回任务列表</Link>} />
      </div>
    );
  }

  if (detailQ.loading && !detail) {
    return (
      <div className="page">
        <Skeleton width={110} height={12} />
        <div style={{ marginTop: 14 }}><Skeleton width="62%" height={26} /></div>
        <div style={{ marginTop: 10 }}><Skeleton width="40%" height={12} /></div>
        <div className="ws-grid" style={{ marginTop: 22 }}>
          <div className="card" style={{ height: 300 }} />
          <div className="card" style={{ height: 560 }} />
          <div className="card" style={{ height: 360 }} />
        </div>
      </div>
    );
  }

  if (detailQ.error && !detail) {
    return (
      <div className="page">
        <Link href="/tasks" className="back-link"><Icon name="arrow-left" size={13} /> 研究任务</Link>
        <ErrorState message={detailQ.error} onRetry={() => detailQ.refresh()} />
      </div>
    );
  }

  if (!detail || !task) return null;

  const cancellable = !['completed', 'cancelled'].includes(task.status);
  const canPlan = !['running', 'synthesizing'].includes(task.status);

  return (
    <div className="page">
      <Link href="/tasks" className="back-link"><Icon name="arrow-left" size={13} /> 研究任务</Link>

      <div className="ws-head">
        <div className="ws-title-row">
          <div className="ws-title-block">
            <div className="row" style={{ marginBottom: 8 }}>
              <StatusBadge status={task.status} kind="task" />
              <span className={`live-indicator ${live ? '' : 'is-off'}`}>
                <span className="status-dot" style={{ background: live ? 'var(--ok)' : 'var(--ghost)' }} />
                {live ? '实时同步' : '定时刷新'}
              </span>
              {task.plan_error && <span className="badge badge-err">规划异常</span>}
            </div>
            <h1 className="ws-title">{task.title || task.goal}</h1>
            {task.goal && task.goal !== task.title && <p className="ws-goal">{task.goal}</p>}
            <div className="ws-meta">
              <span className="mono">{task.id.slice(0, 8)}</span>
              <span className="sep">·</span>
              <span>{DEPTH_TEXT[task.depth] ?? task.depth}深度</span>
              <span className="sep">·</span>
              <span>{PLANNER_TEXT[task.planner_mode] ?? task.planner_mode}规划</span>
              <span className="sep">·</span>
              <span>并发 {task.max_agents}</span>
              <span className="sep">·</span>
              <span>{task.enable_verification ? '交叉验证开启' : '交叉验证关闭'}</span>
              <span className="sep">·</span>
              <span>创建于 {formatDateTime(task.created_at)}</span>
            </div>
          </div>

          <div className="ws-actions">
            {task.status === 'awaiting_approval' && (
              <Button variant="primary" size="sm" icon={<Icon name="play" size={13} />} loading={busy === 'approve'} onClick={() => act('approve')}>批准并执行</Button>
            )}
            {task.status === 'created' && (
              <Button variant="primary" size="sm" icon={<Icon name="play" size={13} />} loading={busy === 'plan'} onClick={() => act('plan')}>开始规划</Button>
            )}
            {(task.status === 'running' || task.status === 'synthesizing') && (
              <Button size="sm" icon={<Icon name="pause" size={13} />} loading={busy === 'pause'} onClick={() => act('pause')}>暂停</Button>
            )}
            {task.status === 'paused' && (
              <Button variant="primary" size="sm" icon={<Icon name="play" size={13} />} loading={busy === 'resume'} onClick={() => act('resume')}>继续执行</Button>
            )}
            {canPlan && task.status !== 'created' && (
              <Button size="sm" variant="ghost" icon={<Icon name="retry" size={13} />} loading={busy === 'plan'} onClick={() => act('plan')}>重新规划</Button>
            )}
            {cancellable && (
              <Button size="sm" variant="danger" icon={<Icon name="x" size={13} />} onClick={() => setConfirmCancel(true)}>取消</Button>
            )}
            <Button size="sm" variant="secondary" icon={<Icon name="download" size={13} />} loading={busy === 'export-md'} onClick={() => exportReport('md')}>导出</Button>
          </div>
        </div>

        <div className="ws-summary">
          <div className="summary-item"><span className="summary-label">节点</span><span className="summary-value">{doneNodes}/{totalNodes || num(task.stats?.nodes_total) || '—'}</span></div>
          <div className="summary-item"><span className="summary-label">来源</span><span className="summary-value">{detail.source_count}</span></div>
          <div className="summary-item"><span className="summary-label">主张</span><span className="summary-value">{num(task.stats?.claims)}</span></div>
          <div className="summary-item"><span className="summary-label">报告</span><span className="summary-value">{detail.report_summary ? `${detail.report_summary.word_count} 字` : '未生成'}</span></div>
          <div className="summary-item"><span className="summary-label">引用</span><span className="summary-value">{detail.report_summary?.citation_count ?? 0}</span></div>
          <div className="summary-item"><span className="summary-label">耗时</span><span className="summary-value">{elapsed}</span></div>
          <div className="summary-item" style={{ flex: 1, minWidth: 160, borderRight: 0 }}>
            <div style={{ width: '100%' }}>
              <Progress value={pct(doneNodes, totalNodes)} tone={task.status === 'failed' ? 'err' : doneNodes === totalNodes && totalNodes > 0 ? 'ok' : 'accent'} />
            </div>
            <span className="summary-value">{pct(doneNodes, totalNodes)}%</span>
          </div>
        </div>

        {(task.error || task.plan_error) && (
          <div className="notice notice-err" style={{ marginTop: 12 }}>
            <Icon name="alert" size={14} />
            <div>
              {task.plan_error && <div><strong>规划错误</strong>{task.plan_error}</div>}
              {task.error && <div><strong>执行错误</strong>{task.error}</div>}
            </div>
          </div>
        )}
      </div>

      <Tabs<Tab>
        className="ws-tabs"
        value={tab}
        onChange={setTab}
        ariaLabel="任务视图"
        items={[
          { value: 'workflow', label: '工作流', count: totalNodes || undefined },
          { value: 'sources', label: '来源', count: detail.source_count || undefined },
          { value: 'evidence', label: '证据', count: num(task.stats?.claims) || undefined },
          { value: 'report', label: '报告', count: detail.report_summary ? 1 : undefined },
          { value: 'agents', label: 'Agents' },
        ]}
      />

      {tab === 'workflow' && (
        <>
          <div className={cx('ws-grid', !outlineOpen && 'is-outline-hidden')}>
            <aside className="card outline-card">
              <div className="panel-head">
                <span className="panel-title">研究提纲</span>
                <span className="panel-meta">{nodes.length} 个节点</span>
              </div>
              <div className="outline-sections">
                <div className="outline-section">
                  <p className="outline-label">研究问题</p>
                  {(task.plan?.research_questions ?? []).length === 0 ? (
                    <p className="muted small">尚未生成研究问题。</p>
                  ) : (
                    (task.plan?.research_questions ?? []).map((question, index) => (
                      <div className="outline-question" key={question.id || index}>
                        <span className="qid">{question.id ?? `q${index + 1}`}</span>
                        <span>{question.question}</span>
                      </div>
                    ))
                  )}
                </div>
                <div className="outline-section">
                  <p className="outline-label">执行节点</p>
                  {nodes.length === 0 ? (
                    <p className="muted small">规划完成后显示 DAG。</p>
                  ) : nodes.map((node) => (
                    <button
                      type="button"
                      key={node.id}
                      className={cx('outline-node', selectedId === node.id && 'is-active')}
                      onClick={() => setSelectedId(node.id)}
                    >
                      <span className={cx('outline-status', `status-${node.status}`)} aria-hidden="true" />
                      <span className="outline-node-title">{node.title}</span>
                      <span className="outline-node-type">{NODE_TYPE_TEXT[node.type] ?? node.type}</span>
                    </button>
                  ))}
                </div>
              </div>
            </aside>

            <section className="card canvas-card">
              <div className="canvas-toolbar">
                <div className="canvas-tools">
                  <span className="panel-title">执行图</span>
                  <span className="panel-meta">自上而下为依赖顺序</span>
                  <button
                    type="button"
                    className={cx('canvas-toggle', outlineOpen && 'is-on')}
                    aria-pressed={outlineOpen}
                    onClick={() => setOutlineOpen((prev) => !prev)}
                  >
                    <Icon name="layers" size={12} />
                    提纲
                  </button>
                </div>
                <div className="canvas-legend">
                  <span><i className="status-dot status-ok" />完成</span>
                  <span><i className="status-dot status-accent" />运行</span>
                  <span><i className="status-dot status-err" />失败</span>
                  <span><i className="status-dot" />待执行</span>
                </div>
              </div>
              <div className="canvas-wrap">
                {nodes.length === 0 ? (
                  <EmptyState
                    title={task.status === 'created' ? '等待规划' : '暂无执行节点'}
                    description={task.status === 'created' ? '点击「开始规划」生成研究提纲与 DAG。' : '节点会在规划完成后出现。'}
                  />
                ) : (
                  <>
                    <WorkflowCanvas nodes={nodes} selectedId={selectedId} onSelect={(node) => setSelectedId(node.id)} />
                    <span className="canvas-hint">点击节点查看输入与输出 · 拖拽平移 / 滚轮缩放</span>
                  </>
                )}
              </div>
            </section>

            <aside className="card inspector-card">
              {selectedNode ? (
                <>
                  <div className="inspector-head">
                    <div className="inspector-eyebrow">
                      <span className="badge badge-neutral">{NODE_TYPE_TEXT[selectedNode.type] ?? selectedNode.type}</span>
                      <StatusBadge status={selectedNode.status} kind="node" />
                    </div>
                    <p className="inspector-title">{selectedNode.title}</p>
                  </div>

                  <div className="inspector-section">
                    <p className="inspector-label">节点信息</p>
                    <div className="meta-row"><span className="meta-label">Agent 角色</span><span className="meta-value">{selectedNode.agent_role || '—'}</span></div>
                    <div className="meta-row"><span className="meta-label">Agent 名称</span><span className="meta-value">{selectedNode.agent_name || '—'}</span></div>
                    <div className="meta-row"><span className="meta-label">尝试次数</span><span className="meta-value">{selectedNode.attempts} / {selectedNode.max_attempts}</span></div>
                    <div className="meta-row"><span className="meta-label">进度</span><span className="meta-value">{selectedNode.progress}%</span></div>
                    <div className="meta-row"><span className="meta-label">Token</span><span className="meta-value">{selectedNode.tokens_total || '—'}</span></div>
                    <div className="meta-row"><span className="meta-label">开始</span><span className="meta-value">{selectedNode.started_at ? formatDateTime(selectedNode.started_at) : '—'}</span></div>
                    <div className="meta-row"><span className="meta-label">完成</span><span className="meta-value">{selectedNode.completed_at ? formatDateTime(selectedNode.completed_at) : '—'}</span></div>
                  </div>

                  {(selectedNode.depends_on ?? []).length > 0 && (
                    <div className="inspector-section">
                      <p className="inspector-label">前置依赖</p>
                      <div className="row" style={{ flexWrap: 'wrap', gap: 4 }}>
                        {(selectedNode.depends_on ?? []).map((dep) => (
                          <button key={dep} type="button" className="chip" onClick={() => setSelectedId(dep)}>{dep}</button>
                        ))}
                      </div>
                    </div>
                  )}

                  <div className="inspector-section">
                    <p className="inspector-label">输入</p>
                    {(() => {
                      const query = inputValue(selectedNode, 'query');
                      const question = inputValue(selectedNode, 'question');
                      const fallback = inputValue(selectedNode, 'fallback_query');
                      return (
                        <>
                          {typeof query === 'string' && query && <div className="meta-row"><span className="meta-label">检索词</span><span className="meta-value">{query}</span></div>}
                          {typeof question === 'string' && question && <div className="meta-row"><span className="meta-label">问题</span><span className="meta-value">{question}</span></div>}
                          {typeof fallback === 'string' && fallback && <div className="meta-row"><span className="meta-label">回退检索</span><span className="meta-value">{fallback}</span></div>}
                          <pre className="inspector-output" style={{ marginTop: 8 }}>{JSON.stringify(selectedNode.input ?? {}, null, 2)}</pre>
                        </>
                      );
                    })()}
                  </div>

                  <div className="inspector-section">
                    <p className="inspector-label">输出摘要</p>
                    <div className="meta-row"><span className="meta-label">结果数</span><span className="meta-value">{String(outputValue(selectedNode, 'result_count') ?? '—')}</span></div>
                    <div className="meta-row"><span className="meta-label">抓取成功</span><span className="meta-value">{String(outputValue(selectedNode, 'fetched') ?? '—')}</span></div>
                    <div className="meta-row"><span className="meta-label">抓取失败</span><span className="meta-value">{String(outputValue(selectedNode, 'failed') ?? '—')}</span></div>
                    <div className="meta-row"><span className="meta-label">Provider</span><span className="meta-value">{Array.isArray(outputValue(selectedNode, 'providers')) ? (outputValue(selectedNode, 'providers') as string[]).join(' · ') : '—'}</span></div>
                    <pre className="inspector-output" style={{ marginTop: 8 }}>{JSON.stringify(selectedNode.output ?? {}, null, 2)}</pre>
                  </div>

                  {selectedNode.error && (
                    <div className="inspector-section">
                      <p className="inspector-label">错误</p>
                      <div className="inspector-error">{selectedNode.error}</div>
                    </div>
                  )}

                  {['failed', 'cancelled', 'skipped'].includes(selectedNode.status) && (
                    <div className="inspector-section">
                      <Button
                        size="sm"
                        variant="secondary"
                        icon={<Icon name="retry" size={13} />}
                        loading={busy === `node-${selectedNode.id}`}
                        onClick={() => retryNode(selectedNode.id)}
                      >
                        重试该节点
                      </Button>
                    </div>
                  )}
                </>
              ) : (
                <div className="inspector-empty">选择左侧提纲或图中的节点<br />查看输入、输出与执行状态</div>
              )}
            </aside>
          </div>

          <section className="card timeline-card">
            <div className="timeline-head">
              <div className="timeline-title-row">
                <span className="timeline-title">事件时间线</span>
                <span className="live-indicator" style={{ color: live ? 'var(--ok)' : 'var(--faint)' }}>
                  <span className="status-dot" style={{ background: live ? 'var(--ok)' : 'var(--ghost)' }} />
                  {live ? '实时' : '定时'}
                </span>
                <span className="inline-stat">{events.length} 条</span>
              </div>
              <div className="timeline-filters">
                {EVENT_FILTERS.map((filter) => {
                  const count = filter.value === 'all' ? events.length : eventCounts[filter.value] ?? 0;
                  if (filter.value !== 'all' && count === 0) return null;
                  return (
                    <button
                      key={filter.value}
                      type="button"
                      className={cx('filter-chip', eventFilter === filter.value && 'is-active')}
                      onClick={() => setEventFilter(filter.value)}
                    >
                      {filter.label}<span className="count">{count}</span>
                    </button>
                  );
                })}
              </div>
            </div>
            {initialEvents.loading && events.length === 0 ? (
              <div className="card-body"><Skeleton height={83} /></div>
            ) : filteredEvents.length === 0 ? (
              <EmptyState compact title="暂无事件" description={initialEvents.error ? `事件流不可用：${initialEvents.error}` : '任务开始执行后，事件会实时出现在这里。'} />
            ) : (
              <div className="timeline-list">
                {filteredEvents.slice(0, 60).map((event) => {
                  const meta = eventMeta(event.event_type);
                  return (
                    <div className="timeline-item" key={event.event_id}>
                      <span className="timeline-time">{formatRelative(event.timestamp)}</span>
                      <span className="timeline-marker"><i className={cx('dot', `tone-${meta.tone}`)} /></span>
                      <span className="timeline-body">
                        <span className="timeline-event-label">{meta.label}</span>
                        {event.node_id && <span className="timeline-event-node">{event.node_id}</span>}
                        {event.event_type && <span className="timeline-event-detail">{event.event_type}</span>}
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
          </section>
        </>
      )}

      {tab === 'sources' && (
        <section className="card">
          <div className="section-head">
            <div className="section-head-title">
              <h2>研究来源</h2>
              <span className="section-hint">{sourcesQ.data?.items.length ?? detail.source_count} 条 · 逐一可追溯</span>
            </div>
            <div className="search-input" style={{ maxWidth: 260 }}>
              <Icon name="search" size={14} />
              <input className="input" value={sourceQuery} onChange={(e) => setSourceQuery(e.target.value)} placeholder="搜索标题或链接" aria-label="搜索来源" />
            </div>
          </div>
          {sourcesQ.loading ? (
            <div className="card-body"><div className="skeleton-block">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} height={30} />)}</div></div>
          ) : sourcesQ.error ? (
            <div style={{ padding: 18 }}><ErrorState message={sourcesQ.error} onRetry={() => sourcesQ.refresh()} /></div>
          ) : filteredSources.length === 0 ? (
            <EmptyState title={sourceQuery ? '没有匹配的来源' : '尚无来源'} description={sourceQuery ? '尝试更短的关键词。' : '检索节点完成后，抓取到的资料会出现在这里。'} />
          ) : (
            <SourceTable sources={filteredSources} onSelect={setSourceOpen} />
          )}
        </section>
      )}

      {tab === 'evidence' && (
        <section className="card">
          <div className="section-head">
            <div className="section-head-title">
              <h2>证据与主张</h2>
              <span className="section-hint">按站点独立性与数值一致性判定支持度</span>
            </div>
            <EvidenceSummary claims={claimsQ.data?.items ?? []} />
          </div>
          {claimsQ.loading ? (
            <div className="card-body"><div className="skeleton-block">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} height={54} />)}</div></div>
          ) : claimsQ.error ? (
            <div style={{ padding: 18 }}><ErrorState message={claimsQ.error} onRetry={() => claimsQ.refresh()} /></div>
          ) : (claimsQ.data?.items ?? []).length === 0 ? (
            <EmptyState title="尚无证据" description="分析节点会从来源正文中抽取主张，并附带可回溯的引文。" />
          ) : (
            <EvidenceList claims={claimsQ.data?.items ?? []} />
          )}
        </section>
      )}

      {tab === 'report' && (
        <section className="card" style={{ overflow: 'hidden' }}>
          {reportQ.loading ? (
            <div className="reader"><div className="skeleton-block">{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} height={i === 0 ? 28 : 14} width={i === 0 ? '55%' : '100%'} />)}</div></div>
          ) : reportQ.data?.report ? (
            <>
              <ReportReader
                report={reportQ.data.report}
                taskTitle={task.title}
                actions={
                  <>
                    <Button size="sm" variant="secondary" icon={<Icon name="download" size={13} />} loading={busy === 'export-md'} onClick={() => exportReport('md')}>Markdown</Button>
                    <Button size="sm" variant="secondary" icon={<Icon name="download" size={13} />} loading={busy === 'export-html'} onClick={() => exportReport('html')}>HTML</Button>
                    <Link href={`/reports?id=${task.id}`} className="btn btn-quiet btn-sm">在报告库中打开 <Icon name="arrow-right" size={13} /></Link>
                  </>
                }
              />
            </>
          ) : task.status === 'completed' ? (
            <div style={{ padding: 18 }}><ErrorState message={reportQ.error ?? '报告尚未生成'} onRetry={() => reportQ.refresh()} /></div>
          ) : (
            <EmptyState
              title="报告尚未生成"
              description="任务完成后会自动生成带引用的研究报告；也可在报告库中查看已完成的报告。"
              action={<Link href="/reports" className="btn btn-secondary btn-sm">前往报告库</Link>}
            />
          )}
        </section>
      )}

      {tab === 'agents' && (
        <section className="card">
          <div className="section-head">
            <div className="section-head-title">
              <h2>Agent 执行记录</h2>
              <span className="section-hint">每个节点的执行者、耗时与 Token 使用</span>
            </div>
          </div>
          {agentsQ.loading ? (
            <div className="card-body"><div className="skeleton-block">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} height={30} />)}</div></div>
          ) : agentsQ.error ? (
            <div style={{ padding: 18 }}><ErrorState message={agentsQ.error} onRetry={() => agentsQ.refresh()} /></div>
          ) : (agentsQ.data?.items ?? []).length === 0 ? (
            <EmptyState title="尚无 Agent 记录" description="执行开始后，节点会分配给对应角色并记录耗时。" />
          ) : (
            <AgentTable agents={agentsQ.data?.items ?? []} />
          )}
        </section>
      )}

      {detailQ.error && (
        <div style={{ marginTop: 14 }}>
          <ErrorState message={`${detailQ.error}（显示上次成功加载的数据）`} onRetry={() => detailQ.refresh()} />
        </div>
      )}

      {sourceOpen && (
        <div className="drawer-layer" role="dialog" aria-modal="true" aria-label="来源详情" onClick={() => setSourceOpen(null)}>
          <div className="drawer-panel" onClick={(e) => e.stopPropagation()}>
            <div className="drawer-head">
              <span className="drawer-title">来源详情</span>
              <button type="button" className="icon-btn" aria-label="关闭" onClick={() => setSourceOpen(null)}><Icon name="x" size={15} /></button>
            </div>
            <div className="drawer-body"><SourceDetail source={sourceOpen} /></div>
            <div className="drawer-foot">
              <a href={sourceOpen.url} target="_blank" rel="noopener noreferrer" className="btn btn-secondary btn-sm">打开原始页面 <Icon name="external" size={12} /></a>
            </div>
          </div>
        </div>
      )}

      <Modal
        open={confirmCancel}
        onClose={() => setConfirmCancel(false)}
        title="取消这次研究？"
        description="已完成的节点与证据会保留，执行中的节点会停止。该操作不可撤销。"
        footer={
          <div className="modal-actions">
            <Button variant="secondary" size="sm" onClick={() => setConfirmCancel(false)}>返回</Button>
            <Button variant="danger" size="sm" loading={busy === 'cancel'} onClick={() => act('cancel')}>确认取消</Button>
          </div>
        }
      >
        <div className="notice">
          <Icon name="info" size={14} />
          <span>任务状态将变为「已取消」，之后可以通过「重新规划」创建新的执行计划。</span>
        </div>
      </Modal>
    </div>
  );
}

export default function TaskPage() {
  return (
    <Suspense fallback={<div className="page"><Skeleton height={260} /></div>}>
      <TaskWorkspace />
    </Suspense>
  );
}