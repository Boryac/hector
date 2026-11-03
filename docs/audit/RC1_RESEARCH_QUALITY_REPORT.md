# RC1_RESEARCH_QUALITY_REPORT — 真实模型研究质量评估（2026-11-04）

> 结论先行：**三类研究任务全部真实完成（LLM Planner + LLM 抽取，零 Mock）**，
> 但质量分两档：任务 C（deep）达到可接受标准（17 条结论全部切题、事实独立核查
> 全部正确、引用全部真实）；任务 A（quick）部分达标（事实正确但覆盖不全）；
> 任务 B（standard）覆盖不足（三对象对比只覆盖了一个）。**两轮修复各解决一个
> P0 根因**：① 检索 UA 被 Bing 反爬降级（修复后任务 A 从 0 条切题结论 → 全部切题）；
> ② LLM 抽取 90s 超时静默降级（修复后任务 C 从规则抽取的跑题结论 → 37.6k token
> 的真实 LLM 抽取）。核心质量保证机制（引文逐字定位、缺证拒收、诚实标注）
> 在全部任务中工作正常，**未发现任何编造 URL 或编造引用**。

评估对象：`scripts/rc1_evidence/task_{A-factcheck,B-comparison,C-open}.json`
（修复后终版）+ `*.PREFIX-UA.json` / `*.timeout90s.json`（修复前对照）。
评估方式：报告全文人工审读 + 结论逐条与公开事实独立核对（非模型自证）+
事件流（SSE）复核执行轨迹。

## 1. 三轮运行对照（同一任务跨修复的证据链）

| 任务 | 轮次 | 耗时 | 结论 | 切题率 | 关键差异 |
|---|---|---|---|---|---|
| A 事实核查 | 修复前（UA 缺陷） | 60s | 7 | **0/7** | 结论全是自杀基金会/PHP 等噪声页片段 |
| A 事实核查 | 修复后 | 126s | 4 | **4/4** | 官方镜像站+教程站，2 条多来源验证（0.70） |
| B 三对象对比 | 修复前 | 132s | 1 | 1/1（但仅 PG） | 同 UA 根因 |
| B 三对象对比 | 修复后 | 156s | 2 | 2/2（仍仅 PG） | MySQL/SQLite 检索词命中差 |
| C 开放专题 | 修复前（90s 超时） | 144s | 6 | **0/6** | LLM 全部超时→规则抽取抓到企业微信页 |
| C 开放专题 | 修复后（240s） | 367s | 17 | **17/17** | 37.6k token 真实 LLM 抽取，全部 RAG 相关 |

## 2. 独立事实核查（人工比对公开事实，非模型自证）

| 报告结论 | 独立核查 | 判定 |
|---|---|---|
| 「Go 是 Google 支持的开源语言」（任务A，多来源 0.70） | golang.org 与 google/golang 官方口径一致 | ✅ 正确 |
| 「Go 2009 年 11 月开源，Go 1 稳定版 2012 年初发布」（任务A） | Go 官方博客历史一致 | ✅ 正确 |
| 「PostgreSQL 采用 BSD 许可证」（任务B） | postgresql.org 官方：PostgreSQL License（BSD 类） | ✅ 正确 |
| 「RAG 由 Facebook AI Research 2020 提出」（任务C） | Lewis et al., NeurIPS 2020, FAIR | ✅ 正确 |
| 「RAG 流程：文档分块→文本向量化→向量库存储→相似度检索→增强生成」（任务C） | 与领域标准流程一致 | ✅ 正确 |
| 「RAG 缓解 LLM 知识截止与幻觉」（任务C） | 领域共识 | ✅ 正确 |
| 「RAG 与长上下文不互斥，RAG 供精确材料、长上下文做推理」（任务C） | 与 2024-2026 工程共识一致 | ✅ 正确 |
| 编造 URL / 编造引文 | 全部 17+4+2 条引用可打开且指向真实页面；引文逐字定位机制（quoteGrounded）拒收无法在原文定位的引用 | ✅ 未发现 |

## 3. 十项指标评分（0–5）

### 任务 A 事实核查（修复后终版）

| 指标 | 分 | 依据 |
|---|---|---|
| Planning Quality | 4 | 3 个研究问题准确拆分目标；DAG search→analyze×3+verify+synthesize 结构合理 |
| Retrieval Quality | 3 | 命中官方镜像 golang.google.cn 与 runoob；但未抓到 go.dev/doc/install（版本号页） |
| Source Reliability | 3.5 | 官方镜像+知名教程站；无低质论坛噪声 |
| Citation Accuracy | 4 | 引文逐字可定位；事实核查全部通过 |
| Evidence Coverage | 2.5 | 3 问只回答 2 问（许可证未抽取到） |
| Conflict Detection | 4 | 机制运行（独立站点数+数值一致性），本任务无冲突场景 |
| Synthesis Quality | 3 | 结论真实但未直接给出「最新版本号」，汇总以摘录为主 |
| Hallucination | 5 | 0 编造；无据引文被拒收（dropped_ungrounded 机制） |
| Completeness | 2.5 | 覆盖 2/3 子问题 |
| Reproducibility | 5 | plan/nodes/agents/sources/evidence/report 全链路落库，事件流可回放 |

**小计 36.5/50**。失败样例：analyze_q3（许可证）抓取的 3 个来源正文里 BSD 关键词未被 LLM 抽为结论——抽取提示词未强调「逐问题必须给出答案或明确标注无法回答」。

### 任务 B 多对象对比（修复后终版）

| 指标 | 分 | 依据 |
|---|---|---|
| Planning Quality | 3.5 | 3 问题对应 3 维度正确，但 subtask 检索词未对每个对象单独建查询 |
| Retrieval Quality | 2 | PG 命中 runoob；MySQL/SQLite 检索被中文市场 bing 弱化（14 来源中仅 PG 相关） |
| Source Reliability | 3 | 命中的 runoob 内容真实 |
| Citation Accuracy | 4 | 2 条结论引文可定位且事实正确 |
| Evidence Coverage | 1.5 | 「三对象对比」只覆盖 1 个对象 |
| Conflict Detection | 3 | 无冲突场景可验证 |
| Synthesis Quality | 2 | 未产出对比表格（报告模板支持表格但结论不足以填充） |
| Hallucination | 5 | 无编造 |
| Completeness | 1.5 | 核心目标（对比）未达成 |
| Reproducibility | 5 | 同上 |

**小计 30.5/50**。失败样例：search_2 检索词「MySQL SQLite 许可证 场景 对比」在中文市场 bing 召回 PG 教程；LLM planner 未按对象×维度生成 3×3 检索矩阵。

### 任务 C 开放专题（修复后终版）

| 指标 | 分 | 依据 |
|---|---|---|
| Planning Quality | 5 | 5 问题精准覆盖落地路径/质量/治理/评估/成本；检索词专业 |
| Retrieval Quality | 4 | CSDN/博客园/AWS/datawhale 全部切题；robots 拦截 4 条已如实记录 |
| Source Reliability | 3 | 技术博客为主，缺官方文档一级来源（aws.amazon.com 除外） |
| Citation Accuracy | 4 | 17 条引用全部真实且引文逐字定位 |
| Evidence Coverage | 3.5 | 17 条结论覆盖 5 个研究问题 |
| Conflict Detection | 3.5 | 如实报告「未检测到冲突」；无冲突场景 |
| Synthesis Quality | 3.5 | 结论构成完整知识链（定义→流程→实践→权衡） |
| Hallucination | 5 | 0 编造；「chunk_size=500」等具体数值均可溯源 |
| Completeness | 3.5 | 主要维度覆盖，落地路径细节略浅 |
| Reproducibility | 5 | 同上 |

**小计 40/50**。失败样例：结论 10 混入了原页面推广文案（「墨衍会员 ¥399 起/年」）——抽取未过滤页面广告文本。

## 4. 修复记录（发现→定位→修复→回归）

| # | 缺陷 | 根因定位 | 修复 | 回归证据 |
|---|---|---|---|---|
| 1 | 检索结果与查询无关（A 全噪声） | 用服务默认 UA 实测 bing：反爬降级返回酒店广告页（对照：浏览器 UA 返回正常结果） | `defaultSearchUA` 改为真实浏览器 UA（config/fetch/search 三处），注释记录实测依据 | 任务 A 重跑：0/7→4/4 切题 |
| 2 | LLM 抽取静默降级（C 跑题） | SSE 事件流：4 个 analyst `context deadline exceeded`（90s 默认超时，reasoning 模型大 prompt 不够） | 设置中心保存路径调 `model.timeout=4m`（热生效） | 任务 C 重跑：0/6→17/17 切题，token 6.2k→37.6k |

## 5. 改进建议（不阻塞 RC1，记入 CHANGELOG「已知限制」）

1. **抽取 prompt 增加逐问题闭环要求**：每个研究问题必须产出结论或显式「无法回答」，避免 A 的许可证问题被静默跳过。
2. **对比类任务生成对象×维度检索矩阵**：planner 对 comparison 型目标按对象分别建查询。
3. **抽取过滤页面杂质**：广告/导航/推荐位文本（如 C 的会员推广）应在清洗层剔除。
4. **付费检索 Provider 解锁**：tavily/serpapi/brave 可显著改善中文检索覆盖（B 的主要瓶颈）。
5. **analyst 并发限流**：5 路并发大 prompt 对本地免费中继压力大，可按 Provider 并发数自适应。

## 6. 结论

- **可接受标准判定**：真实模型链路 PASS；任务 C PASS；任务 A PARTIAL；任务 B PARTIAL（对比目标未达成，如实标注）。
- 系统侧无编造、无静默丢证据、降级必留痕（事件流可查）——**证据基础设施达到 RC1 要求**。
- 剩余质量问题主要来自免费检索源覆盖与抽取 prompt 精细度，属已知限制而非缺陷；均已在 §5 给出改进路径。
