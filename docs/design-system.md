# Hector ResearchOS 设计系统

> 适用范围：`web/` 下的 Next.js 静态导出前端。
> 参考稿：`hector-researchos-dashboard-prototype.html`（只取版式、层级与交互节奏，不复制品牌色与装饰元素）。
> 最后核对：2026-11-03。

## 1. 设计立场

- **Premium Enterprise / Editorial Minimalism**：像一份排版讲究的研究刊物，而不是 AI 控制台。
- **Quiet Luxury**：低饱和、少颜色、靠字号/留白/对齐建立层级；不靠阴影、发光、玻璃拟态。
- **Swiss Typography**：左对齐、明确网格、数字使用等宽数字特性，标签小而克制。
- **Refined Data Dashboard**：每个数字都有来源，没有装饰性图表；加载态用骨架，不用假数据占位。

明确不做：大面积青绿/薄荷品牌色、紫蓝渐变、深色 AI 控制台、霓虹/粒子/光斑、重玻璃拟态、夸张圆角卡片、彩色标签墙、每个模块都配图标、无意义统计卡、装饰波纹、Emoji、编造图表或指标。

## 2. 令牌

定义位置：`web/app/globals.css` 的 `:root`。页面与组件只引用变量，不新增同义色值。

### 颜色

| 语义 | 令牌 | 值 | 用途 |
|---|---|---|---|
| 页面底色 | `--bg` | `#F6F6F4` | 暖白画布 |
| 卡片 | `--surface` | `#FFFFFF` | 主内容面 |
| 次级面 | `--surface-2` | `#F1F2EE` | 表头、代码块、hover |
| 三级面 | `--surface-3` | `#FAFAF8` | 内嵌区块 |
| 侧栏 | `--sidebar` | `#F2F2EE` | 与主区微差 |
| 分隔线 | `--line` | `#E9EAE7` | 卡片/表格边框 |
| 强分隔线 | `--line-strong` | `#D9DBD5` | 输入框、浮层边界 |
| 弱分隔线 | `--line-soft` | `#EFF0EC` | 行内分隔 |
| 主文字 | `--text` | `#252725` | 标题、关键数字 |
| 次文字 | `--text-2` | `#4D5350` | 正文 |
| 辅助文字 | `--muted` | `#777D7B` | 说明、单位 |
| 弱文字 | `--faint` | `#9BA09D` | 时间戳、占位 |
| 强调 | `--accent` | `#526B7B` | 主按钮、选中、链接 |
| 强调深 | `--accent-strong` | `#405867` | hover / active |
| 强调浅 | `--accent-soft` | `#EDF1F3` | 选中底、信息条 |
| 成功 | `--ok` | `#658472` | 完成、健康 |
| 警告 | `--warn` | `#A8875E` | 待批准、降级 |
| 失败 | `--err` | `#A66F6B` | 失败、不可达 |
| 信息 | `--info` | `#73899A` | 中性提示 |

状态色只用于小面积：状态点、徽标文字、左侧 2px 标记；不做整卡着色。

### 字体

```css
--font-sans: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI",
  "PingFang SC", "Microsoft YaHei", sans-serif;
--font-serif: "Source Han Serif SC", "Noto Serif SC", "Songti SC",
  "STSong", Georgia, "Times New Roman", serif;
--font-mono: "SFMono-Regular", "JetBrains Mono", Consolas, monospace;
```

- 正文 13.5px / 1.55；辅助 11–12px；分区标题 15–17px；页面标题 24–28px；关键数据 26–34px。
- 报告正文使用 `--font-serif`，与产品 UI 的无衬线字体形成“研究刊物”对比。
- 数字使用 `font-variant-numeric: tabular-nums`，表格与指标对齐。

### 间距与形状

- 间距阶：4 / 8 / 12 / 16 / 24 / 32。
- 圆角：`5 / 7 / 10 / 14`；卡片默认 10px，输入与按钮 7px，浮层 14px。
- 阴影：仅 `--shadow-xs`（卡片 hover）、`--shadow-sm`（浮层）、`--shadow-modal`（模态）；主界面靠 1px 边框分层。
- 页面：`padding: 28px 30px 72px`，内容最大 `1440px`；卡片间距 14–18px。

### 布局

- 侧栏 `--sidebar-w: 244px`；顶栏 `--topbar-h: 64px`。
- 工作台三栏：`226px minmax(0,1fr) 312px`；提纲收起时 `1fr 312px`。
- 1024px 以下侧栏变抽屉；860px 以下工作台单列；620px 以下 DAG 固定缩放。
- 设置页：左侧 168px 锚点导航 + 右侧单列字段，底部固定保存条。

## 3. 组件约定

| 组件 | 文件 | 约定 |
|---|---|---|
| 按钮 | `web/components/ui.tsx` | `primary / default / ghost / danger` + `sm / md`；主按钮仅每屏一个 |
| 徽标 | 同上 | 状态徽标含 6px 圆点；不用大面积色块 |
| 指标卡 | `web/app/page.tsx` | 标签 11px 大写字距、数值 30px、脚注 11px；无图标 |
| 画布节点 | `web/components/WorkflowCanvas.tsx` | 196px 宽、2 行标题截断、状态左沿 2px、底部依赖标签 |
| 表格 | `SourceTable` / 页面内 | 表头 11px、行高 48px、hover 仅底色变化；数字右对齐 |
| 抽屉 | `ui.tsx` | 右侧 520px，标题 + 关闭按钮，移动端全宽 |
| 空态 | `EmptyState` | 一句标题 + 一句解释 + 可选动作，不放插画 |
| 骨架 | `Skeleton` | 与真实内容同尺寸，避免布局跳动 |
| 保存条 | `web/app/settings/page.tsx` | 固定视口底部，显示未保存项数；`还原 / 保存更改` |

图标统一 16px 线性图标（`web/components/Icon.tsx`），仅导航、按钮、状态使用；正文模块标题不配图标。

## 4. 动效

- 页面进入：220ms `translateY(5px)` + 淡入，只播一次。
- hover：160–180ms 背景/边框变化；卡片 hover 仅阴影从 xs 到 sm，不上浮。
- 图表：首次挂载一次性绘制；轮询更新不重播。
- DAG：节点选中为边框 + 背景变化，不缩放、不发光。
- `@media (prefers-reduced-motion: reduce)`：关闭所有 animation/transition，保留状态颜色。

## 5. 数据与状态

- 所有数值来自真实 API：`/api/v1/research`、`/health`、`/settings`、`/sources`、`/reports`、`/stats`。
- 加载：骨架；无数据：空态；失败：`ErrorState` + 重试；部分失败：`InlineNotice`，不掩盖主内容。
- 不编造指标、不生成占位图表、不把 “—” 包装成 0。
- 时间：相对时间只用于 7 天内，超过显示日期；服务端预渲染不写当前时刻（避免 hydration 文本不一致）。

## 6. 页面状态

| 页面 | 路由 | 数据源 | 状态 |
|---|---|---|---|
| 总览 | `/` | 任务 / 健康 / 设置 / 报告 | 完成 |
| 研究任务 | `/tasks/` | 任务列表 | 完成 |
| 研究工作台 | `/task/?id=` | 任务详情 / 事件 / 来源 / 证据 / 报告 / Agents | 完成 |
| 研究来源 | `/sources/` | 跨任务来源 | 完成 |
| 证据中心 | `/evidence/` | Claim 与证据 | 完成 |
| 研究报告 | `/reports/` | 跨任务报告 | 完成 |
| 设置 | `/settings/` | 设置 schema / MCP | 完成 |

真实截图见 [`docs/screenshots/`](screenshots/)：

- [总览（桌面）](screenshots/dashboard-desktop.png)
- [研究工作台（桌面）](screenshots/workspace-desktop.png)
- [来源（桌面）](screenshots/sources-desktop.png)
- [报告（桌面）](screenshots/reports-desktop.png)
- [设置（桌面）](screenshots/settings-desktop.png)
- [总览（移动）](screenshots/dashboard-mobile.png)
- [工作台（移动）](screenshots/workspace-mobile.png)

## 7. 验证方式

```bash
cd upstream/hector/web
npm run typecheck
npm run build
node scripts/ui-audit.mjs --shots=../../../docs/screenshots
```

`ui-audit.mjs` 需要 Chrome/Edge 以 `--remote-debugging-port=9333` 启动；脚本会自动发现第一条研究任务，检查 7 条路由 × 桌面（1440×900）/ 移动（390×844），断言控制台零错误、文档无横向溢出，并输出截图。

## 8. 已知取舍

- 1440px 下工作台默认收起提纲，把宽度让给 DAG（画布 808px、节点 192px）；需要目录时点工具栏「提纲」。1920px 等更宽屏幕仍沿用同一最大内容宽度，避免画布被拉成空旷的海洋。
- 移动端 DAG 不缩放到全图，保留 0.68 缩放与平移；全图自适应会把节点压到 100px 以下，不可读。
- 系统未安装 Inter 时回落到 Segoe UI / 苹方，行高与字距保持令牌不变。
- PDF 导出未实现（T6.1）；HTML/Markdown 导出已实测。