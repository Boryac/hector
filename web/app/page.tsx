'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useCallback, useEffect, useState } from 'react';
import { apiGet, apiSend, ApiError, getToken, setToken } from '@/lib/api';
import { statusText } from '@/lib/event-map';
import type { Health, Task } from '@/lib/types';

export default function DashboardPage() {
  const router = useRouter();
  const [health, setHealth] = useState<Health | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [total, setTotal] = useState(0);
  const [goal, setGoal] = useState('研究全球 AI Agent 开源生态，筛选 10 个代表性项目并生成竞争分析报告');
  const [depth, setDepth] = useState('standard');
  const [verify, setVerify] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [token, setTokenState] = useState('');

  useEffect(() => setTokenState(getToken()), []);

  const refresh = useCallback(async () => {
    try {
      const [h, list] = await Promise.all([
        apiGet<Health>('/api/v1/health'),
        apiGet<{ items: Task[]; total: number }>('/api/v1/research?limit=20'),
      ]);
      setHealth(h);
      setTasks(list.items ?? []);
      setTotal(list.total ?? 0);
      setError('');
    } catch (e) {
      setError(e instanceof ApiError ? `${e.message}（${e.code}）` : String(e));
    }
  }, []);

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 4000);
    return () => clearInterval(t);
  }, [refresh]);

  const submit = async () => {
    if (goal.trim().length < 4) {
      setError('研究目标至少 4 个字符');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const res = await apiSend<{ task: Task }>('POST', '/api/v1/research', {
        goal: goal.trim(),
        depth,
        enable_verification: verify,
      });
      router.push(`/task/?id=${res.task.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? `${e.message}（${e.code}）` : String(e));
    } finally {
      setBusy(false);
    }
  };

  const healthTone = health?.status === 'ok' ? 'ok' : health?.status === 'down' ? 'err' : 'warn';

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <span className="dot" />
          Hector ResearchOS
          <small>自主研究工作台</small>
        </div>
        <div className="spacer" />
        <span className={`pill ${healthTone}`}>
          <span className="led" />
          {health ? `服务 ${health.status}` : '连接中'}
        </span>
        {health && (
          <span className="pill">
            <span className="led" />
            LLM {health.checks.llm.enabled ? health.checks.llm.model || '已配置' : '未配置'}
          </span>
        )}
        {health && (
          <span className="pill">
            <span className="led" />
            检索 {health.checks.search.providers.join(' / ') || '无'}
          </span>
        )}
        <input
          style={{ width: 190 }}
          value={token}
          onChange={(e) => {
            setTokenState(e.target.value);
            setToken(e.target.value);
          }}
          placeholder="API Token"
          title="本地单用户模式下的访问令牌（RESEARCHOS_API_TOKEN）"
        />
      </header>

      <div className="layout">
        <div>
          <div className="card">
            <h2>新建研究任务</h2>
            <div className="field">
              <label>研究目标</label>
              <textarea rows={4} value={goal} onChange={(e) => setGoal(e.target.value)} />
            </div>
            <div className="row">
              <div className="field">
                <label>研究深度</label>
                <select value={depth} onChange={(e) => setDepth(e.target.value)}>
                  <option value="quick">quick · 2 个问题</option>
                  <option value="standard">standard · 3 个问题</option>
                  <option value="deep">deep · 4 个问题</option>
                </select>
              </div>
              <div className="field">
                <label>交叉验证</label>
                <select value={verify ? 'on' : 'off'} onChange={(e) => setVerify(e.target.value === 'on')}>
                  <option value="on">开启</option>
                  <option value="off">关闭</option>
                </select>
              </div>
            </div>
            <button className="primary" onClick={submit} disabled={busy} style={{ width: '100%', marginTop: 4 }}>
              {busy ? '规划中…' : '创建并规划'}
            </button>
            {error && <div className="err-text" style={{ marginTop: 10 }}>{error}</div>}
            <p className="small muted" style={{ marginTop: 12, marginBottom: 0 }}>
              创建后先由 Planner 拆解 DAG，状态进入「待批准」；批准后 Worker 才会真实检索、抓取与生成报告。
            </p>
          </div>

          {health && (
            <div className="card">
              <h2>运行环境</h2>
              <div className="small">
                <div>版本 <span className="mono">{health.version}</span> · 运行 {Math.round(health.uptime_s / 60)} 分钟</div>
                <div>数据库：{health.checks.database.ok ? `正常 (${health.checks.database.latency_ms ?? 0}ms)` : health.checks.database.error}</div>
                <div>迁移：<span className="mono">{health.checks.database.migrations?.join(', ') || '—'}</span></div>
                {health.checks.llm.note && <div className="muted">{health.checks.llm.note}</div>}
                <div>身份模式：{health.checks.auth.mode}</div>
              </div>
            </div>
          )}
        </div>

        <div>
          <div className="card">
            <h2>研究任务（{total}）</h2>
            {tasks.length === 0 && <div className="empty">还没有任务，先在左侧创建。</div>}
            {tasks.map((t) => {
              const nodeTotal = Number(t.stats?.nodes_total ?? 0) || 0;
              const done = Number(t.stats?.nodes_done ?? 0) || 0;
              const pct = nodeTotal > 0 ? Math.round((done / nodeTotal) * 100) : 0;
              return (
                <Link key={t.id} href={`/task/?id=${t.id}`} className="task-item">
                  <div className="task-title">{t.title || t.goal}</div>
                  <div className="meta">
                    <span className={`badge ${t.status}`}>{statusText(t.status)}</span>
                    <span>{t.depth}</span>
                    <span>规划 {t.planner_mode}</span>
                    <span className="mono">{t.id.slice(0, 8)}</span>
                    <span>{new Date(t.created_at).toLocaleString('zh-CN')}</span>
                    {pct > 0 && <span>{pct}%</span>}
                  </div>
                  {nodeTotal > 0 && <div className="progress"><i style={{ width: `${pct}%` }} /></div>}
                </Link>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
