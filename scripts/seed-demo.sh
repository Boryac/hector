#!/usr/bin/env bash
# Seed the final demo task via API once Phase 1+ is running. Replace base URL as needed.
set -euo pipefail
API="${API:-http://localhost:8080/api/v1}"
curl -s -X POST "$API/research" -H 'Content-Type: application/json' -d '{
  "goal": "研究全球 AI Agent 开源生态，筛选 10 个代表性项目并生成竞争分析报告。",
  "depth": "standard",
  "max_agents": 4,
  "enable_verification": true
}'
echo
echo "Seeded demo task. Check workspace, workflow canvas, sources, evidence, then report."
