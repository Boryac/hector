// 统一 API 客户端：所有请求都带 Bearer Token（身份由服务端决定，前端不传 user_id）。

// 默认同源访问（由 Go 服务的 RESEARCHOS_WEB_DIR 托管静态产物）；
// 本地 `next dev` 时通过 NEXT_PUBLIC_API_BASE=http://127.0.0.1:8080 指定后端。
export const API_BASE = process.env.NEXT_PUBLIC_API_BASE?.replace(/\/$/, '') ?? '';

const TOKEN_KEY = 'researchos_token';
const DEFAULT_TOKEN = 'local-dev-token';

export function getToken(): string {
  if (typeof window === 'undefined') return DEFAULT_TOKEN;
  return window.localStorage.getItem(TOKEN_KEY) || DEFAULT_TOKEN;
}

export function setToken(token: string): void {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(TOKEN_KEY, token);
}

export class ApiError extends Error {
  code: string;
  status: number;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function handle<T>(res: Response): Promise<T> {
  const text = await res.text();
  if (!res.ok) {
    let code = 'http_error';
    let message = `${res.status} ${res.statusText}`;
    try {
      const parsed = JSON.parse(text);
      if (parsed?.error) {
        code = parsed.error.code ?? code;
        message = parsed.error.message ?? message;
      }
    } catch {
      if (text) message = text.slice(0, 300);
    }
    throw new ApiError(res.status, code, message);
  }
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { Authorization: `Bearer ${getToken()}` },
    cache: 'no-store',
  });
  return handle<T>(res);
}

export async function apiSend<T>(
  method: 'POST' | 'PUT' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${getToken()}`,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  return handle<T>(res);
}

// SSE 不能带请求头，只能把凭据放 URL。为了不把长期 API Token 写进 URL
// （浏览器历史 / Referer / 代理日志），先用 Bearer 换一张一次性 ticket，
// 再用 ticket 建流。每次重连都重新签发。
export async function issueEventTicket(taskId: string): Promise<{ ticket: string; expires_in: number }> {
  return apiSend<{ ticket: string; expires_in: number }>(
    'POST',
    `/api/v1/research/${taskId}/events/ticket`,
  );
}

export function eventsURL(taskId: string, after: number, ticket: string): string {
  return `${API_BASE}/api/v1/research/${taskId}/events?after_sequence=${after}&ticket=${encodeURIComponent(ticket)}`;
}

// 导出走带 Authorization 的 fetch 再落盘：Token 不进 URL，不会留在浏览历史与代理日志里。
export async function downloadReport(taskId: string, format: 'md' | 'html'): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/research/${taskId}/report.${format}`, {
    headers: { Authorization: `Bearer ${getToken()}` },
    cache: 'no-store',
  });
  if (!res.ok) throw new ApiError(res.status, 'export_failed', `导出失败：${res.status}`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `report-${taskId.slice(0, 8)}.${format}`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
