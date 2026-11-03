# RC1_MODEL_CAPABILITY_REPORT — 本地模型能力探测报告（2026-11-04）

> 结论先行：**Solar Mini 4 经本地中继成功接入**。实际 Model ID 为
> `cline/cline-free/solar-mini4`（`/v1/models` 实际返回，无独立短 ID `solar-mini4`）。
> Chat / 多轮 / 长输入 / `json_object` / Streaming / Usage / Tool Calling（单工具、
> 并行多工具、第二轮回答、参数校验、拒绝误调）全部实测可用。
> **JSON Schema Structured Output 不可用**（中继接受但模型忽略 schema），ResearchOS
> 使用 `json_object` 模式 + 容忍式 JSON 提取，兼容无影响。
> 中继存在**偶发 502 瞬态错误**（18 项探测中出现 1 次，重试 4/4 成功）。

探测方式：`scripts/rc1_probe_model.py`（18 项自动探测 + 人工重试定性），
原始结果（脱敏）：`scripts/rc1_probe_results.json`。API Key 未记录于任何文件。

## 1. 服务与模型身份

| 项 | 实测值 |
|---|---|
| Base URL | `http://127.0.0.1:18326/v1`（宿主机回环，进程 DeepSeek Harness） |
| 鉴权 | Bearer relay key；无 Key/错 Key 均 401 `missing or invalid relay key` |
| `/v1/models` | 返回 107 个模型 |
| 目标模型 ID | **`cline/cline-free/solar-mini4`**（即 Cline 目录 solar-mini4 条目，对应 Upstage Solar Mini 4） |
| 别名 | 无 `solar-mini4` / `solar-mini` 独立短 ID；未探测到同模型其他别名 |
| 推理标记 | usage 含 `reasoning_tokens`（如 156 total 中 90 为 reasoning），message 可含 `reasoning_content` 字段 |

## 2. 探测结果明细（18 项）

| # | 测试 | 结果 | 关键证据（摘要） |
|---|---|---|---|
| 1 | 中文问答 | PASS | 200，5.0s，幂等性定义回答正确 |
| 2 | 英文问答 | PASS | 200，9.4s，CAP 定理回答正确 |
| 3 | 多轮上下文 | PASS | 第二轮正确答出姓氏「黄」 |
| 4 | 长文本输入 | PASS | ~2010 字输入，200，7.0s，摘要正确 |
| 5 | `json_object` 模式 | PASS | 返回 `{"ok":true,"n":3}` 纯 JSON |
| 6 | `json_schema` Structured Output | **FAIL** | HTTP 200 但模型忽略 schema 返回散文/代码块（重试同样）；**判定不支持** |
| 7 | Streaming | PASS | SSE 72 chunks 正确拼装「1,2,3,4,5」，`[DONE]` 正常 |
| 8 | Usage 统计 | PASS | prompt 55 / completion 101 / total 156，含 reasoning_tokens 细分 |
| 9 | 错误参数（坏模型名） | PASS | 400 `model must use provider/model format` |
| 10 | 错误参数（空 messages） | PASS | 400 `messages must be a non-empty array` |
| 11 | 超时与取消 | PASS | 客户端 50ms 超时立即中断（0.05s） |
| 12 | 单工具调用 | PASS（重试后） | 首次 502 瞬态；重试 4/4 成功，`finish_reason=tool_calls`，`city="北京"` 参数正确 |
| 13 | 并行多工具调用 | PASS | 一次返回 2 个调用 `get_weather`+`get_time`，参数正确 |
| 14 | 工具结果第二轮 | PASS | 喂入 tool 结果后生成自然语言汇总，不再发起调用 |
| 15 | 无效工具参数 | PASS | 空城市时模型反问澄清而非编造调用 |
| 16 | 拒绝误调工具 | PASS | 问 3+4 直接答「7」，不调工具 |
| 17 | `reasoning_effort` 参数 | 不强发 | HTTP 502（中继不透传该参数）；ResearchOS 不使用该参数，无影响 |
| 18 | 模型列表 | PASS | 107 模型，含目标 ID |

## 3. 兼容性结论（对 ResearchOS 的影响）

| ResearchOS 依赖的能力 | 本模型支持 | 说明 |
|---|---|---|
| `Complete`（纯文本补全） | ✅ | 直接可用 |
| `CompleteJSON`（`response_format=json_object` + `extractJSONObject` 容忍解析） | ✅ | `json_object` 可靠；即使模型偶发包裹 ```json 或前后废话，现有提取器也能处理 |
| Tool Calling | ✅（可用但中继偶发 502） | ResearchOS planner/agent 走 JSON 协议而非原生 tool_calls，影响有限；若启用原生工具链需依赖现有重试机制 |
| Streaming | ✅ | ResearchOS 当前非流式消费，无影响 |
| `reasoning_content` 泄漏风险 | ✅ 无 | 客户端只解析 `choices[0].message.content`，reasoning 内容不会进入报告 |
| reasoning tokens 占用补全预算 | ⚠️ 注意 | 该模型会产生 reasoning tokens（实测占 completion 一半左右）；planner 类短任务需保证 `max_tokens` 足够，建议 ≥ 4096 |

## 4. 已知限制

1. **`json_schema` 不支持**——中继接受请求但不强制 schema；需结构化输出时一律用 `json_object` + 明确 Prompt 约定。
2. **偶发 502**——中继上游免费通道瞬态错误（探测中出现 1 次）；调用方需具备重试，且重试有效（4/4 成功）。
3. **响应延迟偏高**——简单问答 1.5~9.4s（含 reasoning），规划类长任务需宽松超时（默认 90s 可满足）。
4. **免费通道配额/稳定性不受 SLA 保护**——研究任务验收结果需记录每次调用的耗时与重试。
5. 模型真实身份为目录映射（`cline/cline-free/solar-mini4` → Upstage Solar Mini 4）；本报告以「实际返回的模型 ID 原样记录」为准，未对上游供应商做二次验证。

## 5. 复现方式

```bash
# key 从本地凭据运行时读入，不落盘
LLM_KEY=<relay key> python scripts/rc1_probe_model.py --model cline/cline-free/solar-mini4
```
