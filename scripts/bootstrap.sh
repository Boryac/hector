#!/usr/bin/env bash
set -euo pipefail
echo "== Hector ResearchOS bootstrap =="
command -v go >/dev/null && go version || echo "[warn] go not found (needed for backend)"
command -v node >/dev/null && node --version || echo "[warn] node not found (needed for web)"
command -v docker >/dev/null && docker --version || echo "[warn] docker not found (needed for deployment)"
if [ ! -f .env.example ]; then
cat > .env.example <<'EOF'
DATABASE_URL=postgres://hector:changeme@localhost:5432/hector?sslmode=disable
REDIS_URL=
LLM_PROVIDER=
LLM_API_KEY=
LLM_MODEL=
SEARCH_PROVIDER=mock
SEARCH_API_KEY=
WORKER_CONCURRENCY=4
NODE_TIMEOUT=300s
TOOL_TIMEOUT=60s
OBJECT_STORAGE_PATH=./data
OTEL_EXPORTER_OTLP_ENDPOINT=
APP_BASE_URL=http://localhost:3000
EOF
echo "created .env.example (fill real keys in .env, never commit .env)"
fi
echo "bootstrap done. Next: Phase 0 audit (see TASKS.md T0.1-T0.7)."
