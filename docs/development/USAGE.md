# 使用手册

面向使用者，按「打开 → 配好 → 建任务 → 批准 → 看过程 → 拿报告」的真实点击顺序写。
接口与字段的一手依据是 `api/openapi.yaml`；本机实测状态见 `docs/demo.md`。

从这一版开始，**日常运行配置不需要改源码、不需要编辑 `.env`、也不需要重新编译**——
模型、检索、并发、MCP、超时全在设置中心里改，保存即入库。环境变量退回到它该待的位置：
首次启动的初始值，以及数据库连接、监听地址这类部署级参数。

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

## 1. 打开界面，先认路

浏览器访问 <http://127.0.0.1:8080/>。前端是静态产物，由同一个 Go 服务托管，不需要另起 Node。

左侧主导航六个入口，加右下角一组设置：

| 入口 | 地址 | 干什么 |
|---|---|---|
| **总览** | `/` | 系统状态、任务统计、Agent 运行情况、最近任务 |
| **研究任务** | `/tasks` | 全部任务列表；点进 `/task?id=…` 看单个任务的执行过程 |
| **研究来源** | `/sources` | 跨任务查所有抓到的页面，可按关键词 / 站点 / 状态 / 任务过滤 |
| **证据中心** | `/evidence` | 结论到原文的引用关系 |
| **研究报告** | `/reports` | 跨任务查全部报告，进去阅读与导出 |
| **设置** | `/settings` | 五个配置页：模型 / 搜索 / MCP Hub / 执行 / 系统 |

页面左上角有个 **API Token** 输入框，默认值 `local-dev-token`，与 `.env.researchos` 里的 `RESEARCHOS_API_TOKEN` 一致，**不用手输**。换过 Token 的话在这里填一次，存在浏览器 localStorage 里，之后不再问。

> 说明：页面本身免鉴权（否则浏览器加载不了静态资源），`/api/` 下的每个请求都带 Bearer Token。
>
> **Token 不进 URL。** 浏览器 `EventSource` 不支持自定义请求头，早期实现只能退而求其次把长期 Token 塞进 `?token=`，代价是它会沉淀在浏览器历史、Referer 和反向代理的 access log 里。现在改成：前端先用 Bearer Token 换一张**一次性 SSE ticket**（`POST /api/v1/research/{id}/events/ticket`），再用 `?ticket=` 建流。ticket 默认 60 秒过期、只对签发它的那个任务有效、用一次即销毁。`?token=` 通道已经删除，不保留兼容。

---

## 2. 首次配置：设置中心

打开 `/settings` 会跳到模型页。五个分页对应五个地址，刷新后停在原地，也能直接收藏某个配置页。

每页底部都有一条状态条，写着三件事：**当前配置版本号**（每次保存 +1，同一进程内递增）、**主密钥是否就绪**、**有没有已保存但还没生效的项**。看到「有 N 项需重启」就去重启服务，别在页面上反复点保存。

### 2.1 模型设置 `/settings/model`

决定规划器与抽取器用不用大模型、用哪家。

| 字段 | 说明 |
|---|---|
| Provider | `none` / `openai` / `deepseek` / `ollama`。选 `none` 时全部降级为本地确定性流程 |
| API Key | 加密保存，读接口只回 `sk-l****7890` 这种掩码。**留空表示不修改已保存的值** |
| Base URL | 接自建网关、代理或 Ollama 时填完整地址（含 `/v1`）；留空用 Provider 默认 |
| 模型名 | 留空用 Provider 默认模型 |
| 请求超时 | 单次补全的超时，写 `90s`、`2m` 这种形式 |

填完先点 **测试连接**，它会用你**当前填在表单里、还没保存**的值真发一次最小补全请求，返回真实耗时与模型回复。这一步不会写任何配置。

三种失败会如实报出来，不会假装成功：

| 返回 | 含义 |
|---|---|
| `reason: provider_none` | Provider 还是 `none`，没东西可测 |
| `reason: missing_credentials` | 缺 API Key，发不出真实请求 |
| `reason: request_failed` | 请求真发了但失败，`message` 里是上游原文，例如 `LLM 返回 401: ... api key: ****7890 is invalid` |

看到 401 就去核对 Key，别怀疑网络。测试通过再点保存，保存后**立即生效**，不需要重启——下一个任务的规划就会用它。

### 2.2 搜索设置 `/settings/search`

| 字段 | 说明 |
|---|---|
| 启用的 Provider | 多选。`bing` / `github` / `wikipedia` / `hackernews` 免密钥；`tavily` / `serpapi` / `brave` 需要对应 Key |
| 每 Provider 结果数 | 单次检索每个引擎返回几条上限 |
| 检索超时 | 单个检索请求的超时 |
| 各 Provider 密钥 | 与 API Key 同样加密保存、只回掩码 |

缺密钥的 Provider 不会让任务整体失败——它会被跳过，并在检索 trace 里标注跳过原因，报告里体现为覆盖缺口。

点 **测试检索** 会用真实关键词打一次检索，逐个 Provider 返回条数、耗时、错误。这个测试用的是**已保存**的配置，不是表单里改了一半的值，所以顺序是「先保存、再测试」。

### 2.3 执行设置 `/settings/execution`

并发、超时、重试与审批策略。这一页混着两类配置，页面会给需要重启的项打标：

| 字段 | 取值 | 生效方式 |
|---|---|---|
| 最大并发 Agent | 1–8 | **即时生效** |
| 单节点超时 | `3m` 等 | **即时生效** |
| 任务超时 | `30m` 等 | **即时生效** |
| 新建任务需人工批准 | 开 / 关 | **即时生效** |
| 默认研究深度 | quick / standard / deep | **即时生效** |
| Worker 数量 | 1–16 | **需重启**——进程启动时按这个数建 goroutine，改了不重启不生效 |
| 节点最大重试次数 | 1–5 | **需重启**——Worker 启动时读取 |
| 遵守 robots.txt | 开 / 关 | **需重启**——Fetcher 启动时构造 |
| 抓取超时 | `25s` 等 | **需重启**——同上 |

保存时的返回会明确告诉你这次改的东西属于哪一类：

- `hot_applied: true` + 「已保存并即时生效。」——已经在用了；
- `hot_applied: false` + `requires_restart: ["execution.workers"]` + 「已保存。含需要重启的项，重启服务后生效。」——值已入库，但运行时还是旧的。

**关掉 robots.txt 就没有站点声明的约束了**，抓取会更"顺利"，代价是你得自己承担合规判断。默认开着。

### 2.4 MCP Hub `/settings/mcp`

登记外部 MCP Server，发现它能提供什么工具。这里有一条必须理解的规则：

> **登记 ≠ 批准 ≠ 运行。**
> 登记只是往库里写一行元数据；只有点了 **批准**，系统才会允许启动那个外部进程。

原因很直白：工具发现要真的把 `command` 当子进程拉起来跑。这是本系统唯一会执行外部命令的地方，所以做成两道闸门。

代理流程：

1. **登记**——填名字、可执行命令、参数（可选环境变量，只传你在这里显式写的，不继承服务进程的环境）；
2. 列表里会出现一条 `approved=false` 的条目，工具数为 0；
3. 此时点 **发现工具** 会被拒绝，返回 `409 not_approved`。这不是 bug；
4. 点 **批准**，条目变成 `approved=true`；
5. 再点 **发现工具**，系统会真实拉起进程，按 MCP 的 stdio JSON-RPC 走一遍 `initialize → notifications/initialized → tools/list`，把工具清单存回库，状态变 `ready`；
6. 之后列表里直接能看到工具名、描述、入参 schema，不用再跑一次。

发现失败的 Server 也会留痕：状态 `error`，`last_error` 里是原始错误。有 20 秒硬超时和 2MB 输出上限，卡死或刷屏的 Server 拖不住服务。

撤销批准会顺带停掉还在跑的进程，不留孤儿。删除是物理删除登记记录。

### 2.5 系统设置 `/settings/system`

**这一页全部只读。** 监听地址、数据库连接串、API Token、数据目录、前端目录、身份，都只能通过环境变量设置。

不是偷懒，是故意的：允许网页端改监听地址，一次误操作就能把服务改到起不来，而你连页面都打不开了——只能去命令行救。数据库连接同理，它坏掉时你还得靠它读配置，是个死循环。

连接串在这里按脱敏形式展示（`postgres://user:****@host/db`），Token 只显示「已配置」或「未配置」，一个字符都不回。

想知道更细的运行环境（Go 版本、CPU、goroutine 数、连接池、迁移版本、主密钥来源）看这一页顶部的三张卡片，数据来自 `GET /api/v1/system`。

### 2.6 配置到底从哪来：优先级与生效规则

三层，优先级从高到低：

```
Web 保存值  >  环境变量  >  内置默认
```

- **部署级（`deploy`）**：只认环境变量。界面上只读，硬改会被拒：`403 deploy_field_readonly`。
- **即时生效（`hot`）**：保存后立刻作用于新任务、新检索、新的模型调用。
- **需重启（`restart`）**：值入库立即持久化，但运行时仍用旧值，直到重启。

每个字段都带 `source` 标记，告诉界面这个值现在打哪儿来：

| source | 含义 | 界面上能做什么 |
|---|---|---|
| `web` | 你在页面上存过 | 可以「清除覆盖」回落到环境变量 |
| `env` | 来自环境变量 | 保存一次即变成 `web` |
| `default` | 内置默认 | 同上 |

**清除覆盖**（`DELETE /api/v1/settings/{section}/{key}`）删的是 Web 层那一行，删完自动回落到环境变量或默认值。这是"恢复出厂"的精确版本——只回滚你点的那一项，不牵连别的。

一个重要边界：环境变量在**进程启动时**读一次，改了 `.env` 不重启服务，设置中心里看到的还是旧值，因为它读的就是内存里那份。想让新的环境变量生效，重启。

### 2.7 密钥是怎么存的

- 算法 AES-256-GCM，随机 nonce，密文入库带 `enc:v1:` 前缀；
- 主密钥来源优先级：环境变量 `RESEARCHOS_SECRET_KEY` > `<DataDir>/secret.key`（权限 0600，不存在则首次启动自动生成）；
- 读取接口**只返回掩码**（前后各留 4 位，长度不足 12 位只回 `****`），保存接口的响应同样只回掩码；
- 前端把掩码原样回传时，后端判定为"用户没动这一项"，不覆盖已存的真值。所以「改模型名但不重填 Key」是安全的。

已验证的边界：这层加密防的是**数据库文件或备份泄露**。主机被攻陷、进程被调试时，密钥在内存里是明文，AES 救不了。

想轮换主密钥：把它从环境变量里换掉后重启。已有密文会解不开——当前版本不做重加密迁移，换密钥等于作废旧密钥加密的所有配置，需要重新填一遍 API Key。

---

## 3. 建一个研究任务

左侧「研究任务」页，右上角新建。表单三样东西：

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

## 4. 批准才会真跑（关键一步）

创建后任务处于 `awaiting_approval`——规划器已经把 DAG 拆好，但**一个网页都没抓**。这是有意设计的执行闸门：先看它打算怎么干，再决定要不要花检索配额。

进任务详情页，右上角 **批准并执行**。点下去状态转 `running`，Worker 才开始检索、抓取、抽取、验证、写报告。

不想跑了就在同一排按钮里点 **取消**；跑到一半想停点 **暂停**，之后点 **恢复** 从断点续跑（已完成的节点不会重跑）。

这个闸门由 `RESEARCHOS_REQUIRE_APPROVAL` 控制，本机是 `true`。设成 `false` 会让任务创建后直接进入 `running`——适合脚本批跑，不适合手工试用（你还没看清它要干什么就已经把检索配额花掉了）。要改的话，现在去设置中心 → 执行设置 → 「新建任务需人工批准」，保存即生效，不用动 `.env`。

---

## 5. 看执行过程

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

## 6. 拿报告

任务 `completed` 后在 **报告** Tab 看正文，或从三个按钮导出：

- **导出 .md**——纯 Markdown，引用编号与正文一致，适合再加工
- **导出 .html**——带样式的单文件，直接发人
- **PDF**——服务端真实排版（不是把 HTML 改名）：A4 版式、中文字体内嵌（不依赖本机装没装字体）、章节层级、表格边框且不溢出、来源 URL 可见、每页页脚「第 N 页 / 共 M 页」。请求示例：

  ```bash
  curl -H "Authorization: Bearer $TOKEN" \
    -o report.pdf http://127.0.0.1:8080/api/v1/research/<任务ID>/report.pdf
  ```

  字体可用环境变量 `RESEARCHOS_PDF_FONT` 换成其他 glyf 型 TTF（TTC/OTF 不支持）；
  同一份报告每次生成的 PDF 字节级相同（页脚不含时间戳），便于归档比对。

三者都由带 Token 的请求取回再落盘，Token 不出现在下载链接里。

报告抬头会写明四件事：执行模型（`本地确定性流程` 表示没用 LLM）、引用条数、字数、版本号。**执行模型这一行请务必看**——它决定了这份报告的结论能信到什么程度。

---

## 7. 跨任务回看：研究来源与研究报告

单个任务看细节，这两个页面看全局。

**研究来源 `/sources`**——把历史上所有任务抓到的页面摊平在一张表里。四个过滤条件可以叠：

| 参数 | 作用 |
|---|---|
| `q` | 标题 / URL / 摘要关键词 |
| `site` | 站点域名 |
| `status` | `ok` / `discovered` / `fetch_failed` / `robots_blocked` |
| `task_id` | 只看某个任务 |

每行带任务标题，点得回去。想知道「上次那份关于 pgvector 的报告都读了些什么」，从这里查最快。

**研究报告 `/reports`**——跨任务列全部报告，带版本号、字数、引用条数、生成模型。报告可以有多版（重跑会升 version），列表里按时间倒序，别拿错版本。

两个列表都吃 `limit` / `offset`，服务端 `limit` 上限 200，默认 50。

---

## 8. 用 API（脚本、批处理、接自己系统）

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

配置也能全程走 API，适合做部署初始化脚本：

```bash
# 读全部配置（敏感项只有掩码）
curl -s "$BASE/api/v1/settings" -H "Authorization: Bearer $TOKEN"

# 改模型（扁平对象，也接受 {"values": {...}}）
curl -s -X PUT "$BASE/api/v1/settings/model" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"model.provider":"deepseek","model.api_key":"sk-...","model.model":"deepseek-chat"}'

# 只改一项，Key 不重填（掩码原样回传 = 不修改）
curl -s -X PUT "$BASE/api/v1/settings/model" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"model.api_key":"sk-l****7890","model.model":"deepseek-reasoner"}'

# 清掉 Web 覆盖，回落到环境变量
curl -s -X DELETE "$BASE/api/v1/settings/model/model.model" -H "Authorization: Bearer $TOKEN"

# 试连，不写配置
curl -s -X POST "$BASE/api/v1/settings/llm/test" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{}'

# 系统状态与统计
curl -s "$BASE/api/v1/system" -H "Authorization: Bearer $TOKEN"
curl -s "$BASE/api/v1/stats"  -H "Authorization: Bearer $TOKEN"
```

保存接口的四种拒绝，都是 4xx 而不是 500，因为都是调用方的问题：

| 状态码 | `error.code` | 触发条件 |
|---|---|---|
| 403 | `deploy_field_readonly` | 试图通过界面/接口改部署级参数 |
| 422 | `unknown_setting` | 提交了不存在的配置键 |
| 422 | `invalid_value` | 值不合法，`message` 是中文可读原因（如「最大并发 Agent 必须在 1 到 8 之间」） |
| 400 | `empty_patch` | 请求体里一项都没有 |

整批校验后才落库：一组里只要有一项非法，整组都不写入，不会留下改了一半的配置。

订阅实时事件：

```bash
# 方式一：命令行/脚本，直接用 Bearer 头（推荐）
curl -N "$BASE/api/v1/research/$ID/events" -H "Authorization: Bearer $TOKEN"

# 方式二：浏览器 EventSource 这类无法设置头的场景，先换一次性 ticket
TK=$(curl -s -X POST "$BASE/api/v1/research/$ID/events/ticket" \
       -H "Authorization: Bearer $TOKEN" | python -c "import sys,json;print(json.load(sys.stdin)['ticket'])")
curl -N "$BASE/api/v1/research/$ID/events?ticket=$TK"

# 断点续接：两种都行
curl -N "$BASE/api/v1/research/$ID/events?after_sequence=50" -H "Authorization: Bearer $TOKEN"
curl -N "$BASE/api/v1/research/$ID/events" -H "Authorization: Bearer $TOKEN" -H "Last-Event-ID: 50"

# 一次性拿历史事件，便于脚本处理
curl -s "$BASE/api/v1/research/$ID/events?format=json" -H "Authorization: Bearer $TOKEN"
```

ticket 的四条硬规则（写代码时别踩）：

| 规则 | 违反后的返回 |
|---|---|
| 一次性，用过的不能重放 | `401 sse_ticket_invalid` |
| 默认 60 秒过期（`RESEARCHOS_SSE_TICKET_TTL`，上限 10 分钟） | `401 sse_ticket_expired` |
| 只能用于签发时绑定的那个任务 | `403 sse_ticket_scope` |
| 只能访问 `GET .../events`，不能当 API Token 调其他接口 | `401 unauthorized` |

所以：**每次建流都要重新签发**，不要缓存 ticket；断线重连也要换新的。

状态机是硬约束：非法跃迁返回 `409 invalid_transition`，不会出现「已取消的任务又被跑起来」这种状态。合法跃迁表见 `docs/DATA_MODEL.md`。

---

## 9. 常见问题

**401 Unauthorized**
Token 不对或没带。检查 `.env.researchos` 的 `RESEARCHOS_API_TOKEN` 与界面输入框是否一致；改过 `.env` 要重启服务。

**报 `sse_ticket_expired` / `sse_ticket_invalid`**
ticket 过期或用过了。ticket 默认只活 60 秒且一次性，前端每次建流（含断线重连）都会重新签发；自己写脚本时别缓存 ticket，用一次就换一张。要调长有效期改 `RESEARCHOS_SSE_TICKET_TTL`（上限 10 分钟）。

**报 `sse_ticket_scope`**
拿着 A 任务的 ticket 去订阅 B 任务的事件流。ticket 与任务绑定，不能跨任务复用——这是防止 ticket 泄露后横向读取其他任务。

**在设置里点了保存，界面显示了新值，但任务行为没变**
先看那条状态条有没有「需重启」提示。`execution.workers`、`node_max_attempts`、`respect_robots`、`fetch_timeout` 四项属于需重启类，值已入库但运行时仍用旧值。重启后 `/api/v1/system` 的 `settings.restart_needed` 会变成空数组，说明已全部生效。

**保存返回 `403 deploy_field_readonly`**
你在改部署级参数（监听地址、数据库、Token、目录）。这些只能改环境变量再重启，界面不提供入口是防误操作，不是权限 bug。

**设置中心显示的值和我刚改的 `.env` 不一样**
环境变量只在进程启动时读一次。改了 `.env` 必须重启，否则页面显示的是内存里那份旧值。

**主密钥相关：`secrets.available: false`**
主密钥没就绪。检查 `RESEARCHOS_SECRET_KEY` 是否是合法的 32 字节（base64/hex 均可），或 `<DataDir>` 是否可写（要在那里生成 `secret.key`）。此时保存密钥类配置会被拒——这是对的，宁可不存也不能明文落库。

**改了主密钥后所有 API Key 都失效**
预期行为。换主密钥等于换锁，旧密文解不开。重新在模型页/搜索页填一遍 Key 即可。

**MCP Server 点「发现工具」报 409 `not_approved`**
还没批准。登记与批准是两步，批准才允许启动外部进程。点批准再试。

**MCP 发现失败，`last_error` 里有内容**
常见原因：`command` 不在 PATH、脚本路径不对、Server 启动后不说话（20 秒超时）、输出超过 2MB。手动在终端跑一遍同样的命令最容易定位。

**任务卡在 `awaiting_approval`**
正常，它在等你批准。这是设计行为，不是卡死。

**任务 `failed`**
先看失败节点的 `error`。最常见的两种：
- 检索全空（`无可用的正文资料`）——换个更具体的目标，或换语种关键词；中文目标召回是当前已知瓶颈
- `没有结论可生成报告`——资料够抓但都不构成证据，同上

**报告结论很少、偏营销句**
模型还在 `none`。去 设置中心 → 模型设置，选 Provider、填 Key、点测试连接、保存。此后的任务 `planner_mode` 会从 `heuristic` 变 `llm`。

也可以走环境变量（首次启动场景）：

```bash
RESEARCHOS_LLM_PROVIDER=openai
RESEARCHOS_LLM_API_KEY=sk-...
RESEARCHOS_LLM_MODEL=gpt-4o-mini
# 接自建网关时再加 RESEARCHOS_LLM_BASE_URL
```

改完重启，`/api/v1/health` 的 `checks.llm.enabled` 应变为 `true`。两种方式效果一致，界面保存的值优先级更高。

开启后行为有三处可见变化：

1. 任务详情的 `planner_mode` 从 `heuristic` 变成 `llm`，研究问题由模型拆解；
2. search 节点的检索式改为取模型给出的 `query`（原来固定用「目标 + 维度词」模板）；
3. 每个 analyze 节点的输出里 `llm_used=true`，并且事件流会出现 `llm_claim_extraction`，
   记录 `kept` 与 `dropped_ungrounded`——**后者是模型编造引文被丢弃的条数**，不是 0 才正常。

> 边界声明：仓库里的 `scripts/mock-llm.py` 能让这条链路跑通，但它是测试替身、不做推理。
> 「链路通」不等于「报告好」，真实质量必须用真实 Key 验收。

**导出按钮点了没反应**
看浏览器控制台。导出是带 Authorization 的 fetch，被浏览器插件拦截会静默失败，页面左上角会出现红色错误条。

**端口 8080 被占**
改 `RESEARCHOS_ADDR`（例如 `RESEARCHOS_ADDR=127.0.0.1:8081`）。或者先 `taskkill //F //IM researchos.exe` 清掉残留进程——异常退出时 Windows 上会留下孤儿进程继续占端口。这个值界面改不了，属部署级参数。

---

## 10. 验收与自检脚本

仓库里五个脚本，各有明确用途，不是摆设：

```bash
# 报告可追溯性：逐条核对「结论 → 引文 → 来源原文」，并检查引用编号闭合、
# 冲突披露是否与证据面板一致。退出码 0 才算通过。
python scripts/verify-citations.py --task <task-id>

# 契约自检：拒绝重复键、校验所有 $ref 不悬空、路径参数都已声明。
# 改过 api/openapi.yaml 就跑一次，比等到生成客户端时才炸要便宜。
python scripts/check-openapi.py api/openapi.yaml

# 并发压测：N 个任务同时跑，输出成功率 / P50 / P95 / 节点数 / 事件数
bash scripts/load-test.sh 5 quick

# 故障注入：强杀 Worker → 等租约过期 → 新 Worker 接管，
# 校验「停机前已完成的节点不被重跑」。（需要独占 8080，先停常驻服务）
bash scripts/fault-injection.sh

# 本地 OpenAI 兼容测试替身（不是模型），用于验证 LLM 客户端链路
python scripts/mock-llm.py --port 11434
```

`verify-citations.py` 的输出可以直接当验收记录用，例如：

```
-- 引文与来源 --
  证据总条数=18 逐字落地=18 悬空来源=0 空引文=0
  [PASS] 引文全部逐字落地  18/18
-- 报告引用编号 --
  [PASS] 引用编号闭合  18 个编号一一对应
-- 交叉验证与冲突 --
  结论状态分布：{'partially_supported': 18}
  [PASS] 冲突披露  无冲突结论，报告声明一致
-- 结论落地 --
  [PASS] 核心发现全部可回溯  10/10
```

`check-openapi.py` 通过时输出一行摘要：

```
[OK] api/openapi.yaml: 37 条路径 / 40 个操作 / 30 个 schema / 107 处引用，无重复键、无悬空引用
```

它会拦下两类"YAML 能解析所以看起来没事"的问题：重复的键（解析器静默保留最后一个，于是"我明明加了这个路径"和"线上就是 404"可以同时成立），以及指向不存在组件的 `$ref`（读契约时完全正常，生成客户端时才炸）。

关于 `scripts/mock-llm.py` 的定位必须说清楚：它不做任何推理，只按提示词类型返回预置 JSON，并且**故意**在每次抽取里混一条编造引文和一条不存在的 `source_id`。它的作用是证明「引擎没有盲信模型输出」——正常跑完 `dropped_ungrounded` 应该 ≥2。用它产出的报告**不能**当作模型质量证据。

### 让校验脚本自己证明会失败

一个永远打 PASS 的校验脚本等于没有校验。`verify-citations.py` 支持用 `--md-file` 把报告换成改坏的本地副本，用来确认对应检查项真的会 FAIL：

```bash
# 把「争议与不确定性」章节内容替换成「未检测到冲突」，再让脚本判定
python scripts/verify-citations.py --task <task-id> \
    --md-file /tmp/report_tampered.md
```

预期输出 `[FAIL] 冲突披露  有 N 条冲突结论，但报告写「未检测到冲突」`，退出码 1。改坏的副本可以自己在报告导出物上编辑，没人替你兜底——这条路径专门用来防「检查项写错键名、恒真通过」这类问题。

---

## 11. 停止与清理

```bash
taskkill //F //IM researchos.exe            # 停服务
.devtools/pg/bin/pg_ctl.exe -D .devtools/pgdata stop   # 停便携数据库
```

数据在 PostgreSQL 里，重启不丢。任务、来源、证据、报告、事件全量留存，可回看任意历史任务。

设置也一样：Web 保存的配置在 `app_settings` 表里，重启后仍然有效；密钥以密文形式存放，主密钥在 `<DataDir>/secret.key`。备份数据库时**别把 `secret.key` 一起备份到不安全的地方**——它加上数据库等于明文。

想彻底恢复出厂：清空 `app_settings` 表，删掉 `secret.key`（下次启动会重新生成），然后重启，全部配置回落到环境变量与内置默认。

---

## 边界声明

这套系统当前的定位是**单用户、本地、可信环境**。没有多租户隔离，不要暴露到公网。安全边界的完整说明见 `docs/security-review.md`，已知限制见 README「已知限制」。

设置中心没有改变这个定位。它防的是误操作与"改配置要改源码"的麻烦，不是防有主机权限的攻击者：能读 `secret.key` 的人就能解开所有密文配置。
