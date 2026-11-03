# 整合后全量测试报告（POST-MERGE TEST REPORT）

> 2026-11-04。**全部为整合（merge `7d8c432` + 修复 `aaafd89` + 托盘/关机端点提交）之后在本机重新执行的测试**；历史报告数字仅作对照，不重复计入。

## 1. 后端（Go 1.27.3，仓库根）

| 检查 | 命令 | 结果 |
|---|---|---|
| 全模块编译 | `go build ./...` | PASS（含新增 desktop 模块与 admin shutdown 端点） |
| vet | `go vet ./internal/researchos/...` | PASS |
| 单元测试 | `go test -count=1 ./internal/researchos/...` | **14 包全绿**（16 包中 3 包无测试文件） |
| 新增端点测试 | `go test ./internal/researchos/api/`（含 admin_test.go 2 用例：401/202/501） | PASS |
| 托盘模块测试 | `desktop/traymanager`：DPAPI 加解密回环、配置原子写、PID 身份校验、启动→健康→优雅停止→端口释放（env 守卫集成测试） | PASS（2.8s 实跑） |

## 2. 前端

| 检查 | 结果 |
|---|---|
| `npx tsc --noEmit`（含发行场景 token 注入接线修改） | PASS |
| `npm run build`（Next.js 14 静态导出，16 页） | PASS |

## 3. 统一校验 verify.sh（整合后重跑，非历史数字）

- 第一次：9 通过 / 3 失败（整合路径问题：gofmt 因 autocrlf 检出 CRLF 全量误报、compose build.context 指向旧布局、E2E 依赖不入库的历史二进制）。
- 修复后：**14 通过 / 0 失败 / 0 跳过**（含两项负向自检、迁移幂等、E2E 起服 + PDF 内容比对）。
- 修复以 `aaafd89` 固化：`.gitattributes` 统一 LF、verify.sh 路径改为仓库根并自构建 E2E 二进制、compose context 修正。

## 4. 服务链路（打包产物 bin/researchos.exe，版本 v1.0.0-rc.1）

| 检查 | 结果 |
|---|---|
| 安全探测（`scripts/rc1_security_probe.py`） | **13/13**（证据 `scripts/rc1_evidence/security_probe.json`） |
| 浏览器审计（`scripts/rc1_browser_audit.py`，Playwright） | **27/27**（七路由 × 功能断言，证据 browser_audit.json） |
| 真实 LLM 研究任务 A（本地 Solar Mini 4，`cline/cline-free/solar-mini4`） | completed，96.3s，8 agents，**14,392 tokens**，8 结论全部「部分支持」级以上，8 条引用 |
| 受控优雅停止（新增 `POST /api/v1/admin/shutdown`） | HTTP 202，服务经 15s 排空后退出（实测两次） |

## 5. Windows 发行链

| 检查 | 结果 |
|---|---|
| 托盘程序构建（wails build） | PASS（researchos-tray.exe） |
| 托盘启动冒烟（3 轮：发现并修复 HWND_MESSAGE 64 位截断、SzTip 越界两处缺陷） | PASS：7s 存活、无错误、WebView2 环境创建 |
| 后端生命周期集成测试（DPAPI→配置→迁移→启动→健康→优雅停止） | PASS |
| 发行脚本链（install/start/health-check/stop/restart/diagnostics.ps1，PS 5.1） | PASS（修复 UTF-8 BOM 缺失导致的解析错误、大小写重复环境变量导致的 Start-Process 异常） |
| ZIP 组装 + SHA256 + BUILD_INFO | 见 docs/audit/RC1_FINAL_ACCEPTANCE_REPORT.md §E |

## 6. 过程中发现并修复的缺陷

| # | 级别 | 缺陷 | 修复 |
|---|---|---|---|
| 1 | P1 | autocrlf=true 检出 CRLF 使 gofmt 全量误报 | `.gitattributes` 统一 LF（aaafd89） |
| 2 | P1 | verify.sh/compose 引用旧目录布局 | 路径修正（aaafd89） |
| 3 | P1 | `all` 模式无受控关闭路径，托盘只能强杀 | 新增鉴权 shutdown 端点 + 单测 |
| 4 | P1 | 托盘 HWND_MESSAGE 32 位截断，message-only 窗口创建失败 | `^uintptr(2)` 全宽常量 |
| 5 | P2 | 托盘 SzTip 定长拷贝越界 panic | copyTip 安全拷贝 |
| 6 | P2 | ps1 无 BOM 在 PS 5.1 下中文注释破坏语法 | 全部 ps1 重写为 UTF-8 BOM |
| 7 | P3 | 大小写重复代理变量使 .NET Start-Process 抛异常 | common.ps1 进程级去重（不改系统配置） |
| 8 | P3 | run_number 唯一约束冲突偶发（research_runs 重放竞态），无用户可见影响 | 记录为已知问题，RC1 后处理 |

## 7. 环境限制（如实）

- Docker：不可用，容器路径未实机验证（BLOCKED，同 RC1 基线）。
- 后台进程跨工具会话持久性：本机沙箱在会话结束回收子进程，脚本链的"长期驻留"无法在沙箱内观察；start→healthy→stop 全链已在单会话内验证，托盘生命周期由集成测试覆盖。真实桌面环境不存在此限制。
- GitHub Actions 远程实跑：需要推送 tag（未授权）；工作流 YAML 已本地解析校验，步骤与本地执行的命令一一对应。
