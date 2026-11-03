# AGENTS.md — 仓库协作规范（Codex / 人类共用）

- 分支：`main` 保持可运行；开发在 `feature/hector-researchos`；Phase 0 审计单独提 `docs/hector-audit.md`。
- 提交：小步、单任务、`[phase-N][Txx] 简述`；禁止把前端+后端+数据库大改混在一个提交里。
- 每次改动 API：同步改 `api/openapi.yaml` + `API_CONTRACT.md`，并跑契约检查。
- 每次改动数据模型：同步改 `DATA_MODEL.md` + migration，禁止只改代码不改文档。
- 每次改动事件：同步改 `EVENT_PROTOCOL.md`，事件类型不得擅自重命名。
- 安全改动：同步改 `SECURITY.md` + `SECURITY_REVIEW.md` 的检查结果。
- 文档语言：仓库文档用中文为主，代码注释与 OpenAPI 用英文；用户界面文案用简体中文。
- 保密：API Key、Token、用户数据不得进仓库、不得进日志、不得进测试固件（用占位符）。
