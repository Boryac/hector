# Hector ResearchOS — Codex 开发包

> 版本 V1.0 · 2026-11-03 · 目标：12 周交付可运行、可观察、可验证的自主研究平台
> 基础项目：Fork 自 https://github.com/verikod/hector ，二次开发，不重写底层框架
> 核心技术：Go / Multi-Agent / MCP / A2A / DAG / RAG / PostgreSQL + pgvector / Next.js + React Flow / SSE

## 这个包是给谁的

给 Codex（或任何接手开发的 Agent）的一份**可直接开工、自迭代到完成**的总纲。
它不是演示稿，也不是静态页面集合。Codex 拿到这个包后，不需要再向用户追问宏观方向，
只需按顺序执行任务、跑通测试、填好进度，最后交付可部署系统。

人类用户只需做三件事：
1. Fork `verikod/hector`，把本包内容复制进仓库根目录（不要覆盖原有源码，先做映射）。
2. 把本包交给 Codex，并说一句话：`按 CODEX.md 自迭代，完成 TASKS.md 全部任务，直到 TESTING_AND_ACCEPTANCE.md 全过。`
3. 在 Phase 0 安全审查、Phase 4 报告导出、高风险工具审批三个节点做人工确认。

## 包内文件地图

| 文件 | 用途，Codex 必读顺序 |
|---|---|
| `CODEX.md` | 自迭代总指令：循环规则、硬禁令、提交与进度规范。先读。 |
| `PRD.md` | 产品需求文档：定位、用户、功能模块、生命周期、V1 范围、验收原则。 |
| `ARCHITECTURE.md` | 总体架构、模块边界、技术选型、目录结构目标。 |
| `DATA_MODEL.md` | Research / Workflow / Node / Agent / Source / Evidence / Report 的数据模型与关系。 |
| `EVENT_PROTOCOL.md` | 前后端统一事件协议、幂等与重放、SSE 断线重连。 |
| `API_CONTRACT.md` + `api/openapi.yaml` | REST + SSE 接口契约，前后端唯一依据。 |
| `SECURITY_REVIEW.md` | Phase 0 必做：A2A Issue #2 身份隔离、MCP 越权、Prompt Injection、API Key、路径穿越等检查表。 |
| `DEVELOPMENT_PLAN.md` | 12 周里程碑 Phase 0–5，每周可验证交付物。 |
| `TASKS.md` | 可执行任务清单 T0.1–T5.x，每项含验收标准与依赖。Codex 按此打勾推进。 |
| `TESTING_AND_ACCEPTANCE.md` | 单元/集成/E2E/故障注入/压力测试、性能目标、最终演示验收脚本。 |
| `DEPLOYMENT.md` + `deployments/` | Docker Compose 一键部署、环境变量、健康检查、备份。 |
| `VISUAL_DESIGN.md` | 视觉方向：暖白 + 浅蓝、信息密度高，禁止深色科幻装饰堆砌。 |
| `DEMO_SCRIPT.md` | 2–3 分钟演示视频分镜与最终演示任务原文。 |
| `prompts/PHASE_PROMPTS.md` | 每个 Phase 可直接粘贴给 Codex 的提示词。 |
| `scripts/` | `bootstrap.sh` / `verify.sh` / `codex-loop.sh` / `seed-demo.sh` |
| `PROGRESS.md` | Codex 每次迭代必须更新的进度账本（模板已给）。 |
| `DECISIONS.md` | 已锁定的工程决策，Codex 不得擅自推翻，改动需记录理由并经人工确认。 |

## 快速开始（给 Codex）

```bash
# 1. Fork 并克隆原项目后，把本包解压到仓库根目录
cp -r hector-researchos-codex-pack/* hector/  # 注意：先做 Phase 0 映射，不要直接覆盖同名源码文件
cd hector
bash scripts/bootstrap.sh   # 检查 Go / Node / Docker / Postgres，生成 .env.example
# 2. 先做 Phase 0：只做审计与映射，不写大功能
#    产出 docs/hector-audit.md + docs/mapping-table.md，人工确认后进入 Phase 1
# 3. 自迭代
bash scripts/codex-loop.sh  # 提示词循环：读 TASKS.md -> 做下一个未完成任务 -> 测试 -> 更新 PROGRESS.md
bash scripts/verify.sh      # 任一阶段结束都可跑：构建 + 单元测试 + 契约检查
```

## 已锁定的三条原则（不可妥协）

1. **不重写 Hector 底层。** 先审查、复用、补齐，再围绕研究产品扩展。目录大搬迁必须先有映射表。
2. **前端体验是主要竞争力，但状态必须真实。** 工作流画布、证据链、报告阅读器是关键；禁止用纯前端模拟执行状态冒充正式功能。
3. **真实工程深度可验证。** Go 调度器、并发、持久化、断点恢复、MCP、可观测性，都要能通过代码和测试验证，不是口号。

## V1.0 范围速览

必须交付：研究任务创建、自然语言规划、多 Agent 执行、DAG 可视化、多源搜索、网页解析、
资料管理、证据引用、报告生成、实时状态、暂停/恢复、MCP 管理、Docker 部署。

暂不纳入：多租户 SaaS 商业化、复杂团队协作、计费系统、移动端原生应用、大规模分布式集群、
完全自主执行高风险操作。

## 最终交付物清单（仓库根目录必须齐全）

README.md / PRD.md / ARCHITECTURE.md / CONTRIBUTING.md / SECURITY.md / API 文档 /
Docker Compose / Demo 视频脚本与成片链接 / Benchmark 报告 / Architecture Diagram /
可运行系统 + 完整源码 + 部署文档 + 演示案例（真实研究任务，见 DEMO_SCRIPT.md）

