'use client';

import React, { useMemo } from 'react';
import type { Report } from '@/lib/types';
import { formatDateTime, hostOf, truncate } from '@/lib/format';
import { headingSlug, Markdown } from './Markdown';
import { Icon } from './Icon';

export function ReportReader({
  report,
  taskTitle,
  actions,
}: {
  report: Report;
  taskTitle?: string;
  actions?: React.ReactNode;
}) {
  const body = useMemo(() => report.markdown.replace(/^#\s+.+\n+/, ''), [report.markdown]);

  const toc = useMemo(() => {
    const fromSections = (report.sections ?? []).map((s) => s.heading).filter(Boolean);
    const fromMarkdown = Array.from(report.markdown.matchAll(/^##\s+(.+)$/gm)).map((m) => m[1].trim());
    const seen = new Set<string>();
    return (fromSections.length ? fromSections : fromMarkdown)
      .filter((heading) => {
        const id = headingSlug(heading);
        if (seen.has(id)) return false;
        seen.add(id);
        return true;
      })
      .map((heading) => ({ heading, id: headingSlug(heading) }));
  }, [report.markdown, report.sections]);

  return (
    <div className="reader">
      <div className="reader-layout">
        <div className="reader-body">
          <p className="reader-kicker">Research Report · v{report.version}</p>
          <h1 className="reader-title">{report.title}</h1>
          <div className="reader-meta">
            {taskTitle && <span>{truncate(taskTitle, 64)}</span>}
            <span>{report.word_count} 字</span>
            <span>{report.citation_count} 条引用</span>
            <span>模型：{report.model || '—'}</span>
            <span>{formatDateTime(report.created_at)}</span>
          </div>
          {actions && <div className="reader-actions">{actions}</div>}

          <div className="report-prose">
            <Markdown source={body} className="md" />
          </div>

          {report.citations.length > 0 && (
            <section className="citations-block" aria-label="参考文献">
              <h2 className="citations-title">参考文献 · {report.citations.length}</h2>
              {report.citations.map((citation) => (
                <div className="citation-item" key={`${citation.index}-${citation.source_id}`} id={`cite-${citation.index}`}>
                  <span className="citation-index">[{citation.index}]</span>
                  <div className="citation-body">
                    <p className="citation-title">
                      <a href={citation.url} target="_blank" rel="noopener noreferrer">
                        {citation.title || citation.url}
                        <Icon name="external" size={10} />
                      </a>
                    </p>
                    <div className="citation-meta">
                      <span>{citation.site || hostOf(citation.url)}</span>
                      {citation.claim_id && <span className="mono">claim {citation.claim_id.slice(0, 8)}</span>}
                    </div>
                    {citation.quote && <p className="citation-quote">「{truncate(citation.quote, 200)}」</p>}
                  </div>
                </div>
              ))}
            </section>
          )}
        </div>

        {toc.length > 0 && (
          <aside className="reader-toc" aria-label="目录">
            <p className="toc-title">目录</p>
            <nav className="toc-list">
              {toc.map((item) => (
                <a className="toc-link" href={`#${item.id}`} key={item.id}>{item.heading}</a>
              ))}
            </nav>
          </aside>
        )}
      </div>
    </div>
  );
}