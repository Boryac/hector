'use client';

import React from 'react';
import type { AgentExecution } from '@/lib/types';
import { formatDurationMs, num } from '@/lib/format';
import { StatusBadge } from './ui';

export function AgentTable({ agents }: { agents: AgentExecution[] }) {
  return (
    <div className="table-scroll">
      <table className="data-table">
        <thead>
          <tr>
            <th>角色</th>
            <th>Agent</th>
            <th>模型</th>
            <th>节点</th>
            <th>状态</th>
            <th style={{ textAlign: 'right' }}>耗时</th>
            <th style={{ textAlign: 'right' }}>Token</th>
            <th style={{ textAlign: 'right' }}>工具调用</th>
          </tr>
        </thead>
        <tbody>
          {agents.map((agent) => {
            const tokens = Object.values(agent.token_usage ?? {}).reduce((sum, v) => sum + num(v), 0);
            return (
              <tr key={agent.id}>
                <td className="cell-strong">{agent.role || '—'}</td>
                <td>{agent.name || '—'}</td>
                <td className="cell-muted">{agent.model || '本地流程'}</td>
                <td className="cell-muted mono" style={{ fontSize: 10.5 }}>{agent.node_id || '—'}</td>
                <td><StatusBadge status={agent.status} kind="agent" /></td>
                <td style={{ textAlign: 'right' }} className="cell-num">{formatDurationMs(agent.duration_ms)}</td>
                <td style={{ textAlign: 'right' }} className="cell-num">{tokens || '—'}</td>
                <td style={{ textAlign: 'right' }} className="cell-num">{agent.tool_calls?.length ?? 0}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}