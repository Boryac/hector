'use client';

import React, { Suspense, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { apiGet } from '@/lib/api';
import type { Claim } from '@/lib/types';
import { cx, CLAIM_STATUS_TEXT, errorText, num, truncate } from '@/lib/format';
import { usePolling, useTaskCatalog } from '@/lib/hooks';
import { Icon } from '@/components/Icon';
import { EmptyState, ErrorState, PageHeader, Skeleton } from '@/components/ui';
import { EvidenceList } from '@/components/EvidenceList';

const STATUS_ORDER = ['supported', 'partially_supported', 'conflicting', 'unverified'];

function EvidencePageInner() {
  const params = useSearchParams();
  const catalog = useTaskCatalog(40, 0);
  const taskParam = params.get('task') ?? '';

  const tasks = catalog.tasks;
  const [taskId, setTaskId] = useState('');
  const [status, setStatus] = useState('all');
  const [query, setQuery] = useState('');

  useEffect(() => {
    if (tasks.length === 0) return;
    if (taskId && tasks.some((t) => t.id === taskId)) return;
    const preferred = tasks.find((t) => t.id === taskParam);
    const withClaims = tasks.find((t) => num(t.stats?.claims) > 0) ?? tasks[0];
    setTaskId(preferred?.id ?? withClaims.id);
  }, [tasks, taskId, taskParam]);

  const selectedTask = tasks.find((t) => t.id === taskId);
  const claimsQ = usePolling<{ items: Claim[] }>(
    () => apiGet<{ items: Claim[] }>(`/api/v1/research/${taskId}/evidence?limit=300`),
    { key: taskId, enabled: Boolean(taskId) },
  );

  const claims = claimsQ.data?.items ?? [];

  const counts = useMemo(() => {
    const map: Record<string, number> = { all: claims.length };
    claims.forEach((claim) => { map[claim.status] = (map[claim.status] ?? 0) + 1; });
    return map;
  }, [claims]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return claims.filter((claim) => {
      if (status !== 'all' && claim.status !== status) return false;
      if (q && !claim.statement.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [claims, status, query]);

  const supportedShare = claims.length
    ? Math.round((((counts.supported ?? 0) + (counts.partially_supported ?? 0)) / claims.length) * 100)
    : 0;

  const loading = claimsQ.loading && !claimsQ.data;

  return (
    <div className="page">
      <PageHeader
        eyebrow="Evidence"
        title="证据中心"
        description="主张不是模型断言：每条结论都附带来源引文、支持度与置信度，可逐条核对。"
        meta={
          <>
            <span>当前任务 {claims.length} 条主张</span>
            <span>有来源支持 {supportedShare}%</span>
            {selectedTask && <span>{truncate(selectedTask.title, 48)}</span>}
          </>
        }
        actions={
          selectedTask && (
            <Link href={`/task/?id=${selectedTask.id}`} className="btn btn-secondary btn-sm">
              在工作流中查看 <Icon name="arrow-right" size={13} />
            </Link>
          )
        }
      />

      {catalog.error && tasks.length === 0 ? (
        <ErrorState message={catalog.error} onRetry={() => catalog.refresh()} />
      ) : catalog.loading && tasks.length === 0 ? (
        <div className="card"><div className="card-body"><div className="skeleton-block">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} height={72} />)}</div></div></div>
      ) : tasks.length === 0 ? (
        <EmptyState
          title="还没有研究任务"
          description="分析节点会从来源正文中抽取主张，并给出支持度与引文。"
          action={<Link href="/" className="btn btn-primary btn-sm">返回总览</Link>}
        />
      ) : (
        <>
          <div className="toolbar">
            <div className="toolbar-left">
              <select
                className="select"
                style={{ maxWidth: 420 }}
                value={taskId}
                onChange={(e) => { setTaskId(e.target.value); setStatus('all'); setQuery(''); }}
                aria-label="选择研究任务"
              >
                {tasks.map((task) => (
                  <option value={task.id} key={task.id}>
                    {truncate(task.title || task.goal, 46)} · {num(task.stats?.claims)} 条主张
                  </option>
                ))}
              </select>
              <div className="search-input" style={{ maxWidth: 260 }}>
                <Icon name="search" size={14} />
                <input className="input" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索主张" aria-label="搜索主张" />
              </div>
            </div>
            <div className="toolbar-right">
              <span className="inline-stat">{filtered.length} / {claims.length} 条</span>
            </div>
          </div>

          <div className="summary-chips" style={{ marginBottom: 12 }}>
            <button type="button" className={cx('chip', status === 'all' && 'is-active')} onClick={() => setStatus('all')}>
              全部<span className="n">{claims.length}</span>
            </button>
            {STATUS_ORDER.map((key) => {
              const count = counts[key] ?? 0;
              if (!count) return null;
              return (
                <button key={key} type="button" className={cx('chip', status === key && 'is-active')} onClick={() => setStatus(key)}>
                  {CLAIM_STATUS_TEXT[key]}<span className="n">{count}</span>
                </button>
              );
            })}
            {selectedTask && (
              <span className="inline-stat" style={{ marginLeft: 'auto' }}>
                来源 {num(selectedTask.stats?.sources)} 条 · 置信度按独立站点与数值一致性计算
              </span>
            )}
          </div>

          <section className="card">
            {claimsQ.error ? (
              <div style={{ padding: 18 }}><ErrorState message={errorText(claimsQ.error)} onRetry={() => claimsQ.refresh()} /></div>
            ) : loading ? (
              <div className="card-body"><div className="skeleton-block">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} height={82} />)}</div></div>
            ) : filtered.length === 0 ? (
              <EmptyState
                title={claims.length === 0 ? '该任务尚无证据' : '没有匹配的主张'}
                description={claims.length === 0 ? '分析节点完成后，带引文的主张会在这里逐条展开。' : '尝试更换支持度筛选或关键词。'}
              />
            ) : (
              <EvidenceList claims={filtered} />
            )}
          </section>

          <div className="notice" style={{ marginTop: 14 }}>
            <Icon name="info" size={14} />
            <span>
              <strong>判定口径</strong>
              已支持：至少两个独立站点给出相同结论；部分支持：仅单一来源或数值不完全一致；存在冲突：同一指标出现互斥数值；未验证：缺少可引用正文。置信度不等于正确概率。
            </span>
          </div>
        </>
      )}
    </div>
  );
}

export default function EvidencePage() {
  return (
    <Suspense fallback={<div className="page"><Skeleton height={260} /></div>}>
      <EvidencePageInner />
    </Suspense>
  );
}