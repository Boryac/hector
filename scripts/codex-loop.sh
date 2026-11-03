#!/usr/bin/env bash
# Codex self-iteration loop prompt (human runs this, Codex reads it each round).
# It does NOT auto-run Codex; it prints the exact next-task instruction for the next round.
set -euo pipefail
echo "== Hector ResearchOS — Codex next round =="
echo "1. Read CODEX.md, PROGRESS.md, TASKS.md."
echo "2. Pick the first unchecked task whose dependencies are done."
grep -m1 "^- \[ \]" TASKS.md || echo "All tasks checked — run full acceptance in TESTING_AND_ACCEPTANCE.md."
echo "3. Implement it, test it, update TASKS.md + PROGRESS.md, commit as [phase-N][Txx] ..."
echo "4. Run: bash scripts/verify.sh"
echo "5. Repeat until no unchecked tasks remain AND full acceptance passes."
