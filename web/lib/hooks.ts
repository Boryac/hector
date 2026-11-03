'use client';

// 轻量数据获取原语：轮询、任务详情缓存与实时事件流。
// 不引入额外状态库，所有数据都直接来自后端 API。

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ApiError, apiGet, eventsURL, issueEventTicket } from './api';
import type { EventItem, Task, TaskDetail } from './types';

export interface Resource<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  refresh: () => Promise<void>;
}

function messageOf(err: unknown): string {
  if (err instanceof ApiError) return `${err.message}（${err.code}）`;
  return err instanceof Error ? err.message : String(err);
}

export function usePolling<T>(
  fetcher: () => Promise<T>,
  options: { intervalMs?: number; key?: string; enabled?: boolean } = {},
): Resource<T> {
  const { intervalMs = 0, key = '', enabled = true } = options;
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const mountedRef = useRef(true);
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    try {
      const next = await fetcherRef.current();
      if (mountedRef.current) {
        setData(next);
        setError(null);
        setLoading(false);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(messageOf(err));
        setLoading(false);
      }
    }
  }, [enabled]);

  useEffect(() => {
    mountedRef.current = true;
    if (!enabled) {
      setLoading(false);
      return () => {
        mountedRef.current = false;
      };
    }
    let disposed = false;
    const run = async (quiet: boolean) => {
      if (!quiet) setLoading(true);
      try {
        const next = await fetcherRef.current();
        if (!disposed) {
          setData(next);
          setError(null);
          setLoading(false);
        }
      } catch (err) {
        if (!disposed) {
          setError(messageOf(err));
          setLoading(false);
        }
      }
    };
    run(false);
    let timer: ReturnType<typeof setInterval> | null = null;
    if (intervalMs > 0) timer = setInterval(() => run(true), intervalMs);
    return () => {
      disposed = true;
      mountedRef.current = false;
      if (timer) clearInterval(timer);
    };
  }, [enabled, intervalMs, key]);

  return { data, error, loading, refresh };
}

// 任务列表 + 最近若干任务的详情。详情用于统计来源数、节点数与报告摘要，
// 只在任务签名（id / updated_at）变化时重新拉取，避免轮询造成请求风暴。
export function useTaskCatalog(taskLimit = 40, detailCount = 12) {
  const tasksRes = usePolling(
    () => apiGet<{ items: Task[]; total: number }>(`/api/v1/research?limit=${taskLimit}`),
    { intervalMs: 10000, key: `tasks-${taskLimit}` },
  );
  const tasks = useMemo(() => tasksRes.data?.items ?? [], [tasksRes.data]);
  const total = tasksRes.data?.total ?? tasks.length;
  const [details, setDetails] = useState<Record<string, TaskDetail>>({});
  const [detailError, setDetailError] = useState<string | null>(null);

  const signature = useMemo(
    () => tasks.slice(0, detailCount).map((t) => `${t.id}:${t.updated_at}`).join('|'),
    [tasks, detailCount],
  );

  useEffect(() => {
    if (!signature) return;
    let disposed = false;
    const targets = signature.split('|').map((s) => s.split(':')[0]).filter(Boolean);
    (async () => {
      const pairs = await Promise.all(
        targets.map(async (id) => {
          try {
            return [id, await apiGet<TaskDetail>(`/api/v1/research/${id}`)] as const;
          } catch {
            return null;
          }
        }),
      );
      if (disposed) return;
      const next: Record<string, TaskDetail> = {};
      for (const pair of pairs) if (pair) next[pair[0]] = pair[1];
      setDetails(next);
      setDetailError(null);
    })().catch((err) => {
      if (!disposed) setDetailError(messageOf(err));
    });
    return () => {
      disposed = true;
    };
  }, [signature]);

  return {
    tasks,
    total,
    details,
    loading: tasksRes.loading,
    error: tasksRes.error ?? detailError,
    refresh: tasksRes.refresh,
  };
}

export interface StreamState {
  live: boolean;
}

const STREAM_EVENTS = [
  'research.created', 'research.planning', 'research.planned', 'research.started',
  'research.paused', 'research.resumed', 'research.completed', 'research.failed',
  'research.synthesizing', 'research.cancelled', 'research.transition_rejected',
  'workflow.created', 'workflow.updated', 'node.queued', 'node.started', 'node.progress',
  'node.completed', 'node.failed', 'node.retrying', 'agent.created', 'agent.started',
  'agent.completed', 'agent.failed', 'tool.invoked', 'tool.completed', 'tool.failed',
  'evidence.created', 'evidence.verified', 'report.started', 'report.completed',
];

// SSE：EventSource 无法携带 Authorization 头，因此每次连接前先换取一次性
// ticket；ticket 只用于单条事件流，长期 Token 不会出现在 URL 中。
export function useEventStream(
  taskId: string,
  handlers: { onEvent: (event: EventItem) => void; onRefresh: () => void },
): StreamState {
  const [live, setLive] = useState(false);
  const handlerRef = useRef(handlers);
  handlerRef.current = handlers;

  useEffect(() => {
    if (!taskId) return;
    let stopped = false;
    let source: EventSource | null = null;
    let retry: ReturnType<typeof setTimeout> | null = null;

    const connect = async () => {
      if (stopped) return;
      let ticket: string;
      try {
        const res = await issueEventTicket(taskId);
        ticket = res.ticket;
      } catch {
        setLive(false);
        if (!stopped) retry = setTimeout(connect, 3000);
        return;
      }
      if (stopped) return;
      source = new EventSource(eventsURL(taskId, 0, ticket));
      source.onopen = () => setLive(true);
      source.onerror = () => {
        setLive(false);
        source?.close();
        if (!stopped) retry = setTimeout(connect, 3000);
      };
      const consume = (raw: string) => {
        try {
          const event = JSON.parse(raw) as EventItem;
          if (!event?.sequence) return;
          handlerRef.current.onEvent(event);
          if (!event.event_type.startsWith('node.progress')) handlerRef.current.onRefresh();
        } catch {
          // 忽略无法解析的帧；不打断事件流。
        }
      };
      source.onmessage = (ev) => consume(ev.data);
      STREAM_EVENTS.forEach((type) => source?.addEventListener(type, (ev) => consume((ev as MessageEvent).data)));
      source.addEventListener('stream.end', () => {
        setLive(false);
        source?.close();
      });
    };

    connect();
    return () => {
      stopped = true;
      setLive(false);
      source?.close();
      if (retry) clearTimeout(retry);
    };
  }, [taskId]);

  return { live };
}

export function useTaskEvents(taskId: string, limit = 500) {
  return usePolling(
    () => apiGet<{ items: EventItem[]; latest_sequence: number; task_status: string }>(
      `/api/v1/research/${taskId}/events?format=json&after_sequence=0`,
    ),
    { key: taskId, enabled: Boolean(taskId) },
  );
}