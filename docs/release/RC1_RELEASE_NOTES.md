# RC1 Release Notes — Hector ResearchOS v1.0.0-rc.1

> 2026-11-04 · Windows x64 便携版 · commit 与构建信息见随包 BUILD_INFO.txt

## 这是什么

Hector ResearchOS 是一个自主研究平台：输入一句研究目标，系统自动规划任务、组织多个 AI Agent 并发检索互联网、提取与交叉验证信息，产出**带真实引用、可追溯**的结构化研究报告（Markdown / HTML / PDF，PDF 为服务端真实排版：内嵌中文字体、页码、表格跨页）。

## 本版本包含

- 后端单二进制（API + Worker）+ Next.js 14 静态前端，Go + Wails 托盘管理程序（窗口关闭/托盘退出均不影响后台研究任务）。
- 首次启动向导：数据目录选择（程序/数据分离）、外置 PostgreSQL 16 连接与幂等迁移、开机自启（默认关，用户级）。
- 设置中心：LLM Provider（OpenAI 兼容）、搜索 Provider、审批策略热更新；API Key 经 AES-256-GCM 加密入库、掩码回显。
- 凭据保护：Token/DSN/加密主钥经 Windows DPAPI（当前用户作用域）加密存储。
- 统一校验 verify.sh 14 项（含负向自检）、浏览器审计 27 项、安全探测 13 项（全部在本轮整合后重测通过）。

## RC1 之前的真实模型验收（如实披露）

使用本地 Solar Mini 4 真实执行了三类研究任务（详见 docs/audit/RC1_RESEARCH_QUALITY_REPORT.md 与 RESEARCH_QUALITY_REPORT.md）：

- 任务 C（开放专题）40/50、任务 A（事实核查）36.5/50：可接受。
- **任务 B（三对象对比）30.5/50：不达标**——对比对象只覆盖 1/3，保留为已知限制与回归样本。
- 独立事实核查 8/8 正确，0 编造 URL、0 编造引用（quoteGrounded 逐字定位机制）。

## 已知限制（必读）

见 `docs/release/KNOWN_LIMITATIONS.md`。要点：任务 B 质量不足、Docker 实机验证 BLOCKED、复杂任务 LLM 超时风险、免费搜索源覆盖限制、单用户本地模式。

## 升级与数据安全

程序目录与数据目录分离，覆盖程序文件不影响数据。**数据库迁移可能不可逆，升级前请 pg_dump 备份**。本版本不包含自动更新器。
