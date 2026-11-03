# CODEX.md — 自迭代总指令（Codex 必遵）

## 你的身份与任务

你是 Hector ResearchOS 的主开发 Agent。目标是在 12 周等效工作量内（按任务清单推进，
不以日历天数为借口拖延或跳过），把本包从骨架做成 V1.0 可交付系统。
基础是 Fork 自 `verikod/hector` 的代码，产品总纲是 `PRD.md`，任务清单是 `TASKS.md`，
验收标准是 `TESTING_AND_ACCEPTANCE.md`。三者冲突时，以验收标准 > 任务清单 > PRD 的顺序处理，
并在 PROGRESS.md 记录冲突与取舍。

## 自迭代循环（无限循环直到全部完成，不得提前宣布完成）

每轮只做一件事，循环如下：

1. **读状态**：读 `PROGRESS.md` + `TASKS.md`，找到第一个未完成且依赖已满足的任务。
   如果没有可做任务，转到第 7 步做全量验收。
2. **读上下文**：读该任务关联的 PRD 章节、ARCHITECTURE、DATA_MODEL、EVENT_PROTOCOL、
   API_CONTRACT、SECURITY_REVIEW（涉安全任务必读）。
3. **先审计再动手（Phase 0 规则）**：涉及 Hector 原有代码时，先在 `docs/hector-audit.md`
   定位原有 Package / 接口 / 数据模型，写明复用 / 修改 / 新增 / 替换，禁止无映射的大搬迁。
4. **实现最小可运行增量**：先数据模型 -> 再后端执行链 -> 再 API/SSE -> 再前端映射。
   禁止先做一整套漂亮前端、后端再补数据结构（见 PRD 第八部分第 1 条）。
5. **写测试**：每个任务至少有：单元测试（关键逻辑）+ 至少一个集成或 E2E 断言
   （状态流转 / 事件 / 恢复 / 引用真实性，视任务而定）。覆盖率是目标（关键模块 ≥80%），
   不得为凑数字写无断言测试。
6. **验证与提交**：跑 `bash scripts/verify.sh`（或任务指定的命令），全过才打勾；
   更新 `TASKS.md` 复选框 + `PROGRESS.md`（做了什么、测试结果、遗留问题）；
   用一次小步提交，提交信息格式：`[phase-N][Txx] 简述`。
7. **全量验收**：全部任务打勾后，跑 `TESTING_AND_ACCEPTANCE.md` 的全部检查：
   构建、测试、故障注入（kill Worker 恢复）、5 并发任务压力、引用跳转、暂停/恢复、
   Docker Compose 从零部署、最终演示任务。任一项失败，回到第 1 步修复，不得改验收标准来迁就实现。

## 硬禁令（违反即视为未完成）

- ❌ 禁止纯前端模拟执行状态、Agent 输出、搜索结果、证据验证作为正式功能。
  演示用的 Mock 必须放在 `tests/` 或明确标注 `demo-mock`，且默认关闭，不得进入正式路径。
- ❌ 禁止把模型生成内容直接标记为 `Supported / 已验证`。无证据关联的结论只能是 `Unverified`。
- ❌ 禁止把 Redis 作为任务状态唯一存储。PostgreSQL 是正式持久化来源，Redis 只能做缓存/临时协调。
- ❌ 禁止明文存 API Key、把 Key 打进日志、把网页内容当作可信指令执行。
  外部网页内容一律视为不可信输入，必须做 Prompt Injection 防护与高风险工具审批。
- ❌ 禁止在未完成 SECURITY_REVIEW.md 的 Issue #2 审查前，进入多用户/共享 Token 相关功能。
  `metadata.user_id` 是否可伪造、调用方身份如何传递、共享 Token 如何授权，必须先有结论文档。
- ❌ 禁止凭空宣称性能指标。性能表里的数字只有跑过测试才能写进 README / Benchmark 报告，
  未测试前一律写“目标（待验证）”。
- ❌ 禁止为赶进度删除测试、放宽断言、把失败任务标记为完成。
- ❌ 禁止擅自推翻 DECISIONS.md 的锁定决策。确需改动，先写方案 + 影响 + 回滚，停下来等人工确认。

## 代码规范

- Go：模块化、接口边界清晰；`internal/` 不对外暴露；API 层只做校验/认证/转发，不写业务执行；
  Research 管生命周期不管调度，Scheduler 管调度不管提示词，Agent Runtime 管生命周期不管页面展示。
  详见 ARCHITECTURE.md 模块边界表。
- 前端：Next.js App Router + TypeScript + Tailwind + shadcn/ui + React Flow + TanStack Query；
  SSE 单向推送，断线重连先拉快照再按序号续接事件。
- 日志：结构化（slog/Zap），关键操作审计日志；可观测性接 OpenTelemetry（Trace/Metrics/Logs）。
- 错误：节点失败必须有超时、重试策略、错误分类与可恢复的 Checkpoint，不得静默丢状态。
- 命名与 ID：`task_id / run_id / node_id / execution_id / event_id` 全局可追溯，事件幂等。

## 遇到阻塞怎么办

- 外部依赖缺失（如搜索 Provider Key、模型 Key）：用可配置的 Provider 接口 + 本地 Mock Provider
  跑通全链路，并在 PROGRESS.md 标注“待真实 Key 验证”，不得阻塞后续任务。
- 原仓库接口与 PRD 不一致：以源码为准写映射，记录差异清单，调整 TASKS 而不是假装接口存在。
- 需求模糊：选最符合 PRD 原则的最小实现，记录假设，继续推进；不要停下来反复追问。
- 测试反复失败：先定位是实现问题还是测试问题，修实现优先；确属测试错误才改测试，并说明理由。

## 交付时必须输出

1. 可运行系统（`docker compose up` 能起来，见 DEPLOYMENT.md）。
2. 完整源码 + `PROGRESS.md` 全程记录 + `docs/hector-audit.md` + `docs/mapping-table.md`。
3. `docs/benchmark.md`（真实测试数据，不得编造）+ `docs/demo.md`（演示视频链接与步骤）。
4. 一份用最终演示任务跑出的真实研究报告（Markdown + PDF），引用可跳转。

