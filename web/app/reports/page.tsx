'use client';

import React, { Suspense, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { apiGet, downloadReport } from '@/lib/api';
import type { Report } from '@/lib/types';
import { cx, errorText, formatDateTime, formatRelative, num, truncate } from '@/lib/format';
import { usePolling } from '@/lib/hooks';
import { Icon } from '@/components/Icon';
import { Button, EmptyState, ErrorState, PageHeader, Skeleton } from '@/components/ui';
import { useToast } from '@/components/Toast';
import { ReportReader } from '@/components/ReportReader';

interface ReportItem {
  id: string;
  task_id: string;
  task_title: string;
  version: number;
  title: string;
  citation_count: number;
  word_count: number;
  model: string;
  created_at: string;
}

interface ReportList {
  items: ReportItem[];
  total: number;
}

function ReportPane({ item }: { item: ReportItem }) {
  const { push } = useToast();
  const [busy, setBusy] = useState<string | null>(null);
  const reportQ = usePolling<{ report: Report }>(
    () => apiGet<{ report: Report }>(`/api/v1/research/${item.task_id}/report`),
    { key: item.task_id },
  );

  const exportReport = async (format: 'md' | 'html') => {
    setBusy(format);
    try {
      await downloadReport(item.task_id, format);
      push(`已导出 .${format}`, 'ok');
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusy(null);
    }
  };

  if (reportQ.loading) {
    return <div className="reader"><div className="skeleton-block">{Array.from({ length: 9 }).map((_, i) => <Skeleton key={i} height={i === 0 ? 28 : 14} width={i === 0 ? '55%' : '100%'} />)}</div></div>;
  }
  if (reportQ.error || !reportQ.data?.report) {
    return <ErrorState message={reportQ.error ?? '报告正文不可用'} onRetry={() => reportQ.refresh()} />;
  }

  return (
    <ReportReader
      report={reportQ.data.report}
      taskTitle={item.task_title}
      actions={
        <>
          <Button size="sm" variant="secondary" icon={<Icon name="download" size={13} />} loading={busy === 'md'} onClick={() => exportReport('md')}>Markdown</Button>
          <Button size="sm" variant="secondary" icon={<Icon name="download" size={13} />} loading={busy === 'html'} onClick={() => exportReport('html')}>HTML</Button>
          <Link href={`/task/?id=${item.task_id}`} className="btn btn-quiet btn-sm">打开工作流 <Icon name="arrow-right" size={13} /></Link>
        </>
      }
    />
  );
}

function ReportsPageInner() {
  const params = useSearchParams();
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<'newest' | 'oldest' | 'citations' | 'words'>('newest');
  const [selectedTaskId, setSelectedTaskId] = useState('');

  const listQ = usePolling<ReportList>(
    () => apiGet<ReportList>('/api/v1/reports?limit=200'),
    { key: 'reports' },
  );

  const items = listQ.data?.items ?? [];
  const total = listQ.data?.total ?? items.length;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q
      ? items.filter((item) => `${item.title} ${item.task_title} ${item.model}`.toLowerCase().includes(q))
      : items;
    return [...list].sort((a, b) => {
      if (sort === 'oldest') return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
      if (sort === 'citations') return b.citation_count - a.citation_count;
      if (sort === 'words') return b.word_count - a.word_count;
      return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    });
  }, [items, query, sort]);

  useEffect(() => {
    if (filtered.length === 0) return;
    if (selectedTaskId && filtered.some((item) => item.task_id === selectedTaskId)) return;
    const fromQuery = filtered.find((item) => item.task_id === params.get('id'));
    setSelectedTaskId(fromQuery?.task_id ?? filtered[0].task_id);
  }, [filtered, selectedTaskId, params]);

  const selected = filtered.find((item) => item.task_id === selectedTaskId);

  const totals = useMemo(() => ({
    citations: items.reduce((sum, item) => sum + item.citation_count, 0),
    words: items.reduce((sum, item) => sum + item.word_count, 0),
  }), [items]);

  return (
    <div className="page">
      <PageHeader
        eyebrow="Reports"
        title="研究报告"
        description="已完成研究的正式产出。每份报告都带版本、引用与生成模型，可在线阅读或导出归档。"
        meta={
          <>
            <span>{num(total)} 份报告</span>
            <span>{num(totals.citations)} 条可追溯引用</span>
            <span>约 {Math.round(totals.words / Math.max(1, items.length))} 字 / 份</span>
          </>
        }
        actions={
          <Button size="sm" variant="secondary" icon={<Icon name="retry" size={13} />} onClick={() => listQ.refresh()}>刷新</Button>
        }
      />

      {listQ.error ? (
        <ErrorState message={errorText(listQ.error)} onRetry={() => listQ.refresh()} />
      ) : listQ.loading ? (
        <div className="card"><div className="card-body"><div className="skeleton-block">{Array.from({ length: 7 }).map((_, i) => <Skeleton key={i} height={34} />)}</div></div></div>
      ) : items.length === 0 ? (
        <EmptyState
          title="还没有生成报告"
          description="研究任务到达「已完成」状态后会生成报告，并自动进入报告库。"
          action={<Link href="/tasks" className="btn btn-secondary btn-sm">查看研究任务</Link>}
        />
      ) : (
        <>
          <div className="toolbar">
            <div className="toolbar-left">
              <div className="search-input" style={{ maxWidth: 300 }}>
                <Icon name="search" size={14} />
                <input className="input" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索报告标题或任务" aria-label="搜索报告" />
              </div>
              <select className="select" style={{ width: 132 }} value={sort} onChange={(e) => setSort(e.target.value as typeof sort)} aria-label="排序">
                <option value="newest">最新生成</option>
                <option value="oldest">最早生成</option>
                <option value="citations">引用最多</option>
                <option value="words">篇幅最长</option>
              </select>
            </div>
            <div className="toolbar-right">
              <span className="inline-stat">{filtered.length} 份</span>
            </div>
          </div>

          <div className="library">
            <aside className="card library-list">
              <div className="panel-head">
                <span className="panel-title">报告列表</span>
                <span className="panel-meta">{query ? `${filtered.length} / ${items.length}` : total}</span>
              </div>
              <div className="library-scroll">
                {filtered.map((report) => (
                  <button
                    type="button"
                    key={report.id}
                    className={cx('library-item', selectedTaskId === report.task_id && 'is-active')}
                    onClick={() => setSelectedTaskId(report.task_id)}
                  >
                    <div className="library-item-top">
                      <span className="library-version">v{report.version}</span>
                      <span className="library-time">{formatRelative(report.created_at)}</span>
                    </div>
                    <p className="library-title">{truncate(report.title || report.task_title, 74)}</p>
                    <div className="library-meta">
                      <span>{report.word_count} 字</span>
                      <span>{report.citation_count} 引用</span>
                      <span>{truncate(report.task_title, 28)}</span>
                    </div>
                  </button>
                ))}
              </div>
            </aside>

            <section className="card library-reader">
              {selected ? (
                <>
                  <div className="library-reader-bar">
                    <div className="row" style={{ gap: 8 }}>
                      <span className="badge badge-neutral">v{selected.version}</span>
                      <span className="muted small">{formatDateTime(selected.created_at)}</span>
                      {selected.model && <span className="muted small">{truncate(selected.model, 36)}</span>}
                    </div>
                  </div>
                  <div className="library-reader-scroll">
                    <ReportPane key={selected.task_id} item={selected} />
                  </div>
                </>
              ) : (
                <EmptyState title="选择一份报告" description="从左侧列表选择报告，即可在此阅读全文与引用。" />
              )}
            </section>
          </div>
        </>
      )}
    </div>
  );
}

export default function ReportsPage() {
  return (
    <Suspense fallback={<div className="page"><Skeleton height={260} /></div>}>
      <ReportsPageInner />
    </Suspense>
  );
}