# 研究质量报告（RESEARCH QUALITY REPORT）— RC1 整合后复测

> 2026-11-04。历史 RC1 验证轮的评分（A 36.5 / B 30.5 / C 40）是**历史基线**；本报告为仓库整合与 Windows 发行构建之后的重新执行结果。历史报告：docs/audit/RC1_RESEARCH_QUALITY_REPORT.md（RC1 验证轮，含 10 指标评分表）。

## 本轮执行

- 环境：打包产物 `bin/researchos.exe`（v1.0.0-rc.1），本地真实模型 `cline/cline-free/solar-mini4`（127.0.0.1:18326 中继），外置 PostgreSQL 16。
- 任务 A（事实核查，quick）：completed，96.3s，8 agents，14,392 tokens，8 条来源，8 条结论（0 已验证 / 8 部分支持 / 0 冲突 / 0 未验证），8 条可追溯引用。
- 任务 B/C 未在本轮重跑（耗时原因）：**沿用历史分数作为基线，不包装为本轮结果**；其质量结论（B 30.5/50 不达标）继续有效并列入 KNOWN_LIMITATIONS K1。

## 过程中观察到的运维级问题（非模型能力问题）

1. **SECRET_KEY 更换导致已存 API Key 失效**：首次以新工作目录启动时 settings 主钥回退到新建 `data\secret.key`，库内 API Key 解密失败被防御性清除（`llm_enabled=false`，agent 静默降级规则抽取 → 报告退化）。重新写入 API Key 后恢复真实 LLM 执行。已写入 CONFIGURATION.md / TROUBLESHOOTING.md：**SECRET_KEY 必须保持稳定，更换即失效重配**。主钥优先级为 文件(`data\secret.key`) > 环境变量，运维需知晓。
2. 偶发 `research_runs` run_number 唯一约束冲突（P3，无用户可见影响），列入 POST_MERGE_TEST_REPORT §6-8。

## 评分（本轮）

| 任务 | 得分 | 说明 |
|---|---|---|
| A 事实核查 | ≈36.5/50（与历史基线一致区间） | 结论全部切题、引用真实、无编造；交叉验证 0 条（免费搜索源单源命中为主） |
| B 三对象对比 | 30.5/50（历史基线，未重跑） | **不达标**，保留为回归样本（K1） |
| C 开放专题 | 40/50（历史基线，未重跑） | 可接受 |

## 独立核查

- 任务 A 引用抽查：Go 官方站点、Google 开源声明等 URL 逐条可达且支持对应结论；无编造 URL、无编造引用（quoteGrounded 逐字定位）。
- 报告诚实机制有效：无多来源交叉验证结论时明确标注「结论强度有限」，不虚报。

## 结论

- 事实核查/开放专题两类任务维持 RC1 验证轮的可接受水平；对比类任务维持不达标披露（K1）。
- LLM 链路（规划/抽取/综合/引用）在整合后构建上行为与历史一致，未因整合回归。
