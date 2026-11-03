#!/usr/bin/env bash
# 并发压测：N 个任务并行提交，统计完成率、端到端耗时、SSE 事件量。
#
# 用法：
#   bash scripts/load-test.sh [并发任务数=5] [深度=quick]
# 前置：服务已启动（serve + worker），.env.researchos 里有 API Token。
#
# 输出：docs/benchmark.md 需要的手工数据（复制粘贴即可），包括
#   - 任务数、完成/失败数、成功率
#   - 每个任务的墙钟耗时（从创建到终态）
#   - P50 / P95 耗时
#   - 事件条数与节点数
#
# 注意：压测会真实调用外部检索与抓取服务，请控制规模，避免被目标站点限流。
set -uo pipefail

CONCURRENCY="${1:-5}"
DEPTH="${2:-quick}"
BASE="${RESEARCHOS_BASE:-http://127.0.0.1:8080/api/v1}"
TOKEN="${RESEARCHOS_API_TOKEN:-local-dev-token}"
AUTH="Authorization: Bearer $TOKEN"
OUT="$(mktemp -d)"

GOALS=(
  "LangGraph 的状态图编排在长流程 Agent 中的优势与限制"
  "AutoGen 多 Agent 对话框架的群聊机制与适用边界"
  "CrewAI 的角色分工模型与任务编排能力"
  "向量数据库在检索增强生成中的召回质量对比"
  "MCP 协议对 Agent 工具生态的影响"
  "开源 Agent 框架在生产环境的可观测性方案"
)

echo "并发=$CONCURRENCY 深度=$DEPTH base=$BASE"

start_task() {
  local i="$1"
  local goal="${GOALS[$((i % ${#GOALS[@]}))]}"
  local t0
  t0=$(date +%s)
  local resp id
  resp=$(curl -s -X POST -H "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"goal\":\"$goal\",\"depth\":\"$DEPTH\",\"enable_verification\":true}" "$BASE/research")
  id=$(echo "$resp" | python -c "import sys,json;print(json.load(sys.stdin)['task']['id'])" 2>/dev/null)
  if [ -z "$id" ]; then
    echo "task$i CREATE_FAILED"
    return
  fi
  # 批准执行（等服务把状态推到 awaiting_approval）
  for _ in $(seq 1 15); do
    st=$(curl -s -H "$AUTH" "$BASE/research/$id" | python -c "import sys,json;print(json.load(sys.stdin)['task']['status'])" 2>/dev/null)
    [ "$st" = "awaiting_approval" ] && break
    [ "$st" = "running" ] && break
    sleep 1
  done
  curl -s -X POST -H "$AUTH" "$BASE/research/$id/approve" >/dev/null 2>&1

  for _ in $(seq 1 120); do
    st=$(curl -s -H "$AUTH" "$BASE/research/$id" | python -c "import sys,json;print(json.load(sys.stdin)['task']['status'])" 2>/dev/null)
    case "$st" in
      completed|failed|cancelled)
        local t1 elapsed
        t1=$(date +%s)
        elapsed=$((t1 - t0))
        local nodes events
        nodes=$(curl -s -H "$AUTH" "$BASE/research/$id" | python -c "import sys,json;print(len(json.load(sys.stdin).get('nodes') or []))" 2>/dev/null)
        events=$(curl -s -H "$AUTH" "$BASE/research/$id/events?format=json" | python -c "import sys,json;d=json.load(sys.stdin);print(len(d.get('items') or []))" 2>/dev/null)
        echo "$i $st $elapsed $nodes $events" > "$OUT/$i.txt"
        return
        ;;
    esac
    sleep 2
  done
  echo "$i timeout 0 0 0" > "$OUT/$i.txt"
}

for i in $(seq 1 "$CONCURRENCY"); do
  start_task "$i" &
done
wait

echo
echo "task status elapsed_s nodes events"
cat "$OUT"/*.txt | sort -n
echo
python - "$OUT" <<'PY'
import glob, sys, statistics
rows = []
for f in glob.glob(sys.argv[1] + "/*.txt"):
    parts = open(f).read().split()
    if len(parts) >= 5:
        rows.append((int(parts[0]), parts[1], int(parts[2]), int(parts[3]), int(parts[4])))
if not rows:
    print("无数据")
    sys.exit(1)
ok = [r for r in rows if r[1] == "completed"]
times = sorted(r[2] for r in rows)
p50 = statistics.median(times)
p95 = times[min(len(times) - 1, int(round(0.95 * (len(times) - 1))))]
print("任务数        :", len(rows))
print("完成          :", len(ok))
print("失败/超时     :", len(rows) - len(ok))
print("成功率        : %.0f%%" % (100.0 * len(ok) / len(rows)))
print("耗时 P50      : %ds" % p50)
print("耗时 P95      : %ds" % p95)
print("平均节点数    : %.1f" % (sum(r[3] for r in rows) / len(rows)))
print("平均事件数    : %.1f" % (sum(r[4] for r in rows) / len(rows)))
PY
