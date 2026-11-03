// 展示层格式化工具：只做确定性转换，不制造数据。

export function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}

export function formatDateTime(iso?: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}/${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function formatDate(iso?: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}/${pad(d.getMonth() + 1)}/${pad(d.getDate())}`;
}

export function formatTime(iso?: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function formatRelative(iso?: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso).getTime();
  if (Number.isNaN(d)) return '—';
  const diff = Date.now() - d;
  if (diff < 0) return '刚刚';
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec} 秒前`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} 分钟前`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} 小时前`;
  const day = Math.floor(hr / 24);
  if (day < 30) return `${day} 天前`;
  return formatDate(iso);
}

export function formatDurationMs(ms?: number | null): string {
  if (ms == null || !Number.isFinite(ms) || ms < 0) return '—';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)} s`;
  const m = Math.floor(s / 60);
  const rem = Math.round(s % 60);
  if (m < 60) return `${m} 分 ${rem} 秒`;
  const h = Math.floor(m / 60);
  return `${h} 小时 ${m % 60} 分`;
}

// Formats Go duration strings such as "3m0s" or "1m30s" for display.
export function formatGoDuration(value?: string | null): string {
  if (!value) return '—';
  const units: Record<string, number> = { ms: 1, h: 3600000, m: 60000, s: 1000 };
  const re = /(\d+(?:\.\d+)?)(ms|h|m|s)/g;
  let total = 0;
  let matched = false;
  let match: RegExpExecArray | null;
  while ((match = re.exec(value)) !== null) {
    matched = true;
    total += Number(match[1]) * units[match[2]];
  }
  return matched ? formatDurationMs(total) : value;
}

export function durationBetween(start?: string | null, end?: string | null): string {
  if (!start) return '—';
  const a = new Date(start).getTime();
  const b = end ? new Date(end).getTime() : Date.now();
  if (Number.isNaN(a) || Number.isNaN(b)) return '—';
  return formatDurationMs(Math.max(0, b - a));
}

export function hostOf(url?: string | null): string {
  if (!url) return '—';
  try {
    return new URL(url).host.replace(/^www\./, '');
  } catch {
    return url.replace(/^https?:\/\//, '').split('/')[0] || '—';
  }
}

export function truncate(text: string | null | undefined, max: number): string {
  if (!text) return '';
  if (text.length <= max) return text;
  return `${text.slice(0, max).trimEnd()}…`;
}

export function pct(done: number, total: number): number {
  if (!total || total <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((done / total) * 100)));
}

export function num(v: unknown, fallback = 0): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
}

export function shortId(id?: string | null): string {
  if (!id) return '—';
  return id.slice(0, 8);
}

export const DEPTH_TEXT: Record<string, string> = {
  quick: '快速',
  standard: '标准',
  deep: '深度',
};

export const PLANNER_TEXT: Record<string, string> = {
  heuristic: '本地启发式',
  llm: '大模型规划',
};

export const TASK_STATUS_TEXT: Record<string, string> = {
  created: '已创建',
  planning: '规划中',
  awaiting_approval: '待批准',
  running: '执行中',
  paused: '已暂停',
  synthesizing: '生成报告',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
};

export const NODE_STATUS_TEXT: Record<string, string> = {
  pending: '待执行',
  queued: '排队中',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  skipped: '已跳过',
  cancelled: '已取消',
};

export const SOURCE_STATUS_TEXT: Record<string, string> = {
  ok: '已抓取',
  discovered: '待抓取',
  fetch_failed: '抓取失败',
  robots_blocked: '协议禁止',
};

export const CLAIM_STATUS_TEXT: Record<string, string> = {
  supported: '已支持',
  partially_supported: '部分支持',
  conflicting: '存在冲突',
  unverified: '未验证',
};

export const AGENT_STATUS_TEXT: Record<string, string> = {
  pending: '等待',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
};

export const CREDIBILITY_TEXT: Record<string, string> = {
  high: '高',
  medium: '中',
  low: '低',
  unknown: '未评估',
};

export const NODE_TYPE_TEXT: Record<string, string> = {
  search: '检索',
  analyze: '分析',
  verify: '验证',
  synthesize: '综述',
  fetch: '抓取',
};

export const RISK_TEXT: Record<string, string> = {
  low: '低风险',
  medium: '中风险',
  high: '高风险',
};

export function taskStatusText(status: string): string {
  return TASK_STATUS_TEXT[status] ?? status;
}

export function nodeStatusText(status: string): string {
  return NODE_STATUS_TEXT[status] ?? status;
}

export function claimStatusText(status: string): string {
  return CLAIM_STATUS_TEXT[status] ?? status;
}

export function claimStatusTone(status: string): 'ok' | 'warn' | 'err' | 'neutral' {
  switch (status) {
    case 'supported':
      return 'ok';
    case 'partially_supported':
      return 'warn';
    case 'conflicting':
      return 'err';
    default:
      return 'neutral';
  }
}

export function sourceStatusTone(status: string): 'ok' | 'warn' | 'err' | 'neutral' {
  switch (status) {
    case 'ok':
      return 'ok';
    case 'robots_blocked':
      return 'warn';
    case 'fetch_failed':
      return 'err';
    default:
      return 'neutral';
  }
}

export function taskStatusTone(status: string): 'ok' | 'warn' | 'err' | 'info' | 'accent' | 'neutral' {
  switch (status) {
    case 'completed':
      return 'ok';
    case 'failed':
      return 'err';
    case 'paused':
    case 'awaiting_approval':
      return 'warn';
    case 'running':
      return 'accent';
    case 'planning':
    case 'synthesizing':
      return 'info';
    default:
      return 'neutral';
  }
}

export function nodeStatusTone(status: string): 'ok' | 'warn' | 'err' | 'info' | 'accent' | 'neutral' {
  switch (status) {
    case 'completed':
      return 'ok';
    case 'failed':
      return 'err';
    case 'running':
      return 'accent';
    case 'queued':
      return 'info';
    default:
      return 'neutral';
  }
}

export function agentStatusTone(status: string): 'ok' | 'warn' | 'err' | 'accent' | 'neutral' {
  switch (status) {
    case 'completed':
      return 'ok';
    case 'failed':
      return 'err';
    case 'running':
      return 'accent';
    case 'pending':
      return 'warn';
    default:
      return 'neutral';
  }
}

export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}