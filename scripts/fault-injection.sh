#!/usr/bin/env bash
# 故障注入：验证 Worker 被杀后任务能否从租约超时自动恢复，且已完成节点不重跑。
#
# 用法：
#   bash scripts/fault-injection.sh
# 可选环境变量：
#   RESEARCHOS_STALE_LEASE  租约时长（默认取服务配置，3m）。压测时可设 45s 缩短等待。
#   RESEARCHOS_API_TOKEN    API Token（默认从 upstream/hector/.env.researchos 读取）
#
# 前置：
#   1. PostgreSQL 已启动并完成迁移
#   2. 8080 端口空闲（脚本自己起 serve，避免与常驻服务抢端口）
#   3. upstream/hector/bin/researchos(.exe) 已构建
#
# 设计说明（踩过的坑）：
#   - 必须自行 source .env.researchos，否则 Token 取默认值，所有 API 调用静默 401；
#   - 不能用 taskkill //PID 杀 MSYS 的 $!：MSYS 的 pid 不是 Windows pid，
#     会杀到别的进程（实测把 serve 杀掉，后面整段都在等一个已死的 API）；
#     这里统一用 kill -9 并回读确认；
#   - 每一步都要校验 API 可达，不能靠「状态为空」一路空转 240 秒。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HECTOR="$ROOT/upstream/hector"
LOG_DIR="$HECTOR/data/faultinjection"
mkdir -p "$LOG_DIR"

ENV_FILE="$HECTOR/.env.researchos"
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
fi

TOKEN="${RESEARCHOS_API_TOKEN:-}"
PORT="${RESEARCHOS_ADDR##*:}"
PORT="${PORT:-8080}"
BASE="http://127.0.0.1:$PORT/api/v1"
AUTH="Authorization: Bearer $TOKEN"
RESULT=PASS

say() { printf '\n=== %s ===\n' "$*"; }
ok()  { printf '  [ok] %s\n' "$*"; }
bad() { printf '  [!!] %s\n' "$*"; RESULT=FAIL; }

# api <path> [extra curl args] —— 打印响应体，失败时返回非 0
api_get() {
  local path="$1"; shift
  curl -s -m 15 -H "$AUTH" "$@" "$BASE$path"
}
json() { python -c "import sys,json;print(json.load(sys.stdin)$1)" 2>/dev/null; }
need_api() {
  local st
  st=$(api_get /health | json "['status']")
  if [ "$st" != "ok" ]; then
    bad "API 不可达（health=$st）"
    return 1
  fi
  ok "API 可达"
}

if [ -z "$TOKEN" ]; then
  echo "缺少 RESEARCHOS_API_TOKEN：请设置环境变量或在 $ENV_FILE 中配置" >&2
  exit 2
fi
BIN="$HECTOR/bin/researchos.exe"
[ -x "$BIN" ] || BIN="$HECTOR/bin/researchos"
[ -x "$BIN" ] || { echo "未找到可执行文件：$HECTOR/bin/researchos(.exe)" >&2; exit 2; }

if (echo > "/dev/tcp/127.0.0.1/$PORT") 2>/dev/null; then
  echo "端口 $PORT 已被占用：请先停掉常驻服务，本脚本需要独占端口" >&2
  exit 2
fi

# --- Windows 进程句柄 -------------------------------------------------------
# 关键：bash 的 $! 是 MSYS pid，不是 Windows pid。直接 kill -9 只会杀掉 MSYS 的
# 那层壳，真正的 researchos.exe 会变成孤儿继续跑、继续给节点续租，
# 于是「杀死 Worker」根本没发生，恢复测试自然等不到结果。
# 这里统一用 ps -W 里的 WINPID 定位真实进程，并用 taskkill //F //PID 终止。
winpids() { ps -W 2>/dev/null | awk '/bin\/researchos/{print $4}' | sort -n; }
new_winpid() { # $1 = 启动前快照
  local after
  after=$(winpids)
  comm -13 <(printf '%s\n' "$1") <(printf '%s\n' "$after") | tail -1
}
alive_winpid() { winpids | grep -qx "$1"; }
kill_winpid() {
  local p="$1"
  [ -z "$p" ] && return 1
  if command -v taskkill >/dev/null 2>&1; then
    taskkill //F //PID "$p" >/dev/null 2>&1
  fi
  for _ in $(seq 1 10); do
    alive_winpid "$p" || return 0
    sleep 0.5
  done
  return 1
}

SNAP=$(winpids)

say "1. 启动 API（serve）"
cd "$HECTOR" || exit 2
"$BIN" serve >"$LOG_DIR/serve.log" 2>&1 &
SERVE_PID=$!
for _ in $(seq 1 15); do need_api && break; sleep 1; done
need_api || { echo "serve 启动失败，见 $LOG_DIR/serve.log" >&2; exit 1; }
SERVE_WIN=$(new_winpid "$SNAP")
SNAP=$(winpids)
ok "serve WINPID=${SERVE_WIN:-未识别}"

say "2. 启动 Worker（worker）"
"$BIN" worker >"$LOG_DIR/worker1.log" 2>&1 &
WORKER_PID=$!
sleep 3
WORKER_WIN=$(new_winpid "$SNAP")
if [ -n "$WORKER_WIN" ] && alive_winpid "$WORKER_WIN"; then
  ok "worker 已启动 WINPID=$WORKER_WIN（MSYS pid=$WORKER_PID）"
else
  bad "worker 未能启动"
fi

say "3. 创建任务并批准执行"
CREATE=$(curl -s -m 15 -X POST -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"goal":"故障注入验证：LangGraph 的状态图编排与检查点机制","depth":"standard","enable_verification":true}' \
  "$BASE/research")
TASK=$(echo "$CREATE" | json "['task']['id']")
if [ -z "$TASK" ]; then
  bad "创建任务失败：$(echo "$CREATE" | head -c 200)"
  kill -9 "$WORKER_PID" "$SERVE_PID" 2>/dev/null
  exit 1
fi
ok "task=$TASK"

ST=""
for _ in $(seq 1 20); do
  ST=$(api_get "/research/$TASK" | json "['task']['status']")
  [ "$ST" = "awaiting_approval" ] && break
  sleep 1
done
if [ "$ST" != "awaiting_approval" ]; then
  bad "任务未进入 awaiting_approval（当前=$ST）"
else
  APPROVE=$(curl -s -m 15 -X POST -H "$AUTH" "$BASE/research/$TASK/approve")
  ST=$(api_get "/research/$TASK" | json "['task']['status']")
  if [ "$ST" = "running" ]; then ok "已批准，status=running"; else
    bad "批准失败（status=$ST）：$(echo "$APPROVE" | head -c 160)"
  fi
fi

say "4. 等待至少 1 个节点完成"
DONE=0
for _ in $(seq 1 30); do
  DONE=$(api_get "/research/$TASK" | python -c "
import sys,json
d=json.load(sys.stdin)
print(sum(1 for n in (d.get('nodes') or []) if n['status']=='completed'))
" 2>/dev/null || echo 0)
  echo "  completed nodes=$DONE"
  [ "${DONE:-0}" -ge 1 ] && break
  sleep 4
done
if [ "${DONE:-0}" -ge 1 ]; then ok "已有 $DONE 个节点完成，可以注入故障"; else bad "超时仍无节点完成"; fi

say "5. 强杀 Worker（模拟进程崩溃）"
# 快照「杀之前已经完成的节点及其 attempts」。只有这批节点必须一个字都不变；
# 杀进程时正在飞的节点被重跑一次是恢复机制的正常行为，不算回归。
api_get "/research/$TASK" > "$LOG_DIR/before_kill.json"
DONE_BEFORE=$(python - "$LOG_DIR/before_kill.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding='utf-8'))
done = [n for n in (d.get('nodes') or []) if n['status'] == 'completed']
print(','.join('%s=%d' % (n['id'], n['attempts']) for n in done))
PY
)
if kill_winpid "$WORKER_WIN"; then
  ok "worker(WINPID=$WORKER_WIN) 已被强制终止"
else
  bad "worker(WINPID=$WORKER_WIN) 未能终止，故障注入无效"
fi
ok "杀之前已完成节点：${DONE_BEFORE:-（无）}"
# 确认没有游魂进程：残留的 researchos(worker) 会继续续租，测试必假
LEFT_PROCS=$(winpids | grep -vx "$SERVE_WIN" | tr '\n' ' ')
if [ -n "$LEFT_PROCS" ]; then
  bad "仍有 researchos 进程存活：$LEFT_PROCS"
else
  ok "除 serve 外无 researchos 进程存活"
fi
need_api || true
LEFT=$(api_get "/research/$TASK" | python -c "
import sys,json
d=json.load(sys.stdin)
ns=d.get('nodes') or []
print('running=%d ready=%d pending=%d' % (
  sum(1 for n in ns if n['status']=='running'),
  sum(1 for n in ns if n['status']=='ready'),
  sum(1 for n in ns if n['status']=='pending')))
" 2>/dev/null)
ok "杀进程后节点分布：$LEFT"

say "6. 等待租约超时后重启 Worker"
echo "  租约=${RESEARCHOS_STALE_LEASE:-取服务默认值}，先等 50s 让租约过期再拉起 worker2"
sleep 50
SNAP=$(winpids)
"$BIN" worker >"$LOG_DIR/worker2.log" 2>&1 &
WORKER2_PID=$!
sleep 3
WORKER2_WIN=$(new_winpid "$SNAP")
ok "worker2 已启动 WINPID=${WORKER2_WIN:-未识别}"

ST=""
for i in $(seq 1 60); do
  ST=$(api_get "/research/$TASK" | json "['task']['status']")
  [ -n "$ST" ] || { bad "API 返回为空，停止等待"; break; }
  case "$ST" in completed|failed|cancelled) echo "  t+$((i*5))s status=$ST"; break;; esac
  [ $((i % 6)) -eq 0 ] && echo "  t+$((i*5))s status=$ST"
  sleep 5
done
case "$ST" in
  completed) ok "任务在 worker 重启后收敛为 completed";;
  failed)    bad "任务收敛为 failed";;
  cancelled) bad "任务意外 cancelled";;
  *)         bad "等待超时，最后状态=$ST";;
esac

say "7. 校验：停机前已完成的节点没有被重跑"
api_get "/research/$TASK" > "$LOG_DIR/result.json"
python - "$LOG_DIR/result.json" "$DONE_BEFORE" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding='utf-8'))
before = dict(p.split('=') for p in sys.argv[2].split(',') if p)
t = d['task']
ns = d.get('nodes') or []
done = {n['id']: n for n in ns if n['status'] == 'completed'}

print('  task status      :', t['status'])
print('  nodes/completed  :', len(ns), '/', len(done))
print('  杀之前已完成      :', ', '.join('%s(att=%s)' % (k[:10], v) for k, v in before.items()) or '（无）')
print('  restored         :', ', '.join('%s(att=%d)' % (k[:10], done[k]['attempts']) for k in before if k in done) or '（无）')

problems = []
for nid, att in before.items():
    if nid not in done:
        problems.append('%s 在停机前已完成，恢复后却不是 completed' % nid[:10])
    elif str(done[nid]['attempts']) != att:
        problems.append('%s 的 attempts 从 %s 变成 %d（被重跑）' % (nid[:10], att, done[nid]['attempts']))

if problems:
    for p in problems:
        print('  RESULT           : FAIL', p)
    sys.exit(1)
if t['status'] != 'completed':
    print('  RESULT           : FAIL 任务未完成(' + t['status'] + ')，无法证明恢复成功')
    sys.exit(1)
print('  RESULT           : PASS 停机前已完成的节点未重跑，任务从租约过期恢复至 completed')
PY
[ $? -eq 0 ] || RESULT=FAIL

say "8. 清理进程"
kill_winpid "$WORKER2_WIN" && ok "worker2 已退出" || bad "worker2 未退出"
kill_winpid "$SERVE_WIN" && ok "serve 已退出" || bad "serve 未退出"
echo "日志：$LOG_DIR"

echo
if [ "$RESULT" = "PASS" ]; then echo "结论：PASS"; else echo "结论：FAIL"; fi
[ "$RESULT" = "PASS" ] || exit 1
