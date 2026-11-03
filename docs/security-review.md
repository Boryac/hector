# docs/security-review.md — 安全审查结论（Phase 0 产出）

**审查日期：** 2026-11-03
**审查对象：** `verikod/hector` commit `abe472b6deb12b3dbba290dbec44cd3be27e163d`
**审查人：** 开发 Agent（待人工复核确认）
**总体结论：** 身份隔离**不成立**。`metadata.user_id` 可伪造且直接决定会话归属，任务 / 会话读接口缺 user 维度。**多用户功能在修复并回归通过前一律禁用，V1 默认单用户本地模式。**
**证据代码：** `upstream/hector/pkg/server/identity_isolation_test.go`（4 条测试，当前全部 PASS，即漏洞可复现）

---

## 1. A2A 身份隔离（Issue #2，最高优先级）

### 1.1 调用方身份如何传递（调用链）

```
客户端 POST /agents/{app}/  (A2A JSON-RPC message/send | message/stream)
  │  ① Authorization: Bearer <JWT>
  ▼
auth.Middleware                     pkg/auth/middleware.go:36
  │  ValidateToken → *Claims{Subject, Email, Role, TenantID}
  │  ContextWithClaims(ctx, claims)    ← 唯一可信身份来源
  ▼
a2a-go JSON-RPC handler
  ▼
auth.Interceptor.Before             pkg/auth/interceptor.go
  │  claims → CallContext.User = AuthenticatedUser
  ▼
server.Executor.Execute             pkg/server/executor.go:122
  ▼
toInvocationMeta(ctx, reqCtx)       pkg/server/events.go:31-68   ← 身份在此定稿
  │  1) claims.Subject            → meta.userID   （可信）
  │  2) Message.Metadata["user_id"] → meta.userID （不可信，且覆盖 1）
  │  3) 空值                       → "default"
  │  meta.sessionID = reqCtx.ContextID（客户端 context_id）
  ▼
Session / Task / Checkpoint 全部按 (app_name, userID, sessionID) 键控
```

### 1.2 共享 Token 如何授权（scope / 有效期 / 吊销）

- **JWT 路径**（`pkg/auth/jwt.go`、`keys.go`、`multi_issuer.go`）：JWKS / OIDC 校验，身份由 IdP 签发，`Subject` 可信。无 Hector 侧吊销列表，依赖 IdP 过期；scope 仅通过 `Role` / `TenantID` 粗粒度表达。
- **共享密钥路径**（`pkg/auth/secret.go`）：单一 secret 校验通过后固定返回 `Subject: "admin-via-secret"`。**所有持有该 secret 的调用方共享同一身份**，无法区分人、无有效期、无吊销、无 scope。
- **Admin 路径**（`pkg/server/admin_handler.go:151-160`）：`Authorization: Bearer == adminKey` 明文比较（非常量时间），无轮换。
- **结论：** 共享 Token 授权链不满足多用户隔离要求；只能用于单用户本地部署。

### 1.3 `metadata.user_id` 是否可被伪造（测试证明）

代码位点 `pkg/server/events.go:55-60`：

```go
// Extract user ID from message metadata (Override if explicitly provided in A2A message)
if reqCtx.Message != nil && reqCtx.Message.Metadata != nil {
    if uid, ok := reqCtx.Message.Metadata["user_id"].(string); ok {
        meta.userID = uid
    }
}
```

实测（`go test ./pkg/server/ -run TestToInvocationMeta -v`）：

| 测试 | 场景 | 结果 |
|---|---|---|
| `TestToInvocationMeta_ForgedUserIDOverridesClaims` | 已认证用户 alice 在 metadata 声明 `user_id=bob` | **PASS：身份被替换为 bob** |
| `TestToInvocationMeta_UnauthenticatedCallerPicksAnyUserID` | 未认证调用方声明 `user_id=victim` | **PASS：身份采纳 victim** |
| `TestToInvocationMeta_ClaimsUsedWhenNoMetadata` | 有 claims、无 metadata | PASS：使用 alice（说明仅在 metadata 存在时被劫持） |
| `TestToInvocationMeta_DefaultUserFallback` | 无 claims、无 metadata | PASS：回落 default |

输出摘要：
```
CONFIRMED: authenticated user "alice" is replaced by forged metadata user_id "bob"
CONFIRMED: unauthenticated request adopts arbitrary user_id "victim"
```

### 1.4 越权影响面（是否可读他人任务 / 资料）

| 数据面 | 代码 | 现状 | 影响 |
|---|---|---|---|
| 会话 / 会话事件 | `pkg/session/store.go` 主键 `(app_name, user_id, id)` | user_id 可伪造、session_id 取自客户端 `context_id` | 伪造后可读写他人会话状态与事件 |
| A2A 任务 | `pkg/task/store.go:209` `WHERE app_name = ? AND id = ?` | **无 user 过滤**，知道 task_id 即可直读 | 跨用户任务读取 |
| 任务列表 | `pkg/task/store.go:324` `WHERE app_name = ? AND context_id = ?` | 无 user 过滤 | 同一 context 下跨用户枚举 |
| Checkpoint | `pkg/checkpoint/storage.go` 按 `(app_name, user_id, session_id)` | 键含 user_id，但来源同上 | 伪造后可覆盖 / 读取他人检查点 |
| Admin 会话查询 | `pkg/server/admin_handler.go:531-583` | `?user_id=` 任意指定，仅 adminKey 保护 | 管理面可审计任意用户数据（设计使然，需审计日志） |
| 远端事件回流 | `pkg/agent/remoteagent/a2a.go:365-380` | 仅剥除 `agent_id` / `author`，`user_id` 可透传 | 远端可污染本地元数据 |

### 1.5 结论

**隔离不成立。** 根因是「服务端签发的身份」被「客户端可写的 metadata」覆盖，且存储层没有把 user 维度作为强制过滤条件。

### 1.6 修复方案（Phase 1 前必须完成，未修复前禁用多用户）

1. **身份只认服务端签发**：`toInvocationMeta` 删除 metadata 覆盖分支；仅在无 claims 且显式开启 `ALLOW_ANONYMOUS=true` 时使用固定 `"anonymous"`，绝不采用客户端 user_id。
2. **会话绑定校验**：`context_id` 映射到 session 时校验其归属 userID，不匹配返回 403；禁止跨用户续接。
3. **存储层强制过滤**：`task.Get` / `task.List` / 会话查询全部补 `user_id` 条件（或服务层先校验归属再读取），无归属一律 404。
4. **元数据白名单**：A2A metadata 只接受白名单键，身份类键（`user_id` / `tenant_id`）出入双向剥除。
5. **共享密钥收敛**：单密钥模式明确定义为「单用户本地模式」，多用户必须走 JWT/OIDC；补 scope、有效期、吊销路径。
6. **管理面加固**：admin key 常量时间比较 + 轮换机制 + 所有 admin 读取写审计日志。
7. **回归测试**：将 `identity_isolation_test.go` 翻转为「claims 胜出」断言，并补三条：伪造 user_id 读他人任务必须 404、跨用户 context 续接必须 403、admin key 无审计日志失败。

**通过标准：** 上述 7 项落地 + 回归测试全绿 + 人工确认后，才允许开启多用户能力。Phase 1 按单用户本地模式推进不受阻。

### 1.7 修复实施状态（2026-11-04，T5.5）

| # | 项目 | 状态 | 证据 |
|---|---|---|---|
| 1 | 身份只认服务端签发 | ✅ 已修复 | `pkg/server/events.go`：删除 metadata 覆盖分支；未认证归入常量 `anonymousUserID = "anonymous"`；metadata 出现 `user_id` 时记审计日志 |
| 2 | 会话绑定校验 | ⏳ 未做 | 需要给 `pkg/session` 增加 owner 字段与 403 分支，涉及上游会话层改造，排在多用户能力之前 |
| 3 | 存储层强制过滤 | ✅ ResearchOS 侧已做 / ⏳ 上游未做 | `internal/researchos/api` 所有读写都以 `s.owner(r)`（服务端身份）过滤；上游 `pkg/task` 仍未补 user 条件 |
| 4 | 元数据白名单 | ✅ 等价实现 | metadata 中的身份类键不再参与身份判定；非身份键仍透传给 Agent 使用 |
| 5 | 共享密钥收敛 | ✅ 已定性 | `DECISIONS.md` 第 13 条：单用户本地模式；多用户能力需 JWT/OIDC 后才开 |
| 6 | 管理面加固 | ✅ 已修复 | `pkg/server/admin_handler.go`：`subtle.ConstantTimeCompare` + 空密钥直接拒绝 |
| 7 | 回归测试翻转 | ✅ 完成 | `pkg/server/identity_isolation_test.go` 6 条用例，断言 claims 胜出、metadata 无效、匿名常量生效 |

验证命令与结果：

```
$ go test ./pkg/server/ -run TestToInvocationMeta -v
--- PASS: TestToInvocationMeta_MetadataMustNotOverrideClaims
--- PASS: TestToInvocationMeta_UnauthenticatedCannotPickUserID
--- PASS: TestToInvocationMeta_ClaimsUsedWhenNoMetadata
--- PASS: TestToInvocationMeta_AnonymousFallback
--- PASS: TestToInvocationMeta_NonStringUserIDIgnored
--- PASS: TestToInvocationMeta_MetadataVariantsIgnored
ok  github.com/verikod/hector/pkg/server
```

**结论修订**：Issue #2 的核心利用链（伪造 `metadata.user_id` 冒充任意用户）已切断。
第 2、3（上游部分）项未完成，因此**多用户模式仍然禁用**，V1 保持单用户本地部署，
与 `DECISIONS.md` 第 13 条一致。以上不构成「安全已完结」结论，只是把最高危的一条封掉。

---

## 2. MCP 工具越权

| 检查项 | 现状 | 结论 |
|---|---|---|
| 工具注册权限校验 | `pkg/tool/mcptoolset` 支持 MCP server 接入；`pkg/guardrails/tool/authorization.go` 提供工具授权策略 | 有基础，需在 Tool Hub 注册时强制校验 |
| 高风险工具默认关闭 / 需审批 | `pkg/tool/approvaltool` 提供 HITL 审批；`pkg/agent/llmagent/flow.go` 有审批状态与会话绑定 | 机制存在，默认策略需在 T4.5 验收「默认需审批」 |
| 工具调用按任务 / 用户隔离 | 工具执行上下文来自 agent invocation，未按 task 维度隔离 | **缺口**，随身份修复一并处理 |
| 工具超时 / 错误隔离 | `pkg/httpclient`、`pkg/tool/webtool` 有超时参数；worker 单任务失败不拖垮池 | 基本满足，需补 tool.failed 事件 |

## 3. 网页内容与 Prompt Injection

| 检查项 | 现状 | 结论 |
|---|---|---|
| 外部内容与系统指令隔离 | `pkg/guardrails/input/injection.go` 有注入模式检测（忽略指令、角色冒充等） | **可直接复用**，Phase 3 接入 Research 抓取链路 |
| 注入测试用例 | `pkg/guardrails/input/injection_test.go` 已覆盖 forget / system impersonation 等 | 有基础，需补「网页正文植入指令」端到端用例 |
| 搜索结果 / 摘要清洗与来源标注 | 上游无统一清洗层 | **缺口**，T3.2 抓取解析时新增 |

## 4. API Key 与敏感信息

| 检查项 | 现状 | 结论 |
|---|---|---|
| Key 加密存储 | `pkg/config/credentials.go` 走环境变量 / 配置文件；未见字段级加密 | **部分满足**，Settings 落地时补加密（AES-GCM + 主密钥） |
| 接口只返回掩码 | 上游无 Settings API（配置在 YAML / Studio） | **新增时强制掩码**（`sk-****abcd`） |
| 仓库 / 测试固件无真实 Key | 抽查 `.env.example`、configs、docs 均为占位符 | 通过 |

## 5. 数据隔离与导出安全

| 检查项 | 现状 | 结论 |
|---|---|---|
| 多用户数据过滤 | 见 1.4，任务 / 会话缺 user 过滤 | **不通过**，随身份修复 |
| 报告导出路径穿越 | 上游无报告导出功能 | 新增时按 `filepath.Clean` + 白名单根目录实现，T4.4 验收 |
| 快照访问鉴权 | 上游无对象存储快照 | 新增时签名 URL + 鉴权，T3.2 设计 |

## 6. 其他

| 检查项 | 现状 | 结论 |
|---|---|---|
| 关键操作审计日志 | `pkg/observability` 有结构化日志；admin 关键操作未统一审计 | 部分满足，Phase 1 补审计事件 |
| 依赖漏洞扫描 | 本次未跑 `govulncheck`（Phase 0 允许，Docker 未装不影响） | 待 T5.5 补，结果写回本节 |
| admin key 比较 | `token == h.adminKey` 非常量时间 | **已知缺陷**，随身份修复 |

---

## 7. 人工确认清单（本次需要你拍板）

- [ ] 认可「身份隔离不成立」的结论与证据（`identity_isolation_test.go` 可本地复跑）
- [ ] 认可 V1 默认单用户本地模式，多用户能力延后到修复完成
- [ ] 认可前端技术栈分歧的处理建议（Studio 保留、产品前端另建 Next.js）
- [ ] 认可 Phase 0 遗留项：T0.7（migration v001）合并到 Phase 1 首任务

**未获确认前，不进入 Phase 1 多用户相关功能。**
