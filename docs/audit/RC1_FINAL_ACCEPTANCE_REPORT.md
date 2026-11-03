# RC1 最终验收报告（RC1 FINAL ACCEPTANCE REPORT）

> 2026-11-04 · Hector ResearchOS v1.0.0-rc.1 · 状态口径：PASS=本轮实测通过 / PARTIAL=部分通过有限制 / BLOCKED=环境受限未验证 / FAIL=实测失败
> 汇总：**PASS 24 · PARTIAL 4 · BLOCKED 3 · FAIL 0**

## A. 执行摘要

- 双仓整合：**完成**。最终仓库 = verikod/hector（本地工作副本 `C:\Users\22334\Desktop\hector-researchos`），开发包 16 commits 经重构提交 `c867229` 并入，merge `7d8c432`，两条历史均为 HEAD 祖先。
- RC1 冻结：**完成**（tag `v1.0.0-rc.1` 本地；不推送远程）。
- Windows ZIP：**已生成**（22.0 MB，含 SHA256/BUILD_INFO）。
- Wails 托盘：**已实现**（Wails 窗口 + 自研 win32 托盘，决策 42）。
- GitHub Actions：工作流已交付并本地校验；**远程实跑 BLOCKED**（无推送授权）。

## B. Git 整合（详见 docs/audit/GIT_CONSOLIDATION_REPORT.md）

| 验收项 | 状态 | 证据 |
|---|---|---|
| 双仓完整备份 | PASS | 桌面外 `hector-researchos-backup-20261104\{dev-pack,upstream-hector}-full.bundle`，克隆验证 HEAD/提交数/标签数一致 |
| 两仓库历史保留 | PASS | rev-list 47 = 上游主线 + 开发包 16 + 重构 1 + 合并 1；`merge-base --is-ancestor` 双向通过 |
| 单一 Git 根 | PASS | 无嵌套 .git |
| 单一主分支 | PASS | `main`（上游 main 为其祖先，快进无丢失） |
| 标签统一 | PASS | 保留上游 17 个历史标签 + 本地新增 `v1.0.0-rc.1` |
| 关键文件完整 | PASS | 16 项关键文件全存在（含 docs 四子目录） |
| 迁移后构建 | PASS | go build/test、tsc/next build、verify.sh 全部整合后重跑 |

## C. ResearchOS 核心（全部整合后重测，命令见 POST_MERGE_TEST_REPORT.md）

| 验收项 | 状态 | 证据 |
|---|---|---|
| 后端构建 / 单测 | PASS | go build；14 包全绿（含 shutdown 端点 2 用例） |
| 前端类型 / 构建 | PASS | tsc 0 错；next build 16 页 |
| 数据库迁移 | PASS | verify.sh 迁移幂等段 |
| 统一校验 verify.sh | PASS | **14/14**（含负向自检） |
| Agent Runtime / Worker / SSE | PASS | 浏览器审计 27/27（真实 UI 建任务、SSE 流） |
| LLM（真实模型） | PASS | 任务 A：cline/cline-free/solar-mini4，96.3s，14,392 tokens，8 结论 8 引用 |
| Search | PASS | 21 来源采集（bing/github 等）；免费源覆盖限制见 K3 |
| RAG / 检索 | PARTIAL | 链路可用；质量受免费搜索源限制 |
| PDF | PASS | verify.sh 内容比对段 + RC1 压测 5 用例（历史，未回归） |
| 安全审计 | PASS | 探测 13/13 + 新增面审查（SECURITY_AUDIT.md） |

## D. Windows

| 验收项 | 状态 | 证据 |
|---|---|---|
| Go + Wails 评估 | PASS | v2.17.1 无 SystemTray → 自研托盘（决策 42，含被弃方案） |
| 托盘程序 | PASS | 构建 + 三轮启动冒烟（修复 2 缺陷后 0 错误） |
| 后台服务分离 | PASS | 集成测试：托盘不绑 Job Object；关窗/退托盘不停服 |
| 单实例 | PASS | wails SingleInstanceLock |
| 启停管理 | PASS | ps1 链实跑（start→200→stop）+ shutdown 202 |
| 退出确认 | PARTIAL | 三选逻辑实现并有绑定；GUI 交互未自动化（WebView2 窗口人工验证留待用户） |
| 开机启动设置 | PARTIAL | HKCU Run 读写实现；UI 开关路径未自动化验证 |
| 首次数据目录选择 | PARTIAL | ValidateDataDir/向导逻辑经集成测试；GUI 目录选择未自动化 |
| PostgreSQL 配置 | PASS | 向导 migrate 检测 + 脚本链（外置 PG 16） |
| 诊断脚本 | PASS | diagnostics.ps1 实跑（输出脱敏验证） |
| 便携 ZIP | PASS | 见 §E |

## E. 发行产物

- **ZIP**：`C:\Users\22334\Desktop\Hector-ResearchOS-v1.0.0-rc.1-windows-x64.zip`（21,916,655 字节 ≈ 21.9 MB；打包脚本二次生成 22,976,655 字节 ≈ 21.9 MB，含最终托盘与文档）
- **SHA-256**：`77F90DE14E03E15DCA069D2032FE6A0E35575E3957E0BA2B76374352B507CD78`（打包脚本产物）
- **BUILD_INFO**：桌面 `Hector-ResearchOS-v1.0.0-rc.1-windows-x64.BUILD_INFO.txt`（commit/时间/go 版本/sha256）
- 结构：bin / web / config / database\migrations / assets\{fonts,licenses} / scripts(7×ps1) / docs / START_HERE / LICENSE / CHANGELOG
- Clean-room 验证：**PARTIAL** — 解压完整性自检、脚本链、生命周期在本机实测；"跨会话长期驻留"受沙箱回收限制无法观察（无沙箱的真实桌面无此问题）

## F. GitHub Actions

| 验收项 | 状态 | 证据 |
|---|---|---|
| 工作流文件 | PASS | `.github/workflows/release-windows.yml`，YAML 解析校验 |
| Tag 触发 / 最小权限 / 不自动 Release | PASS（配置级） | `permissions: contents: read`；仅上传 Artifact |
| Windows x64 构建步骤 | PARTIAL | 步骤与本地执行的命令一一对应；**远程实跑 BLOCKED**（推送需授权） |

## G. 文档

PASS：README（整合后状态表）、CHANGELOG、QUICK_START、CONFIGURATION、TROUBLESHOOTING、WINDOWS_DISTRIBUTION、DEVELOPER_GUIDE、RC1_RELEASE_NOTES、KNOWN_LIMITATIONS、ENVIRONMENT_AUDIT、GIT_CONSOLIDATION_REPORT、POST_MERGE_TEST_REPORT、RESEARCH_QUALITY_REPORT、SECURITY_AUDIT、REPOSITORY_CONSOLIDATION、TASKS/PROGRESS/DECISIONS。

## H. 已知限制（PARTIAL/BLOCKED 汇总）

1. **K1 任务 B 质量 30.5/50（历史基线，未重跑）** — 不达标如实披露。
2. **K6 Docker 实机 BLOCKED** — 构建机无 Docker；compose 静态校验通过。
3. **K13 Actions 远程实跑 BLOCKED** — 无推送授权；本地 YAML 校验通过。
4. **K2 LLM 复杂任务超时风险** — timeout 已调 4m，不宣称全稳定。
5. 托盘 GUI 三项（退出确认/自启动开关/目录选择）逻辑有测试、界面未自动化。
6. 后台进程跨工具会话持久性受沙箱限制（真实桌面无此问题）。
7. `research_runs` run_number 唯一约束偶发冲突（P3，无用户可见影响）。

## I. 结论（三者不混同）

- **RC1 本地交付条件：达成**。代码冻结于 tag `v1.0.0-rc.1`，全部可本地执行的验收项实测通过，限制如实入账。
- **Windows 发行条件：达成**。便携 ZIP + SHA256 + BUILD_INFO + 脚本链 + 托盘可用；普通用户按 QUICK_START 5 步可完成初始化（需自备 PostgreSQL 16）。
- **正式公开发布条件：未达成**。需要：① 用户授权推送 tag/分支并核对 Actions 远程构建；② 用户手动创建 GitHub Release；③ 建议先解决 K1（对比类研究质量）再广泛宣传对比能力。
