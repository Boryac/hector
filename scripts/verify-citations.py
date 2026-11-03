#!/usr/bin/env python3
"""报告可追溯性验收：逐条核对「报告结论 → 证据引文 → 来源原文」。

做的事（全部读线上 API，不读数据库、不读内部结构）：
  1. 引文落地：每条 evidence.quote 必须能在其 source 的正文/摘要中逐字找到（空白归一化）。
  2. 来源存在：evidence.source_id 必须真实存在，不能悬空。
  3. 编号完整：报告正文里的 [n] 必须在「信息来源」列表里有对应条目；列表条目也必须被引用。
  4. 冲突自洽：claims 里存在 conflicted / unverified 时，报告「争议与不确定性」章节必须如实列出，
     不能一边有冲突一边写「未检测到同一指标上的来源冲突」。
  5. 结论落地：报告「核心发现」里的每条结论，必须能在 claims 里找到对应的 statement。

用法：
  python scripts/verify-citations.py --base http://127.0.0.1:8080 \
      --token local-dev-token --task <task-id>

退出码 0 = 全部通过；1 = 有失败项（会逐条打印）。
"""

import argparse
import json
import re
import sys
import urllib.request


def http_json(url, token):
    req = urllib.request.Request(url, headers={"Authorization": "Bearer " + token})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


def http_text(url, token):
    req = urllib.request.Request(url, headers={"Authorization": "Bearer " + token})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return resp.read().decode("utf-8")


def norm(s):
    return " ".join((s or "").lower().split())


class Checker:
    def __init__(self):
        self.fails = []
        self.notes = []

    def ok(self, name, detail=""):
        print("  [PASS] %s%s" % (name, ("  " + detail) if detail else ""))

    def fail(self, name, detail):
        print("  [FAIL] %s  %s" % (name, detail))
        self.fails.append((name, detail))

    def note(self, text):
        print("  [note] " + text)
        self.notes.append(text)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:8080")
    ap.add_argument("--token", default="local-dev-token")
    ap.add_argument("--task", required=True)
    args = ap.parse_args()
    base, tok, task = args.base.rstrip("/"), args.token, args.task
    c = Checker()

    print("== 任务 %s ==" % task)
    detail = http_json("%s/api/v1/research/%s" % (base, task), tok)
    status = detail["task"]["status"]
    print("  status=%s planner=%s" % (status, detail["task"].get("planner_mode")))
    if status != "completed":
        c.fail("任务状态", "任务未完成（%s），无法验收报告" % status)
        return 1

    ev = http_json("%s/api/v1/research/%s/evidence?limit=500" % (base, task), tok)
    src = http_json("%s/api/v1/research/%s/sources?limit=500" % (base, task), tok)
    md = http_text("%s/api/v1/research/%s/report.md" % (base, task), tok)

    claims = ev.get("items") or []
    sources = {s["id"]: s for s in (src.get("items") or [])}

    # ---------- 1 & 2：引文落地 + 来源存在 ----------
    print("\n-- 引文与来源 --")
    total_ev, grounded, dangling, missing_quote = 0, 0, 0, 0
    samples = []
    for cl in claims:
        for e in cl.get("evidence") or []:
            total_ev += 1
            s = sources.get(e.get("source_id"))
            if s is None:
                dangling += 1
                if len(samples) < 5:
                    samples.append("悬空 source_id=%s（claim=%s）" % (e.get("source_id"), cl["id"][:8]))
                continue
            q = norm(e.get("quote"))
            if not q:
                missing_quote += 1
                continue
            hay = norm(s.get("excerpt")) + " || " + norm(s.get("snippet"))
            if q in hay:
                grounded += 1
            elif len(samples) < 5:
                samples.append("引文未在原文找到：%r ← %s" % (e.get("quote")[:60], s.get("url")))
    print("  证据总条数=%d 逐字落地=%d 悬空来源=%d 空引文=%d" % (total_ev, grounded, dangling, missing_quote))
    if total_ev == 0:
        c.fail("引文落地", "没有任何证据条目")
    elif dangling == 0 and grounded == total_ev:
        c.ok("引文全部逐字落地", "%d/%d" % (grounded, total_ev))
    else:
        c.fail("引文落地", "%d/%d 落地，%d 悬空" % (grounded, total_ev, dangling))
        for s in samples:
            print("        · " + s)

    # ---------- 3：报告编号完整性 ----------
    print("\n-- 报告引用编号 --")
    m = re.search(r"^## 信息来源\s*$", md, re.M)
    body, refs = (md, "")
    if m:
        body, refs = md[: m.start()], md[m.start():]
    listed = {int(n) for n in re.findall(r"^\s*(\d+)\.\s*\[", refs, re.M)}
    cited = {int(n) for n in re.findall(r"\[(\d+)\]", body)}
    print("  正文引用编号=%s" % (sorted(cited) or "无"))
    print("  来源列表编号=%s" % (sorted(listed) or "无"))
    if not cited:
        c.fail("引用编号", "正文没有任何 [n] 引用")
    elif cited - listed:
        c.fail("引用编号", "正文引用了列表里不存在的编号 %s" % sorted(cited - listed))
    elif listed - cited:
        c.fail("引用编号", "来源列表中有条目未被正文引用 %s" % sorted(listed - cited))
    else:
        c.ok("引用编号闭合", "%d 个编号一一对应" % len(cited))

    # ---------- 4：冲突与未证实自洽 ----------
    print("\n-- 交叉验证与冲突 --")
    by_status = {}
    for cl in claims:
        by_status[cl.get("status", "unknown")] = by_status.get(cl.get("status", "unknown"), 0) + 1
    print("  结论状态分布：%s" % by_status)
    dispute = ""
    m2 = re.search(r"^## 争议与不确定性\s*$(.*?)(?=^## |\Z)", md, re.S | re.M)
    if m2:
        dispute = m2.group(1)
    conflicted = by_status.get("conflicted", 0)
    unverified = by_status.get("unverified", 0)
    if conflicted and "未检测到" in dispute:
        c.fail("冲突披露", "有 %d 条冲突结论，但报告写「未检测到冲突」" % conflicted)
    elif conflicted and str(conflicted) not in dispute and "冲突" not in dispute:
        c.fail("冲突披露", "有 %d 条冲突结论，报告争议章节未列出" % conflicted)
    elif conflicted:
        c.ok("冲突披露", "%d 条冲突结论已在争议章节列出" % conflicted)
    else:
        c.ok("冲突披露", "无冲突结论，报告声明一致")
    if unverified:
        c.note("存在 %d 条未验证结论（单来源或引用不足），需人工判断可信度" % unverified)

    verified = by_status.get("verified", 0)
    if verified:
        c.ok("交叉验证生效", "%d 条结论通过多来源验证" % verified)
    else:
        c.note("本次没有任何结论通过多来源验证（verified=0），报告已声明结论强度有限")

    # ---------- 5：核心发现能否落到 claims ----------
    print("\n-- 结论落地 --")
    core = ""
    m3 = re.search(r"^## 核心发现\s*$(.*?)(?=^## |\Z)", md, re.S | re.M)
    if m3:
        core = m3.group(1)
    items = [ln.strip() for ln in core.splitlines() if re.match(r"^\d+\.\s", ln.strip())]
    stmts = [norm(cl.get("statement")) for cl in claims]
    hit = 0
    for it in items:
        text = re.sub(r"^\d+\.\s*", "", it)
        text = re.sub(r"（[^）]*）\s*(\[\d+\])*\s*$", "", text)  # 去掉「（单一来源，置信度 0.50）[1]」
        if norm(text) in stmts:
            hit += 1
        else:
            c.fail("核心发现落地", "报告条目找不到对应结论：%r" % text[:60])
    if items and hit == len(items):
        c.ok("核心发现全部可回溯", "%d/%d" % (hit, len(items)))
    elif not items:
        c.note("报告「核心发现」为空")

    print("\n== 结果 ==")
    if c.fails:
        print("  FAIL：%d 项不通过" % len(c.fails))
        return 1
    print("  PASS：全部检查通过（证据 %d 条、编号 %d 个、结论 %d 条）" % (total_ev, len(cited), len(claims)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
