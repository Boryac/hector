# 演示记录（真实运行，非静态图）

> 下面每个数字都来自本机一次真实执行，任务 ID 与产物文件可核对。
> 环境：Windows 11 / AMD64，PostgreSQL 16.2（127.0.0.1:55432），
> `researchos all` 单进程（API + 4 Worker），**未配置 LLM API Key**。
> 执行时间：2026-11-03。

---

## 1. 演示任务

**研究目标**：研究全球 AI Agent 开源生态，筛选 10 个代表性项目并生成竞争分析报告

| 项 | 值 |
|---|---|
| 任务 ID | `aa1af51c-7b20-4578-9065-0dff96faebe3` |
| 深度 | standard |
| 最大并发 Agent | 4 |
| 交叉验证 | 开启 |
| 规划模式 | heuristic（未配置 LLM） |
| 终态 | **completed** |
| 从创建到终态 | **8 秒**（含人工批准等待） |
| 节点 | 8 个，全部 completed |
| 事件 | 110 条 |

### 1.1 节点执行明细

| # | 节点 | 检索式 / 子问题 | 结果 |
|---|---|---|---|
| 1 | search_1 | `AI Agent open source projects` | 20 条结果 |
| 2 | analyze_1 | 现状、主要参与者与代表性项目 | 6 条资料 → 结论若干 |
| 3 | search_2 | `AI Agent framework comparison` | 20 条结果 |
| 4 | analyze_2 | 关键技术路线与能力差异 | 6 条资料 |
| 5 | search_3 | `AI Agent ecosystem adoption risks` | 11 条结果 |
| 6 | analyze_3 | 生态与采用风险 | 6 条资料 |
| 7 | verify | 跨源交叉验证 | 7 条结论参与验证 |
| 8 | synthesize | 汇总成报告 | `report.completed` |

三个检索分支用了三条**不同**的检索式——这正是纯中文目标曾复用同一条检索式的
缺陷修复后的效果（见 `docs/benchmark.md` §1.3）。

### 1.2 资料来源

| 指标 | 值 |
|---|---|
| 收录来源总数 | 39 |
| 抓到正文（status=ok） | 18 |
| 仅发现未抓取 | 20 |
| 抓取失败 | 1 |
| 按 Provider | wikipedia 13 / github 11 / hackernews 10 / bing 5 |

### 1.3 事件流构成（110 条）

```
research.created 1   research.planning 1   workflow.created 1   research.started 1
node.started 8       agent.started 8       node.progress 27     tool.invoked 3
tool.completed 26    node.completed 8      agent.completed 8    evidence.created 7
evidence.verified 7  research.synthesizing 1  report.started 1  report.completed 1
research.completed 1
```

---

## 2. 产物

| 文件 | 大小 | 说明 |
|---|---|---|
| `docs/demo/report.md` | 4895 B | 从 `GET /api/v1/research/{id}/report.md` 原样保存 |
| `docs/demo/report.html` | 7270 B | 从 `GET .../report.html` 原样保存，链接仅保留 http(s) |

报告结构：摘要 → 目标与范围 → 核心发现（7 条，带 `[n]` 引用）→ 分主题分析 →
关键数据对比 → 争议与不确定性 → 信息来源（9 条带 URL）→ 方法与局限性。

### 2.1 报告质量的实际状态（不美化）

没有 LLM 时，结论由规则从正文句子里抽取，实测质量如下：

**可用的结论**（可在原文逐字核对）：
- `Codex is an AI coding agent developed by Open AI for software engineering tasks such as writing code and fixing bugs, released in April 2025 as Codex CLI.`
- `Generative artificial intelligence (Gen AI) is a subfield of artificial intelligence (AI) that uses generative models to generate text, images, videos.`

**偏弱的结论**：
- 部分条目是产品页的营销句（`247 free tools · 48 REST APIs · no account needed…`）
- 部分条目是仓库 README 的一句话简介（`AI/ML research agents from idea to paper-ready evidence.`）

**结论强度**：7 条全部标为「单一来源，置信度 0.50」，报告摘要里也写明了
「本次执行未产出多来源交叉验证通过的结论，结论强度有限」。

**没有做到的事**：任务要求「筛选 10 个代表性项目并做竞争分析」，规则抽取做不到
「筛选与对比」这类需要判断的动作。这是没有 LLM 时的能力边界，已在
`docs/benchmark.md` §4 与报告「方法与局限性」中写明。

---

## 3. 界面操作路径

服务启动后浏览器打开 `http://127.0.0.1:8080/`：

1. **Dashboard**：填写研究目标 → 选深度 → 提交。任务进入 `awaiting_approval`。
2. **批准执行**：点「批准」，任务转 `running`，Worker 开始认领节点。
3. **任务详情页 `/task/`**：左侧任务与节点时间线，画布显示 DAG 与节点状态色，
   下方实时事件流（SSE），右侧节点输出与错误。
4. **来源区**：展示每条资料的站点、Provider、可信度、抓取状态。
5. **报告区**：渲染 Markdown，点 `[n]` 跳到来源，右上角可导出 Markdown / HTML。

页面截图需要在有桌面的环境执行 `bash scripts/screenshot.sh`（本机 Chrome/Edge
无头模式无输出，属环境限制，见 `docs/benchmark.md` §3.1）。

---

## 4. 复现命令

```bash
# 1. 迁移 + 启动
cd upstream/hector
./bin/researchos.exe migrate
./bin/researchos.exe all

# 2. 另开终端，提交同一目标
TOKEN=$(grep '^RESEARCHOS_API_TOKEN=' .env.researchos | cut -d= -f2-)
ID=$(curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"goal":"研究全球 AI Agent 开源生态，筛选 10 个代表性项目并生成竞争分析报告","depth":"standard","enable_verification":true}' \
  http://127.0.0.1:8080/api/v1/research | python -c "import sys,json;print(json.load(sys.stdin)['task']['id'])")

# 3. 批准执行
curl -s -X POST -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/research/$ID/approve

# 4. 等终态
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/research/$ID \
  | python -c "import sys,json;print(json.load(sys.stdin)['task']['status'])"

# 5. 导出报告
curl -s -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:8080/api/v1/research/$ID/report.md" -o report.md
```

---

## 5. 演示视频

未录制。原因：需要图形界面录屏工具，本机只提供无桌面会话；分镜脚本与逐镜头
要点见 `DEMO_SCRIPT.md`，可按脚本在有桌面的机器上一次性录制。
