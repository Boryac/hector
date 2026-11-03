#!/usr/bin/env node
// ui-functional.mjs — 前后端对接深度审计：逐页断言真实发生的 API 调用。
//
// 与 ui-audit.mjs 的区别：ui-audit 只看控制台错误与溢出；这个脚本通过 CDP
// Network 域抓每个页面实际发出的请求，断言「页面加载 = 真实后端调用」，
// 并做一次真实的 UI 交互（打开新建研究对话框 → 填写 → 提交 → 任务出现在列表）。
//
// 前置：Chrome/Edge --remote-debugging-port=9333；服务已在 --base 运行。
// 用法：
//   node scripts/ui-functional.mjs --base=http://127.0.0.1:8080 --cdp=http://127.0.0.1:9333
//
// 退出码 0 = 全部断言通过；1 = 有失败（逐条打印）。

const args = new Map(
  process.argv.slice(2).map((arg) => {
    const [key, ...rest] = arg.replace(/^--/, '').split('=');
    return [key, rest.join('=') || 'true'];
  }),
);
const base = (args.get('base') ?? 'http://127.0.0.1:8080').replace(/\/$/, '');
const cdp = (args.get('cdp') ?? 'http://127.0.0.1:9333').replace(/\/$/, '');
const token = args.get('token') ?? process.env.RESEARCHOS_TOKEN ?? 'local-dev-token';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function firstCompletedTask() {
  const res = await fetch(`${base}/api/v1/research?limit=50`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  const body = await res.json();
  const items = body.items ?? body.tasks ?? [];
  return items.find((t) => t.status === 'completed')?.id ?? items[0]?.id ?? null;
}

const taskId = await firstCompletedTask();
if (!taskId) {
  console.error('FAIL 库中没有可用的研究任务，先创建并完成一个');
  process.exit(1);
}

const target = await (await fetch(`${cdp}/json/new?about:blank`, { method: 'PUT' })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => { ws.onopen = r; });

const pending = new Map();
let nextId = 1;
const pageRequests = []; // 当前页捕获的 {url, status}
const pageErrors = [];

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.id && pending.has(msg.id)) {
    const { resolve: res, reject } = pending.get(msg.id);
    pending.delete(msg.id);
    if (msg.error) reject(new Error(JSON.stringify(msg.error)));
    else res(msg.result);
  }
  if (msg.method === 'Network.responseReceived') {
    const { url, status } = msg.params.response;
    if (url.includes('/api/')) pageRequests.push({ url: url.replace(base, ''), status });
  }
  if (msg.method === 'Runtime.exceptionThrown') {
    pageErrors.push(msg.params.exceptionDetails?.exception?.description ?? msg.params.exceptionDetails?.text);
  }
  if (msg.method === 'Runtime.consoleAPICalled' && msg.params.type === 'error') {
    pageErrors.push(msg.params.args.map((i) => i.value ?? i.description ?? '').join(' '));
  }
};

function send(method, params = {}) {
  return new Promise((resolveCall, reject) => {
    const id = nextId++;
    pending.set(id, { resolve: resolveCall, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });
}

async function evaluate(expression) {
  const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? result.exceptionDetails.text);
  return result.result.value;
}

const results = [];
const failures = [];

function assert(name, ok, detail = '') {
  results.push(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : ` :: ${detail}`}`);
  if (!ok) failures.push(`${name}: ${detail}`);
}

// token 需要在页面 localStorage 里，页面才会带凭据调 API。
// 必须先导航到站点本身——about:blank 上 localStorage 访问被拒绝。
await send('Runtime.enable');
await send('Page.enable');
await send('Network.enable');
await send('Network.setCacheDisabled', { cacheDisabled: true });
await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false });
await send('Page.navigate', { url: `${base}/` });
await sleep(2500);
await evaluate(`localStorage.setItem('researchos_token', ${JSON.stringify(token)})`);

async function visit(path, wait = 3200) {
  pageRequests.length = 0;
  pageErrors.length = 0;
  await send('Page.navigate', { url: base + path });
  await sleep(wait);
}

// 每页：断言必须发生的 API 调用全部 2xx
const pageSpecs = [
  ['Dashboard 总览', '/', ['/api/v1/health', '/api/v1/settings', '/api/v1/research', '/api/v1/stats']],
  ['任务列表', '/tasks/', ['/api/v1/research']],
  ['来源库', '/sources/', ['/api/v1/sources']],
  ['证据页', `/evidence/?id=${taskId}`, [`/api/v1/research/${taskId}/evidence`]],
  ['报告页', '/reports/', ['/api/v1/reports']],
  ['设置中心', '/settings/', ['/api/v1/settings', '/api/v1/tools/servers', '/api/v1/system']],
];

for (const [name, path, mustCall] of pageSpecs) {
  await visit(path);
  const seen = new Set(pageRequests.map((r) => r.url.split('?')[0]));
  for (const api of mustCall) {
    const hit = pageRequests.find((r) => r.url.split('?')[0] === api);
    assert(`${name} → GET ${api.split('/api')[1]}`, !!hit && hit.status >= 200 && hit.status < 300,
      hit ? `status=${hit.status}` : '页面没有发出该请求');
  }
  const failed4xx = pageRequests.filter((r) => r.status >= 400);
  assert(`${name} 无 4xx/5xx 调用`, failed4xx.length === 0,
    failed4xx.map((r) => `${r.url}=${r.status}`).join(', '));
  assert(`${name} 无 React 运行时错误`, pageErrors.length === 0,
    pageErrors[0]?.slice(0, 120) ?? '');
}

// 研究工作台：数据按 Tab 懒加载（enabled: tab === 'sources' 等），逐个点击断言
{
  await visit(`/task/?id=${taskId}`);
  await clickTab('来源');
  await clickTab('证据');
  await clickTab('Agent');
  await clickTab('报告');
  const seen = new Set(pageRequests.map((r) => r.url.split('?')[0]));
  const mustCall = [
    `/api/v1/research/${taskId}`,
    `/api/v1/research/${taskId}/sources`,
    `/api/v1/research/${taskId}/evidence`,
    `/api/v1/research/${taskId}/agents`,
    `/api/v1/research/${taskId}/report`,
  ];
  for (const api of mustCall) {
    const hit = pageRequests.find((r) => r.url.split('?')[0] === api);
    assert(`研究工作台 → GET ${api.split('/api')[1]}`, !!hit && hit.status >= 200 && hit.status < 300,
      hit ? `status=${hit.status}` : '切到对应 Tab 后仍未发出请求');
  }
  const failed4xx = pageRequests.filter((r) => r.status >= 400);
  assert('研究工作台 无 4xx/5xx 调用', failed4xx.length === 0, failed4xx.map((r) => `${r.url}=${r.status}`).join(', '));
  const sse = pageRequests.filter((r) => r.url.includes('/events'));
  assert('工作台建立 SSE 事件流', sse.some((r) => r.url.includes('ticket=')), JSON.stringify(sse.map((r) => r.url.split('?')[0])));
}

async function clickTab(label) {
  await evaluate(`(() => {
    const tab = [...document.querySelectorAll('[role="tab"], .tabs button')]
      .find((el) => (el.textContent || '').trim().startsWith(${JSON.stringify(label)}));
    if (tab) tab.click();
    return !!tab;
  })()`);
  await sleep(1500);
}

// 设置页字段由后端 schema 驱动：统计真实渲染的可编辑控件（.settings-control 内的输入）
await visit('/settings/');
const fieldCount = await evaluate(`document.querySelectorAll('.settings-control input, .settings-control select, .settings-control [role="group"], .settings-readonly').length`);
assert('设置页渲染了后端 schema 字段', fieldCount > 10, `只找到 ${fieldCount} 个控件`);
const sysinfoRows = await evaluate(`document.querySelectorAll('.sysinfo-row').length`);
assert('系统分区展示 /system 运行时信息', sysinfoRows >= 8, `只找到 ${sysinfoRows} 行`);
const statCards = await evaluate(`document.querySelectorAll('.sysinfo-row, .metric').length`);

// 真实 UI 交互：新建研究 → 任务出现在列表 → 清理
await visit('/tasks/');
const opened = await evaluate(`(() => {
  const all = [...document.querySelectorAll('button, a')];
  const create = all.find((el) => /新建|新研究|开始研究/.test(el.textContent || ''));
  if (!create) return false;
  create.click();
  return true;
})()`);
assert('任务列表有「新建研究」入口且可点击', opened === true);
await sleep(900);
const typed = await evaluate(`(() => {
  const ta = document.getElementById('research-goal');
  if (!ta) return false;
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set
    .call(ta, 'verify-ui-functional 临时任务：对比 Redis 与 Dragonfly 的单机吞吐');
  ta.dispatchEvent(new Event('input', { bubbles: true }));
  return true;
})()`);
assert('新建对话框接受目标输入', typed === true);
await sleep(400);
const submitted = await evaluate(`(() => {
  const go = [...document.querySelectorAll('button')].find((el) => (el.textContent || '').trim() === '创建并规划');
  if (!go || go.disabled) return false;
  go.click();
  return true;
})()`);
assert('提交创建任务', submitted === true);
await sleep(4000);
const postHit = pageRequests.find((r) => r.url.split('?')[0] === '/api/v1/research' && r.url.split('?')[0] !== '/api/v1/research?');
const created = await fetch(`${base}/api/v1/research?limit=30`, {
  headers: { Authorization: `Bearer ${token}` },
}).then((r) => r.json());
const hit = (created.items ?? []).find((t) => (t.goal ?? t.title ?? '').includes('verify-ui-functional'));
assert('新任务真实落库并出现在列表', !!hit, 'API 列表中找不到');

// 清理临时任务
if (hit) {
  const del = await fetch(`${base}/api/v1/research/${hit.id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
  assert('清理临时任务', del.ok || del.status === 409, `status=${del.status}`);
}

ws.close();
await fetch(`${cdp}/json/close/${target.id}`);

console.log(results.join('\n'));
console.log(`\n== 汇总：${results.filter((r) => r.startsWith('ok')).length} 通过 / ${failures.length} 失败 ==`);
process.exit(failures.length ? 1 : 0);
