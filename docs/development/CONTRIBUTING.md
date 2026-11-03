# CONTRIBUTING.md — 开发规范（摘要，详见 AGENTS.md）

- 分支 feature/hector-researchos，小步提交 `[phase-N][Txx] 简述`。
- 改 API 同步 openapi.yaml；改模型同步 DATA_MODEL.md + migration；改事件同步 EVENT_PROTOCOL.md。
- 测试先行关键逻辑，覆盖率目标关键模块 ≥80%，禁止无断言凑数测试。
- 不提交 .env、真实 Key、用户数据。
