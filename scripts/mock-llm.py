#!/usr/bin/env python3
"""本地 OpenAI 兼容测试替身（test double），仅用于验证 ResearchOS 的 LLM 链路。

**它不是模型，不做任何推理。** 它按 system prompt 的类型返回预先构造的 JSON，
用来验证：
  1. 客户端请求形状（/chat/completions、response_format、Authorization、计量字段）是否正确；
  2. planner / analyze 的 JSON 解析与容错是否成立；
  3. 引擎对「模型输出不可信」的防线是否真的生效——本替身**故意**在每次抽取里
     混入一条编造引文和一条不存在的 source_id，正常情况下应被引擎丢弃，
     若报告里出现它们，说明校验被绕过了。

因此：用它跑出来的报告**不能**当作「真实大模型质量」的证据，只能证明管线通。
真实质量验收需要真实 API Key。

用法：
  python scripts/mock-llm.py --port 11434
  RESEARCHOS_LLM_PROVIDER=openai \
  RESEARCHOS_LLM_BASE_URL=http://127.0.0.1:11434/v1 \
  RESEARCHOS_LLM_API_KEY=mock-key \
  RESEARCHOS_LLM_MODEL=mock-1 \
  ./bin/researchos.exe all
"""

import argparse
import json
import re
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ANGLE_EN = ["open source projects", "framework comparison", "benchmark evaluation", "adoption risks"]
ANGLE_ZH = ["现状与关键指标", "方案对比与取舍", "生态与落地风险", "基准评测"]

FABRICATED_QUOTE = "这条引文是测试替身编造的，原文中并不存在。"
BOGUS_SOURCE_ID = "source-does-not-exist"


def ascii_tokens(text):
    return [t for t in re.findall(r"[A-Za-z][A-Za-z0-9.+#_-]*", text) if len(t) > 1]


def build_plan(user_text):
    goal = ""
    m = re.search(r"研究目标[：:]\s*(.+)", user_text)
    if m:
        goal = m.group(1).strip()
    depth = "standard"
    md = re.search(r"研究深度[：:]\s*(\w+)", user_text)
    if md:
        depth = md.group(1).strip()
    verify = "true" in user_text.lower() and "是否开启交叉验证：true" in user_text.replace(" ", "")
    n = {"quick": 2, "standard": 3, "deep": 4}.get(depth, 3)

    tokens = ascii_tokens(goal)
    questions, subtasks = [], []
    for i in range(n):
        qid = "q%d" % (i + 1)
        if tokens:
            q = "%s：%s 的%s" % (goal, " ".join(tokens[:3]), ANGLE_EN[i % len(ANGLE_EN)])
            query = "%s %s" % (" ".join(tokens[:3]), ANGLE_EN[i % len(ANGLE_EN)])
        else:
            q = "%s（%s）" % (goal, ANGLE_ZH[i % len(ANGLE_ZH)])
            query = "%s %s" % (goal[:40], ANGLE_ZH[i % len(ANGLE_ZH)])
        questions.append({"id": qid, "question": q, "priority": "high" if i == 0 else "medium"})
        sid, aid = "search_%d" % (i + 1), "analyze_%d" % (i + 1)
        subtasks.append({
            "id": sid, "title": "检索：" + q, "agent_role": "researcher",
            "node_type": "search", "query": query,
            "inputs": [qid], "outputs": ["sources"], "depends_on": [],
        })
        subtasks.append({
            "id": aid, "title": "分析：" + q, "agent_role": "analyst",
            "node_type": "analyze", "inputs": [sid], "outputs": ["claims"],
            "depends_on": [sid],
        })
    last = ["analyze_%d" % (i + 1) for i in range(n)]
    if verify:
        subtasks.append({
            "id": "verify", "title": "交叉验证：来源冲突识别与置信度计算",
            "agent_role": "verifier", "node_type": "verify",
            "inputs": last, "outputs": ["verified_claims"], "depends_on": last,
        })
        last = ["verify"]
    subtasks.append({
        "id": "synthesize", "title": "综述：生成带引用的研究报告",
        "agent_role": "synthesizer", "node_type": "synthesize",
        "inputs": last, "outputs": ["report"], "depends_on": last,
    })
    return {
        "goal": goal,
        "research_questions": questions,
        "subtasks": subtasks,
        "risks_and_gaps": ["本计划由本地测试替身生成，不代表真实模型规划能力。"],
    }


SOURCE_RE = re.compile(r"\[source_id=([^\]]+)\]\s*\n标题：(.*?)\nURL：(.*?)\n正文：(.*?)(?=\n\[source_id=|\Z)", re.S)


def pick_sentence(text):
    parts = re.split(r"(?<=[。！？!?])|\n+", text)
    best = ""
    for p in parts:
        p = p.strip()
        if len(p) < 30:
            continue
        # 优先带数字的句子：更像可核验的事实
        if re.search(r"\d", p) and len(p) <= 160:
            return p
        if not best or (len(p) < len(best) and len(p) >= 40):
            best = p
    return best[:160]


def build_claims(user_text):
    claims = []
    for sid, title, url, body in SOURCE_RE.findall(user_text):
        sent = pick_sentence(body)
        if not sent:
            continue
        num = re.search(r"(\d+(?:\.\d+)?)", sent)
        claims.append({
            "statement": sent if len(sent) <= 120 else sent[:118] + "。",
            "quote": sent,
            "source_id": sid.strip(),
            "confidence": 0.72,
            "metric": "count" if num else "",
            "value": float(num.group(1)) if num else None,
        })
        if len(claims) >= 6:
            break
    # 故意注入的脏数据：引擎必须丢掉，否则说明校验失效
    claims.append({
        "statement": "这是一条测试替身注入的编造结论，用于验证引文校验。",
        "quote": FABRICATED_QUOTE,
        "source_id": (claims[0]["source_id"] if claims else "x"),
        "confidence": 0.99, "metric": "", "value": None,
    })
    claims.append({
        "statement": "这条结论引用了不存在的 source_id，应当被丢弃。",
        "quote": (claims[0]["quote"] if claims else "无"),
        "source_id": BOGUS_SOURCE_ID,
        "confidence": 0.99, "metric": "", "value": None,
    })
    return {"claims": claims}


class Handler(BaseHTTPRequestHandler):
    server_version = "MockLLM/1.0"

    def log_message(self, fmt, *args):
        sys.stderr.write("[mock-llm] " + (fmt % args) + "\n")

    def _send(self, code, obj):
        raw = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        if self.path.rstrip("/").endswith("/v1/models"):
            self._send(200, {"object": "list", "data": [{"id": "mock-1", "object": "model"}]})
        else:
            self._send(404, {"error": {"message": "not found"}})

    def do_POST(self):
        if not self.path.rstrip("/").endswith("/chat/completions"):
            self._send(404, {"error": {"message": "not found"}})
            return
        length = int(self.headers.get("Content-Length") or 0)
        try:
            body = json.loads(self.rfile.read(length).decode("utf-8"))
        except Exception as exc:  # noqa: BLE001
            self._send(400, {"error": {"message": "bad json: %s" % exc}})
            return

        auth = self.headers.get("Authorization") or ""
        if not auth.startswith("Bearer ") or len(auth) <= len("Bearer "):
            self._send(401, {"error": {"message": "missing bearer token"}})
            return

        msgs = body.get("messages") or []
        system = "".join(m.get("content", "") for m in msgs if m.get("role") == "system")
        user = "".join(m.get("content", "") for m in msgs if m.get("role") == "user")

        if "研究规划器" in system:
            content = json.dumps(build_plan(user), ensure_ascii=False)
        elif "研究分析师" in system:
            content = json.dumps(build_claims(user), ensure_ascii=False)
            if body.get("response_format"):
                # 真实模型常把 JSON 包在 ```json 里，这里也带上，顺带验证容错
                content = "```json\n" + content + "\n```"
        else:
            content = "（测试替身回复）这是一段纯文本补全，用于验证非 JSON 通道。"

        prompt_tokens = max(1, (len(system) + len(user)) // 4)
        completion_tokens = max(1, len(content) // 4)
        self._send(200, {
            "id": "chatcmpl-mock",
            "object": "chat.completion",
            "model": body.get("model", "mock-1"),
            "choices": [{"index": 0, "message": {"role": "assistant", "content": content},
                         "finish_reason": "stop"}],
            "usage": {"prompt_tokens": prompt_tokens, "completion_tokens": completion_tokens,
                      "total_tokens": prompt_tokens + completion_tokens},
        })


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=11434)
    ap.add_argument("--host", default="127.0.0.1")
    args = ap.parse_args()
    srv = ThreadingHTTPServer((args.host, args.port), Handler)
    print("mock LLM listening on http://%s:%d/v1  (test double, NOT a model)" % (args.host, args.port),
          flush=True)
    srv.serve_forever()


if __name__ == "__main__":
    main()
