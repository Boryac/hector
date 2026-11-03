'use client';

import React from 'react';
import Link from 'next/link';
import type { Source } from '@/lib/types';
import { cx, formatDateTime, hostOf, truncate, CREDIBILITY_TEXT } from '@/lib/format';
import { StatusBadge } from './ui';
import { Icon } from './Icon';

export function SourceTable({
  sources,
  onSelect,
  showTask = false,
}: {
  sources: Source[];
  onSelect?: (source: Source) => void;
  showTask?: boolean;
}) {
  return (
    <div className="table-scroll">
      <table className="data-table">
        <thead>
          <tr>
            <th style={{ width: '48%' }}>来源</th>
            <th>站点</th>
            <th>Provider</th>
            <th>状态</th>
            {showTask && <th>任务</th>}
            <th style={{ textAlign: 'center' }}>可信度</th>
            <th style={{ textAlign: 'right' }}>证据</th>
            <th style={{ textAlign: 'right' }}>抓取时间</th>
          </tr>
        </thead>
        <tbody>
          {sources.map((source) => (
            <tr
              key={source.id}
              className={cx('row-link')}
              onClick={() => onSelect?.(source)}
            >
              <td>
                <div className="source-cell">
                  <span className="source-title" title={source.title || source.url}>
                    {truncate(source.title || source.url, 86)}
                  </span>
                  <span className="source-url">{source.normalized_url || source.url}</span>
                </div>
              </td>
              <td><span className="host-pill">{hostOf(source.url) || source.site || '—'}</span></td>
              <td className="cell-muted">{source.provider || '—'}</td>
              <td><StatusBadge status={source.status} kind="source" /></td>
              {showTask && (
                <td>
                  {source.task_title ? (
                    <Link
                      href={`/task/?id=${source.task_id}`}
                      className="cell-link"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {truncate(source.task_title, 22)}
                    </Link>
                  ) : <span className="cell-muted">—</span>}
                </td>
              )}
              <td style={{ textAlign: 'center' }} className="cell-muted">
                {CREDIBILITY_TEXT[source.credibility] ?? source.credibility ?? '—'}
              </td>
              <td style={{ textAlign: 'right' }} className="cell-num">{source.evidence_count ?? 0}</td>
              <td style={{ textAlign: 'right' }} className="cell-muted">{formatDateTime(source.fetched_at || source.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function SourceDetail({ source }: { source: Source }) {
  return (
    <div>
      <div className="row" style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <h2 className="drawer-source-title">{source.title || '未命名来源'}</h2>
        <StatusBadge status={source.status} kind="source" />
      </div>
      <a className="drawer-source-url" href={source.url} target="_blank" rel="noopener noreferrer">
        {source.url} <Icon name="external" size={11} />
      </a>

      <div className="drawer-section">
        <p className="drawer-label">来源信息</p>
        <div className="meta-row"><span className="meta-label">站点</span><span className="meta-value">{source.site || hostOf(source.url)}</span></div>
        <div className="meta-row"><span className="meta-label">Provider</span><span className="meta-value">{source.provider || '—'}</span></div>
        <div className="meta-row"><span className="meta-label">HTTP 状态</span><span className="meta-value">{source.http_status || '—'}</span></div>
        <div className="meta-row"><span className="meta-label">可信度</span><span className="meta-value">{CREDIBILITY_TEXT[source.credibility] ?? source.credibility ?? '—'}</span></div>
        <div className="meta-row"><span className="meta-label">抓取时间</span><span className="meta-value">{formatDateTime(source.fetched_at)}</span></div>
        <div className="meta-row"><span className="meta-label">内容指纹</span><span className="meta-value mono" style={{ fontSize: 10 }}>{source.content_hash ? `${source.content_hash.slice(0, 16)}…` : '—'}</span></div>
      </div>

      {source.error && (
        <div className="drawer-section">
          <p className="drawer-label">抓取错误</p>
          <div className="inspector-error">{source.error}</div>
        </div>
      )}

      <div className="drawer-section">
        <p className="drawer-label">摘要</p>
        <p className="excerpt-text">{source.snippet || '该来源没有摘要。'}</p>
      </div>

      <div className="drawer-section">
        <p className="drawer-label">正文摘录</p>
        <p className="excerpt-text">{source.excerpt || '尚未抓取正文，或正文为空。'}</p>
      </div>
    </div>
  );
}