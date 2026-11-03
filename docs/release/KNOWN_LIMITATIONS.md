# 已知限制（KNOWN LIMITATIONS）— v1.0.0-rc.1

> 列入本文件的问题**不是缺陷隐瞒**：每一项都有验证证据与跟踪位置。RC1 只修复 P0/P1 与发行阻断问题，以下按原样披露。

## 功能与质量

| # | 限制 | 证据/位置 | 影响 |
|---|---|---|---|
| K1 | **任务 B（多对象对比）研究质量 30.5/50，不达标** | docs/audit/RESEARCH_QUALITY_REPORT.md | 对比类研究中三个对象只覆盖 1/3；保留为回归样本。替代：使用事实核查/开放专题任务类型 |
| K2 | 复杂任务 LLM 超时风险：`model.timeout` 默认 90s 对 reasoning 模型大 prompt 不够（已调 4m），**不能宣称所有复杂任务已稳定** | docs/audit/RESEARCH_QUALITY_REPORT.md、DECISIONS 41 | 长任务偶发 analyst 静默降级；事件流 `agent.failed` 可回溯 |
| K3 | 免费搜索源（bing 抓取）覆盖有限；中文对比类检索尤其弱 | RC1 质量报告 | 配置付费 Provider（tavily/serpapi/brave）可改善 |
| K4 | 真实模型质量依赖本地免费中继：延迟高、偶发 502、无 SLA | docs/audit/RC1_MODEL_CAPABILITY_REPORT.md | 生产建议配置正式 Provider |
| K5 | 单用户本地模式：身份只认服务端 Token，多用户能力未开放 | docs/audit/SECURITY_AUDIT.md | 不适合多人共享部署 |

## 平台与部署

| # | 限制 | 证据/位置 | 影响 |
|---|---|---|---|
| K6 | **Docker 实机验证 BLOCKED**（构建机无 Docker）；compose 仅静态校验 | docs/audit/POST_MERGE_TEST_REPORT.md | 容器化部署路径未实机验证 |
| K7 | 仅 Windows x64 打包；macOS/Linux 未出发行包 | docs/installation/WINDOWS_DISTRIBUTION.md | 其它平台用源码构建 |
| K8 | 无 MSI 安装器、无自动更新器（RC1 决策） | RC1_RELEASE_NOTES | 升级=覆盖程序目录；先备份数据库 |
| K9 | PostgreSQL 16 外置依赖，ZIP 不捆绑、不自动安装 | QUICK_START | 用户需自备数据库 |
| K10 | 数据库迁移可能不可逆，应用不做自动回滚承诺 | CONFIGURATION | 升级前 pg_dump |
| K11 | Web UI Token 首次经 `#fragment` 注入（不进服务端日志，但在浏览器历史可见直至清除） | SECURITY_AUDIT | 本地单用户可接受 |
| K12 | 托盘依赖系统托盘区；Server Core/特殊远端环境不可用时需用 scripts\*.ps1 直接管理 | WINDOWS_DISTRIBUTION | 降级路径完整 |
| K13 | GitHub Actions 工作流未在本机触发的远端实跑（无推送授权）；RC1 产物为本机构建 | docs/audit/POST_MERGE_TEST_REPORT.md | 首次 tag 推送后需人工核对 CI 产物 |
