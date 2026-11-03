'use client';

import React, { useMemo } from 'react';
import ReactFlow, {
  Background,
  Controls,
  Edge,
  MarkerType,
  Node as RFNode,
  Position,
} from 'reactflow';
import 'reactflow/dist/style.css';
import type { Node } from '@/lib/types';
import { statusText } from '@/lib/event-map';

const NODE_W = 200;
const LAYER_GAP = 280;
const ROW_GAP = 118;

function ResearchNode({ data }: { data: { node: Node } }) {
  const n = data.node;
  return (
    <div className={`node-card ${n.status}`}>
      <div className="n-title">{n.title.length > 34 ? `${n.title.slice(0, 34)}…` : n.title}</div>
      <div className="n-meta">
        {n.type} · {statusText(n.status)}
        {n.attempts > 1 ? ` · 第 ${n.attempts} 次` : ''}
      </div>
      {n.status === 'running' && (
        <div className="progress" style={{ marginTop: 6 }}>
          <i style={{ width: `${Math.max(4, n.progress)}%` }} />
        </div>
      )}
    </div>
  );
}

const nodeTypes = { research: ResearchNode };

// 分层布局：按依赖深度横向铺开，同层纵向排列。
function layout(nodes: Node[]) {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const depth = new Map<string, number>();
  const visiting = new Set<string>();

  const compute = (id: string): number => {
    if (depth.has(id)) return depth.get(id)!;
    if (visiting.has(id)) return 0; // 防御：异常数据不应导致死循环
    visiting.add(id);
    const n = byId.get(id);
    let d = 0;
    for (const dep of n?.depends_on ?? []) {
      if (byId.has(dep)) d = Math.max(d, compute(dep) + 1);
    }
    visiting.delete(id);
    depth.set(id, d);
    return d;
  };

  nodes.forEach((n) => compute(n.id));

  const layers = new Map<number, string[]>();
  nodes.forEach((n) => {
    const d = depth.get(n.id) ?? 0;
    if (!layers.has(d)) layers.set(d, []);
    layers.get(d)!.push(n.id);
  });

  const pos = new Map<string, { x: number; y: number }>();
  layers.forEach((ids, d) => {
    ids.forEach((id, idx) => {
      pos.set(id, { x: d * LAYER_GAP, y: idx * ROW_GAP });
    });
  });
  return pos;
}

export function WorkflowCanvas({ nodes, onSelect }: { nodes: Node[]; onSelect?: (id: string) => void }) {
  const { rfNodes, rfEdges } = useMemo(() => {
    const pos = layout(nodes);
    const rfNodes: RFNode[] = nodes.map((n) => ({
      id: n.id,
      type: 'research',
      position: pos.get(n.id) ?? { x: 0, y: 0 },
      data: { node: n },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      style: { width: NODE_W, background: 'transparent', border: 'none', padding: 0 },
    }));
    const rfEdges: Edge[] = [];
    nodes.forEach((n) => {
      (n.depends_on ?? []).forEach((dep) => {
        if (!nodes.some((x) => x.id === dep)) return;
        const running = n.status === 'running';
        rfEdges.push({
          id: `${dep}->${n.id}`,
          source: dep,
          target: n.id,
          animated: running,
          style: { stroke: running ? '#3b82f6' : '#31405f', strokeWidth: 1.6 },
          markerEnd: { type: MarkerType.ArrowClosed, color: '#31405f', width: 16, height: 16 },
        });
      });
    });
    return { rfNodes, rfEdges };
  }, [nodes]);

  if (nodes.length === 0) {
    return <div className="empty">工作流尚未生成</div>;
  }

  return (
    <div className="canvas-wrap">
      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        proOptions={{ hideAttribution: true }}
        onNodeClick={(_, node) => onSelect?.(node.id)}
        nodesDraggable={false}
        nodesConnectable={false}
        minZoom={0.35}
      >
        <Background color="#1b2440" gap={22} size={1} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
