'use client';

import React, { Suspense, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { cx, formatDateTime, formatRelative, num, pct, truncate, DEPTH_TEXT } from '@/lib/format';
import { useTaskCatalog } from '@/lib/hooks';
import { Icon } from '@/components/Icon';
import {
  Button, EmptyState, ErrorState, PageHeader, Skeleton, StatusBadge,
} from '@/components/ui';
import { useAppActions } from '@/components/AppShell';

type StatusFilter = 'all' | 'active' | 'awaiting_approval' | 'completed' | 'failed' | 'paused' | 'cancelled';
type SortKey = 'updated' | 'created' | 'sources';

const STATUS_OPTIONS: Array<{ value: StatusFilter; label: string }> = [
  { value: 'all', label: '全部状态' },
  { value: 'active', label: '进行中' },
  { value: 'awaiting_approval', label: '待批准' },
  { value: 'completed', label: '已完成' },
  { value: 'failed', label: '失败' },
  { value: 'paused', label: '已暂停' },
  { value: 'cancelled', label: '已取消' },
];

function matchesStatus(status: string, filter: StatusFilter): boolean {
  if (filter === 'all') return true;
  if (filter === 'active') return ['created', 'planning', 'running', 'synthesizing'].includes(status);
  return status === filter;
}

function TasksPageInner() {
  const router = useRouter();
  const params = useSearchParams();
  const { openNewResearch } = useAppActions();
  const catalog = useTaskCatalog(100, 0);
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState<StatusFilter>('all');
  const [sort, setSort] = useState<SortKey>('updated');

  useEffect(() => {
    const fromQuery = params.get('status');
    if (fromQuery && STATUS_OPTIONS.some((o) => o.value === fromQuery)) {
      setStatus(fromQuery as StatusFilter);
    }
  }, [params]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = catalog.tasks.filter((task) => {
      if (!matchesStatus(task.status, status)) return false;
      if (!q) return true;
      return `${task.title} ${task.goal}`.toLowerCase().includes(q);
    });
    return [...list].sort((a, b) => {
      if (sort === 'created') return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
      if (sort === 'sources') return num(b.stats?.sources) - num(a.stats?.sources);
      return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
    });
  }, [catalog.tasks, query, status, sort]);

  const counts = useMemo(() => {
    const all = catalog.tasks;
    return {
      all: all.length,
      active: all.filter((t) => ['created', 'planning', 'running', 'synthesizing'].includes(t.status)).length,
      awaiting: all.filter((t) => t.status === 'awaiting_approval').length,
      completed: all.filter((t) => t.status === 'completed').length,
      failed: all.filter((t) => t.status === 'failed').length,
    };
  }, [catalog.tasks]);

  const loading = catalog.loading && catalog.tasks.length === 0;

  return (
    <div className="page">
      <PageHeader
        eyebrow="Research"
        title="研究任务"
        description="每一次研究都是一条可回溯的执行链：规划、检索、分析、验证与综述。"
        meta={
          <>
            <span>共 {catalog.total} 项</span>
            <span>进行中 {counts.active}</span>
            <span>待批准 {counts.awaiting}</span>
            <span>已完成 {counts.completed}</span>
            {counts.failed > 0 && <span style={{ color: 'var(--err)' }}>失败 {counts.failed}</span>}
          </>
        }
        actions={
          <Button variant="primary" size="sm" icon={<Icon name="plus" size={15} />} onClick={openNewResearch}>
            新建研究
          </Button>
        }
      />

      <div className="toolbar">
        <div className="toolbar-left">
          <div className="search-input">
            <Icon name="search" size={15} />
            <input
              className="input"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="按标题或研究目标搜索"
              aria-label="搜索研究任务"
            />
          </div>
          <select className="select" style={{ width: 132 }} value={status} onChange={(e) => setStatus(e.target.value as StatusFilter)} aria-label="状态筛选">
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
          <select className="select" style={{ width: 132 }} value={sort} onChange={(e) => setSort(e.target.value as SortKey)} aria-label="排序方式">
            <option value="updated">最近更新</option>
            <option value="created">创建时间</option>
            <option value="sources">来源数量</option>
          </select>
          {(query || status !== 'all') && (
            <button type="button" className="btn btn-quiet btn-sm" onClick={() => { setQuery(''); setStatus('all'); }}>
              清除筛选
            </button>
          )}
        </div>
        <div className="toolbar-right">
          <span className="inline-stat">{filtered.length} / {catalog.tasks.length} 项</span>
        </div>
      </div>

      <section className="card">
        {catalog.error && catalog.tasks.length === 0 ? (
          <div style={{ padding: 18 }}>
            <ErrorState message={catalog.error} onRetry={() => catalog.refresh()} />
          </div>
        ) : loading ? (
          <div className="card-body">
            <div className="skeleton-block">
              {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} height={40} />)}
            </div>
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState
            title={catalog.tasks.length === 0 ? '还没有研究任务' : '没有匹配的任务'}
            description={catalog.tasks.length === 0 ? '从一个具体问题开始，系统会先生成规划再执行。' : '尝试更换关键词或状态筛选。'}
            action={catalog.tasks.length === 0
              ? <Button variant="primary" size="sm" onClick={openNewResearch}>新建研究</Button>
              : <Button size="sm" variant="secondary" onClick={() => { setQuery(''); setStatus('all'); }}>清除筛选</Button>}
          />
        ) : (
          <div className="table-scroll">
            <table className="data-table">
              <thead>
                <tr>
                  <th style={{ width: '40%' }}>研究</th>
                  <th>状态</th>
                  <th>深度</th>
                  <th>节点进度</th>
                  <th style={{ textAlign: 'right' }}>来源</th>
                  <th style={{ textAlign: 'right' }}>主张</th>
                  <th style={{ textAlign: 'right' }}>创建</th>
                  <th style={{ textAlign: 'right' }}>更新</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((task) => {
                  const total = num(task.stats?.nodes_total);
                  const done = num(task.stats?.nodes_done);
                  return (
                    <tr key={task.id} className="row-link" onClick={() => router.push(`/task/?id=${task.id}`)}>
                      <td>
                        <div className="task-title-cell">
                          <span className="task-title">{truncate(task.title || task.goal, 58)}</span>
                          <span className="task-goal">{truncate(task.goal, 72)}</span>
                        </div>
                      </td>
                      <td><StatusBadge status={task.status} kind="task" /></td>
                      <td>
                        <span className="badge badge-neutral">{DEPTH_TEXT[task.depth] ?? task.depth}</span>
                      </td>
                      <td>
                        <div className="mini-progress">
                          <div className="progress">
                            <span
                              className={cx('progress-fill', done === total && total > 0 && 'progress-ok', task.status === 'failed' && 'progress-err')}
                              style={{ width: `${pct(done, total)}%` }}
                            />
                          </div>
                          <span className="n">{done}/{total || '—'}</span>
                        </div>
                      </td>
                      <td className="cell-num" style={{ textAlign: 'right' }}>{num(task.stats?.sources)}</td>
                      <td className="cell-num" style={{ textAlign: 'right' }}>{num(task.stats?.claims)}</td>
                      <td className="cell-muted" style={{ textAlign: 'right' }} title={formatDateTime(task.created_at)}>{formatDateTime(task.created_at)}</td>
                      <td className="cell-muted" style={{ textAlign: 'right' }}>{formatRelative(task.updated_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <div className="row" style={{ marginTop: 14, justifyContent: 'space-between' }}>
        <span className="muted small">显示最近 {Math.min(100, catalog.tasks.length)} 项研究；数据每 10 秒自动刷新。</span>
        <Link href="/settings" className="muted small">调整运行时配置 →</Link>
      </div>
    </div>
  );
}

export default function TasksPage() {
  return (
    <Suspense fallback={<div className="page"><Skeleton height={220} /></div>}>
      <TasksPageInner />
    </Suspense>
  );
}