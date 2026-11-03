#!/usr/bin/env python3
"""RC1 本地模型能力探测脚本。

用法（key 通过环境变量传入，绝不写入文件/日志）：
  LLM_KEY=... python scripts/rc1_probe_model.py [--model ID] [--out FILE]

输出：stdout 人读摘要 + JSON 结果文件（不含 key）。
"""
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:18326/v1"
KEY = os.environ.get("LLM_KEY", "")
if not KEY:
    print("错误：未设置 LLM_KEY 环境变量", file=sys.stderr)
    sys.exit(2)

results = []


def record(name, ok, detail, extra=None):
    r = {"test": name, "pass": bool(ok), "detail": detail}
    if extra:
        r["extra"] = extra
    results.append(r)
    print(("PASS " if ok else "FAIL ") + name + " :: " + detail)
    return r


def post(path, payload, timeout=120, stream=False):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer " + KEY,
        },
        method="POST",
    )
    return urllib.request.urlopen(req, timeout=timeout)


def chat(messages, **kw):
    payload = {"model": kw.pop("model", MODEL), "messages": messages, **kw}
    t0 = time.time()
    try:
        with post("/chat/completions", payload, timeout=kw.pop("_timeout", 120)) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            return resp.status, body, time.time() - t0, None
    except urllib.error.HTTPError as e:
        try:
            body = json.loads(e.read().decode("utf-8"))
        except Exception:
            body = {"raw": "<non-json>"}
        return e.code, body, time.time() - t0, None
    except Exception as e:  # 超时/连接错误
        return None, None, time.time() - t0, str(e)


def msg_of(body):
    try:
        return body["choices"][0]["message"]
    except Exception:
        return {}


def content_of(body):
    m = msg_of(body)
    c = m.get("content")
    return c if isinstance(c, str) else json.dumps(c, ensure_ascii=False)[:200]


def main():
    global MODEL
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default="cline/cline-free/solar-mini4")
    ap.add_argument("--out", default=os.path.join(os.path.dirname(__file__), "rc1_probe_results.json"))
    args = ap.parse_args()
    MODEL = args.model
    record("model_id", True, "使用模型 ID: " + MODEL)

    # 2.2.1 中文问答
    st, b, dt, err = chat([{"role": "user", "content": "用一句话说明什么是幂等性。"}])
    ok = st == 200 and bool(content_of(b).strip())
    record("chat_zh", ok, f"HTTP {st} {dt:.1f}s: {content_of(b)[:120]}", {"usage": b.get("usage") if b else None})

    # 2.2.2 英文问答
    st, b, dt, err = chat([{"role": "user", "content": "Explain CAP theorem in one sentence."}])
    ok = st == 200 and bool(content_of(b).strip())
    record("chat_en", ok, f"HTTP {st} {dt:.1f}s: {content_of(b)[:120]}")

    # 2.2.3 多轮上下文
    st1, b1, _, _ = chat([{"role": "user", "content": "我叫黄博，请记住。回复OK即可。"}])
    st2, b2, _, _ = chat([
        {"role": "user", "content": "我叫黄博，请记住。回复OK即可。"},
        {"role": "assistant", "content": content_of(b1)},
        {"role": "user", "content": "我姓什么？"},
    ])
    ok = st2 == 200 and "黄" in content_of(b2)
    record("chat_multiturn", ok, f"round2 HTTP {st2}: {content_of(b2)[:80]}")

    # 2.2.4 长文本输入
    long_text = ("以下是会议纪要片段。" + "系统需要支持导出与权限管理，性能目标为千级并发。 " * 80)
    st, b, dt, err = chat([{"role": "user", "content": long_text + "\n请用10字以内总结主题。"}])
    ok = st == 200 and bool(content_of(b).strip())
    record("chat_long_input", ok, f"输入约{len(long_text)}字, HTTP {st} {dt:.1f}s: {content_of(b)[:60]}")

    # 2.2.5 JSON 输出（response_format json_object）
    st, b, dt, err = chat(
        [{"role": "user", "content": '返回JSON对象：{"ok": true, "n": 3}，不要其他文字。'}],
        response_format={"type": "json_object"},
    )
    txt = content_of(b)
    try:
        parsed = json.loads(txt.strip().removeprefix("```json").removeprefix("```").removesuffix("```").strip())
        ok = st == 200 and parsed.get("ok") is True
    except Exception:
        parsed = None
        ok = False
    record("json_object", ok, f"HTTP {st}: {txt[:120]}", {"parsed": parsed})

    # 2.2.6 JSON Schema Structured Output
    schema = {
        "type": "object",
        "properties": {"name": {"type": "string"}, "version": {"type": "string"}},
        "required": ["name", "version"],
        "additionalProperties": False,
    }
    st, b, dt, err = chat(
        [{"role": "user", "content": "Go 语言是什么？给 name 与 version 字段。"}],
        response_format={"type": "json_schema", "json_schema": {"name": "info", "schema": schema}},
    )
    txt = content_of(b)
    try:
        parsed = json.loads(txt)
        ok = st == 200 and isinstance(parsed.get("name"), str)
    except Exception:
        parsed = None
        ok = False
    record("json_schema", ok, f"HTTP {st}: {txt[:160]}", {"note": "不支持时看错误信息"})

    # 2.2.7 Streaming
    payload = {"model": MODEL, "messages": [{"role": "user", "content": "从1数到5，用逗号分隔。"}], "stream": True}
    chunks = 0
    assembled = []
    usage_seen = None
    try:
        with post("/chat/completions", payload, timeout=60) as resp:
            ok_http = resp.status == 200
            for raw in resp:
                line = raw.decode("utf-8").strip()
                if not line.startswith("data:"):
                    continue
                data = line[5:].strip()
                if data == "[DONE]":
                    break
                try:
                    ev = json.loads(data)
                except Exception:
                    continue
                chunks += 1
                if ev.get("usage"):
                    usage_seen = ev["usage"]
                try:
                    delta = ev["choices"][0]["delta"].get("content")
                    if delta:
                        assembled.append(delta)
                except Exception:
                    pass
        text = "".join(assembled)
        record("streaming", ok_http and chunks > 1 and bool(text.strip()),
               f"HTTP {ok_http}, {chunks} chunks: {text[:80]}", {"stream_usage": usage_seen})
    except Exception as e:
        record("streaming", False, "异常: " + str(e))

    # 2.2.8 Usage 统计（非流式）
    st, b, dt, err = chat([{"role": "user", "content": "回复：好"}])
    u = (b or {}).get("usage")
    ok = st == 200 and isinstance(u, dict) and u.get("total_tokens", 0) > 0
    record("usage_stats", ok, f"HTTP {st}: usage={json.dumps(u, ensure_ascii=False) if u else None}")

    # 2.2.9 错误参数处理（不存在的模型）
    st, b, dt, err = chat([{"role": "user", "content": "hi"}], model="nonexistent-model-xyz")
    ok = st is not None and st >= 400
    record("err_bad_model", ok, f"HTTP {st}: {json.dumps(b, ensure_ascii=False)[:160] if b else err}")

    # 2.2.10 错误参数处理（非法消息体）
    st, b, dt, err = chat([])
    ok = st is not None and st >= 400
    record("err_empty_messages", ok, f"HTTP {st}: {json.dumps(b, ensure_ascii=False)[:160] if b else err}")

    # 2.2.11 超时与取消（客户端 1ms 超时 → 立即取消）
    t0 = time.time()
    st, b, dt, err = chat([{"role": "user", "content": "写一篇500字文章。"}], _timeout=0.05)
    ok = err is not None and dt < 5
    record("timeout_cancel", ok, f"客户端超时生效: err={str(err)[:60]}, 耗时{dt:.2f}s")

    # 2.3 Tool Calling
    tools = [{
        "type": "function",
        "function": {
            "name": "get_weather",
            "description": "查询城市天气",
            "parameters": {
                "type": "object",
                "properties": {"city": {"type": "string"}, "unit": {"type": "string", "enum": ["c", "f"]}},
                "required": ["city"],
            },
        },
    }, {
        "type": "function",
        "function": {
            "name": "get_time",
            "description": "查询时区时间",
            "parameters": {"type": "object", "properties": {"tz": {"type": "string"}}, "required": ["tz"]},
        },
    }]

    # 2.3.1 单工具调用
    st, b, dt, err = chat([{"role": "user", "content": "北京现在天气怎么样？用工具查。"}], tools=tools)
    m = msg_of(b)
    tcs = m.get("tool_calls") or []
    ok = st == 200 and len(tcs) >= 1 and tcs[0].get("function", {}).get("name") == "get_weather"
    try:
        args1 = json.loads(tcs[0]["function"]["arguments"]) if tcs else {}
    except Exception:
        args1 = {}
    ok = ok and "北京" in json.dumps(args1, ensure_ascii=False)
    record("tool_single", ok, f"HTTP {st}: {len(tcs)}个调用, args={json.dumps(args1, ensure_ascii=False)[:80]}", {"finish": (b or {}).get("choices", [{}])[0].get("finish_reason") if b else None})

    # 2.3.2 多工具/并行调用
    st, b, dt, err = chat([{"role": "user", "content": "查北京天气和上海时间，两个都用工具。"}], tools=tools)
    m = msg_of(b)
    tcs = m.get("tool_calls") or []
    names = [t.get("function", {}).get("name") for t in tcs]
    ok = st == 200 and len(tcs) >= 2 and "get_weather" in names and "get_time" in names
    record("tool_parallel", ok, f"HTTP {st}: {len(tcs)}个调用 {names}", {"finish": (b or {}).get("choices", [{}])[0].get("finish_reason") if b else None})

    # 2.3.3 工具调用后第二轮回答
    if tcs:
        follow = [{"role": "user", "content": "查北京天气和上海时间，两个都用工具。"},
                  {"role": "assistant", "content": m.get("content") or "", "tool_calls": tcs}]
        for t in tcs:
            follow.append({"role": "tool", "tool_call_id": t.get("id"),
                           "content": json.dumps({"temp": "22C", "cond": "晴"} if t["function"]["name"] == "get_weather" else {"time": "10:30"}, ensure_ascii=False)})
        st2, b2, _, _ = chat(follow, tools=tools)
        txt2 = content_of(b2)
        ok = st2 == 200 and bool(txt2.strip()) and not (msg_of(b2).get("tool_calls"))
        record("tool_followup", ok, f"HTTP {st2}: {txt2[:100]}")
    else:
        record("tool_followup", False, "前置单工具调用失败，无法测试第二轮")

    # 2.3.4 无效工具参数（模型应给合法参数或报错，不应崩溃）
    st, b, dt, err = chat([{"role": "user", "content": "用工具查天气，城市填空字符串。"}], tools=tools)
    ok = st == 200
    record("tool_invalid_params", ok, f"HTTP {st}: {json.dumps(msg_of(b), ensure_ascii=False)[:160]}")

    # 2.3.5 不需要工具时模型行为（问数学，不该乱调工具）
    st, b, dt, err = chat([{"role": "user", "content": "3+4等于几？直接回答。"}], tools=tools)
    m = msg_of(b)
    ok = st == 200 and "7" in content_of(b) and not m.get("tool_calls")
    record("tool_refusal", ok, f"HTTP {st}: content={content_of(b)[:60]}, tool_calls={bool(m.get('tool_calls'))}")

    # 2.4 reasoning 参数探测（可选支持，不强发）
    st, b, dt, err = chat([{"role": "user", "content": "9.11和9.8哪个大？"}], reasoning_effort="low")
    if st == 200:
        m = msg_of(b)
        extra_keys = [k for k in m.keys() if k not in ("role", "content", "tool_calls", "reasoning_content")]
        record("reasoning_param", True, f"接受 reasoning_effort, message 额外字段: {extra_keys}, content={content_of(b)[:80]}")
    else:
        record("reasoning_param", True, f"不支持 reasoning_effort（HTTP {st}），不强发 — ResearchOS 不使用该参数")

    # 汇总
    npass = sum(1 for r in results if r["pass"])
    summary = {"model": MODEL, "total": len(results), "pass": npass, "fail": len(results) - npass}
    out = {"summary": summary, "results": results, "probed_at": time.strftime("%F %T")}
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
    print(f"\n汇总: {npass}/{len(results)} 通过 → {args.out}")


if __name__ == "__main__":
    main()
