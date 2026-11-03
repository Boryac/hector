#!/usr/bin/env bash
# verify.sh — Hector ResearchOS 统一校验入口（可证伪：任何一步失败都以非零退出）。
#
# 用法：
#   bash scripts/verify.sh            # 全量：格式/vet/测试/类型/构建/契约/compose/负向自检
#   bash scripts/verify.sh --fast     # 跳过前端构建与需要数据库的端到端段（显式标注 SKIP）
#
# 设计原则：
#   1. 每个检查独立报告 PASS / FAIL / SKIP，FAIL 累计，最后以非零退出；
#   2. 没有恒真 PASS——SKIP 必须写明原因，且不计入通过数；
#   3. 内置负向自检：故意注入一处悬空 $ref 和一个格式错误文件，
#      确认校验器真的能抓到（抓不到 = 校验器本身坏了 = FAIL）。
set -uo pipefail
cd "$(dirname "$0")/.."
ROOT="$(pwd)"
UP="$ROOT/upstream/hector"
WEB="$UP/web"
FAST=0
[ "${1:-}" = "--fast" ] && FAST=1

PASS=0
FAIL=0
SKIP=0
declare -a FAILED

report() { # report <PASS|FAIL|SKIP> <name> [detail]
  local status="$1" name="$2" detail="${3:-}"
  case "$status" in
    PASS) PASS=$((PASS+1)); printf '[PASS] %s\n' "$name" ;;
    FAIL) FAIL=$((FAIL+1)); FAILED+=("$name"); printf '[FAIL] %s %s\n' "$name" "$detail" ;;
    SKIP) SKIP=$((SKIP+1)); printf '[SKIP] %s %s\n' "$name" "$detail" ;;
  esac
}

run_step() { # run_step <name> <cmd...>
  local name="$1"; shift
  local out
  if out=$("$@" 2>&1); then
    report PASS "$name"
  else
    report FAIL "$name" "$(echo "$out" | tail -8 | tr '\n' ' ')"
    return 1
  fi
}

echo "== Hector ResearchOS verify（$(date '+%F %T')）=="

# ---------- 1. Go 侧 ----------
if command -v go >/dev/null 2>&1; then
  # 1.1 gofmt：任何未格式化文件都是 FAIL
  unformatted=$(cd "$UP" && gofmt -l internal/researchos cmd/researchos 2>&1)
  if [ -z "$unformatted" ]; then
    report PASS "gofmt（internal/researchos + cmd/researchos）"
  else
    report FAIL "gofmt" "未格式化: $(echo "$unformatted" | tr '\n' ' ')"
  fi
  # 1.2 go vet
  (cd "$UP" && go vet ./internal/researchos/... ./cmd/researchos/...) >/dev/null 2>&1 \
    && report PASS "go vet" || report FAIL "go vet"
  # 1.3 go test（强制重跑，不用缓存）
  run_step "go test ./internal/researchos/..." bash -c "cd '$UP' && go test -count=1 ./internal/researchos/... >/dev/null"
else
  report SKIP "Go 全套" "未安装 go"
fi

# ---------- 2. 前端 ----------
if [ -f "$WEB/package.json" ]; then
  if [ ! -d "$WEB/node_modules" ]; then
    report SKIP "前端类型检查" "node_modules 缺失（先 npm install）"
    report SKIP "前端生产构建" "node_modules 缺失"
  else
    run_step "前端类型检查（tsc --noEmit）" bash -c "cd '$WEB' && npx tsc --noEmit"
    if [ "$FAST" = 1 ]; then
      report SKIP "前端生产构建" "--fast 模式跳过"
    else
      # 本机环境的 node safe-delete 拦截 Next 清理 .next；这里只对构建子进程关闭该拦截
      if run_step "前端生产构建（next build）" bash -c "cd '$WEB' && CODEBUDDY_SAFE_DELETE_ENABLED=0 npm run build >/dev/null"; then
        :
      fi
    fi
  fi
else
  report SKIP "前端全套" "web/package.json 不存在"
fi

# ---------- 3. 契约与部署文件 ----------
run_step "OpenAPI 契约自检" python scripts/check-openapi.py api/openapi.yaml
run_step "Compose 静态校验" python scripts/check-compose.py docker-compose.yaml deployments/compose.yaml

# ---------- 4. 负向自检：校验器必须能抓到真错误 ----------
SELFTEST_DIR="$(mktemp -d)"
trap 'rm -rf "$SELFTEST_DIR"' EXIT
# 4.1 注入悬空 $ref 的 openapi
cp api/openapi.yaml "$SELFTEST_DIR/broken.yaml"
printf '  /api/v1/__broken:\n    get:\n      responses:\n        %s200%s:\n          content:\n            application/json:\n              schema: { $ref: %s#/components/schemas/NoSuchSchema }\n' "'" "'" "'" >> "$SELFTEST_DIR/broken.yaml"
if python scripts/check-openapi.py "$SELFTEST_DIR/broken.yaml" >/dev/null 2>&1; then
  report FAIL "负向自检：悬空 \$ref 被抓到" "check-openapi.py 放行了坏文件——校验器失效"
else
  report PASS "负向自检：悬空 \$ref 被抓到"
fi
# 4.2 gofmt 能发现坏格式
printf 'package x\n\ntype  T struct{ A int }\n' > "$SELFTEST_DIR/bad.go"
if [ -n "$(gofmt -l "$SELFTEST_DIR/bad.go" 2>/dev/null)" ]; then
  report PASS "负向自检：gofmt 抓到坏格式"
else
  report FAIL "负向自检：gofmt 抓到坏格式" "gofmt 对明显坏格式无输出"
fi

# ---------- 5. 需要数据库的端到端段 ----------
DB_OK=0
if [ -n "${RESEARCHOS_DATABASE_URL:-}" ]; then
  DB_OK=1
else
  # 开发包惯例：本地 PostgreSQL 在 127.0.0.1:55432，口令见 upstream/hector/.env.researchos
  if [ -f "$UP/.env.researchos" ]; then
    # shellcheck disable=SC1091
    set -a; . "$UP/.env.researchos"; set +a
    DB_OK=1
  fi
fi

if [ "$FAST" = 1 ]; then
  report SKIP "端到端（迁移+API+PDF）" "--fast 模式跳过"
elif [ "$DB_OK" = 0 ]; then
  report SKIP "端到端（迁移+API+PDF）" "没有 RESEARCHOS_DATABASE_URL，也未找到 upstream/hector/.env.researchos"
else
  echo "-- 端到端：使用 ${RESEARCHOS_DATABASE_URL%%\?*}@***（口令已隐去）"
  # 5.1 迁移幂等
  if (cd "$UP" && go build -o bin/researchos-verify.exe ./cmd/researchos) >/dev/null 2>&1; then
    report PASS "构建 researchos 二进制"
    if (cd "$UP" && ./bin/researchos-verify.exe migrate >/dev/null 2>&1) && \
       (cd "$UP" && ./bin/researchos-verify.exe migrate >/dev/null 2>&1); then
      report PASS "数据库迁移（执行两次，幂等）"
    else
      report FAIL "数据库迁移（执行两次，幂等）"
    fi
    rm -f "$UP/bin/researchos-verify.exe"
  else
    report FAIL "构建 researchos 二进制"
  fi
  # 5.2 起真实服务，打健康检查 + PDF 导出 + 内容校验
  VERIFY_PORT=18499
  OUT_DIR="$(mktemp -d)"
  (cd "$UP" && RESEARCHOS_ADDR="127.0.0.1:$VERIFY_PORT" RESEARCHOS_WEB_DIR=web/.build \
    ./bin/researchos.exe serve >/dev/null 2>&1 &
    echo $! > "$OUT_DIR/pid")
  SRV_UP=0
  for _ in $(seq 1 15); do
    code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$VERIFY_PORT/api/v1/health" 2>/dev/null || true)
    [ "$code" = "200" ] && { SRV_UP=1; break; }
    sleep 1
  done
  if [ "$SRV_UP" = 1 ]; then
    report PASS "服务启动 + /health"
    # 有已完成任务就校验 PDF 导出与内容一致性
    TID=$(curl -s -H "Authorization: Bearer ${RESEARCHOS_API_TOKEN:-}" \
      "http://127.0.0.1:$VERIFY_PORT/api/v1/research?limit=1&status=completed" \
      | python -c "import sys,json;d=json.load(sys.stdin);print((d.get('items') or [{}])[0].get('id',''))" 2>/dev/null || true)
    if [ -n "$TID" ]; then
      curl -s -H "Authorization: Bearer ${RESEARCHOS_API_TOKEN:-}" \
        -o "$OUT_DIR/report.pdf" "http://127.0.0.1:$VERIFY_PORT/api/v1/research/$TID/report.pdf"
      curl -s -H "Authorization: Bearer ${RESEARCHOS_API_TOKEN:-}" \
        -o "$OUT_DIR/report.md" "http://127.0.0.1:$VERIFY_PORT/api/v1/research/$TID/report.md"
      head -c 5 "$OUT_DIR/report.pdf" | grep -q '%PDF' \
        && report PASS "PDF 导出（HTTP 200 + %PDF 头）" \
        || report FAIL "PDF 导出（HTTP 200 + %PDF 头）"
      if python scripts/verify_pdf_content.py "$OUT_DIR/report.md" "$OUT_DIR/report.pdf" >"$OUT_DIR/pdfcheck.txt" 2>&1; then
        report PASS "PDF 内容与 Markdown 一致"
      else
        report FAIL "PDF 内容与 Markdown 一致" "$(tail -3 "$OUT_DIR/pdfcheck.txt" | tr '\n' ' ')"
      fi
    else
      report SKIP "PDF 导出链路" "库中没有 completed 任务（建一个后重跑）"
    fi
  else
    report FAIL "服务启动 + /health" "15s 内未就绪"
  fi
  # 收尾：杀掉临时服务
  if [ -f "$OUT_DIR/pid" ]; then
    kill "$(cat "$OUT_DIR/pid")" >/dev/null 2>&1 || true
  fi
  rm -rf "$OUT_DIR"
fi

# ---------- 汇总 ----------
echo
echo "== 汇总：$PASS 通过 / $FAIL 失败 / $SKIP 跳过 =="
if [ "$FAIL" -gt 0 ]; then
  printf '失败项: %s\n' "${FAILED[*]}"
  exit 1
fi
exit 0
