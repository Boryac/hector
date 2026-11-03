'use client';

import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { apiGet, getToken, setToken } from '@/lib/api';
import type { Health, Task } from '@/lib/types';
import { cx, formatRelative, truncate } from '@/lib/format';
import { usePolling } from '@/lib/hooks';
import { BrandMark, Icon, IconName } from './Icon';
import { Badge, Button, Field, Modal } from './ui';
import { useToast } from './Toast';
import { NewResearchDialog } from './NewResearchDialog';

interface AppActions {
  openNewResearch: () => void;
  openToken: () => void;
}

const AppContext = createContext<AppActions>({ openNewResearch: () => undefined, openToken: () => undefined });
export function useAppActions() {
  return useContext(AppContext);
}

const NAV: Array<{ group: string; items: Array<{ href: string; label: string; icon: IconName }> }> = [
  {
    group: '研究工作台',
    items: [
      { href: '/', label: '总览', icon: 'overview' },
      { href: '/tasks', label: '研究任务', icon: 'tasks' },
      { href: '/sources', label: '研究来源', icon: 'sources' },
      { href: '/evidence', label: '证据中心', icon: 'evidence' },
      { href: '/reports', label: '研究报告', icon: 'reports' },
    ],
  },
  {
    group: '系统',
    items: [
      { href: '/settings', label: '设置', icon: 'settings' },
    ],
  },
];

const PAGE_LABELS: Record<string, { section: string; page: string }> = {
  '/': { section: '研究工作台', page: '总览' },
  '/tasks': { section: '研究工作台', page: '研究任务' },
  '/task': { section: '研究工作台', page: '研究详情' },
  '/sources': { section: '研究工作台', page: '研究来源' },
  '/evidence': { section: '研究工作台', page: '证据中心' },
  '/reports': { section: '研究工作台', page: '研究报告' },
  '/settings': { section: '系统', page: '设置' },
};

function healthMeta(health: Health | null, error: string | null) {
  if (error) return { tone: 'err', label: '服务不可达' };
  if (!health) return { tone: 'neutral', label: '检测中' };
  if (health.status === 'ok') return { tone: 'ok', label: '服务正常' };
  if (health.status === 'degraded') return { tone: 'warn', label: '服务降级' };
  return { tone: 'err', label: '数据库异常' };
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const rawPathname = usePathname() || '/';
  // Static export with trailingSlash emits "/reports/"; normalize for labels and active state.
  const pathname = rawPathname.length > 1 ? rawPathname.replace(/\/+$/, '') : rawPathname;
  const router = useRouter();
  const { push } = useToast();
  const [newOpen, setNewOpen] = useState(false);
  const [tokenOpen, setTokenOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  const healthQ = usePolling<Health>(() => apiGet<Health>('/api/v1/health'), { intervalMs: 15000, key: 'shell-health' });
  const meta = healthMeta(healthQ.data, healthQ.error);
  const db = healthQ.data?.checks?.database;
  const llm = healthQ.data?.checks?.llm;
  const search = healthQ.data?.checks?.search;

  const actions = useMemo<AppActions>(() => ({
    openNewResearch: () => setNewOpen(true),
    openToken: () => setTokenOpen(true),
  }), []);

  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const label = PAGE_LABELS[pathname] ?? PAGE_LABELS[
    pathname.startsWith('/task') ? '/task' : pathname.startsWith('/sources') ? '/sources' : '/'
  ];

  return (
    <AppContext.Provider value={actions}>
      <div className="shell">
        <aside className={cx('sidebar', mobileOpen && 'is-open')}>
          <div className="side-inner">
            <Link href="/" className="brand" aria-label="Hector ResearchOS 首页">
              <BrandMark size={30} />
              <span className="brand-copy">
                <span className="brand-name">Hector</span>
                <span className="brand-sub">ResearchOS</span>
              </span>
            </Link>

            <nav className="nav" aria-label="主导航">
              {NAV.map((group) => (
                <div key={group.group} className="nav-group">
                  <p className="nav-group-label">{group.group}</p>
                  {group.items.map((item) => {
                    const active = item.href === '/' ? pathname === '/' : pathname === item.href || pathname.startsWith(`${item.href}/`);
                    return (
                      <Link key={item.href} href={item.href} className={cx('nav-item', active && 'is-active')}>
                        <Icon name={item.icon} size={16} className="nav-icon" />
                        <span>{item.label}</span>
                        {active && <span className="nav-active-mark" aria-hidden="true" />}
                      </Link>
                    );
                  })}
                </div>
              ))}
            </nav>

            <div className="side-bottom">
              <div className="side-status">
                <div className="side-status-head">
                  <span className={cx('status-dot', `status-${meta.tone}`)} aria-hidden="true" />
                  <span>{meta.label}</span>
                  <span className="side-status-version">{healthQ.data?.version ?? '—'}</span>
                </div>
                <p className="side-status-line">
                  {db?.ok ? `PostgreSQL · ${db.latency_ms ?? 0} ms` : 'PostgreSQL 未连接'}
                </p>
                <p className="side-status-line">
                  {llm?.enabled ? `LLM · ${llm.provider ?? '已配置'}` : 'LLM · 本地降级'}
                  <span className="dot-sep" aria-hidden="true">·</span>
                  {search?.providers?.length ?? 0} 个检索源
                </p>
              </div>

              <button type="button" className="side-user" onClick={() => setTokenOpen(true)}>
                <span className="avatar" aria-hidden="true">L</span>
                <span className="side-user-copy">
                  <span className="side-user-name">本地用户</span>
                  <span className="side-user-sub">单用户模式 · 管理令牌</span>
                </span>
                <Icon name="key" size={14} className="side-user-icon" />
              </button>
            </div>
          </div>
        </aside>

        {mobileOpen && <button type="button" className="sidebar-scrim" aria-label="关闭导航" onClick={() => setMobileOpen(false)} />}

        <div className="main">
          <header className="topbar">
            <div className="topbar-left">
              <button type="button" className="mobile-menu" aria-label="打开导航" onClick={() => setMobileOpen(true)}>
                <Icon name="menu" size={18} />
              </button>
              <div className="crumbs">
                <span>{label.section}</span>
                <Icon name="chevron-right" size={12} className="crumb-sep" />
                <span className="crumb-current">{label.page}</span>
              </div>
            </div>
            <div className="topbar-right">
              <Link href="/settings" className={cx('health-chip', `health-${meta.tone}`)} title={healthQ.error ?? undefined}>
                <span className={cx('status-dot', `status-${meta.tone}`)} aria-hidden="true" />
                <span>{meta.label}</span>
              </Link>
              <button type="button" className="search-trigger" onClick={() => setPaletteOpen(true)}>
                <Icon name="search" size={15} />
                <span>搜索</span>
                <kbd>⌘K</kbd>
              </button>
              <Button variant="primary" size="sm" icon={<Icon name="plus" size={15} />} onClick={() => setNewOpen(true)}>
                新建研究
              </Button>
            </div>
          </header>

          <main className="content">
            <div key={pathname} className="route-fade">
              {children}
            </div>
          </main>
        </div>
      </div>

      <NewResearchDialog open={newOpen} onClose={() => setNewOpen(false)} />
      <TokenDialog open={tokenOpen} onClose={() => setTokenOpen(false)} />
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} onNew={() => { setPaletteOpen(false); setNewOpen(true); }} />
    </AppContext.Provider>
  );
}

function TokenDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { push } = useToast();
  const [value, setValue] = useState('');
  const [reveal, setReveal] = useState(false);

  useEffect(() => {
    if (open) {
      setValue(getToken());
      setReveal(false);
    }
  }, [open]);

  const save = () => {
    const next = value.trim();
    if (!next) {
      push('令牌不能为空。', 'err');
      return;
    }
    setToken(next);
    push('访问令牌已更新，正在重新加载数据。', 'ok');
    onClose();
    window.setTimeout(() => window.location.reload(), 320);
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="访问令牌"
      description="API 使用服务端签发的 Bearer Token。令牌只保存在本机浏览器，不会进入 URL 或日志。"
      width={480}
      footer={<div className="modal-actions"><Button variant="ghost" onClick={onClose}>取消</Button><Button variant="primary" onClick={save}>保存并刷新</Button></div>}
    >
      <div className="form-stack">
        <Field label="Bearer Token" htmlFor="api-token" hint="本地默认令牌为 local-dev-token；生产环境请在服务端配置 RESEARCHOS_API_TOKEN。">
          <div className="input-group">
            <input
              id="api-token"
              className="input mono"
              type={reveal ? 'text' : 'password'}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
            <Button size="sm" variant="ghost" onClick={() => setReveal((v) => !v)}>{reveal ? '隐藏' : '显示'}</Button>
          </div>
        </Field>
        <div className="notice notice-info">
          <Icon name="info" size={15} />
          <div>身份隔离由服务端按 created_by 强制过滤，前端不会也不能提交 user_id。</div>
        </div>
      </div>
    </Modal>
  );
}

function CommandPalette({ open, onClose, onNew }: { open: boolean; onClose: () => void; onNew: () => void }) {
  const router = useRouter();
  const [query, setQuery] = useState('');
  const tasksQ = usePolling(
    () => apiGet<{ items: Task[] }>('/api/v1/research?limit=50'),
    { key: 'palette-tasks', enabled: open },
  );
  const [active, setActive] = useState(0);

  useEffect(() => {
    if (open) setQuery('');
  }, [open]);

  const navMatches = useMemo(() => {
    if (!query.trim()) {
      return [{ href: '/tasks', label: '研究任务', hint: '查看全部研究' }, { href: '/sources', label: '研究来源', hint: '检索采集的资料' }, { href: '/reports', label: '研究报告', hint: '阅读与导出' }, { href: '/settings', label: '设置', hint: '模型、检索与安全' }];
    }
    const q = query.toLowerCase();
    return NAV.flatMap((g) => g.items).filter((i) => i.label.toLowerCase().includes(q)).map((i) => ({ href: i.href, label: i.label, hint: '页面' }));
  }, [query]);

  const taskMatches = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = tasksQ.data?.items ?? [];
    if (!q) return list.slice(0, 5);
    return list.filter((t) => (t.title || t.goal).toLowerCase().includes(q)).slice(0, 6);
  }, [query, tasksQ.data]);

  const total = navMatches.length + taskMatches.length;
  useEffect(() => {
    setActive(0);
  }, [query, total]);

  const go = useCallback((index: number) => {
    if (index < navMatches.length) {
      router.push(navMatches[index].href);
    } else {
      const task = taskMatches[index - navMatches.length];
      if (task) router.push(`/task/?id=${task.id}`);
    }
    onClose();
  }, [navMatches, taskMatches, router, onClose]);

  return (
    <Modal open={open} onClose={onClose} title="快速跳转" width={620}>
      <div className="palette">
        <div className="palette-input">
          <Icon name="search" size={16} />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索研究任务或页面…"
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setActive((v) => Math.min(total - 1, v + 1)); }
              if (e.key === 'ArrowUp') { e.preventDefault(); setActive((v) => Math.max(0, v - 1)); }
              if (e.key === 'Enter' && total > 0) { e.preventDefault(); go(active); }
            }}
          />
          <kbd>ESC</kbd>
        </div>
        <div className="palette-list">
          {navMatches.length > 0 && <p className="palette-group">页面</p>}
          {navMatches.map((item, i) => (
            <button
              type="button"
              key={item.href + item.label}
              className={cx('palette-item', active === i && 'is-active')}
              onMouseEnter={() => setActive(i)}
              onClick={() => go(i)}
            >
              <span>{item.label}</span>
              <span className="palette-hint">{item.hint}</span>
            </button>
          ))}
          <p className="palette-group">研究任务</p>
          {taskMatches.length === 0 && <p className="palette-empty">没有匹配的任务</p>}
          {taskMatches.map((task, i) => {
            const index = navMatches.length + i;
            return (
              <button
                type="button"
                key={task.id}
                className={cx('palette-item', active === index && 'is-active')}
                onMouseEnter={() => setActive(index)}
                onClick={() => go(index)}
              >
                <span className="palette-task">
                  <span>{truncate(task.title || task.goal, 64)}</span>
                  <span className="palette-meta">{formatRelative(task.created_at)} · {task.status}</span>
                </span>
                <Icon name="chevron-right" size={14} />
              </button>
            );
          })}
        </div>
        <div className="palette-foot">
          <button type="button" className="palette-new" onClick={onNew}>
            <Icon name="plus" size={14} /> 新建研究
          </button>
          <span className="palette-tip">↑↓ 选择 · ↵ 打开</span>
        </div>
      </div>
    </Modal>
  );
}