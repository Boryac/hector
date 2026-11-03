'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { WorkflowCanvas } from '@/components/WorkflowCanvas';
import { Markdown } from '@/components/Markdown';
import { apiGet, apiSend, ApiError, downloadReport, eventsURL, issueEventTicket } from '@/lib/api';
import { eventMeta, statusText } from '@/lib/event-map';
import type {
  AgentExecution, Claim, EventItem, Node, Report, Source, TaskDetail,
} from '@/lib/types';

type Tab = 'timeline' | 'sources' | 'evidence' | 'agents' | 'report';

function TaskView() {
  const params = useSearchParams();
  const id = params.get('id') ?? '';
  const [detail, setDetail] = useState<TaskDetail | null>(null);
  const [events, setEvents] = useState<EventItem[]>([]);
  const [tab, setTab] = useState<Tab>('timeline');
  const [sources, setSources] = useState<Source[]>([]);
  const [claims, setClaims] = useState<Claim[]>([]);
  const [agents, setAgents] = useState<AgentExecution[]>([]);
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [live, setLive] = useState(false);
  const lastSeq = useRef(0);

  const loadDetail = useCallback(async () => {
    if (!id) return;
    try {
      const d = await apiGet<TaskDetail>(`/api/v1/research/${id}`);
      setDetail(d);
      setError('');
    } catch (e) {
      setError(e instanceof ApiError ? `${e.message}（${e.code}）` : String(e));
    }
  }, [id]);

  useEffect(() => {
    loadDetail();
    const t = setInterval(loadDetail, 2500);
    return () => clearInterval(t);
  }, [loadDetail]);

  // SSE 实时事件流：断线自动重连，并按 sequence 续接。
  useEffect(() => {
    if (!id) return;
    let es: EventSource | null = null;
    let stopped = false;
    let retry: ReturnType<typeof setTimeout> | null = null;

    const connect = async () => {
      if (stopped) return;
      // 先用 Bearer 换一次性 ticket：EventSource 无法带请求头，
      // 但绝不能让长期 API Token 出现在 URL 里。
      let ticket: string;
      try {
        ticket = (await issueEventTicket(id)).ticket;
      } catch {
        setLive(false);
        if (!stopped) retry = setTimeout(connect, 2500);
        return;
      }
      if (stopped) return;
      es = new EventSource(eventsURL(id, lastSeq.current, ticket));
      es.onopen = () => setLive(true);
      es.onerror = () => {
        setLive(false);
        es?.close();
        if (!stopped) retry = setTimeout(connect, 2500);
      };
      es.onmessage = (ev) => consume(ev.data);
      const types = [
        'research.created', 'research.planning', 'research.planned', 'research.started',
        'research.paused', 'research.resumed', 'research.completed', 'research.failed',
        'workflow.created', 'workflow.updated', 'node.queued', 'node.started', 'node.progress',
        'node.completed', 'node.failed', 'node.retrying', 'agent.created', 'agent.started',
        'agent.completed', 'agent.failed', 'tool.invoked', 'tool.completed', 'tool.failed',
        'evidence.created', 'evidence.verified', 'report.started', 'report.completed',
      ];
      types.forEach((t) => es!.addEventListener(t, (ev) => consume((ev as MessageEvent).data)));
      es.addEventListener('stream.end', () => {
        setLive(false);
        es?.close();
      });
    };

    const consume = (raw: string) => {
      try {
        const ev = JSON.parse(raw) as EventItem;
        if (!ev?.sequence) return;
        lastSeq.current = Math.max(lastSeq.current, ev.sequence);
        setEvents((prev) => (prev.some((x) => x.event_id === ev.event_id) ? prev : [...prev, ev].slice(-400)));
      } catch {
        /* 忽略心跳与异常帧 */
      }
    };

    void connect();
    return () => {
      stopped = true;
      if (retry) clearTimeout(retry);
      es?.close();
    };
  }, [id]);

  // 首次进入时用 JSON 模式补齐历史事件（SSE 只推增量）。
  useEffect(() => {
    if (!id) return;
    apiGet<{ items: EventItem[] }>(`/api/v1/research/${id}/events?format=json&after_sequence=0`)
      .then((r) => {
        const items = r.items ?? [];
        setEvents(items);
        lastSeq.current = items.length ? items[items.length - 1].sequence : 0;
      })
      .catch(() => undefined);
  }, [id]);

  const loadTab = useCallback(async () => {
    if (!id) return;
    try {
      if (tab === 'sources' && sources.length === 0) {
        const r = await apiGet<{ items: Source[] }>(`/api/v1/research/${id}/sources?limit=200`);
        setSources(r.items ?? []);
      }
      if (tab === 'evidence' && claims.length === 0) {
        const r = await apiGet<{ items: Claim[] }>(`/api/v1/research/${id}/evidence?limit=200`);
        setClaims(r.items ?? []);
      }
      if (tab === 'agents' && agents.length === 0) {
        const r = await apiGet<{ items: AgentExecution[] }>(`/api/v1/research/${id}/agents`);
        setAgents(r.items ?? []);
      }
      if (tab === 'report' && !report) {
        const r = await apiGet<{ report: Report }>(`/api/v1/research/${id}/report`);
        setReport(r.report);
      }
    } catch {
      /* 报告/证据可能尚未生成，静默处理 */
    }
  }, [id, tab, sources.length, claims.length, agents.length, report]);

  useEffect(() => { loadTab(); }, [loadTab]);

  const act = async (action: 'approve' | 'pause' | 'resume' | 'cancel' | 'plan') => {
    if (!id) return;
    setBusy(true);
    try {
      await apiSend('POST', `/api/v1/research/${id}/${action}`);
      setReport(null);
      setSources([]);
      setClaims([]);
      await loadDetail();
    } catch (e) {
      setError(e instanceof ApiError ? `${e.message}（${e.code}）` : String(e));
    } finally {
      setBusy(false);
    }
  };

  const retryNode = async (nodeId: string) => {
    if (!id) return;
    setBusy(true);
    try {
      await apiSend('POST', `/api/v1/research/${id}/nodes/${nodeId}/retry`);
      await loadDetail();
    } catch (e) {
      setError(e instanceof ApiError ? `${e.message}（${e.code}）` : String(e));
    } finally {
      setBusy(false);
    }
  };

  const nodes: Node[] = useMemo(() => detail?.nodes ?? [], [detail]);
  const task = detail?.task;
  const counts = detail?.node_status ?? {};
  const totalNodes = Object.values(counts).reduce((a, b) => a + b, 0);
  const doneNodes = (counts.completed ?? 0) + (counts.failed ?? 0) + (counts.cancelled ?? 0);
  const pct = totalNodes > 0 ? Math.round((doneNodes / totalNodes) * 100) : 0;

  if (!id) return <div className="empty">缺少任务 ID</div>;

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <span className="dot" />
          <Link href="/">Hector ResearchOS</Link>
          <small>/ 任务详情</small>
        </div>
        <div className="spacer" />
        <span className={`pill ${live ? 'ok' : 'warn'}`}>
          <span className="led" />
          {live ? 'SSE 实时' : '轮询兜底'}
        </span>
        {task && <span className={`badge ${task.status}`}>{statusText(task.status)}</span>}
      </header>

      <div className="layout" style={{ gridTemplateColumns: '1fr 420px' }}>
        <div>
          <div className="card">
            <h2>研究目标</h2>
            <div style={{ fontSize: 15, fontWeight: 600, lineHeight: 1.5 }}>{task?.title || task?.goal || '加载中…'}</div>
            {task && (
              <div className="meta" style={{ marginTop: 8 }}>
                <span className="mono">{task.id.slice(0, 8)}</span>
                <span>深度 {task.depth}</span>
                <span>规划 {task.planner_mode}</span>
                <span>并发 {task.max_agents}</span>
                <span>资料 {detail?.source_count ?? 0}</span>
                <span>节点 {doneNodes}/{totalNodes}</span>
              </div>
            )}
            {task && task.status !== 'completed' && totalNodes > 0 && (
              <div className="progress"><i style={{ width: `${pct}%` }} /></div>
            )}
            {task?.error && <div className="err-text" style={{ marginTop: 10 }}>{task.error}</div>}
            {task?.plan_error && <div className="err-text" style={{ marginTop: 6 }}>规划错误：{task.plan_error}</div>}
            <div className="actions">
              {task?.status === 'awaiting_approval' && (
                <button className="primary" disabled={busy} onClick={() => act('approve')}>批准并执行</button>
              )}
              {(task?.status === 'running' || task?.status === 'synthesizing') && (
                <button disabled={busy} onClick={() => act('pause')}>暂停</button>
              )}
              {task?.status === 'paused' && (
                <button className="primary" disabled={busy} onClick={() => act('resume')}>恢复</button>
              )}
              {task && !['completed', 'cancelled'].includes(task.status) && (
                <button className="danger" disabled={busy} onClick={() => act('cancel')}>取消</button>
              )}
              {task && !['running', 'synthesizing'].includes(task.status) && (
                <button className="ghost" disabled={busy} onClick={() => act('plan')}>重新规划</button>
              )}
              {task && (
                <>
                  <button
                    className="ghost"
                    disabled={busy}
                    onClick={() => downloadReport(task.id, 'md').catch((e) => setError(String(e.message ?? e)))}
                  >
                    导出 .md
                  </button>
                  <button
                    className="ghost"
                    disabled={busy}
                    onClick={() => downloadReport(task.id, 'html').catch((e) => setError(String(e.message ?? e)))}
                  >
                    导出 .html
                  </button>
                </>
              )}
            </div>
            {error && <div className="err-text" style={{ marginTop: 10 }}>{error}</div>}
          </div>

          <div className="card">
            <h2>工作流 DAG</h2>
            <WorkflowCanvas nodes={nodes} />
            <div className="list" style={{ marginTop: 12 }}>
              {nodes.map((n) => (
                <div key={n.id} className="list-item" style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                  <span className={`badge ${n.status}`}>{statusText(n.status)}</span>
                  <div style={{ flex: 1 }}>
                    <div className="t">{n.title}</div>
                    <div className="d">
                      {n.type} · {n.agent_role} · 尝试 {n.attempts}/{n.max_attempts}
                      {n.tokens_total ? ` · ${n.tokens_total} tokens` : ''}
                      {n.error ? ` · ${n.error}` : ''}
                    </div>
                  </div>
                  {(n.status === 'failed' || n.status === 'cancelled') && (
                    <button className="ghost" disabled={busy} onClick={() => retryNode(n.id)}>重试</button>
                  )}
                </div>
              ))}
              {nodes.length === 0 && <div className="empty">规划完成后显示节点</div>}
            </div>
          </div>
        </div>

        <div>
          <div className="card">
            <div className="tabs">
              {([
                ['timeline', `事件流 ${events.length}`],
                ['sources', `资料 ${detail?.source_count ?? sources.length}`],
                ['evidence', `证据 ${claims.length}`],
                ['agents', `Agent ${agents.length}`],
                ['report', '报告'],
              ] as [Tab, string][]).map(([key, label]) => (
                <div key={key} className={`tab ${tab === key ? 'active' : ''}`} onClick={() => setTab(key)}>
                  {label}
                </div>
              ))}
            </div>

            {tab === 'timeline' && (
              <div className="feed">
                {events.length === 0 && <div className="empty">等待事件…</div>}
                {events.slice().reverse().map((ev) => {
                  const meta = eventMeta(ev.event_type);
                  return (
                    <div key={ev.event_id} className="feed-row">
                      <span className="seq">#{ev.sequence}</span>
                      <span className="type" data-tone={meta.tone}>{meta.label}</span>
                      <span className="payload">
                        {ev.node_id ? <span className="muted">{ev.node_id} · </span> : null}
                        {summarize(ev.payload)}
                      </span>
                    </div>
                  );
                })}
              </div>
            )}

            {tab === 'sources' && (
              <div className="list">
                {sources.length === 0 && <div className="empty">尚无资料</div>}
                {sources.map((s) => (
                  <div key={s.id} className="list-item">
                    <div className="t">
                      <a href={s.url} target="_blank" rel="noreferrer">{s.title || s.url}</a>
                    </div>
                    <div className="d">
                      <span className="badge">{s.provider}</span> {s.site} · HTTP {s.http_status} · 可信度 {s.credibility} · 状态 {s.status}
                    </div>
                    {(s.excerpt || s.snippet) && (
                      <div className="small muted" style={{ marginTop: 4 }}>
                        {(s.excerpt || s.snippet).slice(0, 220)}…
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}

            {tab === 'evidence' && (
              <div className="list">
                {claims.length === 0 && <div className="empty">尚无证据</div>}
                {claims.map((c) => (
                  <div key={c.id} className="list-item">
                    <div className="t">{c.statement}</div>
                    <div className="d">
                      <span className={`badge ${c.status === 'supported' ? 'completed' : c.status === 'conflicting' ? 'failed' : ''}`}>
                        {c.status}
                      </span>{' '}
                      置信度 {c.confidence.toFixed(2)} · 来源 {c.source_count}
                      {c.verifier_notes ? ` · ${c.verifier_notes}` : ''}
                    </div>
                    {(c.evidence ?? []).map((e) => (
                      <div key={e.id} className="small muted" style={{ marginTop: 4, paddingLeft: 10, borderLeft: '2px solid #26304d' }}>
                        「{e.quote.slice(0, 160)}」
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            )}

            {tab === 'agents' && (
              <div className="list">
                {agents.length === 0 && <div className="empty">尚无 Agent 执行记录</div>}
                {agents.map((a) => (
                  <div key={a.id} className="list-item">
                    <div className="t">{a.name || a.role}</div>
                    <div className="d">
                      {a.role} · {a.status} · {a.duration_ms}ms
                      {a.model ? ` · ${a.model}` : ''}
                      {a.token_usage?.total_tokens ? ` · ${a.token_usage.total_tokens} tokens` : ''}
                      {a.node_id ? ` · ${a.node_id}` : ''}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {tab === 'report' && (
              <>
                {!report && <div className="empty">报告尚未生成</div>}
                {report && (
                  <>
                    <div className="meta" style={{ marginBottom: 10 }}>
                      <span className="badge">v{report.version}</span>
                      <span>{report.citation_count} 条引用</span>
                      <span>{report.word_count} 字</span>
                      <span className="muted">{report.model}</span>
                    </div>
                    <Markdown source={report.markdown} />
                  </>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function summarize(payload: Record<string, unknown>): string {
  if (!payload) return '';
  const keys = ['query', 'title', 'progress_text', 'error', 'statement', 'status', 'count', 'url', 'tool', 'citation_count'];
  for (const k of keys) {
    const v = payload[k];
    if (v !== undefined && v !== null && v !== '') {
      const text = typeof v === 'object' ? JSON.stringify(v) : String(v);
      return text.length > 160 ? `${text.slice(0, 160)}…` : text;
    }
  }
  const raw = JSON.stringify(payload);
  return raw.length > 160 ? `${raw.slice(0, 160)}…` : raw;
}

export default function TaskPage() {
  return (
    <Suspense fallback={<div className="empty">加载中…</div>}>
      <TaskView />
    </Suspense>
  );
}
