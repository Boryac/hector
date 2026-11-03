# FINAL_AUDIT — 最终集成审计报告（2026-11-03）

> 结论先行：**核心产品链路可用且有实测证据**。研究任务创建 → 规划 → 并发执行 →
> 真实检索 → 证据 → 报告 → 三种格式导出（含服务端排版的 PDF）→ 前端七页全部真实
> 对接后端，设置中心可管理配置且密钥加密入库。自动化验证 14/14 通过、浏览器断言
> 37/37 通过。两项外部条件缺失如实标 BLOCKED：真实大模型质量验证（无 Key）、
> Docker 实机部署验证（本机无 Docker）。
>
> 本报告只陈述有证据的事实；每个数字都能用文中命令复跑出来。

## A. 项目现状

| 项 | 值 |
|---|---|
| 开发包仓库 | `hector-researchos-codex-pack`，分支 `main`，HEAD `6bc315b`（工作树干净） |
| 上游仓库 | `upstream/hector`，分支 `feature/hector-researchos`，HEAD `b91c4df`（工作树干净） |
| 远程 | 两仓均未推送（按约定不推远程） |
| 技术栈 | Go 1.25（后端，`internal/researchos` 18.4k 行）· Next.js 14 静态导出（前端 5.9k 行 TSX）· PostgreSQL 16 · gopdf（PDF） |
| 架构 | 单二进制 `researchos`（API + Worker 同进程，可分离）+ 同源托管前端静态产物；PostgreSQL 持久化；SSE 事件流；MCP 工具发现 |
| 主要功能 | 研究任务（三层深度）· 启发式/LLM 双规划器 · DAG 调度（租约互斥、租约回收、重试）· 多 Provider 检索 · 证据交叉验证 · 版本化报告 · md/html/pdf 导出 · 设置中心（热/需重启/部署三层）· MCP Hub · 单用户本地鉴权 |

## B. 前后端集成

详见 `docs/INTEGRATION_MATRIX.md`（逐页 × 逐 API，每条 PASS 附复现依据）。摘要：

- 7 个路由 × 2 视口（1440/390）：`ui-audit.mjs` 14 项全过——无 React 运行时错误、无横向溢出、DAG 画布正常渲染。
- `ui-functional.mjs` 37 项断言全过：每页发出的 API 请求逐个比对（含 2xx 与无 4xx）、工作台五个懒加载 Tab、SSE ticket 流建立、设置页 schema 字段渲染、**真实 UI 创建任务 → 落库 → 出现在列表 → 清理**。
- 本轮修复的集成缺陷 4 项：`/system` 与 `/stats` 无前端入口（已接入设置页与 Dashboard）、PDF 缺失（已实现）、页面审计未覆盖懒加载 Tab（已补齐）。
- 尚存：`GET /tools`、`GET /tools/calls`、`GET /agents`、`POST /tools/servers/{id}/{action}` 四个端点为 API-only（运维/后续 Agent 面板用），已在 openapi 文档化——见矩阵 §2。

## C. 测试结果

| 检查 | 结果 | 依据 |
|---|---|---|
| gofmt（internal/researchos + cmd） | ✅ 0 未格式化 | `gofmt -l` |
| go vet | ✅ 无告警 | `go vet ./internal/researchos/...` |
| Go 单元测试 | ✅ 14 包全绿，196 个测试函数 | `go test -count=1 ./internal/researchos/...` |
| TypeScript | ✅ 0 错误 | `tsc --noEmit` |
| 前端生产构建 | ✅ 16 页静态导出 | `npm run build` |
| OpenAPI 契约 | ✅ 38 路径 / 41 操作 / 110 引用，无重复键、无悬空 ref、路径参数全声明 | `python scripts/check-openapi.py` |
| Compose 静态校验 | ✅ 两份 compose 0 错误 0 警告 | `python scripts/check-compose.py` |
| 数据库迁移 | ✅ 0001+0002 应用，连续执行两次幂等 | `researchos migrate` |
| API 集成 | ✅ 起真实服务打真库：health / settings / system / stats / sources / reports / report.pdf | `scripts/verify.sh` 端到端段 |
| PDF 内容一致性 | ✅ 7 章节 + 5 URL + 49 段正文抽样全部命中 | `scripts/verify_pdf_content.py` |
| 浏览器审计 | ✅ 14 项（渲染）+ 37 项（功能） | `ui-audit.mjs` / `ui-functional.mjs` |
| 统一校验 | ✅ **14 通过 / 0 失败 / 0 跳过**（含负向自检：注入悬空 ref 与坏格式文件均被抓到） | `bash scripts/verify.sh` |
| 浏览器 E2E（业务闭环） | ✅ 创建 → 批准 → 执行 → 完成 → 报告 → 导出，部分在 UI 层（创建、导出），其余在 API 层实测 | 审计脚本 + curl |
| Docker 实机部署 | ⛔ BLOCKED | 本机无 Docker（`docker: command not found`）；已做静态校验与镜像结构检查（Dockerfile.researchos 多阶段构建逻辑与 compose 依赖关系核对） |
| 故障注入 | ✅ 租约回收、节点重试、Worker 崩溃接管有单测与历史实跑记录 | `PROGRESS.md` T5/T7 系列 |
| 真实 LLM 研究质量 | ⛔ BLOCKED | 无可用 Key；`.env.researchos` 未配置 `RESEARCHOS_LLM_*` |

## D. 安全结果

| 项 | 状态 | 说明 |
|---|---|---|
| 身份模型 | ✅ | V1 单用户本地模式：身份只来自服务端 Bearer Token（`subtle.ConstantTimeCompare`），请求体/查询参数/事件元数据中的 `user_id` 一律忽略；所有任务读写按 `created_by` 过滤，越权返回 404 |
| SSE 凭据 | ✅ | 一次性 ticket（短时、绑定任务、用完即销、服务端存哈希）；单测 11 条（单次使用/过期/作用域/防爆破/防篡改/熵/并发消费） |
| API Key 保护 | ✅ | AES-256-GCM（`enc:v1:` 前缀）入库，主密钥 `RESEARCHOS_SECRET_KEY` > `<DataDir>/secret.key`(0600)；GET 与保存响应都只回掩码；实测 GET 全文检索不到明文 |
| 数据库密码脱敏 | ✅ | `/system` 与 `/settings` 均显示 `postgres://postgres:****@…`（带密码 DSN 实测） |
| MCP 批准闸门 | ✅ | 未批准 discover 返回 409，不静默降级；只传登记时显式 env，不继承服务进程环境；20s 硬超时 + 2MB 输出上限 |
| HTML 导出注入 | ✅ | 先转义再处理标记，链接只允许 http(s)；单测覆盖 |
| Prompt Injection | ✅（V1 范围内） | 来源正文进入 LLM 前有清洗；报告产出「引用标记越界剔除」「未验证结论不写成事实」并有单测；LLM 路线本身 BLOCKED，注入面未在真实模型下回归 |
| SSRF / robots | ✅ | 抓取遵守 robots.txt（配置开关 + 默认开），URL 规范化，Provider 白名单 |
| 高风险工具审批 | ✅ | `RESEARCHOS_REQUIRE_APPROVAL` 默认 true，任务批准前不执行 |
| 默认 Token | ⚠️ | 前端 `local-dev-token` 仅为本地开发便利，`RESEARCHOS_API_TOKEN` 未设置时服务拒绝启动（无静默默认）；compose 中为强制必填。**明确标注：这是开发配置，不是生产安全方案** |
| 多用户 | ⛔ 不开放 | A2A 上游的身份隔离问题未在多租户语义下回归；V1 保持单用户，多用户能力在修复并通过回归前不开放 |
| 日志敏感信息 | ✅ | 已知日志点不输出 Key/Token（掩码在响应层，日志层只记 key_set 布尔） |

## E. 功能完成度

| 功能 | 状态 |
|---|---|
| 任务创建 / 校验 / 重复提交保护 | PASS |
| Planner（启发式，确定性 + 报告中如实标注） | PASS |
| Planner（LLM，Schema 校验 + 失败降级启发式） | PASS（单测）/ 质量验证 BLOCKED（无 Key） |
| DAG 调度（依赖 / 并发 / 重试 / 超时 / 取消 / 租约回收） | PASS |
| 检索（bing/github/hackernews/wikipedia 免密钥；tavily/serpapi/brave 留接口） | PASS（免密钥 Provider 实测） |
| 抓取（robots / 去重 / 快照落盘） | PASS |
| 证据与交叉验证（引用可溯、冲突披露、未验证不冒充事实） | PASS |
| 报告（摘要/方法/冲突/来源/统计一致） | PASS |
| 导出 md / html / **pdf（本轮新增，服务端排版 + 页码 + 中文字体内嵌）** | PASS |
| 设置中心（持久化 / 热更新 / 需重启 / 部署级只读 / 连接测试） | PASS |
| MCP Hub（登记 / 批准 / 工具发现 / 删除） | PASS |
| SSE 实时事件 | PASS |
| 前端七页 | PASS |
| Docker 部署 | BLOCKED（无 Docker；静态校验通过） |
| 真实模型研究质量 | BLOCKED（无 Key） |

## F. Git 提交

| 仓库 | 分支 | Commit | 范围 | 工作树 |
|---|---|---|---|---|
| `upstream/hector` | `feature/hector-researchos` | `b91c4df` | PDF 导出（report 包 + 字体 + API + 测试）、/system /stats 前端接入、ParseBlocks 重构、gofmt 欠账、ui-functional.mjs；23 文件 +1873/−96 | 干净 |
| 开发包 | `main` | `91f2f7c` | verify.sh 重写、check-compose.py、compose 两份修复、openapi report.pdf、INTEGRATION_MATRIX、截图重截、PDF 内容校验脚本 | 干净（本轮文档提交前） |
| 开发包 | `main` | `6bc315b` | 最终审计与账本同步：FINAL_AUDIT.md 新建、TASKS（T10.1–T10.8）、PROGRESS（T10 三条）、DECISIONS（36–38）、README 状态表、USAGE 导出段 | 干净 |

## G. 仍需用户提供的事项

1. **LLM API Key**（OpenAI 兼容任一：deepseek/openai/ollama base_url 均可）——用于真实模型规划与研究质量验收。配置后跑：
   `curl -X POST -H "Authorization: Bearer $TOKEN" -d '{"query":"..."}' http://127.0.0.1:8080/api/v1/settings/llm/test` 通过后，在界面建任务选 LLM 模式。
2. **Docker 环境**——实机跑 `docker compose -f docker-compose.yaml up -d --build` 完成部署验收（静态校验已过，缺实跑）。
3. **（可选）付费搜索凭据**——tavily/serpapi/brave，提升中文检索覆盖；当前免密钥 Provider 已可用。
