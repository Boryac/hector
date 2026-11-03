'use client';

import React, { Suspense, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { apiGet } from '@/lib/api';
import type { Source } from '@/lib/types';
import { cx, errorText, num, truncate } from '@/lib/format';
import { usePolling, useTaskCatalog } from '@/lib/hooks';
import { Icon } from '@/components/Icon';
import { Button, EmptyState, ErrorState, PageHeader, Skeleton } from '@/components/ui';
import { SourceDetail, SourceTable } from '@/components/SourceTable';

const STATUS_FILTERS = [
  { value: '', label: '全部' },
  { value: 'ok', label: '已抓取' },
  { value: 'discovered', label: '待抓取' },
  { value: 'fetch_failed', label: '抓取失败' },
  { value: 'robots_blocked', label: '协议禁止' },
];

interface SourcePage {
  items: Source[];
  total: number;
  limit: number;
  offset: number;
}

function SourcesPageInner() {
  const params = useSearchParams();
  const catalog = useTaskCatalog(100, 0);
  const taskParam = params.get('task') ?? '';

  const [taskId, setTaskId] = useState(taskParam);
  const [status, setStatus] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const [open, setOpen] = useState<Source | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const pageSize = 50;

  useEffect(() => {
    const timer = window.setTimeout(() => { setQuery(searchInput); setPage(0); }, 320);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const apiQuery = useMemo(() => {
    const p = new URLSearchParams();
    p.set('limit', String(pageSize));
    p.set('offset', String(page * pageSize));
    if (taskId) p.set('task_id', taskId);
    if (status) p.set('status', status);
    if (query.trim()) p.set('q', query.trim());
    return p.toString();
  }, [taskId, status, query, page]);

  const sourcesQ = usePolling<SourcePage>(
    () => apiGet<SourcePage>(`/api/v1/sources?${apiQuery}`),
    { key: apiQuery },
  );

  const data = sourcesQ.data;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const filtered = items;

  const taskOptions = catalog.tasks;
  const selectedTask = taskOptions.find((task) => task.id === taskId);

  const openSource = async (source: Source) => {
    setOpen(source);
    setDetailLoading(true);
    try {
      const full = await apiGet<Source>(`/api/v1/research/${source.task_id}/sources/${source.id}`);
      setOpen(full);
    } catch {
      // Keep the list projection visible; the drawer still shows what we know.
    } finally {
      setDetailLoading(false);
    }
  };

  const resetFilters = () => {
    setTaskId('');
    setStatus('');
    setSearchInput('');
    setQuery('');
    setPage(0);
  };

  const loading = sourcesQ.loading && !data;

  return (
    <div className="page">
      <PageHeader
        eyebrow="Sources"
        title="研究来源"
        description="跨任务索引所有抓取资料。每条都保留原始链接、抓取状态与内容指纹，报告引用可回溯到这里。"
        meta={
          <>
            <span>共 {num(total)} 条来源</span>
            <span>当前显示 {items.length} 条</span>
            {selectedTask && <span>任务：{truncate(selectedTask.title, 42)}</span>}
          </>
        }
        actions={
          selectedTask && (
            <Link href={`/task/?id=${selectedTask.id}`} className="btn btn-secondary btn-sm">
              打开任务 <Icon name="arrow-right" size={13} />
            </Link>
          )
        }
      />

      <div className="toolbar">
        <div className="toolbar-left">
          <select
            className="select"
            style={{ maxWidth: 320 }}
            value={taskId}
            onChange={(e) => { setTaskId(e.target.value); setPage(0); }}
            aria-label="按任务筛选"
          >
            <option value="">全部任务</option>
            {taskOptions.map((task) => (
              <option value={task.id} key={task.id}>
                {truncate(task.title || task.goal, 40)} · {num(task.stats?.sources)} 条
              </option>
            ))}
          </select>
          <div className="search-input" style={{ maxWidth: 260 }}>
            <Icon name="search" size={14} />
            <input className="input" value={searchInput} onChange={(e) => setSearchInput(e.target.value)} placeholder="搜索标题、链接或摘要" aria-label="搜索来源" />
          </div>
          {(taskId || status || query) && (
            <Button size="sm" variant="quiet" onClick={resetFilters}>清除筛选</Button>
          )}
        </div>
        <div className="toolbar-right">
          <span className="inline-stat">{filtered.length} / {num(total)} 条</span>
        </div>
      </div>

      <div className="summary-chips" style={{ marginBottom: 12 }}>
        {STATUS_FILTERS.map((filter) => (
          <button
            key={filter.value || 'all'}
            type="button"
            className={cx('chip', status === filter.value && 'is-active')}
            onClick={() => { setStatus(filter.value); setPage(0); }}
          >
            {filter.label}
          </button>
        ))}
        <span className="inline-stat" style={{ marginLeft: 'auto' }}>
          HTTP 200 且正文入库后标记为「已抓取」；协议禁止表示 robots.txt 拒绝抓取
        </span>
      </div>

      <section className="card">
        {sourcesQ.error ? (
          <div style={{ padding: 18 }}><ErrorState message={errorText(sourcesQ.error)} onRetry={() => sourcesQ.refresh()} /></div>
        ) : loading ? (
          <div className="card-body"><div className="skeleton-block">{Array.from({ length: 7 }).map((_, i) => <Skeleton key={i} height={34} />)}</div></div>
        ) : filtered.length === 0 ? (
          <EmptyState
            title="没有匹配的来源"
            description="换一个任务、状态或关键词，或先运行一个带检索节点的研究。"
            action={<Button size="sm" variant="secondary" onClick={resetFilters}>清除筛选</Button>}
          />
        ) : (
          <SourceTable sources={filtered} onSelect={(source) => void openSource(source)} showTask />
        )}
      </section>

      <div className="pagination">
        <span className="pagination-info">
          第 {page + 1} / {totalPages} 页 · 共 {num(total)} 条
        </span>
        <div className="pagination-actions">
          <Button size="sm" variant="secondary" disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>上一页</Button>
          <Button size="sm" variant="secondary" disabled={page + 1 >= totalPages} onClick={() => setPage((p) => p + 1)}>下一页</Button>
        </div>
      </div>

      {open && (
        <div className="drawer-layer" role="dialog" aria-modal="true" aria-label="来源详情" onClick={() => setOpen(null)}>
          <div className="drawer-panel" onClick={(e) => e.stopPropagation()}>
            <div className="drawer-head">
              <span className="drawer-title">来源详情</span>
              <div className="row" style={{ gap: 8 }}>
                {detailLoading && <span className="spinner" aria-label="加载正文" />}
                <button type="button" className="icon-btn" aria-label="关闭" onClick={() => setOpen(null)}><Icon name="x" size={15} /></button>
              </div>
            </div>
            <div className="drawer-body">
              {open.task_title && (
                <p className="drawer-task">
                  所属任务：<Link href={`/task/?id=${open.task_id}`}>{open.task_title}</Link>
                </p>
              )}
              <SourceDetail source={open} />
            </div>
            <div className="drawer-foot">
              <a href={open.url} target="_blank" rel="noopener noreferrer" className="btn btn-secondary btn-sm">打开原始页面 <Icon name="external" size={12} /></a>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default function SourcesPage() {
  return (
    <Suspense fallback={<div className="page"><Skeleton height={260} /></div>}>
      <SourcesPageInner />
    </Suspense>
  );
}