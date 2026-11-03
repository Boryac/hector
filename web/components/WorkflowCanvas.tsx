'use client';

import React, { useEffect, useMemo, useState } from 'react';
import ReactFlow, {
  Background, BackgroundVariant, MarkerType, Position, ReactFlowProvider, useReactFlow,
  type Edge, type Node as FlowNode, type NodeProps,
} from 'reactflow';
import 'reactflow/dist/style.css';
import type { Node } from '@/lib/types';
import { cx, NODE_TYPE_TEXT, nodeStatusText } from '@/lib/format';

// Layered top-to-bottom: the workspace canvas is tall and narrow, so a vertical
// DAG keeps every node readable instead of shrinking the graph to fit the width.
const LAYER_GAP = 138;
const ROW_GAP = 216;

interface ResearchNodeData {
  node: Node;
}

function ResearchNode({ data, selected }: NodeProps<ResearchNodeData>) {
  const node = data.node;
  return (
    <div className={cx('rf-node', `status-${node.status}`, selected && 'is-selected')}>
      <div className="rf-node-head">
        <span className="rf-node-type">{NODE_TYPE_TEXT[node.type] ?? node.type}</span>
        <span className="rf-node-status">
          <i className={cx('dot', `status-${node.status}`)} aria-hidden="true" />
          {nodeStatusText(node.status)}
        </span>
      </div>
      <div className="rf-node-title" title={node.title}>{node.title}</div>
      {node.status === 'running' && (
        <div className="rf-node-progress" aria-hidden="true"><span /></div>
      )}
      <div className="rf-node-foot">
        <span className="rf-node-agent">{node.agent_role || 'agent'}</span>
        <span className="rf-node-attempts">{node.attempts}/{node.max_attempts}</span>
      </div>
    </div>
  );
}

const nodeTypes = { research: ResearchNode };

function layerize(nodes: Node[]): Map<string, number> {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const depth = new Map<string, number>();
  const visiting = new Set<string>();
  const resolve = (id: string): number => {
    if (depth.has(id)) return depth.get(id)!;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const node = byId.get(id);
    if (!node) return 0;
    const parents = (node.depends_on ?? []).filter((d) => byId.has(d));
    const level = parents.length === 0 ? 0 : Math.max(...parents.map(resolve)) + 1;
    depth.set(id, level);
    visiting.delete(id);
    return level;
  };
  nodes.forEach((n) => resolve(n.id));
  return depth;
}

function AutoFit({ signature, narrow }: { signature: string; narrow: boolean }) {
  const { fitView, setViewport } = useReactFlow();
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (narrow) {
        // Keep nodes legible on small screens and let the user pan the graph.
        void setViewport({ x: 14, y: 18, zoom: 0.68 }, { duration: 0 });
        return;
      }
      void fitView({ padding: 0.1, duration: 320, maxZoom: 1 });
    }, 80);
    return () => window.clearTimeout(timer);
  }, [signature, narrow, fitView, setViewport]);
  return null;
}

function useNarrow() {
  const [narrow, setNarrow] = useState(false);
  useEffect(() => {
    const query = window.matchMedia('(max-width: 620px)');
    const update = () => setNarrow(query.matches);
    update();
    query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, []);
  return narrow;
}

export function WorkflowCanvas({
  nodes,
  selectedId,
  onSelect,
}: {
  nodes: Node[];
  selectedId?: string | null;
  onSelect?: (node: Node) => void;
}) {
  const narrow = useNarrow();
  const { flowNodes, edges, signature } = useMemo(() => {
    const depth = layerize(nodes);
    const layers = new Map<number, Node[]>();
    nodes.forEach((n) => {
      const level = depth.get(n.id) ?? 0;
      const bucket = layers.get(level) ?? [];
      bucket.push(n);
      layers.set(level, bucket);
    });
    const maxCount = Math.max(1, ...Array.from(layers.values()).map((l) => l.length));

    const positioned: FlowNode<ResearchNodeData>[] = [];
    layers.forEach((bucket, level) => {
      const sorted = [...bucket].sort((a, b) => a.position - b.position);
      const offset = ((maxCount - sorted.length) * ROW_GAP) / 2;
      sorted.forEach((node, row) => {
        positioned.push({
          id: node.id,
          type: 'research',
          position: { x: offset + row * ROW_GAP, y: level * LAYER_GAP },
          data: { node },
          selected: node.id === selectedId,
          sourcePosition: Position.Bottom,
          targetPosition: Position.Top,
        });
      });
    });

    const runningIds = new Set(nodes.filter((n) => n.status === 'running' || n.status === 'queued').map((n) => n.id));
    const flowEdges: Edge[] = [];
    nodes.forEach((node) => {
      (node.depends_on ?? []).forEach((parent) => {
        if (!nodes.some((n) => n.id === parent)) return;
        flowEdges.push({
          id: `${parent}-${node.id}`,
          source: parent,
          target: node.id,
          type: 'smoothstep',
          className: runningIds.has(node.id) ? 'edge-active' : undefined,
          markerEnd: {
            type: MarkerType.ArrowClosed,
            width: 14,
            height: 14,
            color: runningIds.has(node.id) ? '#7d96a3' : '#c8cfcb',
          },
        });
      });
    });

    return {
      flowNodes: positioned,
      edges: flowEdges,
      signature: nodes.map((n) => n.id).join('|'),
    };
  }, [nodes, selectedId]);

  return (
    <div style={{ width: '100%', height: '100%' }}>
      <ReactFlowProvider>
        <ReactFlow
          nodes={flowNodes}
          edges={edges}
          nodeTypes={nodeTypes}
          minZoom={0.25}
          maxZoom={1.35}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable
          proOptions={{ hideAttribution: true }}
          onNodeClick={(_, node) => {
            const original = nodes.find((n) => n.id === node.id);
            if (original) onSelect?.(original);
          }}
          onPaneClick={() => undefined}
        >
          <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="#e1e3dd" />
          <AutoFit signature={signature} narrow={narrow} />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}