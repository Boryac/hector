# RC1_BASELINE — RC1 验证基线（2026-11-04 00:48 实测）

> 结论先行：**两仓干净、服务健康、PostgreSQL 在线、前端构建产物就绪；本地模型服务在
> 127.0.0.1:18326 运行（进程 DeepSeek Harness，dsh-our-free-model v2.7.2 中继）。
> LLM 配置此前未接入（provider=none），本轮将接入并验证。Docker 仍不可用。**

所有数字均为本轮重新实测，非沿用历史报告。

## 1. Git 状态（实测）

| 仓库 | 分支 | HEAD | 工作树 | 未跟踪文件 |
|---|---|---|---|---|
| 开发包 `hector-researchos-codex-pack` | `main` | `6f20eca` | 干净 | 无 |
| 上游 `upstream/hector` | `feature/hector-researchos` | `b91c4df` | 干净 | 无 |

两仓均未推远程（符合约定）。与历史记录一致：upstream `b91c4df`、开发包 `6f20eca`。

## 2. 运行时状态（实测）

| 项 | 状态 | 证据 |
|---|---|---|
| ResearchOS 服务 | ✅ 运行中 | `127.0.0.1:8080` 进程 `researchos`，前端 `/` 返回 200 |
| 服务版本/启动时间 | 1.0.0-dev，2026-11-03T16:10:03Z | `GET /api/v1/system`（带 Token 实测） |
| PostgreSQL | ✅ 监听中 | `127.0.0.1:55432` 进程 `postgres` PID 26404 |
| 鉴权 | ✅ 生效 | 无 Token 请求受保护端点返回 401 |
| Docker | ⛔ 不可用 | `docker: command not found`（PATH 与系统均无） |
| 本地模型服务 | ✅ 运行中 | `127.0.0.1:18326` 进程 `DeepSeek Harness`（PID 30604） |

## 3. 网络拓扑确认（阶段前置要求）

- ResearchOS API 与 Worker 为**同一进程**（单二进制同进程模式），运行在**宿主机**。
- 模型 API 也在**宿主机** `127.0.0.1:18326`——ResearchOS 进程访问 `127.0.0.1` 即为用户本机，**无容器网络隔离问题**。
- Docker 路线（若后续启用）：容器内需将 Base URL 的 `127.0.0.1` 替换为 `host.docker.internal:18326`，且中继 `lan.enabled` 当前为 false，仅绑定本机回环——Docker 实机验证前必须先解决该暴露面（见部署报告）。

## 4. LLM 配置现状（实测）

- `.env.researchos`（本地开发环境文件，不提交）含 9 个变量：DATABASE_URL / API_TOKEN / LOCAL_USER / ADDR / SEARCH_PROVIDERS / WORKERS / REQUIRE_APPROVAL / DATA_DIR / WEB_DIR；**无任何 `RESEARCHOS_LLM_*`**。
- 运行中服务 `GET /api/v1/settings?section=model` 实测：`model.provider = none`（source=env，configured=false），API Key 未配置。
- 历史报告「真实 LLM 质量验证 BLOCKED（无 Key）」**在基线时点仍然成立**；本轮以本地中继 + relay key 解除该阻塞。
- Settings 模型段可配置字段（schema 实测）：`model.provider`(enum) / `model.api_key`(secret) / `model.base_url` / `model.model` / `model.timeout`，均为 hot 作用域（保存即生效，无需重启）。

## 5. 前端构建状态（实测）

- `web/` 源码在 upstream `b91c4df` 内；`npm run build` 产物为 16 页静态导出（上一轮 verify.sh 全量验证 14/14 时构建通过，本轮阶段 9 回归将重跑确认）。

## 6. 中继凭据来源（安全说明）

- relay key 存放于本机 `~/.dsh/our-free-model/settings.json` 的 `chanGateway.relay.key`（36 字符），属用户本地凭据。
- 使用方式：运行时读入进程环境变量，**不写入源码 / Git / 日志 / 文档 / 测试快照**；所有输出对该值脱敏。
- 中继鉴权实测：无 Key 与错误 Key 均返回 401 `missing or invalid relay key`。

## 7. 历史 BLOCKED 项目复核

| 历史 BLOCKED | 基线判定 |
|---|---|
| 真实 LLM 质量验证（无 Key） | **本轮解除**——本地 OpenAI 兼容中继可用 |
| Docker 实机部署（无 Docker） | **仍成立**——本机无 Docker 命令 |

## 8. RC1 阶段计划

阶段2 模型探测 → 阶段3 Provider 接入（设置中心 UI）→ 阶段4 三类真实研究任务 →
阶段5 质量评估 → 阶段6 PDF 压测 → 阶段7 Docker（预期 BLOCKED，保留静态校验）→
阶段8 安全复查 → 阶段9 最终回归 → 阶段10 发布准备。

## 9. RC1 阶段执行结果（2026-11-04 收口时补记）

| 阶段 | 结果 |
|---|---|
| 2 模型探测 | 16/18 直接过 + 2 项定性（`docs/RC1_MODEL_CAPABILITY_REPORT.md`） |
| 3 模型接入 | UI 链路 14/14；加密落库；重启持久化验证 |
| 4 研究任务 | A/B/C 真实完成，修复前/后双轮证据（`scripts/rc1_evidence/`） |
| 5 质量评估 | C 40/50、A 36.5/50、B 30.5/50；独立核查 8/8；0 编造 |
| 6 PDF 压测 | 5 用例全绿 |
| 7 Docker | BLOCKED（无 Docker），静态校验保留 |
| 8 安全复查 | 13/13（CORS 缺陷修复 + 回归） |
| 9 最终回归 | verify.sh 14/14；Playwright 浏览器审计 27/27 |
| 10 发布准备 | CHANGELOG 新增；账本同步；双仓分别提交 |

本轮修复的缺陷：P0 检索 UA 反爬降级、P1 CORS 任意 Origin 反射、P1 LLM 抽取超时静默降级（决策 39–41）。
基线时的两条 BLOCKED：LLM 质量 → 已解除；Docker → 仍成立。
