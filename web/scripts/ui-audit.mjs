#!/usr/bin/env node
// Headless Chrome audit for the static export. It checks every page for React
// console errors and horizontal document overflow, and optionally writes PNGs.
//
// Prerequisite: Chrome/Edge started with --remote-debugging-port=9333.
// Usage:
//   node scripts/ui-audit.mjs --base=http://127.0.0.1:8080 --cdp=http://127.0.0.1:9333
//   node scripts/ui-audit.mjs --shots=/absolute/path/to/screenshots
//
// The first research task is discovered through the API so the workspace route
// can be checked without hardcoding an id. Override it with --task=<uuid>.

import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const args = new Map(
  process.argv.slice(2).map((arg) => {
    const [key, ...rest] = arg.replace(/^--/, '').split('=');
    return [key, rest.join('=') || 'true'];
  }),
);
const base = (args.get('base') ?? 'http://127.0.0.1:8080').replace(/\/$/, '');
const cdp = (args.get('cdp') ?? 'http://127.0.0.1:9333').replace(/\/$/, '');
const token = args.get('token') ?? process.env.RESEARCHOS_TOKEN ?? 'local-dev-token';
const shots = args.get('shots') ? resolve(args.get('shots')) : null;
const sleep = (ms) => new Promise((resolveSleep) => setTimeout(resolveSleep, ms));

if (typeof WebSocket === 'undefined') {
  console.error('Node 22+ is required (global WebSocket).');
  process.exit(2);
}

async function firstTaskId() {
  if (args.get('task')) return args.get('task');
  try {
    const res = await fetch(`${base}/api/v1/research?limit=1`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return null;
    const body = await res.json();
    return body.items?.[0]?.id ?? body.tasks?.[0]?.id ?? null;
  } catch {
    return null;
  }
}

const taskId = await firstTaskId();
if (shots) mkdirSync(shots, { recursive: true });

const target = await (await fetch(`${cdp}/json/new?about:blank`, { method: 'PUT' })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
const pending = new Map();
let nextId = 1;
let routeErrors = [];

await new Promise((resolveOpen) => {
  ws.onopen = resolveOpen;
});
ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve: resolveCall, reject } = pending.get(message.id);
    pending.delete(message.id);
    if (message.error) reject(new Error(JSON.stringify(message.error)));
    else resolveCall(message.result);
  }
  if (message.method === 'Runtime.exceptionThrown') {
    routeErrors.push(message.params.exceptionDetails?.exception?.description ?? message.params.exceptionDetails?.text);
  }
  if (message.method === 'Runtime.consoleAPICalled' && message.params.type === 'error') {
    routeErrors.push(message.params.args.map((item) => item.value ?? item.description ?? '').join(' '));
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
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.exception?.description ?? result.exceptionDetails.text);
  }
  return result.result.value;
}

async function navigate(path, wait = 2800) {
  routeErrors = [];
  await send('Page.navigate', { url: base + path });
  await sleep(wait);
}

async function measure() {
  return evaluate(`(() => {
    const box = (selector) => {
      const node = document.querySelector(selector);
      if (!node) return null;
      const rect = node.getBoundingClientRect();
      return { w: Math.round(rect.width), h: Math.round(rect.height) };
    };
    const nodes = [...document.querySelectorAll('.rf-node')].map((node) => {
      const rect = node.getBoundingClientRect();
      return { w: Math.round(rect.width), h: Math.round(rect.height) };
    });
    return {
      viewport: innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      overflow: document.documentElement.scrollWidth > innerWidth + 1,
      canvas: box('.canvas-wrap'),
      node: nodes[0] ?? null,
      nodes: nodes.length,
      outline: document.querySelector('.outline-card')
        ? getComputedStyle(document.querySelector('.outline-card')).display
        : null,
    };
  })()`);
}

async function screenshot(name) {
  const { data } = await send('Page.captureScreenshot', { format: 'png', fromSurface: true });
  writeFileSync(resolve(shots, `${name}.png`), Buffer.from(data, 'base64'));
}

const routes = [
  ['dashboard', '/'],
  ['tasks', '/tasks/'],
  ['sources', '/sources/'],
  ['evidence', taskId ? `/evidence/?id=${taskId}` : null],
  ['reports', '/reports/'],
  ['settings', '/settings/'],
  ['workspace', taskId ? `/task/?id=${taskId}` : null],
].filter(([, path]) => path);

await send('Runtime.enable');
await send('Page.enable');
await send('Network.enable');
await send('Network.setCacheDisabled', { cacheDisabled: true });

const results = [];
for (const viewport of [
  { width: 1440, height: 900, mobile: false, suffix: '' },
  { width: 390, height: 844, mobile: true, suffix: '@390' },
]) {
  await send('Emulation.setDeviceMetricsOverride', {
    width: viewport.width,
    height: viewport.height,
    deviceScaleFactor: 1,
    mobile: viewport.mobile,
  });
  for (const [name, path] of routes) {
    await navigate(path);
    const metrics = await measure();
    results.push({
      name: name + viewport.suffix,
      path,
      metrics,
      errors: routeErrors.map((error) => error.split('\n')[0]),
    });
    if (shots && !viewport.suffix) await screenshot(`${name}-desktop`);
    if (shots && viewport.suffix && ['dashboard', 'settings', 'workspace'].includes(name)) {
      await screenshot(`${name}-mobile`);
    }
  }
}

ws.close();
await fetch(`${cdp}/json/close/${target.id}`);

let failed = false;
for (const result of results) {
  const bad = result.metrics.overflow || result.errors.length > 0;
  failed ||= bad;
  const canvas = result.metrics.canvas ? `${result.metrics.canvas.w}x${result.metrics.canvas.h}` : '-';
  const node = result.metrics.node ? `${result.metrics.node.w}x${result.metrics.node.h}` : '-';
  console.log(
    `${bad ? 'FAIL' : 'ok  '} ${result.name.padEnd(18)} viewport=${result.metrics.viewport} ` +
      `doc=${result.metrics.documentWidth} canvas=${canvas} node=${node} outline=${result.metrics.outline ?? '-'} ` +
      `errors=${result.errors.length}`,
  );
  for (const error of result.errors) console.log(`     ${error.slice(0, 160)}`);
}
process.exit(failed ? 1 : 0);