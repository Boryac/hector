#!/usr/bin/env python3
"""rc1_browser_audit.py — RC1 阶段9：浏览器审计（Playwright 版）。

Chrome 156 在本机存在 headless 启动即退出的回归，改用 Playwright Chromium。
覆盖：7 路由渲染（无 React 运行时错误、无 4xx/5xx API 调用）+ 真实 UI 交互
（新建研究 → 落库 → 清理；设置页字段渲染）。
"""
import json
import os
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

BASE = os.environ.get("RC1_BASE", "http://127.0.0.1:8080")
TOKEN = os.environ.get("RESEARCHOS_TOKEN")
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "rc1_evidence", "browser_audit.json")

if not TOKEN:
    print("FAIL 需要 RESEARCHOS_TOKEN")
    sys.exit(2)

results, failures = [], []


def assert_(name, ok, detail=""):
    results.append({"check": name, "pass": bool(ok), "detail": detail})
    if not ok:
        failures.append(f"{name}: {detail}")
    print(("PASS " if ok else "FAIL ") + name + ("" if ok else " :: " + detail))


def api_get(path):
    req = urllib.request.Request(BASE + path, headers={"Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode("utf-8"))


PAGES = [
    ("Dashboard", "/", ["/api/v1/stats", "/api/v1/research"]),
    ("任务列表", "/tasks/", ["/api/v1/research"]),
    ("来源库", "/sources/", ["/api/v1/sources"]),
    ("报告页", "/reports/", ["/api/v1/reports"]),
    ("设置中心", "/settings/", ["/api/v1/settings", "/api/v1/system"]),
]

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    js_errors = []
    page.on("pageerror", lambda e: js_errors.append(str(e)))
    api_calls = []
    page.on("response", lambda r: api_calls.append({"url": r.url.replace(BASE, ""), "status": r.status})
            if "/api/" in r.url else None)

    # token 注入
    page.goto(BASE + "/", wait_until="domcontentloaded")
    page.evaluate("t => localStorage.setItem('researchos_token', t)", TOKEN)

    # 7 路由渲染审计
    routes = [("/", "Dashboard")] + [(p_, n) for n, p_, _ in PAGES[1:]] + [(None, None)]
    for name, path, must_call in PAGES:
        js_errors.clear()
        api_calls.clear()
        page.goto(BASE + path, wait_until="domcontentloaded")
        page.wait_for_timeout(2500)
        assert_(f"{name} 无 React 运行时错误", not js_errors, js_errors[0][:120] if js_errors else "")
        bad = [c for c in api_calls if c["status"] >= 400]
        assert_(f"{name} 无 4xx/5xx API 调用", not bad, json.dumps(bad)[:150])
        for api in must_call:
            hit = any(c["url"].split("?")[0] == api and 200 <= c["status"] < 300 for c in api_calls)
            assert_(f"{name} 调用 {api}", hit, "未发出或非 2xx")

    # 研究工作台逐 Tab 懒加载
    items = api_get("/api/v1/research?limit=10").get("items") or []
    completed = next((t for t in items if t.get("status") == "completed"), None)
    if completed:
        tid = completed["id"]
        api_calls.clear()
        page.goto(BASE + f"/task/?id={tid}", wait_until="domcontentloaded")
        page.wait_for_timeout(2500)
        for label, api in [("来源", f"/api/v1/research/{tid}/sources"), ("证据", f"/api/v1/research/{tid}/evidence"),
                           ("Agent", f"/api/v1/research/{tid}/agents"), ("报告", f"/api/v1/research/{tid}/report")]:
            page.locator(f'[role="tab"], .tabs button').filter(has_text=label).first.click()
            page.wait_for_timeout(1500)
            hit = any(c["url"].split("?")[0] == api and 200 <= c["status"] < 300 for c in api_calls)
            assert_(f"工作台 Tab「{label}」懒加载 {api.split('/')[-1]}", hit)
        sse = any("ticket=" in c["url"] for c in api_calls)
        assert_("工作台建立 SSE 事件流", sse)
    else:
        assert_("研究工作台审计", False, "无 completed 任务")

    # 设置页字段渲染（模型配置应显示已保存值）
    page.goto(BASE + "/settings/", wait_until="domcontentloaded")
    page.wait_for_selector("#setting-model-provider", timeout=15000)
    provider = page.input_value("#setting-model-provider")
    model_val = page.input_value("#setting-model-model")
    assert_("设置页显示已接入模型", provider == "openai" and "solar-mini4" in model_val,
            f"provider={provider} model={model_val}")
    fields = page.locator(".settings-control input, .settings-control select").count()
    assert_("设置页渲染 schema 字段控件", fields >= 8, f"控件数={fields}")

    # 真实 UI 交互：新建研究 → 落库 → 清理
    page.goto(BASE + "/tasks/", wait_until="domcontentloaded")
    page.wait_for_timeout(2000)
    page.locator("button, a").filter(has_text="新建").first.click()
    page.wait_for_timeout(800)
    ta = page.locator("#research-goal")
    assert_("新建对话框有目标输入框", ta.count() > 0)
    ta.fill("rc1-browser-audit 临时任务：验证 UI 创建链路")
    go = page.locator("button").filter(has_text="创建并规划").first
    go.click()
    page.wait_for_timeout(4000)
    items = api_get("/api/v1/research?limit=30").get("items") or []
    hit = next((t for t in items if "rc1-browser-audit" in (t.get("goal") or t.get("title") or "")), None)
    assert_("UI 创建任务真实落库", hit is not None)
    if hit:
        req_del = urllib.request.Request(BASE + f"/api/v1/research/{hit['id']}", method="DELETE",
                                         headers={"Authorization": "Bearer " + TOKEN})
        try:
            urllib.request.urlopen(req_del, timeout=15)
            assert_("清理临时任务", True)
        except urllib.error.HTTPError as e:
            assert_("清理临时任务", e.code in (409, 404), f"HTTP {e.code}")

    browser.close()

ok_n = sum(1 for r in results if r["pass"])
with open(OUT, "w", encoding="utf-8") as f:
    json.dump({"audited_at": time.strftime("%F %T"), "summary": {"total": len(results), "pass": ok_n,
               "fail": len(failures)}, "results": results}, f, ensure_ascii=False, indent=1)
print(f"\n== 汇总：{ok_n} 通过 / {len(failures)} 失败 → {OUT} ==")
sys.exit(1 if failures else 0)
