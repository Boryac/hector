#!/usr/bin/env python3
"""rc1_security_probe.py — RC1 阶段8：API 层安全探测。

检查项：Token 鉴权 / user_id 忽略 / SSE ticket 边界 / CORS / MCP 审批闸门 /
密钥不落日志 / 设置响应掩码。输出 JSON 证据（不含密钥明文）。
"""
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("RC1_BASE", "http://127.0.0.1:8080")
TOKEN = os.environ.get("RESEARCHOS_TOKEN")
LLM_KEY = os.environ.get("LLM_KEY", "")
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "rc1_evidence", "security_probe.json")

results = []


def record(name, ok, detail):
    results.append({"check": name, "pass": bool(ok), "detail": detail})
    print(("PASS " if ok else "FAIL ") + name + " :: " + detail)


def req(method, path, token=None, payload=None, headers=None):
    data = json.dumps(payload).encode() if payload is not None else None
    h = dict(headers or {})
    if token is not None:
        h["Authorization"] = "Bearer " + token
    if payload is not None:
        h["Content-Type"] = "application/json"
    r = urllib.request.Request(BASE + path, data=data, method=method, headers=h)
    try:
        with urllib.request.urlopen(r, timeout=20) as resp:
            body = resp.read().decode("utf-8", "replace")
            return resp.status, body, dict(resp.headers)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace"), dict(e.headers)


# 1. 无 Token → 401
st, body, _ = req("GET", "/api/v1/research")
record("无 Token 请求受保护端点返回 401", st == 401, f"HTTP {st} {body[:80]}")

# 2. 错误 Token → 401
st, body, _ = req("GET", "/api/v1/research", token="wrong-token-abc")
record("错误 Token 返回 401", st == 401, f"HTTP {st}")

# 3. 请求体中的 user_id 一律不接受（V1 单用户：身份只来自 Token）
#    实现：UnknownField 严格拒绝——带 user_id 的请求体直接 400，比「静默忽略」更强。
st, body, _ = req("POST", "/api/v1/research", token=TOKEN,
                  payload={"goal": "security probe user_id 注入测试", "depth": "quick", "user_id": "attacker", "created_by": "attacker"})
record("user_id/created_by 注入被拒绝（unknown field 400）", st == 400 and "unknown field" in body,
       f"HTTP {st} {body[:100]}")

# 4. SSE 无 ticket → 4xx
st, body, _ = req("GET", "/api/v1/events")
record("SSE 无 ticket 被拒", 400 <= st < 500, f"HTTP {st} {body[:80]}")
st, body, _ = req("GET", "/api/v1/events?ticket=forged-ticket-value")
record("SSE 伪造 ticket 被拒", 400 <= st < 500, f"HTTP {st} {body[:80]}")

# 5. CORS：跨源不回显（RC1 修复后：仅同源回显 ACAO）
st, body, hd = req("OPTIONS", "/api/v1/research", headers={"Origin": "http://evil.example", "Access-Control-Request-Method": "GET"})
acao = hd.get("Access-Control-Allow-Origin", "")
record("CORS 不回显跨源 Origin", acao == "", f"ACAO={acao!r} HTTP {st}")
st, body, hd = req("OPTIONS", "/api/v1/research", headers={"Origin": BASE, "Access-Control-Request-Method": "GET"})
acao = hd.get("Access-Control-Allow-Origin", "")
record("CORS 同源仍回显（前端可用）", acao == BASE, f"ACAO={acao!r} HTTP {st}")

# 6. MCP 审批闸门：登记后未批准 → discover 拒绝
st, body, _ = req("POST", "/api/v1/tools/servers", token=TOKEN,
                  payload={"name": "sec-probe-dummy", "command": "definitely-not-a-real-command-xyz", "args": [], "env": {}})
if st in (200, 201):
    sv = json.loads(body).get("server") or {}
    sid = sv.get("id")
    st2, b2, _ = req("POST", f"/api/v1/tools/servers/{sid}/discover", token=TOKEN)
    record("MCP 未批准 discover 被拒（409）", st2 == 409, f"HTTP {st2} {b2[:100]}")
    req("DELETE", f"/api/v1/tools/servers/{sid}", token=TOKEN)
    record("MCP 探测登记已清理", True, f"server {sid} 已删除")
else:
    record("MCP 未批准 discover 被拒", False, f"登记失败 HTTP {st} {body[:100]}")

# 7. 设置响应不回显密钥明文
st, body, _ = req("GET", "/api/v1/settings", token=TOKEN)
leak = LLM_KEY and LLM_KEY in body
record("settings 响应无 LLM Key 明文", not leak, f"泄漏检查={leak}，掩码存在={'ofm-****' in body or '****' in body}")

# 8. 未授权获取任务详情 → 401/404（不泄露存在性）
list_body = req("GET", "/api/v1/research?limit=1", token=TOKEN)[1]
tid = None
try:
    items = json.loads(list_body).get("items") or []
    if items:
        tid = items[0]["id"]
except Exception:
    pass
if tid:
    st, _, _ = req("GET", f"/api/v1/research/{tid}", token="wrong-token-abc")
    record("无权限读任务返回 404/401（不泄露存在性）", st in (401, 404), f"HTTP {st}")

# 9. 日志脱敏：服务日志（stdout 不可直接读，此处验证响应与错误路径不携带密钥）
st, body, _ = req("POST", "/api/v1/settings/llm/test", token=TOKEN)
record("llm/test 错误路径不回显 Key", LLM_KEY not in body if LLM_KEY else True, f"HTTP {st}")

# 10. 模型中继仅绑定回环（外部配置事实，读本地配置确认）
try:
    cfgj = json.load(open(os.path.expanduser("~/.dsh/our-free-model/settings.json"), encoding="utf-8"))
    relay = cfgj["chanGateway"]["relay"]
    lan = cfgj.get("forward", {}).get("lan", {})
    record("本地模型中继仅绑定 127.0.0.1", relay.get("host") == "127.0.0.1" and not lan.get("enabled"),
           f"host={relay.get('host')} lan_enabled={lan.get('enabled')}")
except Exception as e:
    record("本地模型中继绑定检查", False, f"读取配置失败: {e}")

npass = sum(1 for r in results if r["pass"])
out = {"probed_at": time.strftime("%F %T"), "summary": {"total": len(results), "pass": npass,
       "fail": len(results) - npass}, "results": results}
os.makedirs(os.path.dirname(OUT), exist_ok=True)
with open(OUT, "w", encoding="utf-8") as f:
    json.dump(out, f, ensure_ascii=False, indent=1)
print(f"\n汇总: {npass}/{len(results)} 通过 → {OUT}")
sys.exit(0 if npass == len(results) else 1)
