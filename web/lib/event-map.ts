// EVENT_PROTOCOL.md 规定的前端事件映射表：组件里禁止散落硬编码事件名。
// 新增事件类型必须同时更新这里、EVENT_PROTOCOL.md 与后端常量。

export type EventCategory =
  | 'research' | 'workflow' | 'node' | 'agent' | 'tool' | 'evidence' | 'report';

export interface EventMeta {
  category: EventCategory;
  label: string;
  tone: 'info' | 'ok' | 'warn' | 'err';
}

export const EVENT_MAP: Record<string, EventMeta> = {
  'research.created': { category: 'research', label: '任务已创建', tone: 'info' },
  'research.planning': { category: 'research', label: '规划中', tone: 'info' },
  'research.planned': { category: 'research', label: '规划完成', tone: 'ok' },
  'research.started': { category: 'research', label: '开始执行', tone: 'info' },
  'research.paused': { category: 'research', label: '已暂停', tone: 'warn' },
  'research.resumed': { category: 'research', label: '已恢复', tone: 'info' },
  'research.completed': { category: 'research', label: '研究完成', tone: 'ok' },
  'research.failed': { category: 'research', label: '研究失败', tone: 'err' },
  'research.synthesizing': { category: 'research', label: '收尾综述', tone: 'info' },
  'research.cancelled': { category: 'research', label: '已取消', tone: 'err' },
  'research.transition_rejected': { category: 'research', label: '状态跃迁被拒绝', tone: 'warn' },

  'workflow.created': { category: 'workflow', label: '工作流已生成', tone: 'ok' },
  'workflow.updated': { category: 'workflow', label: '工作流更新', tone: 'info' },

  'node.queued': { category: 'node', label: '节点入队', tone: 'info' },
  'node.started': { category: 'node', label: '节点开始', tone: 'info' },
  'node.progress': { category: 'node', label: '节点进度', tone: 'info' },
  'node.completed': { category: 'node', label: '节点完成', tone: 'ok' },
  'node.failed': { category: 'node', label: '节点失败', tone: 'err' },
  'node.retrying': { category: 'node', label: '节点重试', tone: 'warn' },

  'agent.created': { category: 'agent', label: 'Agent 创建', tone: 'info' },
  'agent.started': { category: 'agent', label: 'Agent 启动', tone: 'info' },
  'agent.completed': { category: 'agent', label: 'Agent 完成', tone: 'ok' },
  'agent.failed': { category: 'agent', label: 'Agent 失败', tone: 'err' },

  'tool.invoked': { category: 'tool', label: '工具调用', tone: 'info' },
  'tool.completed': { category: 'tool', label: '工具返回', tone: 'ok' },
  'tool.failed': { category: 'tool', label: '工具失败', tone: 'err' },

  'evidence.created': { category: 'evidence', label: '新增证据', tone: 'info' },
  'evidence.verified': { category: 'evidence', label: '证据核验', tone: 'ok' },

  'report.started': { category: 'report', label: '报告生成中', tone: 'info' },
  'report.completed': { category: 'report', label: '报告完成', tone: 'ok' },
};

export function eventMeta(type: string): EventMeta {
  return EVENT_MAP[type] ?? { category: 'node', label: type, tone: 'info' };
}

export const STATUS_TEXT: Record<string, string> = {
  created: '已创建',
  planning: '规划中',
  awaiting_approval: '待批准',
  running: '执行中',
  paused: '已暂停',
  synthesizing: '生成报告',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
  pending: '待执行',
  queued: '排队中',
  skipped: '已跳过',
};

export function statusText(s: string): string {
  return STATUS_TEXT[s] ?? s;
}
