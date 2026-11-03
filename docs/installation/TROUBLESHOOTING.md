# 故障排查（TROUBLESHOOTING）

先运行 `scripts\diagnostics.ps1`（只读、输出脱敏），再对照下表。

| 症状 | 可能原因 | 处理 |
|---|---|---|
| 托盘窗口空白/无法显示 | WebView2 Runtime 缺失 | 安装 WebView2 Runtime；`diagnostics.ps1` 会显示检测版本 |
| 「未找到应用配置」 | 未完成首次初始化 | 双击 `researchos-tray.exe` 走向导 |
| 初始化时迁移失败 | PostgreSQL 未启动 / DSN 错误 / 版本 < 16 / 无权限 | 确认服务运行、账号可连库；DSN 格式 `postgres://user:pass@host:5432/db?sslmode=disable` |
| 服务启动后健康检查不通过 | 端口被占用 / 数据库不可达 | `diagnostics.ps1` 查看端口占用；日志在 数据目录\logs\backend-*.log |
| 端口 8080 被其它程序占用 | 默认端口冲突 | 删除 config.json 后重新初始化选其它端口，或手工改 `addr` 字段 |
| Web UI 一直 401 | 浏览器 localStorage 无 Token | 用托盘「打开 ResearchOS」重新进入（自动注入）；或开发者工具执行 `localStorage.setItem('researchos_token','<你的Token>')` |
| API Key 保存后模型不可用 | Base URL / Model ID 错误或中继未运行 | 设置中心「测试连接」验证；本机模型服务需先启动 |
| 检索结果明显不相关 | 免费搜索源反爬/降级 | 更换搜索 Provider（tavily/serpapi）；RC1 已修复默认 UA 降级问题（决策 39） |
| 长任务 agent 报 `context deadline exceeded` | LLM 超时不足 | 设置中心把 `model.timeout` 调大到 4m（决策 41） |
| 托盘退出后服务还在 | 这是有意设计 | 「退出…」选「停止后台服务并退出」才会停止 |
| 解密失败回到向导 | config.json 被手改或换了 Windows 用户 | 重新初始化（数据库数据不受影响，凭据需重填） |
| 升级后数据是否丢失 | — | 程序目录与数据目录分离，覆盖程序文件不影响数据；**升级前备份数据库**（迁移可能不可逆） |

## 日志位置

- 后端日志：`<数据目录>\logs\backend-<时间戳>.log`（托盘/脚本启动时自动落盘）。
- 托盘诊断：托盘「查看服务状态」；或 `diagnostics.ps1` 尾部会输出最近日志（已脱敏）。

## 完全重置（危险操作，需自行确认）

1. `scripts\stop.ps1` 停止服务。
2. 备份并删除 `<数据目录>`（日志/pid 随之清理）。
3. 删除 `%LOCALAPPDATA%\Hector ResearchOS\config.json`。
4. 重新运行 `researchos-tray.exe` 初始化。数据库本身不受影响（数据仍在 PostgreSQL 中）。
