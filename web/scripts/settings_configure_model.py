#!/usr/bin/env python3
"""settings_configure_model.py — 通过设置中心真实 UI 接入本地模型（RC1 阶段3）。

用 Playwright 驱动真实浏览器操作设置页（选 Provider / 填 Base URL / 模型 ID /
API Key → 点击「保存更改」→ 点击「测试连接」→ 刷新验证持久化）。
本机 Chrome 156 headless 存在启动即退出的回归，故使用 Playwright 自带 Chromium。

key/token 通过环境变量传入，绝不写入文件/日志：
  LLM_KEY=... RESEARCHOS_TOKEN=... python settings_configure_model.py
"""
import json
import os
import sys
import urllib.request

from playwright.sync_api import sync_playwright

BASE = os.environ.get("RC1_BASE", "http://127.0.0.1:8080")
TOKEN = os.environ.get("RESEARCHOS_TOKEN")
LLM_KEY = os.environ.get("LLM_KEY")
MODEL_ID = os.environ.get("RC1_MODEL_ID", "cline/cline-free/solar-mini4")
API_BASE = os.environ.get("RC1_LLM_BASE_URL", "http://127.0.0.1:18326/v1")

if not TOKEN or not LLM_KEY:
    print("FAIL 需要 RESEARCHOS_TOKEN 与 LLM_KEY 环境变量")
    sys.exit(2)

results, failures = [], []


def assert_(name, ok, detail=""):
    results.append(f"{'ok  ' if ok else 'FAIL'} {name}" + ("" if ok else f" :: {detail}"))
    if not ok:
        failures.append(f"{name}: {detail}")
    print(("PASS " if ok else "FAIL ") + name + (" :: " + detail if not ok else ""))


def api(path):
    req = urllib.request.Request(BASE + path, headers={"Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode("utf-8"))


with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    # 先到站点再写 token（localStorage 需同源）
    page.goto(BASE + "/", wait_until="domcontentloaded")
    page.evaluate("t => localStorage.setItem('researchos_token', t)", TOKEN)
    page.goto(BASE + "/settings/", wait_until="domcontentloaded")
    page.wait_for_selector("#section-model", timeout=20000)
    page.locator("#section-model").scroll_into_view_if_needed()

    # 1) Provider=openai（React 受控 select）
    page.select_option("#setting-model-provider", "openai")
    assert_("UI 选择 Provider=openai", page.input_value("#setting-model-provider") == "openai")

    # 2) Base URL
    page.fill("#setting-model-base_url", API_BASE)
    assert_("UI 填写 Base URL", page.input_value("#setting-model-base_url") == API_BASE)

    # 3) 模型 ID
    page.fill("#setting-model-model", MODEL_ID)
    assert_("UI 填写模型 ID", page.input_value("#setting-model-model") == MODEL_ID)

    # 4) API Key（password 输入，不回显）
    page.fill("#setting-model-api_key", LLM_KEY)
    kt = page.locator("#setting-model-api_key").evaluate("el => ({type: el.type, len: el.value.length})")
    assert_("UI 填写 API Key（password）", kt["type"] == "password" and kt["len"] > 10, json.dumps(kt))

    # 5) savebar 计数与保存（捕获 PUT 响应）
    savebar_text = page.locator(".savebar-text").inner_text()
    assert_("savebar 显示 4 项待保存", "4" in savebar_text, savebar_text)
    with page.expect_response(
        lambda r: r.url.endswith("/api/v1/settings/model") and r.request.method == "PUT", timeout=20000
    ) as resp_info:
        page.click(".savebar button:has-text('保存更改')")
    put = resp_info.value
    assert_("PUT /api/v1/settings/model 返回 2xx", put.ok, f"status={put.status}")
    page.wait_for_timeout(1500)

    # 6) 服务端核验：落库 + 掩码
    snap = api("/api/v1/settings")
    section = next(s for s in snap["sections"] if s["id"] == "model")
    fv = {f["key"]: f for f in section["fields"]}
    assert_("provider 落库 openai（source=web）",
            fv["model.provider"]["value"] == "openai" and fv["model.provider"]["source"] == "web",
            json.dumps(fv["model.provider"], ensure_ascii=False))
    assert_("base_url 落库正确", fv["model.base_url"]["value"] == API_BASE, str(fv["model.base_url"]["value"]))
    assert_("model 落库正确", fv["model.model"]["value"] == MODEL_ID, str(fv["model.model"]["value"]))
    assert_("api_key configured 且全响应无明文泄漏",
            fv["model.api_key"]["configured"] is True and LLM_KEY not in json.dumps(snap),
            f"configured={fv['model.api_key']['configured']}")

    # 7) /system 显示 LLM 已启用（同进程 Worker 热生效）
    sysinfo = api("/api/v1/system")
    assert_("/system llm.enabled=true 且模型正确",
            sysinfo["llm"]["enabled"] is True and sysinfo["llm"]["model"] == MODEL_ID,
            json.dumps(sysinfo["llm"], ensure_ascii=False))

    # 8) 「测试连接」真实调用（UI 按钮 → 服务端最小 LLM 请求）
    with page.expect_response(lambda r: r.url.endswith("/api/v1/settings/llm/test"), timeout=60000) as tinfo:
        page.click(".settings-test button:has-text('测试连接')")
    tr = tinfo.value
    body = tr.json() if tr.ok else {}
    assert_("测试连接真实 LLM 请求 2xx", tr.ok, f"status={tr.status}")
    print(f"测试连接结果: ok={body.get('ok')} provider={body.get('provider')} model={body.get('model')} "
          f"latency={body.get('latency_ms')}ms reply={str(body.get('reply'))[:60]}")
    assert_("测试连接返回连接正常", body.get("ok") is True, json.dumps(body, ensure_ascii=False)[:200])

    # 9) 刷新验证持久化
    page.reload(wait_until="domcontentloaded")
    page.wait_for_selector("#setting-model-provider", timeout=20000)
    page.wait_for_timeout(800)
    persisted = {
        "provider": page.input_value("#setting-model-provider"),
        "url": page.input_value("#setting-model-base_url"),
        "model": page.input_value("#setting-model-model"),
    }
    assert_("刷新后 UI 显示已保存配置", persisted["provider"] == "openai"
            and persisted["url"] == API_BASE and persisted["model"] == MODEL_ID, json.dumps(persisted))

    browser.close()

ok_n = sum(1 for r in results if r.startswith("ok"))
print(f"\n== 汇总：{ok_n} 通过 / {len(failures)} 失败 ==")
sys.exit(1 if failures else 0)
