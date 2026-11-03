# CHANGELOG — Hector ResearchOS

## v1.0.0-rc.1（2026-11-04）— 仓库整合 · RC1 冻结 · Windows 发行

### 仓库整合

- **双仓合一**：开发包仓库（16 commits）经纯移动重构提交 c867229 并入
  verikod/hector（main 快进自 feature/hector-researchos），merge `7d8c432`
  保留两条完整历史；冲突仅 README/Makefile/.gitignore 三处并按语义解决。
- 双仓完整 bundle 备份（克隆验证通过）于仓库外；方案与验证记录：
  docs/architecture/REPOSITORY_CONSOLIDATION.md、docs/audit/GIT_CONSOLIDATION_REPORT.md。

### 新增（Windows 发行）

- **托盘管理程序**（desktop/traymanager，独立 Go 模块）：Wails + WebView2 窗口
  + 自研 win32 Shell_NotifyIcon 托盘（wails v2.17.1 无 SystemTray API，决策 42）。
  首次启动向导（数据目录选择/外置 PG 配置/自启动）、服务启停/状态/日志/数据目录、
  退出三选确认（停服退出/仅退托盘/取消）；单实例锁；服务与托盘生命周期严格分离。
- **受控优雅停止**：新增 `POST /api/v1/admin/shutdown`（鉴权保护，与 Ctrl+C 同路径），
  托盘/脚本经其停止服务，超时且身份三重校验后才允许强制终止。
- **首次启动与数据目录**：程序目录与用户数据严格分离（默认
  %LOCALAPPDATA%\Hector ResearchOS）；敏感配置（DSN/Token/SECRET_KEY）
  Windows DPAPI（当前用户）加密存储，原子写入，解密失败回退向导。
- **外置 PostgreSQL 16**：不捆绑不自动安装；连接检测 + 幂等迁移集成于向导与脚本。
- **发行脚本**：install/start/stop/restart/health-check/diagnostics.ps1
  （PS 5.1 兼容、UTF-8 BOM、诊断输出脱敏、进程级环境去重）。
- **便携 ZIP**：`Hector-ResearchOS-v1.0.0-rc.1-windows-x64.zip`（bin/web/fonts/
  licenses/migrations/scripts/docs），附 SHA256SUMS 与 BUILD_INFO。
- **GitHub Actions**：`.github/workflows/release-windows.yml`——tag 触发、
  最小 GITHUB_TOKEN 权限、构建+测试+打包+Smoke+Artifact 上传，不自动建 Release。

### 修复

- [P1] 整合路径：verify.sh/compose 指向旧双仓布局；`.gitattributes` 统一 LF
  （修复 autocrlf 导致的 gofmt 全量误报）。
- [P1] 托盘 HWND_MESSAGE 64 位截断导致 message-only 窗口创建失败。
- [P2] 托盘 SzTip 定长拷贝越界 panic；ps1 无 BOM 在 PS 5.1 下解析错误。
- [P3] 大小写重复环境变量导致 .NET Start-Process 异常（脚本内进程级去重）。

### 验证（整合后全部重测）

verify.sh 14/14 · Go 14 包全绿 · tsc/build 通过 · 安全探测 13/13 ·
浏览器审计 27/27 · 真实 LLM 任务 A（96s/14.4k tokens/8 结论 8 引用）·
托盘 DPAPI/生命周期集成测试 · 发行脚本链实跑 · shutdown 端点 202。

### 已知限制

见 docs/release/KNOWN_LIMITATIONS.md（K1 任务 B 质量不达标 … K13 Actions 远程未实跑）。

---


## RC1（2026-11-04）

RC1 候选版本：真实模型接入、LLM 能力验证、研究质量验收与安全复查。

### 新增

- **本地模型接入（Solar Mini 4）**：经设置中心真实 UI 接入本地 OpenAI 兼容中继；
  实际 Model ID `cline/cline-free/solar-mini4`；API Key AES-256-GCM 加密落库、
  掩码回显、重启持久化验证通过；Worker 同进程热生效。
  接入链路 14/14 断言（`web/scripts/settings_configure_model.py`）。
- **模型能力探测**：18 项自动探测（中英文/多轮/长文本/json_object/streaming/usage/
  错误处理/超时取消/工具调用 6 分项），脚本 `scripts/rc1_probe_model.py`，
  报告 `docs/RC1_MODEL_CAPABILITY_REPORT.md`。
- **真实模型研究任务验收**：事实核查 / 多对象对比 / 开放专题三类任务全部以
  LLM Planner + LLM 抽取真实执行（零 Mock），全链路证据落盘
  `scripts/rc1_evidence/`；运行器 `scripts/rc1_run_research.py` 支持审批闸门自动放行。
- **研究质量评估**：10 项指标 0-5 分评分 + 独立事实核查（非模型自证），
  报告 `docs/RC1_RESEARCH_QUALITY_REPORT.md`。
- **安全探测**：13 项 API 层安全检查脚本 `scripts/rc1_security_probe.py`，
  报告 `docs/RC1_SECURITY_REPORT.md`。
- **PDF 压力测试**：大报告（12 章×30 行表格+15 超长 URL）、空表格/单列表格、
  8 路并发渲染一致性、缺字替换、非法外部字体拒绝（`report/pdf_stress_test.go` 5 用例）。
- **浏览器审计（Playwright）**：27 项断言（5 路由渲染 + 工作台逐 Tab 懒加载 +
  SSE + 设置页 + UI 建任务落库清理），脚本 `scripts/rc1_browser_audit.py`。
  （Chrome 156 本机 headless 启动即退出，审计改用 Playwright Chromium。）

### 修复

- **[P0] 检索 UA 被 Bing 反爬降级**：默认自标识 UA 对任意检索式返回无关结果
  （实测 Go 检索召回到酒店广告页）。`defaultSearchUA` 改为浏览器 UA
  （config/fetch/search 三处），robots 合规行为不变。
  回归：任务 A 切题率 0/7 → 4/4。
- **[P1] CORS 任意 Origin 反射**：`withCORS` 原样回显任何 Origin；
  修复为仅同源回显（`api/cors_test.go` 6 断言 + API 探测复验）。
- **[P1] LLM 抽取超时静默降级**：90s 默认超时对 reasoning 模型的大 prompt
  不足，4/5 analyst 超时降级为规则抽取（事件流留痕）。
  经设置中心调 `model.timeout=4m` 后任务 C 切题率 0/6 → 17/17。
- **verify_pdf_content.py**：表格行（`|` 开头）比对改为第一列单元格抽样，
  修复表格单元格换行重排导致的误报；verify.sh 恢复 14/14。

### 变更

- `model.timeout` 热更新字段经 UI 保存路径调整（90s→4m），来源 `web`。

### 已知限制

- `json_schema` Structured Output 被中继接受但模型忽略；系统使用
  `json_object` + 容忍式 JSON 提取，不受影响。
- 中继免费通道存在偶发 502（探测 1 次，重试 4/4 成功）；调用方需具备重试。
- 任务 B「三对象对比」覆盖不足（仅 PostgreSQL 达成结论）——检索词矩阵与
  抽取逐问题闭环为后续改进项（见质量报告 §5）。
- Docker 实机部署 BLOCKED（本机无 Docker）；静态校验 0 错 0 警，容器网络
  注意事项见 `docs/RC1_DEPLOYMENT_REPORT.md`。
- Chrome 156 本机 headless 回归：浏览器自动化使用 Playwright Chromium 替代。

### 验证汇总

| 检查 | 结果 |
|---|---|
| verify.sh（含负向自检 + PDF 端到端） | 14 通过 / 0 失败 |
| Go 单元测试（含压力测试） | 全绿（report 包新增 5 用例） |
| 浏览器审计（Playwright） | 27/27 |
| 安全探测 | 13/13 |
| 模型能力探测 | 16/18（1 项定性后可用、1 项有兼容替代） |
| 真实研究任务 | A/B/C 全部 completed（LLM Planner + LLM 抽取） |
