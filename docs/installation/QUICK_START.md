# Hector ResearchOS — 快速开始（Windows 便携版）

> 目标平台：Windows 10 / 11 x64。便携 ZIP，免安装，不需要管理员权限。

## 你需要准备

- **PostgreSQL 16**（外置必需依赖）：需要已安装并可访问的 PostgreSQL 16 实例，以及一个可用数据库账号。ResearchOS 不捆绑、不自动安装数据库。
- WebView2 Runtime（Windows 10/11 通常自带，托盘窗口依赖）。

## 5 分钟上手

1. **解压** 本 ZIP 到任意目录（路径建议不含空格与中文，例如 `D:\ResearchOS\`）。
2. **双击 `researchos-tray.exe`**。托盘区出现图标，并自动打开初始化向导。
3. **选择数据目录**（默认 `%LOCALAPPDATA%\Hector ResearchOS\data`，也可选 `D:\ResearchOS\Data`）。程序与数据分离：升级时只替换程序文件，数据目录不动。
4. **配置 PostgreSQL**：填连接串（`postgres://用户:口令@主机:5432/数据库名?sslmode=disable`），点击「测试连接并执行迁移」。口令以 Windows DPAPI（当前用户）加密存储，不写明文日志。
5. **完成初始化**：服务自动启动，点击「打开 ResearchOS」进入 Web 界面。
6. **配置模型与搜索**：Web 界面右上「设置中心」配置 LLM Provider（OpenAI 兼容 Base URL + Model ID + API Key）与搜索 Provider。

## 日常使用

- 托盘图标：右键菜单可 启动/停止/重启服务、打开日志与数据目录、查看状态。
- **关窗不停服务**：窗口关闭只隐藏托盘；研究任务在后台继续。
- **退出三选一**：托盘右键「退出…」时可选 停止服务并退出 / 仅退出托盘（后台继续）/ 取消。
- 开机自启默认关闭；可在托盘窗口「选项」中开启（仅当前用户级，不注册系统服务）。

## 脚本方式（开发者/无人值守）

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install.ps1     # 完整性自检
powershell -ExecutionPolicy Bypass -File scripts\start.ps1       # 启动后台
powershell -ExecutionPolicy Bypass -File scripts\health-check.ps1
powershell -ExecutionPolicy Bypass -File scripts\stop.ps1
powershell -ExecutionPolicy Bypass -File scripts\diagnostics.ps1 # 只读诊断（输出脱敏）
```

## 安全说明

- 后台服务只监听本机回环地址（127.0.0.1），API 全部要求 Bearer Token（初始化时随机生成）。
- Token / 数据库口令 / 加密主钥经 DPAPI 加密存储于 `%LOCALAPPDATA%\Hector ResearchOS\config.json`，绝不写入 ZIP、日志或 Git。
- Web UI 首次打开经 `#token=` 注入凭据，持久化后地址栏凭据即被清除。

## 遇到问题

见 `docs\installation\TROUBLESHOOTING.md`，或运行 `scripts\diagnostics.ps1` 生成脱敏诊断信息。
