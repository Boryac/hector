# RC1_SECURITY_REPORT — 安全复查报告（2026-11-04）

> 结论先行：**API 层安全探测 13/13 通过**（本轮发现并修复 1 个 CORS 缺陷）。
> V1 保持单用户模式，未为测试开放多用户。本地模型中继仅绑定回环。

探测脚本：`scripts/rc1_security_probe.py`（原始证据：
`scripts/rc1_evidence/security_probe.json`）。两项修复均有单元回归
（`api/cors_test.go` 6 断言）。

## 1. 探测结果明细

| # | 检查项 | 结果 | 证据 |
|---|---|---|---|
| 1 | 无 Token 请求受保护端点 | ✅ 401 | `unauthorized`，无信息泄露 |
| 2 | 错误 Token | ✅ 401 | 常数时间比较（`subtle.ConstantTimeCompare`） |
| 3 | 请求体 `user_id`/`created_by` 注入 | ✅ 400 | `unknown field` 严格拒绝（比静默忽略更强） |
| 4 | SSE 无 ticket | ✅ 401 | 事件流必须有短时一次性 ticket |
| 5 | SSE 伪造 ticket | ✅ 401 | 服务端存哈希，用后即焚 |
| 6 | CORS 跨源回显 | ✅（已修复） | 修复前任意 Origin 被反射；现仅同源回显，跨源 ACAO 为空 |
| 7 | CORS 同源 | ✅ | 前端同源部署正常放行 |
| 8 | MCP 未批准 discover | ✅ 409 | `not_approved`，不静默降级 |
| 9 | MCP 探测登记清理 | ✅ | DELETE 生效 |
| 10 | settings 响应密钥回显 | ✅ | 仅掩码（`ofm-****Zh3E`），全响应无明文 |
| 11 | 无权限读任务 | ✅ 401 | 不泄露任务存在性 |
| 12 | llm/test 错误路径 | ✅ | 不回显 Key |
| 13 | 本地模型中继绑定 | ✅ | `127.0.0.1`，LAN 转发关闭 |

## 2. 本轮修复

### CORS 任意 Origin 反射（已修复，P1）

- **发现**：`withCORS` 把请求的任意 `Origin` 原样写入 `Access-Control-Allow-Origin`。
- **实际可利用性评估**：低——鉴权走 `Authorization` 头且未开启 `Allow-Credentials`，
  攻击者页面无 Token 时跨域读写均不成立；但违反最小暴露原则。
- **修复**：仅当 Origin 的 host 与请求 `Host` 一致（同源）时回显；解析失败一律视为跨源。
- **回归**：`api/cors_test.go`（同源回显 / 跨源拒绝 / 畸形 Origin / 预检 204 四类 6 断言），
  加上探测脚本 API 级复验。

## 3. 承接上一轮审计的安全基线（本轮复核未发现退化）

| 项 | 状态 |
|---|---|
| API Key AES-256-GCM 加密入库（`enc:v1:` 前缀） | ✅ 重启后解密正常（阶段3 持久化验证） |
| 数据库 DSN 脱敏显示 | ✅ `/system` 显示 `postgres:****@…` |
| 凭据不落日志 | ✅ 服务日志与 API 响应均无 relay key / API token 明文 |
| Prompt Injection 防护 | ✅ 引用越界剔除、未验证结论不写成事实；引文逐字定位（quoteGrounded）拒收无据引用 |
| SSRF / robots | ✅ Provider 白名单 + robots 默认遵守（任务 C 中 4 条 robots_blocked 如实记录） |
| 高风险工具审批 | ✅ `RESEARCHOS_REQUIRE_APPROVAL=true`，任务批准闸门真实生效（三任务均需 approve 才执行） |
| 默认 Token 定位 | ⚠️ 保持开发配置标注；生产必须显式设置 `RESEARCHOS_API_TOKEN` |

## 4. 已知边界（V1 明确不做）

- 多用户/多租户：不开放（上游 A2A 身份隔离问题未在多租户语义下回归）。
- 本地模型中继暴露 LAN：属网络安全配置变更，未经用户批准不开启（Docker 部署时需要，见 RC1_DEPLOYMENT_REPORT §3）。
