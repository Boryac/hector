#!/usr/bin/env bash
# Hector ResearchOS 本机初始化：检查工具链 + 生成 .env.researchos
#
# 说明：本脚本只处理 ResearchOS 产品链路（cmd/researchos）的配置。
# 上游 Hector 自己的 .env.example（含 OPENAI_API_KEY 等）属于另一子系统，本脚本不碰。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HECTOR="$ROOT/upstream/hector"
ENV_FILE="$HECTOR/.env.researchos"

echo "== Hector ResearchOS bootstrap =="

check() { # $1=命令（可带参数） $2=缺失时的说明
  local cmd="$1" hint="$2" name out
  name="${cmd%% *}"
  if ! command -v "$name" >/dev/null 2>&1; then
    printf '  [warn] %-10s 未安装（%s）\n' "$name" "$hint"
    return 1
  fi
  out="$($cmd 2>&1 | head -1)"
  printf '  [ok]   %-10s %s\n' "$name" "$out"
  return 0
}
check "go version" "后端构建需要 Go 1.25+"
check "node --version" "仅在重新构建前端时需要"
check "psql --version" "可选用系统 PostgreSQL；也可用便携版或 Docker" || true

gen_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
  elif command -v python >/dev/null 2>&1; then
    python -c "import secrets;print(secrets.token_hex(24))"
  else
    # 最后兜底：时间戳 + 进程号，安全性弱于随机源，仅用于本机
    printf '%s-%s' "$(date +%s%N)" "$$"
  fi
}

if [ -f "$ENV_FILE" ]; then
  echo "  [ok]   $ENV_FILE 已存在，保持不变"
else
  TOKEN="$(gen_token)"
  cat > "$ENV_FILE" <<EOF
# Hector ResearchOS 本机配置（自动生成；已被 .gitignore 忽略，勿提交）
# 变量名必须带 RESEARCHOS_ 前缀，否则服务读不到。

# --- 必填 ---
RESEARCHOS_DATABASE_URL=postgres://postgres@127.0.0.1:5432/researchos?sslmode=disable
RESEARCHOS_API_TOKEN=$TOKEN
RESEARCHOS_ADDR=127.0.0.1:8080

# --- 单用户本地身份（服务端唯一身份，客户端无法指定）---
RESEARCHOS_LOCAL_USER=local-user

# --- LLM（留空则规划与抽取降级为本地确定性流程，报告会如实标注 heuristic）---
RESEARCHOS_LLM_PROVIDER=none
RESEARCHOS_LLM_API_KEY=
RESEARCHOS_LLM_MODEL=
RESEARCHOS_LLM_BASE_URL=

# --- 检索 Provider（github / hackernews / wikipedia / bing 免密钥）---
RESEARCHOS_SEARCH_PROVIDERS=github,hackernews,wikipedia,bing
RESEARCHOS_GITHUB_TOKEN=

# --- 执行与并发 ---
RESEARCHOS_WORKERS=4
RESEARCHOS_MAX_AGENTS=4
RESEARCHOS_NODE_TIMEOUT=180s
RESEARCHOS_STALE_LEASE=3m
RESEARCHOS_REQUIRE_APPROVAL=true

# --- 数据与静态前端 ---
RESEARCHOS_DATA_DIR=./data
RESEARCHOS_WEB_DIR=./web/.build
EOF
  echo "  [ok]   已生成 $ENV_FILE（API Token 随机生成，长度 48）"
fi

echo
echo "下一步："
echo "  1. 起数据库（三选一）：系统 PostgreSQL / .devtools/pg 便携版 / docker compose up db"
echo "  2. cd upstream/hector && go run ./cmd/researchos migrate"
echo "  3. go run ./cmd/researchos all     # API + Worker 同进程，默认 127.0.0.1:8080"
echo "  4. 浏览器打开 http://127.0.0.1:8080/ ，Token 取 $ENV_FILE 中的 RESEARCHOS_API_TOKEN"
