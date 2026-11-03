# 安全审计报告（SECURITY AUDIT）— RC1 整合后复测

> 2026-11-04。整合后构建实测；历史基线：docs/audit/RC1_SECURITY_REPORT.md（RC1 验证轮 13/13 + CORS 修复）。

## 自动探测（13 项，`scripts/rc1_security_probe.py`）

**13/13 通过**（证据：`scripts/rc1_evidence/security_probe.json`，本轮生成）。覆盖：

无 Token 401、伪造 Token 401、请求体身份注入拒绝（DisallowUnknownFields → 400）、SSE ticket 一次性与过期拒绝、越权任务访问 404、MCP 审批闸门 409、LLM 测试错误路径不回显 Key、本地中继仅绑定 127.0.0.1 等。

## 本轮新增面的审查

| 面 | 结论 |
|---|---|
| `POST /api/v1/admin/shutdown`（新增） | 位于 withAuth 之后，需 Bearer Token；未注入回调时 501 不虚报；单测覆盖 401/202/501（api/admin_test.go）。触发路径与 Ctrl+C 相同（先停接入再排空），无数据丢失窗口 |
| 托盘 config.json（新增） | 敏感字段（DSN/Token/SECRET_KEY）DPAPI 加密（CurrentUser 作用域）；原子写（tmp+rename）；解密失败自动回退向导不静默；文件权限 0600 |
| 托盘进程管理（新增） | 强制终止前必须通过 PID + 镜像名 + 创建时间三重身份校验，不按进程名误杀；托盘崩溃不影响后台（无 Job Object 绑定） |
| 自启动（新增） | 仅 HKCU Run（当前用户级），不注册系统服务，不写其它用户配置 |
| Token 注入 Web UI（新增接线） | 优先 `#fragment`（不发送到服务端、不进访问日志）；持久化后立即 `history.replaceState` 清除地址栏；query 形式仅作兼容。浏览器历史残留窗口期已列入 KNOWN_LIMITATIONS K11 |
| 发行脚本（新增） | diagnostics 输出强制脱敏（DSN 口令、Bearer、sk-）；脚本只读不修改系统配置；start.ps1 的环境去重仅作用于当前进程 |
| 凭据泄漏扫描 | 两仓全历史 `sk-` 模式 0 命中；发行目录无任何真实凭据；本轮全部验证凭据走环境变量运行时读入，未落日志/文档/Git |

## 已知残余风险（与历史一致）

- CORS 仅同源回显（RC1 轮修复），本轮探测确认跨源拒绝。
- V1 单用户：身份只认服务端 Token；请求体身份字段一律拒绝。
- Web UI Token 首次经 URL 注入的浏览器历史窗口期（K11）。
- `RESEARCHOS_SECRET_KEY` 稳定性要求：更换主钥 → 库内 API Key 解密失败被清除（本轮实测确认该防御行为），需重配（文档已写明）。

## 结论

整合与 Windows 发行引入的新攻击面（托盘、脚本、shutdown 端点）均已审查并有测试覆盖；无未缓解的高危项。
