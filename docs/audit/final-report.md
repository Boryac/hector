# 最终交付报告

> 生成时间：2026-11-03
> 代码位置：`upstream/hector`（分支 `feature/hector-researchos`）
> 开发包位置：`hector-researchos-codex-pack`（本目录）
> 两个仓库均**只在本地提交，未推送任何远程**。

---

## 1. 一句话结论

ResearchOS 的研究主链路（提交目标 → 计划 → DAG 执行 → 真实检索抓取 → 证据与交叉验证 →
报告与引用）在本机跑通了，并发、崩溃恢复、身份隔离都有可复现的验证；
**唯一没跑通的是「真实大模型」这一项——因为始终没有提供 LLM API Key**，
系统在缺 Key 时显式降级为本地规则抽取，`/health` 会直接回报 `llm.enabled=false`。

---

## 2. 16 项验收标准逐条对照

| # | 标准 | 结论 | 证据 |
|---|---|---|---|
| 1 | Go 构建通过 | ✅ | `go build ./...` 通过；`bin/researchos.exe` 可执行，`version` 输出 `1.0.0-dev` |
| 2 | Next.js 构建通过 | ✅ | `npx next build` → 静态导出 5 页（`/` 3.85kB、`/task` 53.8kB、`_not-found`），`web/.build` 由 Go 同源托管 |
| 3 | 迁移可执行 | ✅ | `researchos migrate` 执行 `0001_init.sql`，`/health` 回报 `migrations:["0001_init.sql"]` |
| 4 | API 可调用 | ✅ | 24 个 path 全部实现，见 `api/openapi.yaml`；`/health`、`POST /research`、`/approve`、`/report.md` 均实测 200 |
| 5 | SSE 实时推送 | ✅ | 一次演示任务产生 110 条事件（`node.progress` 27、`tool.completed` 26 等）；`Last-Event-ID` / `?after_sequence=` 两种续接均实现 |
| 6 | 真实 LLM 调用 | ❌ **未验证** | 未提供 API Key。`/health` 返回 `llm.enabled=false` 且 `note` 写明降级原因；报告「方法与局限性」也写明「执行模型：本地确定性流程（未使用大模型）」 |
| 7 | 真实搜索 | ✅ | 演示任务收录 39 条来源，来自 wikipedia 13 / github 11 / hackernews 10 / bing 5，其中 18 条抓到正文 |
| 8 | Agent 并发 | ✅ | 4 Worker 并发认领；一次 standard 任务 3 条检索分支并行，压测 5 任务并发 P50 13s |
| 9 | DAG 正确性 | ✅ | `dag` 包 8 条用例（覆盖率 96.2%）：依赖就绪、环检测、`synthesize` 唯一收尾；执行顺序由 `depends_on` 决定 |
| 10 | 来源可追溯 | ✅ | 每条结论带 `source_id` 与逐字 `quote`；`evidence.created`/`evidence.verified` 事件可回放 |
| 11 | 报告引用可跳转 | ✅ | 核心发现里的 `[n]` 与「信息来源」第 n 条一一对应，`report.html` 内为可点击链接 |
| 12 | Worker 中断恢复 | ✅ | `scripts/fault-injection.sh` 实测 **PASS**：强杀 worker → 租约过期 → 新 worker 接管 → t+15s 任务 completed，停机前已完成的 4 个节点 `attempts` 未变 |
| 13 | 节点失败重试 | ✅ | `attempts`/`max_attempts`/`next_attempt_at` 退避重试；压测中观察到 `attempts=3` 后置 failed 的样本，故障注入中被打断节点重跑一次后成功 |
| 14 | 身份安全回归 | ✅ | Issue #2 修复 + 6 条翻转用例；伪造 `metadata.user_id` 不再生效，日志记录 `ignored client-supplied user_id` |
| 15 | Docker Compose 可验证 | ⚠️ **未验证** | `docker-compose.yaml` + `Dockerfile.researchos` 已交付（三阶段构建、非 root、healthcheck、端口仅绑 127.0.0.1），但本机未安装 Docker，未实际 `up` 过 |
| 16 | 文档完整 | ✅ | 见第 5 节文档索引，全部按实际落码书写，实测数字与未实测项分别标注 |

15 条通过（含 1 条「降级但诚实」）、1 条明确未验证。**没有把未验证项写成通过。**

---

## 3. 本轮（自主模式）修掉的真实缺陷

按发现顺序，全部有回归用例或可复现命令：

### 3.1 检索质量（决定成败的一类）
1. **中文检索式被自己切坏**：`condenseCJK` 把「检索增强生成」里的「生成」当动词删掉，检索式退化成「向量数据库在检索增强 中 召回质量对比」→ 改为只剥句首句尾动作词，不删任何其它汉字。
2. **纯中文目标所有检索分支复用同一条检索式**：`SearchQueryFor` 忽略 index → 追加中文维度词（现状 / 方案对比 / 风险与限制 / 趋势 / 评测基准）。
3. **Bing 固定 `mkt=en-US`**：中文检索式在英文市场被弱化，实测召回完全无关页面 → 按检索式语种选市场。
4. **GitHub / HN 接收纯中文整句**：召回到「README 里恰好有这些汉字」的个人书单仓库 → 语言路由跳过并写 trace。

### 3.2 抽取与报告质量
5. **单段落长正文被判为不可用**：`excerptUsable` 要求「≥3 行、每行 ≥40 字」，维基条目常是单段落 → 增加「≥400 字 + ≥3 句末标点 + 信息 token ≥12」并列分支。
6. **snippet 长达 5.9 万字符**：GitHub Provider 把整份 README 塞进摘要 → 截断到 600 rune。
7. **书名罗列被当成结论**：增加《》成对、分隔符密集、多句拼接的拒绝规则。
8. **界面文案进结论**：GitHub「Notifications You must be signed in…」、维基「This article is about…」「redirects here」→ 拉黑。
9. **结论以半个单词开头**：`Excerpt` 窗口切在词中间 → 新增 `startsWithSentenceCase`，并把窗口边界对齐到句子边界。
10. **长度上限误杀英文长句**：120 字上限让报告一度只剩 3 条结论 → 改成「≤300 字且句末标点 ≤2」。

### 3.3 恢复与安全
11. **心跳续租硬编码 3 分钟**：`RESEARCHOS_STALE_LEASE` 调小不生效，崩溃恢复永远等 3 分钟 → 新增 `exec.Config.LeaseTTL` 并由配置注入。
12. **govulncheck 命中 3 条可达漏洞**（pgx SQL 注入、x/text 死循环 ×2）→ 升级 pgx v5.9.2 + x/text v0.41.0，研究域扫描清零；副作用是 `go.mod` 最低版本抬到 1.25，Dockerfile 同步。
13. **全模块 20 条上游漏洞** → 只抬依赖版本（不碰上游业务代码）降到 2 条，剩余 2 条如实记录。

### 3.4 测试脚本自身的坑（不修就会得到假结论）
14. 故障注入脚本没读 `.env.researchos` → Token 错，所有 API 调用静默 401。
15. `taskkill //PID $!` 用 MSYS pid 杀错进程（实测把 serve 杀掉）；`kill -9 $!` 只杀外层壳，真正的 exe 变孤儿继续续租 → 改用 `ps -W` 的 WINPID。
16. 验收不变量写错：把「被打断节点的重跑」也算成回归 → 改为只校验停机前已完成的节点。

---

## 4. 实测数字（摘要）

| 指标 | 值 | 出处 |
|---|---|---|
| 并发压测成功率 | 80%（4/5），与第 2 轮一致 | `docs/benchmark.md` §1 |
| 端到端耗时 | P50 13s / P95 17s（5 并发 quick） | 同上 |
| 6 并发退化 | P50 52s / P95 56s，根因为外部检索配额 | 同上 §1.1 |
| 单测 | 12 个包 100+ 用例全绿；dag 96.2%、domain 90.5% | 同上 §2 |
| 演示任务 | 8s、8/8 节点、39 来源、110 事件、报告 4895B | `docs/demo.md` |
| 故障注入 | PASS，恢复耗时 15s（租约 40s） | §3.2 / `scripts/fault-injection.sh` |
| 依赖漏洞 | 研究域 0；全模块 2（上游未发版修复） | `docs/dependency-scan.md` |
| 前端产物 | 5 个静态页面，首屏 JS 87.3kB | `npx next build` 输出 |

---

## 5. 文档索引

| 文档 | 内容 |
|---|---|
| `README.md` | 是什么、怎么起、界面长什么样、状态表（逐项标 ✅/⚠️/🚫 与证据） |
| `ARCHITECTURE.md` | 已实现架构：代码地图（实测行数）、执行链路、状态机、并发恢复、身份边界 |
| `DEPLOYMENT.md` | 三种启动方式、配置表、备份恢复、运维检查、升级流程 |
| `DECISIONS.md` | 20 条已锁定决策（含本轮新增 18–20 条） |
| `TASKS.md` | 三态任务清单（`[x]`/`[~]`/`[ ]`），每项附验收与证据 |
| `PROGRESS.md` | 按轮次的实施记录：做了什么、测试结果、坑与修复、遗留问题 |
| `docs/DATA_MODEL.md` | 13 张表字段、状态机全矩阵、事件类型、认领 SQL 语义 |
| `docs/benchmark.md` | 压测与覆盖率实测、未实测项、已知限制 |
| `docs/demo.md` | 演示任务真实运行记录与产物路径 |
| `docs/demo/report.md|html` | 真实报告产物 |
| `docs/dependency-scan.md` | govulncheck 扫描与依赖升级记录 |
| `docs/security-review.md` | Issue #2 审查结论与 7 项修复状态 |
| `docs/hector-audit.md` / `docs/mapping-table.md` | Phase 0 上游审计与模块映射 |
| `api/openapi.yaml` | OpenAPI 3.1，24 path / 15 schema |
| `DEMO_SCRIPT.md` | 2–3 分钟录屏分镜（含必须讲清的边界） |
| `docs/final-report.md` | 本文件 |

---

## 6. 没做到的事（明确列出）

1. **真实 LLM 未接入**：没有 Key，所以「模型规划 / 模型抽取 / 模型改写」三条链路都走的是本地确定性实现。报告质量因此受限，做不到「筛选 10 个项目并做竞争分析」这类需要判断的产出。
2. **Docker Compose 未实跑**：本机没装 Docker，Compose 与镜像定义只做了静态审阅。
3. **截图与录屏未产出**：本机 Chrome/Edge 无头模式完全无输出（`--version` 都没有），需在有桌面的机器跑 `scripts/screenshot.sh` 并按 `DEMO_SCRIPT.md` 录制。
4. **前端浏览器 E2E 未做**（T6.10）：无 Playwright、无可用无头浏览器；界面核对路径写在 `docs/demo.md` §3。
5. **PDF 导出未做**（T6.1）：当前支持 Markdown 与 HTML 导出；PDF 需要纯 Go 排版实现，尚未开始。
6. **中文检索召回仍是瓶颈**：Bing zh-CN 对复合词过度切分，缺 LLM 查询改写时，部分中文目标会出现「资料池为空 → 任务 failed」。这是真实的能力边界，没有靠放宽判定来掩盖。
7. **多用户仍禁用**：共享密钥固定 `Subject`，会话归属校验未做，与 `DECISIONS.md` 第 13 条一致。

---

## 7. Git 状态

| 仓库 | 分支 | 状态 |
|---|---|---|
| `upstream/hector` | `feature/hector-researchos` | 本地提交，**未推送** |
| `hector-researchos-codex-pack` | 默认分支 | 本地提交，**未推送** |

推送到远程需要仓库地址与凭据，按安全边界未擅自配置；`git remote -v` 为空时不执行任何 push。
