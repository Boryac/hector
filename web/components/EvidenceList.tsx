'use client';

import React from 'react';
import type { Claim } from '@/lib/types';
import { claimStatusTone, cx, hostOf, truncate } from '@/lib/format';
import { StatusBadge } from './ui';

export function EvidenceList({ claims }: { claims: Claim[] }) {
  return (
    <div className="evidence-list">
      {claims.map((claim) => {
        const tone = claimStatusTone(claim.status);
        const confidence = Math.max(0, Math.min(1, Number(claim.confidence) || 0));
        return (
          <article className="claim-card" key={claim.id}>
            <div className="claim-head">
              <p className="claim-statement">{claim.statement}</p>
              <div className="claim-tools">
                <StatusBadge status={claim.status} kind="claim" />
              </div>
            </div>
            <div className="claim-meta">
              <span className="confidence">
                <span className="confidence-track">
                  <span className={cx('confidence-fill', `is-${tone}`)} style={{ width: `${confidence * 100}%` }} />
                </span>
                置信度 {confidence.toFixed(2)}
              </span>
              <span>来源 {claim.source_count ?? 0}</span>
              {claim.node_id && <span className="mono">{claim.node_id}</span>}
              {claim.conflict_group && <span>冲突组 {claim.conflict_group}</span>}
            </div>

            {claim.verifier_notes && (
              <div className="verifier-note">{claim.verifier_notes}</div>
            )}

            {(claim.evidence ?? []).length > 0 && (
              <div className="evidence-quotes">
                {(claim.evidence ?? []).map((evidence) => (
                  <div className="evidence-quote" key={evidence.id}>
                    <p className="quote-text">「{truncate(evidence.quote, 260)}」</p>
                    <div className="quote-source">
                      {evidence.source?.url ? (
                        <a href={evidence.source.url} target="_blank" rel="noopener noreferrer">
                          {evidence.source.title || hostOf(evidence.source.url)}
                        </a>
                      ) : (
                        <span>来源 {evidence.source_id.slice(0, 8)}</span>
                      )}
                      <span>·</span>
                      <span>{evidence.stance === 'support' ? '支持' : evidence.stance === 'refute' ? '反驳' : evidence.stance || '引用'}</span>
                      {typeof evidence.confidence === 'number' && <><span>·</span><span>证据强度 {evidence.confidence.toFixed(2)}</span></>}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </article>
        );
      })}
    </div>
  );
}

export function EvidenceSummary({ claims }: { claims: Claim[] }) {
  const groups = claims.reduce<Record<string, number>>((acc, claim) => {
    acc[claim.status] = (acc[claim.status] ?? 0) + 1;
    return acc;
  }, {});
  const order: Array<[string, string]> = [
    ['supported', '已支持'],
    ['partially_supported', '部分支持'],
    ['conflicting', '存在冲突'],
    ['unverified', '未验证'],
  ];
  return (
    <div className="summary-chips">
      <span className="chip">主张 <span className="n">{claims.length}</span></span>
      {order.filter(([key]) => groups[key]).map(([key, label]) => (
        <span className="chip" key={key}>{label} <span className="n">{groups[key]}</span></span>
      ))}
    </div>
  );
}