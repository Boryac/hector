'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { apiGet, apiSend } from '@/lib/api';
import { cx, errorText, formatDateTime } from '@/lib/format';
import { usePolling } from '@/lib/hooks';
import { Icon } from '@/components/Icon';
import {
  Button, EmptyState, ErrorState, Field, InlineNotice, Modal, PageHeader, Skeleton, StatusBadge, Switch,
} from '@/components/ui';
import { useToast } from '@/components/Toast';

/* -------------------------------------------------------------------------- */
/* Types                                                                       */
/* -------------------------------------------------------------------------- */

interface SettingsField {
  key: string;
  label: string;
  help?: string;
  kind: string;
  scope: 'hot' | 'restart' | 'deploy' | string;
  env: string;
  value: unknown;
  source: string;
  configured: boolean;
  default?: unknown;
  min?: number;
  max?: number;
  options?: string[];
  unit?: string;
  editable: boolean;
}

interface SettingsSectionData {
  id: string;
  title: string;
  description: string;
  fields: SettingsField[];
}

interface SettingsSnapshot {
  sections: SettingsSectionData[];
  secrets: { available: boolean; key_source: string; cipher: string };
  revision: number;
  last_error?: string;
  precedence: string;
  restart_needed: boolean;
  pending_restart?: string[];
}

interface PatchResponse {
  result: { warnings?: string[]; requires_restart?: string[]; message?: string };
  settings: SettingsSnapshot;
}

interface MCPServer {
  id: string;
  name: string;
  command: string;
  args: string | string[];
  status: string;
  approved: boolean;
  tools: Array<{ name: string; description?: string }>;
  tool_count: number;
  last_error?: string;
  created_at: string;
}

const SECTION_NAV = [
  { id: 'model', label: '模型' },
  { id: 'search', label: '检索' },
  { id: 'execution', label: '执行' },
  { id: 'system', label: '系统' },
  { id: 'mcp', label: 'MCP' },
];

const SOURCE_TEXT: Record<string, string> = {
  web: '界面保存',
  env: '环境变量',
  default: '默认值',
};

const DURATION_RE = /^(\d+(\.\d+)?(ms|s|m|h))+$/;

/* -------------------------------------------------------------------------- */
/* Field helpers                                                               */
/* -------------------------------------------------------------------------- */

function fieldInputValue(field: SettingsField, value: unknown): string {
  if (value === null || value === undefined) return '';
  if (Array.isArray(value)) return value.join(', ');
  return String(value);
}

function readonlyValue(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    if ('configured' in obj) return obj.configured ? '已配置（不回显）' : '未配置';
    return Object.entries(obj).map(([k, v]) => `${k}: ${String(v)}`).join(' · ') || '—';
  }
  return String(value) || '—';
}

function parseList(raw: string): string[] {
  return raw.split(/[,，\s]+/).map((s) => s.trim()).filter(Boolean);
}

function validate(field: SettingsField, value: unknown): string | null {
  if (field.kind === 'int') {
    const n = Number(value);
    if (!Number.isFinite(n) || !Number.isInteger(n)) return '请输入整数';
    if (typeof field.min === 'number' && n < field.min) return `不能小于 ${field.min}`;
    if (typeof field.max === 'number' && n > field.max) return `不能大于 ${field.max}`;
    if (typeof field.min !== 'number' && n < 0) return '不能为负数';
  }
  if (field.kind === 'duration') {
    const text = String(value ?? '').trim();
    if (!text) return '不能为空';
    if (!DURATION_RE.test(text)) return '格式如 30s、3m、1h30m';
  }
  if (field.kind === 'list') {
    if (!Array.isArray(value) || value.length === 0) return '至少要选一项';
  }
  if (field.kind === 'string' && field.key.endsWith('base_url')) {
    const text = String(value ?? '').trim();
    if (text && !/^https?:\/\//i.test(text)) return '需要以 http:// 或 https:// 开头';
  }
  return null;
}

function SectionFields({
  section, drafts, errors, onDraft, onReset, resetting,
}: {
  section: SettingsSectionData;
  drafts: Record<string, unknown>;
  errors: Record<string, string>;
  onDraft: (field: SettingsField, value: unknown) => void;
  onReset: (field: SettingsField) => void;
  resetting: string | null;
}) {
  return (
    <div className="settings-rows">
      {section.fields.map((field) => {
        const value = Object.prototype.hasOwnProperty.call(drafts, field.key) ? drafts[field.key] : field.value;
        const error = errors[field.key];
        const id = `setting-${field.key.replace(/\./g, '-')}`;
        const dirty = Object.prototype.hasOwnProperty.call(drafts, field.key);
        return (
          <div className={cx('settings-row', dirty && 'is-dirty', error && 'has-error')} key={field.key}>
            <div className="settings-label">
              <label htmlFor={id} className="settings-name">
                {field.label}
                {field.scope === 'restart' && <span className="settings-tag">需重启</span>}
                {field.scope === 'deploy' && <span className="settings-tag">只读</span>}
              </label>
              {field.help && <p className="settings-help">{field.help}</p>}
              <p className="settings-meta">
                <span>{SOURCE_TEXT[field.source] ?? field.source}</span>
                {field.source === 'web' && field.editable && (
                  <button
                    type="button"
                    className="link-btn"
                    disabled={resetting === field.key}
                    onClick={() => onReset(field)}
                  >
                    {resetting === field.key ? '恢复中…' : '恢复默认'}
                  </button>
                )}
                {field.kind === 'secret' && (
                  <span>{field.configured || field.source !== 'default' ? '已保存（留空不修改）' : '未配置'}</span>
                )}
              </p>
            </div>
            <div className="settings-control">
              {field.kind === 'bool' ? (
                <Switch compact label={field.label} checked={Boolean(value)} onChange={(next) => onDraft(field, next)} />
              ) : field.scope === 'deploy' || !field.editable ? (
                <div className="settings-readonly">{readonlyValue(value)}</div>
              ) : field.kind === 'enum' ? (
                <select id={id} className="select" value={String(value ?? '')} onChange={(e) => onDraft(field, e.target.value)}>
                  {(field.options ?? []).map((option) => <option key={option} value={option}>{option}</option>)}
                </select>
              ) : field.kind === 'list' && field.options?.length ? (
                <div className="check-group" role="group" aria-label={field.label}>
                  {field.options.map((option) => {
                    const list = Array.isArray(value) ? (value as string[]) : [];
                    const checked = list.includes(option);
                    return (
                      <label key={option} className={cx('check-chip', checked && 'is-checked')}>
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => onDraft(field, checked ? list.filter((item) => item !== option) : [...list, option])}
                        />
                        {option}
                      </label>
                    );
                  })}
                </div>
              ) : field.kind === 'int' ? (
                <input
                  id={id}
                  type="number"
                  className="input"
                  value={fieldInputValue(field, value)}
                  min={field.min}
                  max={field.max}
                  onChange={(e) => onDraft(field, e.target.value === '' ? '' : Number(e.target.value))}
                />
              ) : field.kind === 'secret' ? (
                <input
                  id={id}
                  type="password"
                  className="input"
                  autoComplete="new-password"
                  placeholder={field.configured ? '已保存，留空保持不变' : '未配置'}
                  value={fieldInputValue(field, value)}
                  onChange={(e) => onDraft(field, e.target.value)}
                />
              ) : field.kind === 'list' ? (
                <input
                  id={id}
                  className="input"
                  value={fieldInputValue(field, value)}
                  placeholder="用逗号分隔"
                  onChange={(e) => onDraft(field, parseList(e.target.value))}
                />
              ) : (
                <input
                  id={id}
                  className="input"
                  value={fieldInputValue(field, value)}
                  placeholder={field.kind === 'duration' ? '如 3m、30s、1h' : undefined}
                  onChange={(e) => onDraft(field, e.target.value)}
                />
              )}
              {field.unit && field.unit !== 'duration' && field.kind !== 'duration' && (
                <span className="settings-unit">{field.unit}</span>
              )}
              {error && <p className="settings-error">{error}</p>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Connection tests                                                            */
/* -------------------------------------------------------------------------- */

interface LlmTestResult {
  ok: boolean;
  reason?: string;
  message?: string;
  provider?: string;
  model?: string;
  latency_ms?: number;
  reply?: string;
}

function LlmTester() {
  const { push } = useToast();
  const [result, setResult] = useState<LlmTestResult | null>(null);
  const [running, setRunning] = useState(false);

  const run = async () => {
    setRunning(true);
    setResult(null);
    try {
      setResult(await apiSend<LlmTestResult>('POST', '/api/v1/settings/llm/test', {}));
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="settings-test">
      <div className="settings-test-head">
        <div>
          <p className="settings-test-title">真实连接测试</p>
          <p className="settings-test-desc">使用已保存的配置发起一次最小请求，不写入任何设置。</p>
        </div>
        <Button size="sm" variant="secondary" loading={running} onClick={() => void run()}>测试连接</Button>
      </div>
      {result && (
        <InlineNotice tone={result.ok ? 'ok' : 'warn'} title={result.message ?? (result.ok ? '连接正常' : '连接失败')}>
          {result.ok ? (
            <span>
              {result.provider} · {result.model}
              {typeof result.latency_ms === 'number' && ` · ${result.latency_ms}ms`}
              {result.reply && ` · 回复「${result.reply}」`}
            </span>
          ) : (
            <span>{result.reason === 'provider_none' ? '当前 Provider 为 none，规划与抽取走本地确定性流程。' : result.message}</span>
          )}
        </InlineNotice>
      )}
    </div>
  );
}

interface SearchTrace { provider: string; ok: boolean; count: number; latency_ms: number; error?: string }
interface SearchTestResult { ok: boolean; query: string; total: number; latency_ms: number; providers: SearchTrace[]; message: string }

function SearchTester() {
  const { push } = useToast();
  const [result, setResult] = useState<SearchTestResult | null>(null);
  const [running, setRunning] = useState(false);

  const run = async () => {
    setRunning(true);
    setResult(null);
    try {
      setResult(await apiSend<SearchTestResult>('POST', '/api/v1/settings/search/test', { query: 'vector database benchmark' }));
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="settings-test">
      <div className="settings-test-head">
        <div>
          <p className="settings-test-title">检索连通性</p>
          <p className="settings-test-desc">用「vector database benchmark」实际检索一次，逐个 Provider 返回结果数与耗时。</p>
        </div>
        <Button size="sm" variant="secondary" loading={running} onClick={() => void run()}>测试检索</Button>
      </div>
      {result && (
        <div className="test-traces">
          <p className={cx('test-summary', result.ok ? 'is-ok' : 'is-warn')}>{result.message} · 共 {result.total} 条 · {result.latency_ms}ms</p>
          <div className="test-trace-list">
            {result.providers.map((trace) => (
              <div className="test-trace" key={trace.provider}>
                <span className="test-trace-name">{trace.provider}</span>
                <span className={cx('test-trace-state', trace.ok ? 'is-ok' : 'is-err')}>{trace.ok ? `${trace.count} 条` : '失败'}</span>
                <span className="test-trace-time">{trace.latency_ms}ms</span>
                {trace.error && <span className="test-trace-error">{trace.error}</span>}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* MCP                                                                         */
/* -------------------------------------------------------------------------- */

function McpSection() {
  const { push } = useToast();
  const serversQ = usePolling<{ items: MCPServer[] }>(() => apiGet<{ items: MCPServer[] }>('/api/v1/tools/servers'), { key: 'mcp-servers' });
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState('');
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState('');
  const [env, setEnv] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<MCPServer | null>(null);

  const servers = serversQ.data?.items ?? [];

  const register = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !command.trim()) {
      push('名称与命令必填', 'warn');
      return;
    }
    let parsedEnv: Record<string, string> = {};
    if (env.trim()) {
      try {
        const raw = JSON.parse(env);
        if (typeof raw !== 'object' || Array.isArray(raw)) throw new Error('env 需要是对象');
        parsedEnv = Object.fromEntries(Object.entries(raw).map(([k, v]) => [k, String(v)]));
      } catch (err) {
        push(`环境变量 JSON 无效：${errorText(err)}`, 'err');
        return;
      }
    }
    setSubmitting(true);
    try {
      await apiSend('POST', '/api/v1/tools/servers', {
        name: name.trim(),
        command: command.trim(),
        args: args.trim() ? args.trim().split(/\s+/) : [],
        env: parsedEnv,
      });
      push('已登记，需批准后才能启动', 'ok');
      setName(''); setCommand(''); setArgs(''); setEnv(''); setAdding(false);
      serversQ.refresh();
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setSubmitting(false);
    }
  };

  const approve = async (server: MCPServer) => {
    setBusyId(server.id);
    try {
      await apiSend('POST', `/api/v1/tools/servers/${server.id}/approve`, { approved: !server.approved });
      push(server.approved ? '已撤销批准并停止' : '已批准，可做工具发现', 'ok');
      serversQ.refresh();
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusyId(null);
    }
  };

  const discover = async (server: MCPServer) => {
    setBusyId(server.id);
    try {
      const res = await apiSend<{ ok: boolean; message?: string; tool_count?: number }>(
        'POST', `/api/v1/tools/servers/${server.id}/discover`,
      );
      push(res.ok ? `发现 ${res.tool_count ?? 0} 个工具` : (res.message ?? '发现失败'), res.ok ? 'ok' : 'warn');
      serversQ.refresh();
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async () => {
    if (!pendingDelete) return;
    setBusyId(pendingDelete.id);
    try {
      await apiSend('DELETE', `/api/v1/tools/servers/${pendingDelete.id}`);
      push('已删除登记', 'ok');
      setPendingDelete(null);
      serversQ.refresh();
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <section className="card settings-card" id="section-mcp">
      <div className="card-head">
        <div>
          <h2 className="card-title">MCP Servers</h2>
          <p className="card-desc">外部工具通过 MCP 协议接入。登记不会拉起进程；必须显式批准后才能启动与发现工具。</p>
        </div>
        <Button size="sm" variant="secondary" icon={<Icon name="plus" size={13} />} onClick={() => setAdding((v) => !v)}>
          {adding ? '取消' : '登记 Server'}
        </Button>
      </div>

      {adding && (
        <form className="mcp-form" onSubmit={(e) => void register(e)}>
          <div className="form-grid">
            <Field label="名称" htmlFor="mcp-name">
              <input id="mcp-name" className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="filesystem" />
            </Field>
            <Field label="命令" htmlFor="mcp-command" hint="可执行文件的绝对路径或 PATH 中的命令">
              <input id="mcp-command" className="input" value={command} onChange={(e) => setCommand(e.target.value)} placeholder="npx" />
            </Field>
          </div>
          <Field label="参数" htmlFor="mcp-args" hint="以空格分隔，参数内含空格时请谨慎">
            <input id="mcp-args" className="input" value={args} onChange={(e) => setArgs(e.target.value)} placeholder="-y @modelcontextprotocol/server-filesystem C:\\data" />
          </Field>
          <Field label="环境变量（JSON）" htmlFor="mcp-env" hint="只写入登记元数据，请勿在此填写长期密钥">
            <textarea id="mcp-env" className="input textarea" rows={3} value={env} onChange={(e) => setEnv(e.target.value)} placeholder={'{"LOG_LEVEL":"info"}'} />
          </Field>
          <div className="row" style={{ justifyContent: 'flex-end', gap: 8 }}>
            <Button size="sm" variant="quiet" type="button" onClick={() => setAdding(false)}>取消</Button>
            <Button size="sm" variant="primary" type="submit" loading={submitting}>登记</Button>
          </div>
        </form>
      )}

      {serversQ.error ? (
        <ErrorState message={errorText(serversQ.error)} onRetry={() => serversQ.refresh()} />
      ) : serversQ.loading && servers.length === 0 ? (
        <div className="skeleton-block">{Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} height={56} />)}</div>
      ) : servers.length === 0 ? (
        <EmptyState
          title="尚未接入 MCP Server"
          description="登记后先批准，再执行工具发现。外部进程不会常驻，工具清单按需刷新。"
        />
      ) : (
        <div className="mcp-list">
          {servers.map((server) => {
            const argsText = Array.isArray(server.args) ? server.args.join(' ') : server.args;
            return (
              <div className="mcp-item" key={server.id}>
                <div className="mcp-main">
                  <div className="mcp-title-row">
                    <span className="mcp-name">{server.name}</span>
                    <StatusBadge status={server.status} kind="server" />
                    {!server.approved && <span className="badge badge-warn">未批准</span>}
                    <span className="muted small">{server.tool_count} 个工具</span>
                  </div>
                  <code className="mcp-command">{server.command} {argsText}</code>
                  {server.last_error && <p className="mcp-error">{server.last_error}</p>}
                </div>
                <div className="mcp-actions">
                  <Button size="sm" variant="ghost" loading={busyId === server.id} onClick={() => void approve(server)}>
                    {server.approved ? '撤销批准' : '批准'}
                  </Button>
                  <Button size="sm" variant="ghost" disabled={!server.approved} loading={busyId === server.id} onClick={() => void discover(server)}>发现工具</Button>
                  <Button size="sm" variant="ghost" onClick={() => setPendingDelete(server)}>删除</Button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <div className="notice" style={{ marginTop: 14 }}>
        <Icon name="info" size={14} />
        <span><strong>安全边界</strong>MCP 工具来自外部进程，可能读写文件或发起网络请求。批准即表示允许在后续任务中启动该进程；撤销批准会同时停止。工具调用记录保留在任务详情中。</span>
      </div>

      <Modal
        open={Boolean(pendingDelete)}
        onClose={() => setPendingDelete(null)}
        title="删除 MCP Server"
        description={`将删除「${pendingDelete?.name ?? ''}」的登记与工具缓存，不会影响已经生成的研究。`}
        footer={
          <>
            <Button variant="secondary" size="sm" onClick={() => setPendingDelete(null)}>取消</Button>
            <Button variant="danger" size="sm" loading={Boolean(busyId)} onClick={() => void remove()}>删除</Button>
          </>
        }
      >
        <p className="muted">删除后如仍需要，可以重新登记并批准。</p>
      </Modal>
    </section>
  );
}

/* -------------------------------------------------------------------------- */
/* Page                                                                        */
/* -------------------------------------------------------------------------- */

interface SystemInfo {
  version: string;
  uptime_s: number;
  runtime: { go: string; os: string; arch: string; cpus: number; goroutines: number };
  service: { addr: string; web_dir: string; data_dir: string; auth: string; user: string };
  database: { ok: boolean; latency_ms?: number; dsn?: string; migrations?: string[]; error?: string; pool?: { total: number; idle: number; acquired: number; max: number } };
  llm: { enabled: boolean; provider: string; model: string; key_set: boolean };
  search: { configured?: string[]; available?: string[] | null };
}

// SystemRuntime 展示 /api/v1/system 的运行时事实（版本、进程、连接池、迁移）。
// 部署级参数在上方字段里只读展示环境值；这里补齐「服务此刻的状态」，
// 让运维不必开 psql 或 curl 就能回答「现在跑的是什么、健康吗」。
function SystemRuntime() {
  const info = usePolling<SystemInfo>(() => apiGet<SystemInfo>('/api/v1/system'), { key: 'settings-system' });

  if (info.loading && !info.data) {
    return <div className="skeleton-block" style={{ marginTop: 14 }}>{Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} height={36} />)}</div>;
  }
  if (info.error || !info.data) {
    return <InlineNotice tone="err">运行环境信息不可用：{info.error ?? '未知错误'}</InlineNotice>;
  }
  const d = info.data;
  const rows: Array<[string, React.ReactNode]> = [
    ['版本', d.version || '—'],
    ['运行时长', `${Math.floor(d.uptime_s / 3600)}h ${Math.floor((d.uptime_s % 3600) / 60)}m`],
    ['Go / 平台', `${d.runtime.go} · ${d.runtime.os}/${d.runtime.arch} · ${d.runtime.cpus} 核`],
    ['goroutines', d.runtime.goroutines],
    ['监听地址', d.service.addr],
    ['身份', `${d.service.auth}（${d.service.user}）`],
    [
      '数据库',
      d.database.ok
        ? `正常 · ${d.database.latency_ms}ms · 连接池 ${d.database.pool?.acquired ?? 0}/${d.database.pool?.total ?? 0}（上限 ${d.database.pool?.max ?? '—'}）`
        : `异常：${d.database.error ?? '未知'}`,
    ],
    ['迁移', (d.database.migrations ?? []).join(', ') || '—'],
    ['规划 / 模型', d.llm.enabled ? `${d.llm.provider} · ${d.llm.model}` : '启发式（未配置大模型）'],
    ['检索源', (d.search.available ?? d.search.configured ?? []).join(', ') || '未配置'],
  ];
  return (
    <div className="card-body" style={{ paddingTop: 0 }}>
      <div className="sysinfo-grid" role="table" aria-label="运行环境">
        {rows.map(([label, value]) => (
          <div className="sysinfo-row" key={label} role="row">
            <span className="sysinfo-label">{label}</span>
            <span className={cx('sysinfo-value', label === '数据库' && !d.database.ok && 'sysinfo-bad')}>{value}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function SettingsPage() {
  const { push } = useToast();
  const [snapshot, setSnapshot] = useState<SettingsSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, unknown>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [resetting, setResetting] = useState<string | null>(null);
  const [lastSaved, setLastSaved] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setSnapshot(await apiGet<SettingsSnapshot>('/api/v1/settings'));
      setError(null);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const dirtyKeys = Object.keys(drafts);
  const dirtyCount = dirtyKeys.length;

  const setDraft = (field: SettingsField, value: unknown) => {
    setDrafts((prev) => {
      const next = { ...prev };
      if (JSON.stringify(value) === JSON.stringify(field.value ?? null)) delete next[field.key];
      else next[field.key] = value;
      return next;
    });
    setErrors((prev) => {
      if (!prev[field.key]) return prev;
      const next = { ...prev };
      delete next[field.key];
      return next;
    });
  };

  const resetField = async (field: SettingsField) => {
    setResetting(field.key);
    try {
      await apiSend('DELETE', `/api/v1/settings/${field.key.split('.')[0]}/${field.key}`);
      setDrafts((prev) => { const next = { ...prev }; delete next[field.key]; return next; });
      await load();
      push('已恢复为环境变量或默认值', 'ok');
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setResetting(null);
    }
  };

  const save = async () => {
    if (!snapshot || dirtyCount === 0) return;
    const nextErrors: Record<string, string> = {};
    const bySection: Record<string, Record<string, unknown>> = {};
    for (const section of snapshot.sections) {
      for (const field of section.fields) {
        if (!Object.prototype.hasOwnProperty.call(drafts, field.key)) continue;
        const value = drafts[field.key];
        const message = validate(field, value);
        if (message) nextErrors[field.key] = message;
        else {
          bySection[section.id] = bySection[section.id] ?? {};
          bySection[section.id][field.key] = value;
        }
      }
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      push('有配置项未通过校验', 'warn');
      return;
    }

    setSaving(true);
    const warnings: string[] = [];
    const restarts: string[] = [];
    try {
      for (const [section, values] of Object.entries(bySection)) {
        const res = await apiSend<PatchResponse>('PUT', `/api/v1/settings/${section}`, { values });
        warnings.push(...(res.result?.warnings ?? []));
        restarts.push(...(res.result?.requires_restart ?? []));
        setSnapshot(res.settings);
      }
      setDrafts({});
      setLastSaved(new Date().toISOString());
      setErrors({});
      if (restarts.length > 0) push(`已保存，${restarts.length} 项需重启后生效`, 'warn');
      else if (warnings.length > 0) push(`已保存：${warnings[0]}`, 'warn');
      else push('设置已保存并即时生效', 'ok');
    } catch (err) {
      push(errorText(err), 'err');
    } finally {
      setSaving(false);
    }
  };

  const sections = snapshot?.sections ?? [];
  const activeIds = useMemo(() => new Set(sections.map((s) => s.id)), [sections]);

  if (loading && !snapshot) {
    return (
      <div className="page">
        <PageHeader eyebrow="Settings" title="设置" description="模型、检索、执行与外部工具的运行配置。" />
        <div className="card"><div className="card-body"><div className="skeleton-block">{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} height={44} />)}</div></div></div>
      </div>
    );
  }

  if (error && !snapshot) {
    return (
      <div className="page">
        <PageHeader eyebrow="Settings" title="设置" description="模型、检索、执行与外部工具的运行配置。" />
        <ErrorState message={error} onRetry={() => void load()} />
      </div>
    );
  }

  return (
    <div className="page settings-page">
      <PageHeader
        eyebrow="Settings"
        title="设置"
        description="这里的改动写入数据库，重启后依然有效。标注「需重启」的项在重启后生效；部署级参数只读，只能改环境变量。"
        meta={
          <>
            <span>配置版本 #{snapshot?.revision ?? 0}</span>
            <span>密钥保护 {snapshot?.secrets.available ? snapshot.secrets.cipher : '不可用'}</span>
            <span>{snapshot?.restart_needed ? '有配置等待重启生效' : '全部配置已生效'}</span>
          </>
        }
        actions={
          <Button size="sm" variant="secondary" icon={<Icon name="retry" size={13} />} onClick={() => void load()}>刷新</Button>
        }
      />

      {snapshot?.last_error && (
        <div className="notice notice-warn" style={{ marginBottom: 14 }}>
          <Icon name="alert" size={14} /><span><strong>最近一次保存失败</strong>{snapshot.last_error}</span>
        </div>
      )}

      <div className="settings-layout">
        <nav className="settings-nav" aria-label="设置分组">
          {SECTION_NAV.map((item) => (
            <a key={item.id} href={`#section-${item.id}`} className={cx('settings-nav-item', !activeIds.has(item.id) && item.id !== 'mcp' && 'is-disabled')}>
              {item.label}
            </a>
          ))}
          <div className="settings-nav-foot">
            <p className="settings-nav-note">{snapshot?.precedence}</p>
            <p className="settings-nav-note">主密钥来源 {snapshot?.secrets.key_source || '未配置'}</p>
            {lastSaved && <p className="settings-nav-note">最近保存 {formatDateTime(lastSaved)}</p>}
          </div>
        </nav>

        <div className="settings-content">
          {sections.map((section) => (
            <section className="card settings-card" id={`section-${section.id}`} key={section.id}>
              <div className="card-head">
                <div>
                  <h2 className="card-title">{section.title}</h2>
                  <p className="card-desc">{section.description}</p>
                </div>
                {section.fields.some((f) => f.editable && f.scope !== 'deploy') && (
                  <span className="settings-scope">Web 值优先于环境变量</span>
                )}
              </div>

              <SectionFields
                section={section}
                drafts={drafts}
                errors={errors}
                onDraft={setDraft}
                onReset={(field) => void resetField(field)}
                resetting={resetting}
              />

              {section.id === 'model' && <LlmTester />}
              {section.id === 'search' && <SearchTester />}
              {section.id === 'system' && <SystemRuntime />}
            </section>
          ))}

          <McpSection />
        </div>
      </div>

      {dirtyCount > 0 && (
        <div className="savebar" role="region" aria-label="未保存的更改">
          <span className="savebar-text"><strong>{dirtyCount}</strong> 项未保存</span>
          <div className="savebar-actions">
            <Button size="sm" variant="quiet" disabled={saving} onClick={() => { setDrafts({}); setErrors({}); }}>还原</Button>
            <Button size="sm" variant="primary" loading={saving} onClick={() => void save()}>保存更改</Button>
          </div>
        </div>
      )}
    </div>
  );
}