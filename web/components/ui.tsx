'use client';

import React, { useEffect, useId, useRef } from 'react';
import { Icon } from './Icon';
import {
  agentStatusTone, claimStatusTone, cx, nodeStatusTone, sourceStatusTone,
  taskStatusTone, taskStatusText, nodeStatusText, claimStatusText, SOURCE_STATUS_TEXT, AGENT_STATUS_TEXT,
} from '@/lib/format';

/* -------------------------------------------------------------------------- */
/* Buttons                                                                     */
/* -------------------------------------------------------------------------- */

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'quiet';

export function Button({
  variant = 'secondary',
  size = 'md',
  loading = false,
  icon,
  className,
  children,
  disabled,
  ...rest
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: 'sm' | 'md';
  loading?: boolean;
  icon?: React.ReactNode;
}) {
  return (
    <button
      type="button"
      className={cx('btn', `btn-${variant}`, size === 'sm' && 'btn-sm', className)}
      disabled={disabled || loading}
      {...rest}
    >
      {loading ? <span className="spinner" aria-hidden="true" /> : icon ? <span className="btn-icon">{icon}</span> : null}
      {children}
    </button>
  );
}

export function IconButton({
  label,
  icon,
  className,
  ...rest
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { label: string; icon: React.ReactNode }) {
  return (
    <button type="button" className={cx('icon-btn', className)} aria-label={label} title={label} {...rest}>
      {icon}
    </button>
  );
}

/* -------------------------------------------------------------------------- */
/* Badges                                                                      */
/* -------------------------------------------------------------------------- */

export type Tone = 'neutral' | 'ok' | 'warn' | 'err' | 'info' | 'accent';

export function Badge({
  tone = 'neutral',
  dot = false,
  children,
  className,
}: {
  tone?: Tone;
  dot?: boolean;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span className={cx('badge', `badge-${tone}`, className)}>
      {dot && <span className="badge-dot" aria-hidden="true" />}
      {children}
    </span>
  );
}

export function StatusBadge({ status, kind }: { status: string; kind: 'task' | 'node' | 'source' | 'claim' | 'agent' | 'server' }) {
  if (kind === 'task') {
    return <Badge tone={taskStatusTone(status)} dot>{taskStatusText(status)}</Badge>;
  }
  if (kind === 'node') {
    return <Badge tone={nodeStatusTone(status)} dot>{nodeStatusText(status)}</Badge>;
  }
  if (kind === 'source') {
    return <Badge tone={sourceStatusTone(status)} dot>{SOURCE_STATUS_TEXT[status] ?? status}</Badge>;
  }
  if (kind === 'claim') {
    return <Badge tone={claimStatusTone(status)} dot>{claimStatusText(status)}</Badge>;
  }
  if (kind === 'agent') {
    return <Badge tone={agentStatusTone(status)} dot>{AGENT_STATUS_TEXT[status] ?? status}</Badge>;
  }
  const tone: Tone = status === 'running' ? 'accent' : status === 'pending_approval' ? 'warn' : status === 'stopped' ? 'neutral' : 'neutral';
  const text = status === 'running' ? '运行中' : status === 'pending_approval' ? '待批准' : status === 'stopped' ? '已停止' : status;
  return <Badge tone={tone} dot>{text}</Badge>;
}

/* -------------------------------------------------------------------------- */
/* Structure                                                                   */
/* -------------------------------------------------------------------------- */

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  meta,
}: {
  eyebrow?: string;
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  meta?: React.ReactNode;
}) {
  return (
    <header className="page-head">
      <div className="page-head-main">
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1 className="page-title">{title}</h1>
        {description && <p className="page-desc">{description}</p>}
        {meta && <div className="page-meta">{meta}</div>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}

export function SectionHead({
  title,
  hint,
  action,
  className,
}: {
  title: React.ReactNode;
  hint?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cx('section-head', className)}>
      <div className="section-head-title">
        <h2>{title}</h2>
        {hint && <span className="section-hint">{hint}</span>}
      </div>
      {action && <div className="section-action">{action}</div>}
    </div>
  );
}

export function EmptyState({
  title,
  description,
  action,
  compact = false,
}: {
  title: string;
  description?: React.ReactNode;
  action?: React.ReactNode;
  compact?: boolean;
}) {
  return (
    <div className={cx('empty-state', compact && 'empty-state-compact')}>
      <span className="empty-rule" aria-hidden="true" />
      <p className="empty-title">{title}</p>
      {description && <p className="empty-desc">{description}</p>}
      {action && <div className="empty-action">{action}</div>}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="error-state" role="alert">
      <div className="error-state-text">
        <strong>数据加载失败</strong>
        <span>{message}</span>
      </div>
      {onRetry && (
        <Button size="sm" variant="secondary" onClick={onRetry} icon={<Icon name="refresh" size={14} />}>
          重试
        </Button>
      )}
    </div>
  );
}

export function Skeleton({ width, height = 12, className }: { width?: number | string; height?: number | string; className?: string }) {
  return <span className={cx('skeleton', className)} style={{ width: width ?? '100%', height }} aria-hidden="true" />;
}

export function SkeletonBlock({ rows = 3 }: { rows?: number }) {
  return (
    <div className="skeleton-block" aria-hidden="true">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} height={12} width={`${92 - i * 12}%`} />
      ))}
    </div>
  );
}

export function Field({
  label,
  hint,
  htmlFor,
  children,
  aside,
}: {
  label: React.ReactNode;
  hint?: React.ReactNode;
  htmlFor?: string;
  children: React.ReactNode;
  aside?: React.ReactNode;
}) {
  return (
    <div className="field">
      <div className="field-head">
        <label className="field-label" htmlFor={htmlFor}>{label}</label>
        {aside}
      </div>
      {children}
      {hint && <p className="field-hint">{hint}</p>}
    </div>
  );
}

export function Progress({ value, tone = 'accent' }: { value: number; tone?: 'accent' | 'ok' | 'warn' | 'err' }) {
  const clamped = Math.max(0, Math.min(100, value));
  return (
    <div className="progress" role="progressbar" aria-valuenow={Math.round(clamped)} aria-valuemin={0} aria-valuemax={100}>
      <span className={cx('progress-fill', `progress-${tone}`)} style={{ width: `${clamped}%` }} />
    </div>
  );
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (next: T) => void;
  ariaLabel?: string;
}) {
  return (
    <div className="segmented" role="radiogroup" aria-label={ariaLabel}>
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          role="radio"
          aria-checked={value === opt.value}
          className={cx('segment', value === opt.value && 'is-active')}
          onClick={() => onChange(opt.value)}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

export function Tabs<T extends string>({
  value,
  items,
  onChange,
  ariaLabel,
  className,
}: {
  value: T;
  items: Array<{ value: T; label: React.ReactNode; count?: number }>;
  onChange: (next: T) => void;
  ariaLabel?: string;
  className?: string;
}) {
  return (
    <div className={cx('tabs', className)} role="tablist" aria-label={ariaLabel}>
      {items.map((item) => (
        <button
          key={item.value}
          type="button"
          role="tab"
          aria-selected={value === item.value}
          className={cx('tab', value === item.value && 'is-active')}
          onClick={() => onChange(item.value)}
        >
          <span>{item.label}</span>
          {typeof item.count === 'number' && <span className="tab-count">{item.count}</span>}
        </button>
      ))}
    </div>
  );
}

export function Switch({
  checked,
  onChange,
  label,
  description,
  id,
  compact = false,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  description?: string;
  id?: string;
  compact?: boolean;
}) {
  const generated = useId();
  const inputId = id ?? generated;
  return (
    <label className={cx('switch-row', compact && 'switch-row-compact')} htmlFor={inputId}>
      {!compact && (
        <span className="switch-copy">
          <span className="switch-label">{label}</span>
          {description && <span className="switch-desc">{description}</span>}
        </span>
      )}
      <span className="switch">
        <input
          id={inputId}
          type="checkbox"
          checked={checked}
          aria-label={compact ? label : undefined}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="switch-track" aria-hidden="true"><span className="switch-thumb" /></span>
      </span>
    </label>
  );
}

/* -------------------------------------------------------------------------- */
/* Overlays                                                                    */
/* -------------------------------------------------------------------------- */

function useDismiss(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    window.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = previous;
      window.removeEventListener('keydown', onKey);
    };
  }, [open, onClose]);
}

export function Modal({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  width = 520,
}: {
  open: boolean;
  onClose: () => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  width?: number;
}) {
  const panelRef = useRef<HTMLDivElement>(null);
  useDismiss(open, onClose);
  useEffect(() => {
    if (open) panelRef.current?.focus();
  }, [open]);
  if (!open) return null;
  return (
    <div className="modal-layer" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div
        className="modal-panel"
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        tabIndex={-1}
        ref={panelRef}
        style={{ width }}
      >
        <div className="modal-head">
          <div>
            <h2 className="modal-title">{title}</h2>
            {description && <p className="modal-desc">{description}</p>}
          </div>
          <IconButton label="关闭" icon={<Icon name="x" size={16} />} onClick={onClose} />
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  );
}

export function Drawer({
  open,
  onClose,
  title,
  children,
  footer,
  width = 460,
}: {
  open: boolean;
  onClose: () => void;
  title: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  width?: number;
}) {
  useDismiss(open, onClose);
  if (!open) return null;
  return (
    <div className="drawer-layer" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <aside className="drawer-panel" role="dialog" aria-modal="true" style={{ width }} aria-label={typeof title === 'string' ? title : undefined}>
        <div className="drawer-head">
          <h2 className="drawer-title">{title}</h2>
          <IconButton label="关闭" icon={<Icon name="x" size={16} />} onClick={onClose} />
        </div>
        <div className="drawer-body">{children}</div>
        {footer && <div className="drawer-foot">{footer}</div>}
      </aside>
    </div>
  );
}

export function InlineNotice({
  tone = 'info',
  title,
  children,
}: {
  tone?: 'info' | 'warn' | 'err' | 'ok';
  title?: string;
  children: React.ReactNode;
}) {
  const icon = tone === 'ok' ? 'check' : tone === 'warn' || tone === 'err' ? 'alert' : 'info';
  return (
    <div className={cx('notice', `notice-${tone}`)}>
      <Icon name={icon} size={15} />
      <div>{title && <strong>{title}</strong>}{children}</div>
    </div>
  );
}

export function MetaRow({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="meta-row">
      <span className="meta-label">{label}</span>
      <span className="meta-value">{children}</span>
    </div>
  );
}

export function KeyValueList({ data, limit = 8 }: { data: Record<string, unknown> | undefined; limit?: number }) {
  if (!data || Object.keys(data).length === 0) return <p className="muted small">无</p>;
  const entries = Object.entries(data).slice(0, limit);
  return (
    <dl className="kv-list">
      {entries.map(([key, value]) => (
        <div key={key} className="kv-row">
          <dt>{key}</dt>
          <dd>{typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value)}</dd>
        </div>
      ))}
    </dl>
  );
}