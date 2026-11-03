# Benchmark 报告（实测）

> 本文件只写**本机实测**数字。凡是没跑过的项目一律标注「未实测」并给出原因，
> 不把目标值当实测值。
>
> 采集环境：Windows 11 / AMD64，Go 1.27.3（`go.mod` 声明 1.24.4），
> PostgreSQL 16.2 便携二进制（127.0.0.1:55432，非 Docker），
> 单机单进程 `researchos all`（API + 4 并发 Worker），无 LLM API Key，
> 检索 Provider = github / hackernews / wikipedia / bing。
> 采集时间：2026-11-03。

---

## 1. 端到端并发压测

执行脚本：`scripts/load-test.sh`（真实调用外部检索与抓取，非 Mock）

| 轮次 | 并发任务 | 深度 | 任务数 | 完成 | 失败 | 成功率 | P50 | P95 | 平均节点 | 平均事件 |
|---|---|---|---|---|---|---|---|---|---|---|
| 第 1 轮（修复前） | 5 | quick | 5 | 1 | 4 | 20% | 18s | 20s | 6.0 | 59.4 |
| 第 2 轮（补充降级后） | 5 | quick | 5 | 4 | 1 | 80% | 13s | 18s | 6.0 | 69.8 |
| 第 3 轮（6 并发） | 6 | quick | 6 | 5 | 1 | 83% | 52s | 56s | 6.0 | 69.3 |
| 第 4 轮（相关性硬闸门，已退回） | 5 | quick | 5 | 2 | 3 | 40% | 20s | 20s | 6.0 | 65.2 |
| 第 5 轮（当前代码） | 5 | quick | 5 | 4 | 1 | **80%** | **13s** | **17s** | 6.0 | 67.0 |

### 1.1 第 3 轮耗时为什么退化到 52s

第 3 轮把并发从 5 提到 6，P50 从 13s 涨到 52s，P95 56s。原因是外部检索
Provider 的匿名配额与抓取超时，不是本地调度：同一时刻 6 个任务各开 2 个
search 节点，共 12 路外部请求并发打向 bing/wikipedia，多路触发等待或重试。
本地节点执行与 Worker 抢占未出现排队（平均节点数仍为 6，事件数稳定）。

### 1.2 两种失败模式（未掩盖）

五次压测里失败样本的根因只有两类，均已定位到具体代码位置：

| 失败模式 | 触发条件 | 当前行为 | 定性 |
|---|---|---|---|
| 检索召回不足导致 0 结论 | 中文长句检索式在 Bing zh-CN 下被过度切分（"向量数据库"→"向量"），召回百科「向量（数学用语）」类页面；页面正文被 robots 拒绝，资料池为空或只剩 1 条无关条目 | `analyze` 标记 skipped、`synthesize` 失败，任务落 `failed` 并写明「有 N 个节点失败」 | 已知限制，见 §4 |
| 资料不足导致无法成文 | 同上，资料池只有 1 条且抽不出可用陈述 | 同上 | 同上 |

**没有为了让成功率好看而把「无结论」的任务标成 completed。** 一份没有证据的
报告比一份失败记录更容易误导人。

### 1.3 本轮为提升成功率做的修复（都有回归用例）

1. `condenseCJK` 曾把「检索增强生成」的「生成」当动词删掉，检索式退化成
   「向量数据库在检索增强 中 召回质量对比」→ 改为只剥句首句尾动作词，
   不删任何其它汉字。
2. 纯中文目标的所有检索分支曾生成同一条检索式 → 追加中文维度词
   （现状 / 方案对比 / 风险与限制 / 趋势 / 评测基准）。
3. Bing 固定 `mkt=en-US` → 改为按检索式语种选市场。
4. GitHub / Hacker News 曾接收纯中文整句 → 语言路由跳过并写入 trace。
5. `excerptUsable` 曾要求「≥3 行、每行 ≥40 字」，把单段落的长正文（维基条目
   常见形态）判成不可用 → 增加「≥400 字 + ≥3 个句末标点 + 信息 token ≥12」的
   并列分支。
6. 落库 snippet 曾长达 5.9 万字符（GitHub Provider 把整份 README 塞进摘要）
   → 截断到 600 rune。
7. 「一句话结论」曾放行书名罗列与整段罗列 → 增加 120 字上限、《》成对出现、
   逗号/顿号 ≥5 的拒绝规则。

---

## 2. 单元测试覆盖率

命令：`go test -cover ./internal/researchos/...`

| 包 | 覆盖率 | 说明 |
|---|---|---|
| `dag` | 96.2% | 依赖就绪、租约、退避、暂停闸门 |
| `llm` | 94.5% | 请求形状、JSON 模式、围栏剥离、错误截断、上下文取消 |
| `domain` | 90.5% | 状态机全矩阵、事件类型 |
| `config` | 78.4% | 环境变量解析、默认值、非法值、TTL 上下限 |
| `report` | 76.7% | 章节渲染、引用编号、争议列表 |
| `textutil` | 74.0% | 正文清洗、模板剥离、截断、逐字子串保证 |
| `planner` | 73.3% | 计划归一化、环检测、检索式优先级 |
| `fetch` | 37.8% | robots、SSRF 守卫、JSON 渲染 |
| `exec` | 31.3% | 结论闸门、相关性、交叉验证状态映射、永久错误分类 |
| `search` | 19.7% | 去重、降级、语言路由、市场选择 |
| `api` | 12.7% | 鉴权中间件、SSE ticket 全生命周期（新增 24 条） |
| `db` / `store` / `worker` | 0.0% | 以端到端与故障注入验证，见 §2.1 |

### 2.1 为什么 api / store / worker 覆盖率低

- `api`：25 个路由处理器需要真实数据库连接才能跑通。当前覆盖的是**不依赖数据库**
  的部分：鉴权中间件与 SSE ticket 状态机（ticket 是独立内存结构，可以完整测）。
  用 sqlmock 造假库只能覆盖分支形状、验证不了 SQL 语义，所以没有为数字堆这类测试。
- `store` / `worker` / `db`：都是 `pgx` + `sqlc` 生成代码的直接封装。
  真实语义靠 §1 的端到端压测、`scripts/fault-injection.sh` 与
  `exec` 包的集成测试（`RESEARCHOS_TEST_DSN=...` 直连真实库跑 verify 节点）验证。
- `worker` 的重试分支不是靠覆盖率证明的：永久错误（`exec.IsPermanent`）走「失败不重试」，
  实测把 `synthesize` 的 attempts 从 3 次降到 1 次。
- `go test -cover` 对这四个包会报 `cmd/internal/cov is not in std`（Go 1.27.3
  工具链与 `go.mod` 声明的 1.24 不一致时的已知问题），`go test ./...` 本身正常。

---

## 3. 其他实测项

| 项目 | 实测结果 | 命令 / 证据 |
|---|---|---|
| 迁移可执行 | 通过，`research_schema` 表记录 `0001_init.sql` | `researchos migrate` |
| 健康检查 | `status=ok`，pool total=5，migrations 已列出 | `GET /api/v1/health` |
| SSE 事件量 | quick 任务 63–75 条事件 | 压测脚本 `events` 列 |
| 报告导出 | `report.md` 200 / `text/markdown`；`report.html` 200 / `text/html` | `GET /api/v1/research/{id}/report.md` |
| 报告体积 | 正常任务 8–15KB；异常样本曾达 284KB（书单罗列结论导致，修复后回落） | 实测对比 |
| 静态页面免鉴权 | `/` 200、`/task/` 200；`/api/v1/research` 无 token → 401 | `curl -i` |
| 单测规模 | 13 个包 140+ 用例，`go test ./internal/researchos/...` 全绿 | 见 §2 |
| SSE ticket 全生命周期 | 签发 200 → 建流 200 → 重放 401 → 跨任务 403 → 过期 401 → 当 Token 用 401 | 实测 curl 序列，见 `docs/security-review.md` §1.8 |
| 事件断点续接 | `?after_sequence=10` 从 id=11 开始；`Last-Event-ID: 50` 从 id=51 开始 | 116 条事件、序列严格递增无空洞、`event_id` 唯一 |
| 报告引用可追溯性 | 18 条证据全部逐字落地来源原文，18 个编号一一对应，10 条核心发现全部可回溯 | `scripts/verify-citations.py --task ee9d550d` |
| 交叉验证 / 冲突识别 | 集群 3 组：supported=2、conflicting=2、unverified=1，冲突双方共享冲突组 | `RESEARCHOS_TEST_DSN=... go test ./internal/researchos/exec -run TestVerifyIntegration` |
| 中断恢复 | 强杀 Worker → 租约过期 → 接管，8/8 节点完成，停机前已完成节点 attempts 保持 1 | `scripts/fault-injection.sh`（PASS） |
| 节点重试语义 | 确定性失败 attempts=1（修复前 3 次）；人工 retry 后 attempts=2；对已完成节点 retry → 409 | `POST /nodes/{id}/retry` 实测 |

### 3.1 未实测项（明确标注）

| 项目 | 状态 | 原因 |
|---|---|---|
| Docker Compose 启动 | 未实测 | 本机未安装 Docker Desktop（用户决策：Phase 5 前不装），`docker-compose.yaml` 与 `Dockerfile.researchos` 已写好但只做过语法审阅 |
| 真实大模型质量 | 未实测 | 未提供 LLM API Key。**链路**已用本地 OpenAI 兼容测试替身验证通（`planner_mode=llm`、`llm_used=true`、`dropped_ungrounded=2`/次），但「链路通」不等于「效果好」，报告质量仍需真实 Key 验收 |
| 浏览器端 E2E | 未实测 | 本机 Chrome/Edge 无头模式无输出（环境限制），`scripts/screenshot.sh` 需人工在有桌面的环境执行 |
| 大批量压测（≥20 并发） | 未实测 | 会把匿名检索 Provider 打到限流，压测脚本注释里已写明控制规模 |
| Windows 以外平台 | 未实测 | 仅在 Windows 11 / AMD64 验证 |

---

## 4. 已知性能与质量限制

1. **中文检索召回是当前最大瓶颈。** Bing zh-CN 对「向量数据库」这类复合词会
   切到「向量」，召回线性代数页面；Wikipedia 中文接口对长句检索式敏感。
   没有 LLM 做查询改写与重排时，这类目标会出现「资料池为空 → 任务失败」。
   配置真实 LLM Key 后这条链路才谈得上稳定。
2. **无 LLM 时结论由规则抽取**，质量上限是「能逐字引用原文的句子」。规则层已
   挡掉书名罗列、整段罗列、标签串、导航残渣，但挡不住「相关但琐碎」的陈述。
3. **单机单进程**。4 个 Worker 实测足够跑 5–6 并发 quick 任务；更高并发未验证。
4. **没有缓存层**。同一目标重复提交会重新打外部 Provider，配额消耗是线性的。

---

## 5. 复现方式

```bash
# 1. 起数据库（本机用便携二进制，端口 55432）
# 2. 迁移并启动
cd upstream/hector
./bin/researchos.exe migrate
./bin/researchos.exe all

# 3. 并发压测
bash ../../scripts/load-test.sh 5 quick

# 4. 覆盖率
go test -cover ./internal/researchos/...

# 5. 故障注入（需两个终端）
bash ../../scripts/fault-injection.sh
```
