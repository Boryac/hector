# prompts/PHASE_PROMPTS.md — 可直接粘贴给 Codex 的分阶段提示词

## 启动提示词（第 1 条，发给 Codex 时连同整个包一起发）
> 你是 Hector ResearchOS 的主开发 Agent。先读 CODEX.md、PRD.md、ARCHITECTURE.md、
> TASKS.md、TESTING_AND_ACCEPTANCE.md。按 CODEX.md 的自迭代循环推进，
> 从 T0.1 开始逐项完成 TASKS.md，直到 TESTING_AND_ACCEPTANCE.md 全过。
> 禁止纯前端模拟、禁止无证据标记已验证、禁止 Redis 做唯一状态源、禁止明文存 Key。
> 每完成一项更新 TASKS.md 打勾与 PROGRESS.md，小步提交。Phase 0 安全审查结论必须停下来等我确认后再进 Phase 1。

## Phase 0 提示词
> 只做审计与映射，不写大功能。Fork verikod/hector，输出 docs/hector-audit.md、
> docs/mapping-table.md、docs/security-review.md（重点 Issue #2：身份传递、共享 Token、
> metadata.user_id 伪造测试）。锁定 ORM 与 Provider 接口，跑通 scripts/verify.sh 空骨架。

## Phase 1 提示词
> 做 T1.1–T1.5：任务状态机 + REST + 事件/SSE + Dashboard/New Research/详情 + React Flow 画布初版。
> 验收：画布状态与后端快照一致，断线能续接，非法状态跃迁被拒绝。

## Phase 2 提示词
> 做 T2.1–T2.6：Planner 结构化校验、DAG 编译（环检测）、Scheduler/Worker Pool、6 角色 Agent Runtime、
> 节点重试与 Checkpoint、暂停/恢复/取消。验收：并行确实并行，依赖确实等待，单节点失败可恢复。

## Phase 3 提示词
> 做 T3.1–T3.6：搜索 Provider 接口 + Mock + 真实 Provider、抓取解析、URL 规范化去重、
> 来源中心、Evidence 关联、Verifier 冲突识别。验收：无来源不得标 Supported，冲突能被标出。

## Phase 4 提示词
> 做 T4.1–T4.6：Synthesizer、9 节 Reporter、Report Viewer 引用跳转、Markdown/PDF/HTML 导出、
> Tool Hub（高风险审批）、节点详情与重试 UI。验收：引用可跳转，PDF 与 Markdown 一致，无路径穿越。

## Phase 5 提示词
> 做 T5.1–T5.7：补测试到关键模块 ≥80% 目标、集成+E2E、kill Worker 故障注入、5 并发压力、
> 隔离测试与漏洞扫描、Docker Compose 从零部署、最终演示任务与 2–3 分钟视频。
> 全部通过后输出 docs/benchmark.md（实测数据）与 docs/demo.md，方可宣布 V1.0 完成。

## 自查提示词（每轮结束 Codex 自己问自己）
> 我这轮改动有没有用前端模拟冒充后端状态？有没有无证据标已验证？事件是否幂等？
> API 改动是否同步了 openapi.yaml？数据模型改动是否同步了 migration 与 DATA_MODEL.md？
> 测试是真实断言还是凑数？PROGRESS.md 是否已更新？
