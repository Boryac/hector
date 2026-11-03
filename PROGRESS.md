# PROGRESS.md — 进度账本（Codex 每次迭代必须更新）

## 当前状态
- 当前 Phase：Phase 0（审计完成，**暂停待人工确认**）
- 已完成任务：T0.1 / T0.2 / T0.3 / T0.4 / T0.5 / T0.6
- 下一个任务：T0.7（migration v001，建议并入 Phase 1 首任务）
- 阻塞与待人工确认：安全审查结论（身份隔离不成立）、前端技术栈分歧、Phase 0 是否放行进入 Phase 1

## 迭代记录（每轮追加一条，不覆盖历史）

### 2026-11-03 — [phase-0][T0.1] Fork 与仓库接入
- 做了什么：`git clone https://github.com/verikod/hector.git` 到 `upstream/hector/`（保留完整上游历史）；自 `main` 建 `feature/hector-researchos` 分支；记录基线 commit `abe472b6deb12b3dbba290dbec44cd3be27e163d`（2026-05-06，`fixed agent tool calling/streaming`）。
- 测试结果：`git clone` 成功；`git branch --show-current` → `feature/hector-researchos`；受跟踪文件 488 个。
- 遗留问题 / 假设：远程推送地址与凭据后补，不阻塞。
- 下一步：T0.2 模块映射。

### 2026-11-03 — [phase-0][T0.2] 原生结构与模块映射
- 做了什么：通读 `cmd/`、`pkg/`（33 个顶级包、277 文件）、`studio/`（122 文件 Vite+React）、`ui/embed.go`、`deploy*/`；产出 `docs/hector-audit.md`（目录树、模块能力映射、队列/Checkpoint 现状、环境基线）与 `docs/mapping-table.md`（保留 17 项 / 修改 10 项 / 新增 14 项 / 替换 5 项 / 技术债 9 项 + 开发包文件并入方案）。
- 测试结果：`go build ./...` 通过（exit 0）。
- 遗留问题 / 假设：上游前端技术栈（Vite+React）与 DECISIONS 第 5 条（Next.js + React Flow）冲突，建议 Studio 保留、产品前端另建，待确认。
- 下一步：T0.3 身份隔离审查。

### 2026-11-03 — [phase-0][T0.3] A2A 身份隔离审查（Issue #2）
- 做了什么：定位身份定稿点 `pkg/server/events.go:31-68`，确认 `metadata.user_id` 无条件覆盖已验证 JWT claims；核查共享密钥（固定 Subject `admin-via-secret`）、admin key 明文比较、task/session 读接口无 user 过滤、remoteagent metadata 透传；新增可运行证据测试 `pkg/server/identity_isolation_test.go`（4 条），结论写入 `docs/security-review.md`。
- 测试结果：`go test ./pkg/server/ -run TestToInvocationMeta -v` → 4/4 PASS，日志确认 `authenticated user "alice" is replaced by forged metadata user_id "bob"`、`unauthenticated request adopts arbitrary user_id "victim"`。
- 遗留问题 / 假设：结论为「隔离不成立」，修复方案 7 项已列入 security-review 第 1.6 节；修复前多用户功能禁用。
- 下一步：T0.4 队列与恢复审查。

### 2026-11-03 — [phase-0][T0.4] 队列 / Checkpoint / 恢复审查
- 做了什么：核查 `pkg/execution/native`（ack/nack、指数退避、死信、RecoverStale、WorkerPool 指标、ShutdownTimeout 30s、StaleRecoveryInterval 1m）与 `pkg/checkpoint`（RecoverPendingTasks、WORKING 自动续跑、过期置 FAILED）；列出 6 项相对 PRD 缺口写入 `docs/hector-audit.md` 第 4 节。
- 测试结果：`go test ./pkg/server/... ./pkg/auth/... ./pkg/session/... ./pkg/task/... ./pkg/checkpoint/...` → 5 包全 ok。
- 遗留问题 / 假设：缺节点级 Checkpoint、任务级并发上限、暂停闸门、全局 event_id 幂等键；均列入 T2.x 范围。
- 下一步：T0.5 技术选型锁定。

### 2026-11-03 — [phase-0][T0.5] 技术选型锁定
- 做了什么：按人工决策锁定 **sqlc + PostgreSQL**；`DECISIONS.md` 新增第 11（sqlc）、12（上游为基线）、13（身份未修复不出多用户）条。
- 测试结果：文档一致性检查（DECISIONS 与 mapping-table 第 5/6 节口径一致）。
- 遗留问题 / 假设：历史手写 SQL 双方言在迁移时收敛，MySQL 分支清理排期。
- 下一步：T0.6 测试基线。

### 2026-11-03 — [phase-0][T0.6] 测试与开发环境基线
- 做了什么：跑通上游构建与核心包测试；跑 `bash scripts/verify.sh`（开发包骨架）确认 openapi 解析正常、go/web 检查按预期跳过。
- 测试结果：`go build ./...` exit 0；核心 5 包 ok；`verify.sh` 输出 `ok: openapi yaml parses`。
- 遗留问题 / 假设：Docker 未安装（按决策推迟 Phase 5）；本地无 PostgreSQL，Phase 1 起用 Testcontainers 或远程实例。
- 下一步：T0.7（migration v001）建议并入 Phase 1 首任务，**Phase 0 到此暂停**。

## 风险与决策变更
- 2026-11-03：建议新增「上游前端技术栈与 DECISIONS 第 5 条冲突」的处理方案（Studio 保留 / 产品前端另建 Next.js），待人工确认后补入 DECISIONS。
- 2026-11-03：T0.7 未完成（migration v001 未建），不属于跳过验收，属范围调整提议，待人工确认。
