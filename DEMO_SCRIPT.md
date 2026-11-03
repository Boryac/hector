# DEMO_SCRIPT.md — 2–3 分钟演示视频分镜 + 最终演示任务

## 演示任务原文（最终验收唯一任务）
> 研究全球 AI Agent 开源生态，筛选 10 个代表性项目并生成竞争分析报告。

## 分镜（总长 2–3 分钟，Codex 录制时按此剪辑，不得用静态图冒充运行）
- 0:00 输入研究目标（New Research 页，目标原文、深度 standard、默认并发 4、开启交叉验证）。
- 0:20 自动生成工作流（Planner 输出结构化计划 → DAG 自动生成，画布节点逐个出现）。
- 0:40 多个 Agent 并发研究（Agent 中心：至少 3 个 Researcher 并行，工具调用实时滚动）。
- 1:10 实时搜索和证据收集（Sources 中心出现真实 URL、标题、摘要；Evidence 中心出现 Claim 与来源关联）。
- 1:40 故障注入与恢复（人为终止一个 Worker，任务 Paused/重试，恢复后从 Checkpoint 继续，不重头）。
- 2:00 展示完整研究报告（Report Viewer：目录、对比表、争议章节，点引用跳转到 Source；导出 Markdown 与 PDF）。

## README 首屏要求
产品是什么、解决什么问题、一张完整工作流运行画面、一份真实研究报告截图、
如何启动（docker compose up）。首屏不得只罗列 Go / MCP / RAG / A2A 关键词。
