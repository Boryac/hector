# 仓库整合方案与决策（REPOSITORY CONSOLIDATION）

> 2026-11-04。本文记录双仓合一的仓库选择理由、迁移方式与被放弃的备选方案。

## 1. 背景

同一产品此前由两个独立 Git 仓库承载：

- **A（upstream）**：`verikod/hector`，PUBLIC，默认分支 main，17 个历史标签（v1.21.0–v1.41.2），GitHub Releases/Issues 存续，ResearchOS 产品链路在 `feature/hector-researchos` 分支开发（HEAD `b3c6ac3`）。
- **B（开发包）**：本地仓库（无远程），main @ `2d5d4d9`，15 个提交，承载 ResearchOS 的产品文档、审计报告、验收脚本、账本（TASKS/PROGRESS/DECISIONS）与少量配置文件（api/openapi.yaml、deployments/compose.yaml 等）。其根目录另有大量**未跟踪**的应用源码副本（历史遗留快照），从未入库。

## 2. 最终仓库选择：A（verikod/hector）

| 判断维度 | A（upstream） | B（开发包） |
|---|---|---|
| 项目身份 | GitHub 上已有 identity、star/issue/release 历史 | 纯本地 |
| 历史 | 33 commits + 17 tags | 15 commits |
| 可见性 | PUBLIC（沿用，不改动） | 私有内容（仅本地） |
| 远程推送能力 | 有 origin，权限具备（gh 已认证） | 无 remote |
| 维护便利 | 单一远程，CI 可挂 | 需新建远程 |

**决定**：以 A 为唯一正式仓库。B 的跟踪内容经一次"纯移动重构提交"后并入。

## 3. 公开性风险审查（重要）

B 将并入 PUBLIC 仓库，逐项核查其全部 85 个跟踪文件：

- 凭据扫描：提交历史内容级 `sk-…` 模式 **0 命中**；`.env`/`.env.researchos` 从未入库（.gitignore 双侧均覆盖）；`rc1_evidence/*.json` 证据文件在 RC1 轮提交前已做 `ofm-`/`sk-` 泄漏扫描，无泄漏。
- 内部信息：审计报告包含本机端口（8080/55432/18326）与本地中继的**存在性**描述，不含密钥与不可公开的凭据；属可接受披露范围。
- 残留判断：B 的文档明示"Fork 自 verikod/hector"，与 A 公开仓库合并无版权/出处冲突。

**结论**：可并入。远程推送属于用户授权事项，本轮不执行（见最终报告 H 节）。

## 4. 迁移方式与被放弃方案

**采用**：`git merge --allow-unrelated-histories`（B 侧先做一次纯移动重构提交 `c867229` 归位目录）。

理由：
- 两仓文件路径几乎不重叠（B 仅 85 个跟踪文件，多为 A 没有的 docs/scripts/api 配置），冲突仅 3 个文件（README.md / Makefile / .gitignore），人工解决成本最低且完全可控。
- 双方历史均完整保留为 HEAD 祖先，`git log --follow` 可穿越重构提交追溯。

**放弃的备选**：
- `git subtree add --prefix=dev-pack/`：会把 B 的文档埋进子目录，违背"以核心应用布局为基础、docs 归位到语义目录"的目标，且后续同步 subtree 的复杂度无必要。
- `git filter-repo` 重写历史加前缀：会改变 B 全部提交哈希，破坏"尽可能保留提交历史"的可追溯性要求；且 A 侧路径本就无需调整。
- 只复制文件重建历史：直接违反任务要求（不允许丢历史），否决。

## 5. 目录归位映射（B → 最终仓库）

| B 原路径 | 最终路径 |
|---|---|
| README.md / CHANGELOG.md / TASKS.md / PROGRESS.md / DECISIONS.md / SECURITY.md / Makefile / .gitignore | 根目录（同名合并/择优） |
| ARCHITECTURE / API_CONTRACT / EVENT_PROTOCOL / DATA_MODEL / docs/INTEGRATION_MATRIX | `docs/architecture/` |
| USAGE / DEVELOPMENT_PLAN / AGENTS / CODEX / CONTRIBUTING / TESTING_AND_ACCEPTANCE / DEMO_SCRIPT / DEPLOYMENT / docs/demo* | `docs/development/` |
| FINAL_AUDIT / RC1_*.md(6) / SECURITY_REVIEW / docs/security-review / docs/hector-audit / docs/dependency-scan / docs/final-report | `docs/audit/` |
| PRD / VISUAL_DESIGN / docs/design-system / docs/mapping-table / docs/benchmark / docs/DATA_MODEL(详版) / docs/screenshots | `docs/product/` |
| scripts/**、api/openapi.yaml、cmd/{api,worker}/main.go、deployments/**、docker-compose.yaml、prompts/** | **路径不变**（A 侧无同名冲突） |

A 侧原 610 行平台 README 保留为 `docs/development/HECTOR_PLATFORM.md`（平台能力文档仍有参考价值）；B 侧 ResearchOS 产品 README 成为根 README。

## 6. 冲突解决记录

| 文件 | 解决方式 |
|---|---|
| .gitignore | 并集：A 的完整规则 + B 的 `upstream/`（废弃，删除）、`.workbuddy/`、`.devtools/`、`node_modules/` |
| Makefile | A 的全量 Makefile + 追加 ResearchOS 段（`researchos-verify/build/up/down`） |
| README.md | 取 B（ResearchOS 产品 README，含当前状态表）；A 版存档至 docs/development/HECTOR_PLATFORM.md |

## 7. 最终 Git 形态

- 单一仓库、单一主分支 **main**（快进自 feature/hector-researchos，A 原 main 是其祖先，无历史丢失）。
- 保留 A 全部 17 个历史标签；实验分支/引用不入正式分支集（对象仍在历史中可达）。
- 整合提交：`7d8c432`（merge commit，两个历史来源）。
- 迁移验证详见 `docs/audit/GIT_CONSOLIDATION_REPORT.md`。

## 8. 备份

- `C:\Users\22334\Desktop\hector-researchos-backup-20261104\dev-pack-full.bundle`（2.4MB，HEAD 2d5d4d9，15 commits，克隆测试通过）
- `C:\Users\22334\Desktop\hector-researchos-backup-20261104\upstream-hector-full.bundle`（含全部分支/标签，HEAD b3c6ac3，33 commits + 17 tags，克隆测试通过）
- `BACKUP_METADATA.txt`：两仓 show-ref 与提交清单快照。
- 备份位于两个原始仓库目录之外，迁移前创建并验证。
