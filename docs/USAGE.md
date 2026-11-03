# 使用手册

面向使用者，按「打开 → 建任务 → 批准 → 看过程 → 拿报告」的真实点击顺序写。
接口与字段的一手依据是 `api/openapi.yaml`；本机实测状态见 `docs/demo.md`。

---

## 0. 先确认它在跑

```bash
curl -s http://127.0.0.1:8080/api/v1/health
```

返回 `status: ok` 即可用。三个字段值得先看一眼：

| 字段 | 含义 | 影响 |
|---|---|---|
| `checks.database.ok` | 数据库是否连通、迁移是否已执行 | false 时任何写操作都会 500 |
| `checks.llm.enabled` | 是否配了大模型 Key | false 时规划与抽取降级为本地规则，报告质量受限 |
| `checks.search.providers` | 可用检索引擎列表 | 空列表时任务一定失败 |

没起来就按 README「快速开始」的方式 A/C 启动，或直接：

```bash
cd upstream/hector && ./bin/researchos.exe all
```

`all` 是一个进程同时跑 API 与 Worker。调试时用 `serve` / `worker` 分开跑。

---

## 1. 打开界面

浏览器访问 <http://127.0.0.1:8080/>。前端是静态产物，由同一个 Go 服务托管，不需要另起 Node。

页面左上角有个 **API Token** 输入框，默认值 `local-dev-token`，与 `.env.researchos` 里的 `RESEARCHOS_API_TOKEN` 一致，**不用手输**。换过 Token 的话在这里填一次，存在浏览器 localStorage 里，之后不再问。

> 说明：页面本身免鉴权（否则浏览器加载不了静态资源），`/api/` 下的每个请求都带 Bearer Token。Token 不进 URL，唯一的例外是 SSE 长连接的 `?token=`——浏览器原生 EventSource 不支持自定义请求头，这是没有办法的折中。

---

## 2. 建一个研究任务

左侧表单三样东西：

**研究目标**——写成一句「要研究什么 + 要产出什么」的话，不要只写关键词。

- 好：`对比 pgvector、Qdrant、Milvus 在千万级向量下的召回率与写入吞吐，给出选型建议`
- 差：`向量数据库`（没有可比对象、没有产出形态，规划器只能瞎猜子问题）

**研究深度**——决定拆几个子问题、几个节点：

| 取值 | 子问题数 | 典型节点数 | 典型耗时（本机实测） |
|---|---|---|---|
| `quick` | 2 | 6 | 10–20s |
| `standard` | 3 | 8 | 15–40s |

**交叉验证**——勾上后，同一结论必须在 ≥2 个独立站点出现才算「已验证」，只有单一来源的会被标成待核验并在报告「争议与未证实」里列出。要严谨就勾，只想快速看个大概可以不勾。

点 **新建研究任务**。

---

## 3. 批准才会真跑（关键一步）

创建后任务处于 `awaiting_approval`——规划器已经把 DAG 拆好，但**一个网页都没抓**。这是有意设计的执行闸门：先看它打算怎么干，再决定要不要花检索配额。

进任务详情页，右上角 **批准并执行**。点下去状态转 `running`，Worker 才开始检索、抓取、抽取、验证、写报告。

不想跑了就在同一排按钮里点 **取消**；跑到一半想停点 **暂停**，之后点 **恢复** 从断点续跑（已完成的节点不会重跑）。

这个闸门由 `RESEARCHOS_REQUIRE_APPROVAL` 控制，本机是 `true`。设成 `false` 会让任务创建后直接进入 `running`——适合脚本批跑，不适合手工试用（你还没看清它要干什么就已经把检索配额花掉了）。

---

## 4. 看执行过程

详情页顶部是状态条与 DAG 图，节点按依赖推进，颜色即状态。下面五个 Tab：

| Tab | 看什么 |
|---|---|
| **时间线** | 事件流，逐条 SSE 实时推送。断线自动重连，按 `sequence` 从断点续接，不丢事件也不重复 |
| **来源** | 抓到的每个页面：URL、站点、状态（`ok` / `discovered` / `fetch_failed` / `robots_blocked`）、可信度、正文摘要 |
| **证据** | 每条结论到原文的引用关系，点开能看到被引用的原句 |
| **代理** | 各 Agent 的并发执行情况、尝试次数、耗时分段 |
| **报告** | 成稿正文，带引用编号 |

时间线右上角有个 `SSE 实时` / `轮询兜底` 标识。显示「轮询兜底」说明长连接没建起来（代理、浏览器插件或网络策略拦截），功能不受影响，只是刷新粒度变粗。

单个节点失败会显示红色并带 **重试** 按钮。节点的失败与重试是隔离的：一个检索分支失败不会拖垮整张图，下游会把该分支标成覆盖缺口写进报告，而不是伪造结论。

---

## 5. 拿报告

任务 `completed` 后在 **报告** Tab 看正文，或从两个按钮导出：

- **导出 .md**——纯 Markdown，引用编号与正文一致，适合再加工
- **导出 .html**——带样式的单文件，直接发人

两者都由带 Token 的请求取回再落盘，Token 不出现在下载链接里。

报告抬头会写明四件事：执行模型（`本地确定性流程` 表示没用 LLM）、引用条数、字数、版本号。**执行模型这一行请务必看**——它决定了这份报告的结论能信到什么程度。

---

## 6. 用 API（脚本、批处理、接自己系统）

全部接口见 `api/openapi.yaml`。最短路径：

```bash
TOKEN=local-dev-token
BASE=http://127.0.0.1:8080

# 1) 建任务
ID=$(curl -s -X POST "$BASE/api/v1/research" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"goal":"对比 pgvector 与 Qdrant 的召回率与写入吞吐","depth":"quick","enable_verification":true}' \
  | python -c "import sys,json;print(json.load(sys.stdin)['task']['id'])")

# 2) 批准（跳过这步任务永远不动）
curl -s -X POST "$BASE/api/v1/research/$ID/approve" -H "Authorization: Bearer $TOKEN"

# 3) 轮询状态
curl -s "$BASE/api/v1/research/$ID" -H "Authorization: Bearer $TOKEN" \
  | python -c "import sys,json;print(json.load(sys.stdin)['task']['status'])"

# 4) 取报告
curl -s "$BASE/api/v1/research/$ID/report.md" -H "Authorization: Bearer $TOKEN" -o report.md
```

订阅实时事件：

```bash
# SSE（会一直挂着推增量）
curl -N "$BASE/api/v1/research/$ID/events?token=$TOKEN"

# 一次性拿历史事件，便于脚本处理
curl -s "$BASE/api/v1/research/$ID/events?format=json" -H "Authorization: Bearer $TOKEN"
```

状态机是硬约束：非法跃迁返回 `409 invalid_transition`，不会出现「已取消的任务又被跑起来」这种状态。合法跃迁表见 `docs/DATA_MODEL.md`。

---

## 7. 常见问题

**401 Unauthorized**
Token 不对或没带。检查 `.env.researchos` 的 `RESEARCHOS_API_TOKEN` 与界面输入框是否一致；改过 `.env` 要重启服务。

**任务卡在 `awaiting_approval`**
正常，它在等你批准。这是设计行为，不是卡死。

**任务 `failed`**
先看失败节点的 `error`。最常见的两种：
- 检索全空（`无可用的正文资料`）——换个更具体的目标，或换语种关键词；中文目标召回是当前已知瓶颈
- `没有结论可生成报告`——资料够抓但都不构成证据，同上

**报告结论很少、偏营销句**
没配 `RESEARCHOS_LLM_API_KEY`（且 `RESEARCHOS_LLM_PROVIDER` 仍是 `none`）。此时抽取是规则驱动的（分句 + 质量闸门），能过滤垃圾但不能替代理解。要真实质量就配 Key，例如：

```bash
RESEARCHOS_LLM_PROVIDER=openai
RESEARCHOS_LLM_API_KEY=sk-...
RESEARCHOS_LLM_MODEL=gpt-4o-mini
# 接自建网关时再加 RESEARCHOS_LLM_BASE_URL
```

改完重启服务，`/api/v1/health` 的 `checks.llm.enabled` 应变为 `true`。

**导出按钮点了没反应**
看浏览器控制台。导出是带 Authorization 的 fetch，被浏览器插件拦截会静默失败，页面左上角会出现红色错误条。

**端口 8080 被占**
改 `RESEARCHOS_ADDR`（例如 `RESEARCHOS_ADDR=127.0.0.1:8081`）。或者先 `taskkill //F //IM researchos.exe` 清掉残留进程——异常退出时 Windows 上会留下孤儿进程继续占端口。

---

## 8. 停止与清理

```bash
taskkill //F //IM researchos.exe            # 停服务
.devtools/pg/bin/pg_ctl.exe -D .devtools/pgdata stop   # 停便携数据库
```

数据在 PostgreSQL 里，重启不丢。任务、来源、证据、报告、事件全量留存，可回看任意历史任务。

---

## 边界声明

这套系统当前的定位是**单用户、本地、可信环境**。没有多租户隔离，不要暴露到公网。安全边界的完整说明见 `docs/security-review.md`，已知限制见 README「已知限制」。
