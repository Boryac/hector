#!/usr/bin/env python3
"""rc1_run_research.py — RC1 阶段4：真实模型研究任务运行器。

创建三类研究任务（事实核查 / 多对象对比 / 开放式专题），轮询至终态，
抓取全链路证据（plan/DAG/agents/sources/evidence/report/usage/耗时）保存为 JSON。
证据文件只含任务数据，不含任何密钥。

用法：
  RESEARCHOS_TOKEN=... python scripts/rc1_run_research.py [--base URL] [--timeout 秒]
"""
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("RC1_BASE", "http://127.0.0.1:8080")
TOKEN = os.environ.get("RESEARCHOS_TOKEN")
OUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "rc1_evidence")

TASKS = [
    {
        "key": "A-factcheck",
        "goal": "核实 Go 编程语言当前最新的稳定大版本号、官方维护组织与开源许可证类型，并给出依据",
        "depth": "quick",
    },
    {
        "key": "B-comparison",
        "goal": "对比 PostgreSQL、MySQL、SQLite 三种开源数据库在许可证、典型应用场景与嵌入式适用性三个维度上的差异",
        "depth": "standard",
    },
    {
        "key": "C-open",
        "goal": "检索增强生成（RAG）技术在企业知识库场景的落地路径、主要技术挑战与工程最佳实践",
        "depth": "deep",
    },
]


def call(method, path, payload=None, timeout=60):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        BASE + path, data=data, method=method,
        headers={"Authorization": "Bearer " + TOKEN, "Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8"))


def get(path):
    return call("GET", path)


def run_one(spec, timeout_s):
    print(f"\n===== 任务 {spec['key']} (depth={spec['depth']}) =====")
    t0 = time.time()
    created = call("POST", "/api/v1/research", {"goal": spec["goal"], "depth": spec["depth"]})
    task = created.get("task") or created
    tid = task["id"]
    print(f"task id: {tid} | status: {task.get('status')} | planner: {task.get('planner_mode', '待规划')}")

    evidence = {"key": spec["key"], "task_id": tid, "goal": spec["goal"], "depth": spec["depth"],
                "created_at": time.strftime("%F %T"), "model_id_expected": "cline/cline-free/solar-mini4"}

    # 轮询至终态（RESEARCHOS_REQUIRE_APPROVAL=true 时先批准再执行）
    final, last, approved = None, "", False
    deadline = t0 + timeout_s
    while time.time() < deadline:
        time.sleep(6)
        try:
            cur = get(f"/api/v1/research/{tid}")
        except urllib.error.HTTPError as e:
            print(f"  轮询 HTTP {e.code}，继续")
            continue
        detail = cur.get("task") or cur
        st = detail.get("status")
        if (st, detail.get("progress")) != last:
            print(f"  [{time.time()-t0:6.0f}s] status={st} progress={detail.get('progress')} phase={detail.get('phase','')}")
            last = (st, detail.get("progress"))
        if st == "awaiting_approval" and not approved:
            try:
                ap = call("POST", f"/api/v1/research/{tid}/approve")
                approved = True
                print(f"  [{time.time()-t0:6.0f}s] 已批准（approve 2xx），开始执行")
            except urllib.error.HTTPError as e:
                print(f"  批准失败 HTTP {e.code}")
        if st in ("completed", "failed", "cancelled"):
            final = detail
            break
    evidence["wall_seconds"] = round(time.time() - t0, 1)
    if not final:
        evidence["status"] = "timeout"
        print("  超时未终态")
        return evidence

    evidence["status"] = final.get("status")
    evidence["planner_mode"] = final.get("planner_mode")
    evidence["task_detail"] = final

    # 全链路证据抓取（plan/nodes 在 task_detail 内；agents/sources/evidence/report 走子端点）
    for name, path in [
        ("agents", f"/api/v1/research/{tid}/agents"),
        ("sources", f"/api/v1/research/{tid}/sources"),
        ("evidence", f"/api/v1/research/{tid}/evidence"),
        ("report", f"/api/v1/research/{tid}/report"),
    ]:
        try:
            evidence[name] = get(path)
            print(f"  {name}: OK")
        except urllib.error.HTTPError as e:
            evidence[name] = {"error": f"HTTP {e.code}"}
            print(f"  {name}: HTTP {e.code}")

    # 摘要统计
    def count_of(x):
        if isinstance(x, list):
            return len(x)
        if isinstance(x, dict):
            for k in ("items", "nodes", "agents", "sources", "evidence"):
                if isinstance(x.get(k), list):
                    return len(x[k])
        return None

    agents_items = (evidence.get("agents") or {}).get("items", []) if isinstance(evidence.get("agents"), dict) else []
    token_total = 0
    models = set()
    for a in agents_items:
        tu = a.get("token_usage") or {}
        if isinstance(tu, dict):
            token_total += tu.get("total_tokens", 0) or 0
        if a.get("model"):
            models.add(a["model"])
    plan = final.get("plan") or {}
    rep = evidence.get("report") or {}
    md = rep.get("report") if isinstance(rep, dict) else None
    evidence["summary"] = {
        "agents": count_of(evidence.get("agents")),
        "sources": count_of(evidence.get("sources")),
        "evidence_items": count_of(evidence.get("evidence")),
        "dag_nodes": len(final.get("nodes") or []),
        "research_questions": len(plan.get("research_questions") or []),
        "subtasks": len(plan.get("subtasks") or []),
        "agent_models": sorted(models),
        "agent_token_usage_total": token_total,
        "report_md_chars": len(md) if isinstance(md, str) else 0,
        "report_versions": rep.get("versions") if isinstance(rep, dict) else None,
    }
    print(f"  终态 {evidence['status']}，耗时 {evidence['wall_seconds']}s，summary={json.dumps(evidence['summary'], ensure_ascii=False)[:400]}")
    return evidence


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--timeout", type=int, default=1200, help="单任务超时秒数")
    ap.add_argument("--only", default=None, help="只跑指定 key（A-factcheck 等）")
    args = ap.parse_args()

    os.makedirs(OUT_DIR, exist_ok=True)
    all_results = {}
    for spec in TASKS:
        if args.only and spec["key"] != args.only:
            continue
        ev = run_one(spec, args.timeout)
        path = os.path.join(OUT_DIR, f"task_{spec['key']}.json")
        with open(path, "w", encoding="utf-8") as f:
            json.dump(ev, f, ensure_ascii=False, indent=1)
        print(f"  证据已保存: {path}")
        all_results[spec["key"]] = {"status": ev["status"], "wall": ev["wall_seconds"], "task_id": ev["task_id"]}

    print("\n== 运行汇总 ==")
    for k, v in all_results.items():
        print(f"  {k}: {v['status']} ({v['wall']}s) task={v['task_id']}")
    bad = [k for k, v in all_results.items() if v["status"] != "completed"]
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
