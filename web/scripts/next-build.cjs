#!/usr/bin/env node
/**
 * next-build.cjs — 无 trace 的 Next.js 构建包装器。
 *
 * 背景：在部分 Windows 环境（沙箱 / 杀软 / 受限账户）下，Next.js 的 trace 上报器
 * 以 `flags: "a"` 打开 `<cwd>/.next/trace` 会抛出 EPERM 并导致构建中断（即使该路径
 * 对本进程可写）。trace 只是构建遥测，与产物无关，因此在构建入口把 trace reporter
 * 替换为 no-op，可让构建稳定完成。
 *
 * 用法：node scripts/next-build.cjs [next-build-args...]
 */
const Module = require('module');
const path = require('path');

const originalLoad = Module._load;
Module._load = function patchedLoad(request, parent, isMain) {
  const fromTraceDir = parent && /trace[\\/]report[\\/]index\.js$/.test(parent.filename || '');
  if (fromTraceDir && (request === './to-json' || request.endsWith('trace/report/to-json'))) {
    return {
      __esModule: true,
      default: {
        report() {},
        flushAll() { return Promise.resolve(); },
      },
    };
  }
  return originalLoad.apply(this, arguments);
};

const nextBin = require.resolve('next/dist/bin/next');
process.argv = [process.argv[0], nextBin, 'build', ...process.argv.slice(2)];
require(path.resolve(nextBin));
