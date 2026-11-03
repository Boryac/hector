#!/usr/bin/env bash
# 生成界面截图（人工在普通桌面环境执行）。
#
# 为什么需要人工执行：本项目当前开发环境无法启动图形/无头浏览器
# （Chrome、Edge 均无任何输出），截图只能在可用桌面上跑。
#
# 用法：
#   bash scripts/screenshot.sh                       # 默认截 Dashboard 与最近一个任务详情
#   bash scripts/screenshot.sh <task_id>             # 指定任务详情
# 依赖：本机 Chrome 或 Edge。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/docs/screenshots"
mkdir -p "$OUT"

BASE="${RESEARCHOS_BASE:-http://127.0.0.1:8080}"
TOKEN="${RESEARCHOS_API_TOKEN:-local-dev-token}"
TASK="${1:-}"

if [ -z "$TASK" ]; then
  TASK=$(curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/research?limit=1" \
    | python -c "import sys,json;d=json.load(sys.stdin);print((d.get('items') or [{}])[0].get('id',''))" 2>/dev/null || true)
fi

find_browser() {
  for p in \
    "/c/Program Files/Google/Chrome/Application/chrome.exe" \
    "/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "$(command -v google-chrome || true)" \
    "$(command -v chromium || true)"; do
    [ -n "$p" ] && [ -x "$p" ] && { echo "$p"; return; }
  done
}

BROWSER="$(find_browser || true)"
if [ -z "${BROWSER:-}" ]; then
  echo "未找到 Chrome/Edge，请手动截图："
  echo "  1) $BASE/                      → Dashboard"
  echo "  2) $BASE/task/?id=$TASK        → 任务详情（DAG + 事件流 + 报告）"
  exit 1
fi

shot() {
  local url="$1" file="$2"
  echo "截取 $url → $file"
  "$BROWSER" --headless=new --disable-gpu --no-sandbox --hide-scrollbars \
    --window-size=1600,1200 --virtual-time-budget=8000 \
    --screenshot="$file" "$url" >/dev/null 2>&1 || true
  [ -s "$file" ] || echo "  ! 未生成（浏览器可能在受限环境无法启动）"
}

shot "$BASE/" "$OUT/dashboard.png"
if [ -n "$TASK" ]; then
  shot "$BASE/task/?id=$TASK" "$OUT/task-detail.png"
fi
ls -la "$OUT"
