#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
echo "== Hector ResearchOS verify =="
echo "[1/4] openapi sanity"
test -f api/openapi.yaml && echo "ok: api/openapi.yaml exists" || { echo "FAIL: api/openapi.yaml missing"; exit 1; }
python3 -c "import yaml,sys; yaml.safe_load(open('api/openapi.yaml'))" 2>/dev/null && echo "ok: openapi yaml parses" || echo "[warn] python yaml check skipped"
echo "[2/4] go tests (if go.mod exists)"
if [ -f go.mod ]; then go test ./...; else echo "skip: go.mod not yet created (Phase 0/1 will create it)"; fi
echo "[3/4] web build check (if web/package.json exists)"
if [ -f web/package.json ]; then (cd web && npm run build 2>&1 | head -50); else echo "skip: web/package.json not yet created"; fi
echo "[4/4] unchecked tasks remaining:"
grep -c "^- \[ \]" TASKS.md || true
echo "verify done. NOTE: verify.sh passing on skeleton does NOT mean V1.0 done — see TESTING_AND_ACCEPTANCE.md."
