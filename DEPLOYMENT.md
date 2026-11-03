# DEPLOYMENT.md — 部署（一键启动是 V1.0 硬要求）

## Docker Compose 一键部署（目标）
```bash
cp .env.example .env   # 填模型 Key、搜索 Key、数据库密码（不得提交 .env）
docker compose up --build
# 打开 http://localhost:3000（前端）与 http://localhost:8080/health（后端健康检查）
```
服务：api（Go）、worker（Go，与 api 独立启动）、web（Next.js）、postgres（+ pgvector）、
redis（可选）、otel-collector（可选）。Postgres 数据用命名卷持久化；worker 挂了不影响 api 查状态。

## 环境变量（.env.example 必须齐全，占位符不得是真实 Key）
DATABASE_URL, REDIS_URL（可选）, LLM_PROVIDER, LLM_API_KEY（加密存储提示）, LLM_MODEL,
SEARCH_PROVIDER, SEARCH_API_KEY, WORKER_CONCURRENCY（默认 4）, NODE_TIMEOUT, TOOL_TIMEOUT,
OBJECT_STORAGE_PATH, OTEL_EXPORTER_OTLP_ENDPOINT, APP_BASE_URL。

## 健康检查与排障
- `GET /health` 分级返回 api / database / worker 状态，DB 断连不得只报 ok。
- 常见问题表（Codex 在实现中补齐）：端口占用、pgvector 扩展未启用、模型 Key 无效、搜索 Provider 限流。
- 备份：Postgres pg_dump + 对象存储快照目录，恢复步骤写进 docs/deployment.md。

## 从零部署验收
全新目录（不含旧卷）执行 `docker compose up --build` 后，能完成最终演示任务的前半段
（创建→规划→至少 2 个 Agent 并发），才算部署通过。不得依赖开发者本机已装的 Go/Node 环境。
