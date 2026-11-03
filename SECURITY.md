# SECURITY.md — 安全策略（对外版，Phase 0 审查后由 Codex 同步完善）

- API Key 加密存储，接口只返回掩码，日志不得含 Key。
- 外部网页内容视为不可信输入，防 Prompt Injection，高风险工具调用需审批。
- 用户数据隔离：查询必须按服务端签发的用户身份过滤，不得信任客户端传入的 user_id。
- 发现安全问题请按仓库 CONTRIBUTING / 私下联系维护者报告，不得公开利用细节。
- 完整审查结论见 docs/security-review.md。
