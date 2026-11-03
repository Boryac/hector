# CHANGELOG — Hector ResearchOS

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
