# 配置参考（CONFIGURATION）

## 配置文件布局

| 内容 | 位置 | 说明 |
|---|---|---|
| 应用配置（托盘） | `%LOCALAPPDATA%\Hector ResearchOS\config.json` | 数据目录、监听地址、自启动；敏感字段 DPAPI 加密 |
| 用户数据 | 首次启动自选，默认 `%LOCALAPPDATA%\Hector ResearchOS\data` | `logs\`（日志）、`run\`（pid 记录）、`tmp\` |
| 数据库 | **外置 PostgreSQL 16**，物理数据由 PostgreSQL 管理 | 应用绝不直接触碰数据库文件 |
| 后端运行配置 | Web 设置中心（Settings Center，持久化于数据库） | 模型 Provider、搜索 Provider、审批策略等 |

## config.json 字段

```json
{
  "version": 1,
  "initialized": true,
  "dataDir": "C:\\Users\\<you>\\AppData\\Local\\Hector ResearchOS\\data",
  "addr": "127.0.0.1:8080",
  "autostart": false,
  "pgDsnEnc": "dpapi:…",
  "apiTokenEnc": "dpapi:…",
  "secretKeyEnc": "dpapi:…"
}
```

- `pgDsnEnc` / `apiTokenEnc` / `secretKeyEnc`：`dpapi:` 前缀 + base64 的 DPAPI 密文（CurrentUser 作用域）。**不要手工编辑**——解密失败会自动回到首次初始化向导。
- `addr`：后端监听地址。默认 `127.0.0.1:8080`，**不要改成 0.0.0.0**（会向局域网暴露无差别访问面）。

## 后端环境变量（脚本/容器场景）

| 变量 | 必填 | 说明 |
|---|---|---|
| `RESEARCHOS_DATABASE_URL` | 是 | PostgreSQL DSN |
| `RESEARCHOS_API_TOKEN` | 是 | API Bearer Token（未设置时服务**拒绝启动**） |
| `RESEARCHOS_SECRET_KEY` | 建议 | 设置中心 API Key 的 AES-256-GCM 加密主钥（未设则用派生默认，见安全文档） |
| `RESEARCHOS_ADDR` | 否 | 监听地址，默认 127.0.0.1:8080 |
| `RESEARCHOS_WEB_DIR` | 否 | 前端静态资源目录（便携包内 `web\.build` 由托盘/脚本自动设置） |
| `RESEARCHOS_PDF_FONT` | 否 | 覆盖内嵌中文字体的 TTF 路径 |
| `RESEARCHOS_USER_AGENT` | 否 | 检索/抓取 UA（默认为浏览器 UA，见决策 39） |

## PostgreSQL 16 要求

- 版本 ≥ 16；应用迁移由后端 `migrate` 子命令执行（幂等，可重复运行）。
- 建议专用数据库与账号；应用只会创建/修改自身迁移定义的表。
- **迁移可能不可逆**：升级前请先备份数据库（`pg_dump`）。

## LLM / 搜索 Provider

经 Web 设置中心或托盘「快速配置」写入，保存于数据库并加密：
- 模型：OpenAI 兼容 `base_url` + `model` + `api_key`（掩码回显）。
- 搜索：`bing`（免费，无需 Key）/ `tavily` / `serpapi` / `brave`（需要各自凭据）。
