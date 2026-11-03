'use client';

import React, { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { apiGet } from '@/lib/api';
import type { Health, SettingsSnapshot, TaskDetail } from '@/lib/types';
import {
  cx, errorText, formatDateTime, formatGoDuration, formatRelative, num, pct, truncate,
  DEPTH_TEXT, taskStatusText,
} from '@/lib/format';
import { usePolling, useTaskCatalog, useTaskEvents } from '@/lib/hooks';
import { eventMeta } from '@/lib/event-map';
import { Icon } from '@/components/Icon';
import { Button, EmptyState, ErrorState, PageHeader, SectionHead, Skeleton, StatusBadge } from '@/components/ui';
import { useToast } from '@/components/Toast';
import { useAppActions } from '@/components/AppShell';

const DAY_MS = 86_400_000;

// GET /api/v1/stats 的响应（服务端 SQL 全量计数）
interface ServerStats {
  tasks: { by_status: Record<string, number>; total: number };
  nodes: { by_status: Record<string, number>; total: number };
  agents: { by_role: Record<string, number> };
  sources: number;
  claims: number;
  reports: number;
}

interface DayBucket {
  key: string;
  label: string;
  created: number;
  completed: number;
}

function buildBuckets(tasks: Array<{ created_at: string; finished_at?: string; status: string }>): DayBucket[] {
  const now = new Date();
  const buckets: DayBucket[] = [];
  for (let i = 6; i >= 0; i -= 1) {
    const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() - i);
    buckets.push({ key: d.toDateString(), label: `${d.getMonth() + 1}/${d.getDate()}`, created: 0, completed: 0 });
  }
  const index = new Map(buckets.map((b, i) => [b.key, i]));
  tasks.forEach((task) => {
    const created = new Date(task.created_at);
    if (!Number.isNaN(created.getTime())) {
      const hit = index.get(new Date(created.getFullYear(), created.getMonth(), created.getDate()).toDateString());
      if (hit !== undefined) buckets[hit].created += 1;
    }
    if (task.status === 'completed') {
      const finished = task.finished_at ? new Date(task.finished_at) : null;
      if (finished && !Number.isNaN(finished.getTime())) {
        const hit = index.get(new Date(finished.getFullYear(), finished.getMonth(), finished.getDate()).toDateString());
        if (hit !== undefined) buckets[hit].completed += 1;
      }
    }
  });
  return buckets;
}

function ActivityChart({ buckets }: { buckets: DayBucket[] }) {
  const W = 720;
  const H = 188;
  const PAD_L = 26;
  const PAD_R = 8;
  const PAD_T = 10;
  const PAD_B = 24;
  const plotW = W - PAD_L - PAD_R;
  const plotH = H - PAD_T - PAD_B;
  const max = Math.max(1, ...buckets.map((b) => Math.max(b.created, b.completed)));
  const step = plotW / buckets.length;
  const barW = Math.min(15, step * 0.28);
  const gap = 4;
  const ticks = [0, 0.5, 1].map((r) => Math.round(max * r));

  return (
    <svg className="chart-svg" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="近 7 天研究任务创建与完成数量">
      {[0, 0.5, 1].map((r, i) => {
        const y = PAD_T + plotH * (1 - r);
        return (
          <g key={i}>
            <line className="chart-grid-line" x1={PAD_L} y1={y} x2={W - PAD_R} y2={y} />
            <text className="chart-label" x={PAD_L - 8} y={y + 3} textAnchor="end">{ticks[i]}</text>
          </g>
        );
      })}
      {buckets.map((b, i) => {
        const cx0 = PAD_L + step * i + step / 2;
        const hCreated = (b.created / max) * plotH;
        const hDone = (b.completed / max) * plotH;
        return (
          <g key={b.key}>
            <rect
              className="chart-bar-created"
              x={cx0 - barW - gap / 2}
              y={PAD_T + plotH - hCreated}
              width={barW}
              height={Math.max(hCreated, b.created ? 2 : 0)}
              rx={2}
              style={{ animationDelay: `${i * 28}ms` }}
            >
              <title>{b.label} 创建 {b.created} 项</title>
            </rect>
            <rect
              className="chart-bar-completed"
              x={cx0 + gap / 2}
              y={PAD_T + plotH - hDone}
              width={barW}
              height={Math.max(hDone, b.completed ? 2 : 0)}
              rx={2}
              style={{ animationDelay: `${i * 28 + 60}ms` }}
            >
              <title>{b.label} 完成 {b.completed} 项</title>
            </rect>
            <text className="chart-label" x={cx0} y={H - 6} textAnchor="middle">{b.label}</text>
          </g>
        );
      })}
    </svg>
  );
}

function MetricCard({
  label, value, suffix, foot, tone,
}: {
  label: string;
  value: React.ReactNode;
  suffix?: string;
  foot: React.ReactNode;
  tone?: 'ok' | 'warn' | 'err';
}) {
  return (
    <div className="metric">
      <div className="metric-label">
        <span>{label}</span>
        {tone && <span className={cx('status-dot', `status-${tone}`)} aria-hidden="true" />}
      </div>
      <div className="metric-value">{value}{suffix && <small>{suffix}</small>}</div>
      <div className="metric-foot">{foot}</div>
    </div>
  );
}

export default function DashboardPage() {
  const router = useRouter();
  const { push } = useToast();
  const { openNewResearch } = useAppActions();
  const catalog = useTaskCatalog(100, 12);
  const health = usePolling<Health>(() => apiGet<Health>('/api/v1/health'), { intervalMs: 20000, key: 'dash-health' });
  const settings = usePolling<SettingsSnapshot>(() => apiGet<SettingsSnapshot>('/api/v1/settings'), { key: 'dash-settings' });
  // 服务端 SQL 全量计数：任务列表接口最多拉 100 条，客户端累加在任务多于 100 时会失真。
  const serverStats = usePolling<ServerStats>(() => apiGet<ServerStats>('/api/v1/stats'), { intervalMs: 30000, key: 'dash-stats' });
  const [refreshing, setRefreshing] = useState(false);
  const [lastSync, setLastSync] = useState<string | null>(null);

  const tasks = catalog.tasks;
  const details = catalog.details;

  const stats = useMemo(() => {
    const byStatus = serverStats.data?.tasks?.by_status;
    const sum = (keys: string[]) => keys.reduce((s, k) => s + (byStatus?.[k] ?? 0), 0);
    const weekAgo = Date.now() - 7 * DAY_MS;
    return {
      // 有服务端计数就优先用（全量），否则退回列表推算
      total: serverStats.data?.tasks?.total ?? catalog.total,
      running: byStatus ? sum(['planning', 'running', 'synthesizing']) : tasks.filter((t) => ['planning', 'running', 'synthesizing'].includes(t.status)).length,
      awaiting: byStatus ? (byStatus.awaiting_approval ?? 0) : tasks.filter((t) => t.status === 'awaiting_approval').length,
      paused: byStatus ? (byStatus.paused ?? 0) : tasks.filter((t) => t.status === 'paused').length,
      failed: byStatus ? (byStatus.failed ?? 0) : tasks.filter((t) => t.status === 'failed').length,
      completed: byStatus ? (byStatus.completed ?? 0) : tasks.filter((t) => t.status === 'completed').length,
      sources: serverStats.data?.sources ?? tasks.reduce((sum, t) => sum + num(t.stats?.sources), 0),
      claims: serverStats.data?.claims ?? tasks.reduce((sum, t) => sum + num(t.stats?.claims), 0),
      recent: tasks.filter((t) => new Date(t.created_at).getTime() >= weekAgo).length,
    };
  }, [tasks, catalog.total, serverStats.data]);

  const buckets = useMemo(() => buildBuckets(tasks), [tasks]);

  const weekCreated = buckets.reduce((s, b) => s + b.created, 0);
  const weekCompleted = buckets.reduce((s, b) => s + b.completed, 0);

  const reports = useMemo(() => {
    return Object.values(details)
      .filter((d): d is TaskDetail & { report_summary: NonNullable<TaskDetail['report_summary']> } => Boolean(d.report_summary))
      .sort((a, b) => new Date(b.report_summary.created_at).getTime() - new Date(a.report_summary.created_at).getTime())
      .slice(0, 5);
  }, [details]);

  const attention = useMemo(() => {
    const priority: Record<string, number> = { failed: 0, awaiting_approval: 1, paused: 2 };
    return tasks
      .filter((t) => t.status in priority)
      .sort((a, b) => (priority[a.status] ?? 9) - (priority[b.status] ?? 9) || new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
      .slice(0, 4);
  }, [tasks]);

  const latestTask = tasks[0];
  const eventsQ = useTaskEvents(latestTask?.id ?? '');
  const recentEvents = useMemo(() => {
    const items = eventsQ.data?.items ?? [];
    return items.slice(-9).reverse();
  }, [eventsQ.data]);

  const recentTasks = tasks.slice(0, 7);

  const onRefresh = async () => {
    setRefreshing(true);
    try {
      await catalog.refresh();
      push('数据已刷新', 'ok');
    } finally {
      setRefreshing(false);
    }
  };

  const loading = catalog.loading && tasks.length === 0;

  // The build-time prerender cannot know the client clock; set the sync label after mount.
  useEffect(() => {
    if (!catalog.loading && !catalog.error) setLastSync(new Date().toISOString());
  }, [catalog.loading, catalog.error]);

  const effective = useMemo(() => {
    const fields = new Map(
      (settings.data?.sections ?? []).flatMap((section) => section.fields.map((field) => [field.key, field.value] as const)),
    );
    return {
      provider: String(fields.get('model.provider') ?? 'none'),
      maxAgents: fields.get('execution.max_agents') as number | undefined,
      workers: fields.get('execution.workers') as number | undefined,
      searchResults: fields.get('search.results') as number | undefined,
      requireApproval: fields.get('execution.require_approval') as boolean | undefined,
      respectRobots: fields.get('execution.respect_robots') as boolean | undefined,
      nodeTimeout: fields.get('execution.node_timeout') as string | undefined,
      taskTimeout: fields.get('execution.task_timeout') as string | undefined,
    };
  }, [settings.data]);

  return (
    <div className="page">
      <PageHeader
        eyebrow="Overview"
        title="研究总览"
        description="从一个研究问题到可追溯报告的全过程视图：任务、节点、来源、证据与产出。"
        meta={
          <>
            <span>{stats.total} 项研究 · {stats.completed} 项已产出报告</span>
            <span>单用户本地部署</span>
            {tasks[0] && <span>最近更新 {formatRelative(tasks[0].updated_at)}</span>}
          </>
        }
        actions={
          <>
            <Button variant="ghost" size="sm" icon={<Icon name="refresh" size={14} />} onClick={onRefresh} loading={refreshing}>
              刷新
            </Button>
            <Button variant="primary" size="sm" icon={<Icon name="plus" size={15} />} onClick={openNewResearch}>
              新建研究
            </Button>
          </>
        }
      />

      {catalog.error && tasks.length === 0 ? (
        <ErrorState message={catalog.error} onRetry={() => catalog.refresh()} />
      ) : (
        <>
          <div className="metrics">
            {loading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <div className="metric" key={i}>
                  <Skeleton width={70} height={10} />
                  <div style={{ marginTop: 14 }}><Skeleton width={90} height={28} /></div>
                  <div style={{ marginTop: 'auto', paddingTop: 10 }}><Skeleton width={110} height={9} /></div>
                </div>
              ))
            ) : (
              <>
                <MetricCard
                  label="研究任务"
                  value={stats.total}
                  foot={<>进行中 {stats.running} · 待批准 {stats.awaiting}</>}
                />
                <MetricCard
                  label="已采集来源"
                  value={stats.sources}
                  foot={<>覆盖 {health.data?.checks?.search?.providers?.length ?? 0} 个检索源 · {stats.claims} 条主张</>}
                />
                <MetricCard
                  label="近 7 天活动"
                  value={weekCreated}
                  foot={<>完成 {weekCompleted} 项 · 本周新增占比 {stats.total ? Math.round((weekCreated / stats.total) * 100) : 0}%</>}
                />
                <MetricCard
                  label="完成率"
                  value={pct(stats.completed, stats.total)}
                  suffix="%"
                  tone={stats.failed > 0 ? 'warn' : 'ok'}
                  foot={<>已完成 {stats.completed} · 失败 {stats.failed}{stats.paused > 0 ? ` · 暂停 ${stats.paused}` : ''}</>}
                />
              </>
            )}
          </div>

          <div className="dash-grid">
            <section className="card chart-card span-8">
              <SectionHead
                title="研究活动"
                hint="近 7 天创建与完成的任务数"
                action={
                  <div className="chart-legend">
                    <span><i className="legend-swatch is-created" />创建</span>
                    <span><i className="legend-swatch" />完成</span>
                  </div>
                }
              />
              <div className="card-body">
                {loading ? <Skeleton height={188} /> : <ActivityChart buckets={buckets} />}
                <div className="chart-foot">
                  <div className="chart-foot-stat"><span className="k">本周创建</span><span className="v">{weekCreated}</span></div>
                  <div className="chart-foot-stat"><span className="k">本周完成</span><span className="v">{weekCompleted}</span></div>
                  <div className="chart-foot-stat"><span className="k">平均完成节点</span><span className="v">{stats.completed ? `${Math.round(tasks.filter((t) => t.status === 'completed').reduce((s, t) => s + num(t.stats?.nodes_done), 0) / stats.completed)} / 项` : '—'}</span></div>
                  <div className="chart-foot-stat"><span className="k">累计来源</span><span className="v">{stats.sources}</span></div>
                </div>
              </div>
            </section>

            <section className="card span-4">
              <SectionHead
                title="运行时"
                hint={settings.data ? '生效配置' : undefined}
                action={<Link href="/settings" className="btn btn-quiet btn-sm">配置</Link>}
              />
              <div className="runtime-list">
                <div className="runtime-row"><span className="runtime-key">规划 / 抽取</span><span className="runtime-val">{settings.data ? (effective.provider === 'none' ? '本地确定性流程' : effective.provider) : '—'}</span></div>
                <div className="runtime-row"><span className="runtime-key">Agent 并发</span><span className="runtime-val">{effective.maxAgents ?? '—'} / {effective.workers ?? '—'}</span></div>
                <div className="runtime-row"><span className="runtime-key">检索结果</span><span className="runtime-val">{effective.searchResults ?? '—'} 条 / 查询</span></div>
                <div className="runtime-row"><span className="runtime-key">人工批准</span><span className="runtime-val">{effective.requireApproval === undefined ? '—' : effective.requireApproval ? '必须批准' : '自动执行'}</span></div>
                <div className="runtime-row"><span className="runtime-key">robots.txt</span><span className="runtime-val">{effective.respectRobots === undefined ? '—' : effective.respectRobots ? '遵守' : '忽略'}</span></div>
                <div className="runtime-row"><span className="runtime-key">节点 / 任务超时</span><span className="runtime-val mono">{effective.nodeTimeout && effective.taskTimeout ? `${formatGoDuration(effective.nodeTimeout)} / ${formatGoDuration(effective.taskTimeout)}` : '—'}</span></div>
                <div className="runtime-row"><span className="runtime-key">数据库</span><span className="runtime-val">{health.data?.checks?.database?.ok ? `正常 · ${health.data.checks.database.latency_ms ?? 0} ms` : '不可用'}</span></div>
                <div className="runtime-row"><span className="runtime-key">模型</span><span className="runtime-val">{health.data?.checks?.llm?.enabled ? health.data.checks.llm.provider : '本地降级'}</span></div>
              </div>
            </section>

            <section className="card span-8">
              <SectionHead
                title="最近研究"
                hint={`${stats.total} 项`}
                action={<Link href="/tasks" className="btn btn-quiet btn-sm">全部任务 <Icon name="arrow-right" size={13} /></Link>}
              />
              {loading ? (
                <div className="card-body"><div className="skeleton-block">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} height={34} />)}</div></div>
              ) : recentTasks.length === 0 ? (
                <EmptyState title="还没有研究任务" description="创建第一个研究目标后，规划与执行会显示在这里。" action={<Button variant="primary" size="sm" onClick={openNewResearch}>新建研究</Button>} />
              ) : (
                <div className="table-scroll">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th style={{ width: '46%' }}>研究</th>
                        <th>状态</th>
                        <th>节点</th>
                        <th>来源</th>
                        <th style={{ textAlign: 'right' }}>更新</th>
                      </tr>
                    </thead>
                    <tbody>
                      {recentTasks.map((task) => {
                        const detail = details[task.id];
                        const total = num(task.stats?.nodes_total);
                        const done = num(task.stats?.nodes_done);
                        return (
                          <tr key={task.id} className="row-link" onClick={() => router.push(`/task/?id=${task.id}`)}>
                            <td>
                              <div className="task-title-cell">
                                <span className="task-title">{truncate(task.title || task.goal, 46)}</span>
                                <span className="task-goal">{DEPTH_TEXT[task.depth] ?? task.depth} · {task.enable_verification ? '交叉验证' : '单轮分析'}</span>
                              </div>
                            </td>
                            <td><StatusBadge status={task.status} kind="task" /></td>
                            <td>
                              <div className="mini-progress">
                                <div className="progress"><span className={`progress-fill ${done === total && total > 0 ? 'progress-ok' : ''}`} style={{ width: `${pct(done, total)}%` }} /></div>
                                <span className="n">{done}/{total || '—'}</span>
                              </div>
                            </td>
                            <td className="cell-num">{num(task.stats?.sources)}</td>
                            <td style={{ textAlign: 'right' }} className="cell-muted">{formatRelative(task.updated_at)}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            <section className="card span-4">
              <SectionHead title="需要关注" hint={attention.length ? `${attention.length} 项` : undefined} />
              {attention.length === 0 ? (
                <div className="attention-empty">
                  <span className="attention-empty-mark"><Icon name="check" size={12} /></span>
                  <p><strong>没有阻塞项</strong>所有研究都在正常推进或已产出结果。</p>
                </div>
              ) : (
                <div className="attention-list">
                  {attention.map((task) => (
                    <Link key={task.id} href={`/task/?id=${task.id}`} className="attention-item">
                      <span className={cx('attention-mark', task.status === 'failed' && 'is-err')} aria-hidden="true" />
                      <span className="attention-copy">
                        <span className="attention-title">{truncate(task.title || task.goal, 52)}</span>
                        <span className="attention-meta">
                          <span>{taskStatusText(task.status)}</span>
                          <span>·</span>
                          <span>{formatRelative(task.updated_at)}</span>
                          {task.error && <><span>·</span><span>{truncate(task.error, 32)}</span></>}
                        </span>
                      </span>
                    </Link>
                  ))}
                </div>
              )}
              <div style={{ padding: '10px 18px', borderTop: '1px solid var(--line-soft)' }}>
                <Link href="/tasks?status=failed" className="btn btn-quiet btn-sm">查看全部异常</Link>
              </div>
            </section>

            <section className="card span-7">
              <SectionHead
                title="最新报告"
                hint={reports.length ? `最近 ${reports.length} 份` : undefined}
                action={<Link href="/reports" className="btn btn-quiet btn-sm">报告库 <Icon name="arrow-right" size={13} /></Link>}
              />
              {reports.length === 0 ? (
                <EmptyState compact title="暂无报告" description="研究完成后，带引用的报告会出现在这里。" />
              ) : (
                <div className="table-scroll">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th>报告</th>
                        <th>引用</th>
                        <th>字数</th>
                        <th style={{ textAlign: 'right' }}>生成时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {reports.map((detail) => (
                        <tr key={detail.task.id} className="row-link" onClick={() => router.push(`/reports?id=${detail.task.id}`)}>
                          <td>
                            <div className="task-title-cell">
                              <span className="task-title">{truncate(detail.report_summary.title, 52)}</span>
                              <span className="task-goal">{truncate(detail.task.title, 56)}</span>
                            </div>
                          </td>
                          <td className="cell-num">{detail.report_summary.citation_count}</td>
                          <td className="cell-num">{detail.report_summary.word_count}</td>
                          <td style={{ textAlign: 'right' }} className="cell-muted">{formatRelative(detail.report_summary.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            <section className="card span-5">
              <SectionHead
                title="最新活动"
                hint={latestTask ? truncate(latestTask.title, 24) : undefined}
                action={latestTask ? <Link href={`/task/?id=${latestTask.id}`} className="btn btn-quiet btn-sm">打开任务</Link> : undefined}
              />
              {eventsQ.loading ? (
                <div className="card-body"><div className="skeleton-block">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} height={18} />)}</div></div>
              ) : recentEvents.length === 0 ? (
                <EmptyState compact title="暂无事件" description={eventsQ.error ? `事件流不可用：${eventsQ.error}` : '任务产生事件后会在这里显示。'} />
              ) : (
                <div className="activity-list">
                  {recentEvents.map((event) => {
                    const meta = eventMeta(event.event_type);
                    return (
                      <div className="activity-item" key={event.event_id}>
                        <span className="activity-time">{formatRelative(event.timestamp)}</span>
                        <span className={cx('activity-dot', `tone-${meta.tone}`)} aria-hidden="true" />
                        <span className="activity-copy">
                          <span className="activity-label">{meta.label}</span>
                          {event.node_id && <span className="activity-node">{event.node_id}</span>}
                        </span>
                      </div>
                    );
                  })}
                </div>
              )}
            </section>
          </div>

          {catalog.error && tasks.length > 0 && (
            <div style={{ marginTop: 14 }}>
              <ErrorState message={`${catalog.error}（当前展示上次成功加载的数据）`} onRetry={() => catalog.refresh()} />
            </div>
          )}

          <p className="muted small" style={{ marginTop: 18 }}>
            最后同步 {lastSync ? formatDateTime(lastSync) : '—'} · 数据直接来自本地 API，未使用示例数据
          </p>
        </>
      )}
    </div>
  );
}