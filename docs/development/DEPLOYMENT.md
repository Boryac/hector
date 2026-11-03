# DEPLOYMENT.md — 部署与运维

> 状态标注：标 ⚠️ 的路径**未在本机验证**（开发环境无 Docker、无图形浏览器）。
> 已验证的路径在文中直接给出命令与预期输出。

## 1. 部署形态

| 形态 | 适用 | 状态 |
|---|---|---|
| 单二进制 + 外部 PostgreSQL | 本地开发、单用户试用 | ✅ 已验证 |
| Docker Compose（app + postgres） | 一台机器上运行、可重复部署 | ⚠️ 未验证 |
| 分进程（serve / worker 分离） | 调并发、做故障注入 | ✅ 已验证 |

前端为静态产物，由 Go 服务同源托管（`RESEARCHOS_WEB_DIR`），不需要单独的 Node 进程。

## 2. 环境要求

- Go 1.24+（构建期）
- PostgreSQL 14+（运行期）
- Node.js 20+（仅构建前端时需要）
- 端口：`8080`（API + UI）、`5432`（DB，或自定义）

## 3. 方式 A：单二进制（已验证）

```bash
cd upstream/hector
cp .env.example .env.researchos        # 填 DATABASE_URL 与 API_TOKEN
go build -o bin/researchos ./cmd/researchos

./bin/researchos migrate               # 幂等，可重复执行
RESEARCHOS_WEB_DIR=./web/.build ./bin/researchos all
```

验证：

```bash
curl -s -H "Authorization: Bearer $RESEARCHOS_API_TOKEN" \
  http://127.0.0.1:8080/api/v1/health
# 期望：{"status":"ok", ... "migrations":["0001_init.sql"]}
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/    # 期望 200
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/v1/research  # 期望 401
```

前端产物构建（可选，不构建则只有 API）：

```bash
cd web && npm ci && npx next build     # 产物在 web/.build
```

> Windows 下 `next build` 可能因沙箱写 `.next/trace` 报 EPERM，仓库内提供了
> `web/scripts/next-build.cjs` 作为构建包装器：`node scripts/next-build.cjs`。

## 4. 方式 B：Docker Compose（⚠️ 未验证）

```bash
cd hector-researchos-codex-pack
export RESEARCHOS_API_TOKEN=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
docker compose up -d --build
docker compose logs -f app
```

约定：

- `db` 有 healthcheck，`app` 等 `service_healthy` 才启动
- 数据库与数据目录分别落在 `pgdata` / `appdata` 命名卷
- 端口默认只绑 `127.0.0.1`，不暴露到局域网
- `RESEARCHOS_API_TOKEN` 为必填，未设置时 Compose 直接报错退出（不静默用默认值）

镜像构建结构见 `upstream/hector/Dockerfile.researchos`（web 构建 → Go 构建 → alpine 运行，非 root，带 HEALTHCHECK）。

## 5. 方式 C：分进程（已验证）

```bash
./researchos serve       # API，必须设 RESEARCHOS_API_TOKEN，否则拒绝启动
./researchos worker      # 执行器，可多开
```

多 Worker 之间靠 `workflow_nodes` 的行锁与租约互斥，不需要额外协调组件。

## 6. 配置项

完整清单见 `upstream/hector/.env.example`。运行时最常改的几项：

| 变量 | 影响 | 备注 |
|---|---|---|
| `RESEARCHOS_WORKERS` | 并发节点数 | 与 DB 连接池上限相关，建议 ≤ 8 |
| `RESEARCHOS_MAX_AGENTS` | 单任务 Agent 上限 | 与 `max_agents` 字段共同生效 |
| `RESEARCHOS_NODE_TIMEOUT` | 单节点超时 | 默认 180s；深检索可调到 300s |
| `RESEARCHOS_STALE_LEASE` | 租约超时 | 默认 3min；决定 Worker 崩溃后的接管延迟 |
| `RESEARCHOS_REQUIRE_APPROVAL` | 是否需人工批准再执行 | 本地试用建议 `false` |
| `RESEARCHOS_SEARCH_PROVIDERS` | 检索源 | 免密钥：`github,hackernews,wikipedia,bing` |

配置加载顺序：shell 环境变量 > `.env.researchos`（自动加载，不覆盖已有变量）。

## 7. 数据与备份

| 内容 | 位置 |
|---|---|
| 结构化数据 | PostgreSQL（13 张表，见 `docs/DATA_MODEL.md`） |
| 网页原文快照 | `RESEARCHOS_DATA_DIR/snapshots/<sha256>.txt` |

备份：

```bash
pg_dump "$RESEARCHOS_DATABASE_URL" -Fc > researchos-$(date +%F).dump
tar czf snapshots-$(date +%F).tgz "$RESEARCHOS_DATA_DIR/snapshots"
```

恢复后重跑 `researchos migrate` 即可（迁移幂等）。

## 8. 运维检查

```bash
# 健康与依赖状态
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/health

# 卡住的任务（running 但无进展）
curl -s -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:8080/api/v1/research?status=running"

# 重启丢失租约的节点（服务启动后每 10s 自动做一次，也可手动触发）
```

Worker 自带看门狗：每 10 秒调用 `ResetStaleNodes` 回收过期租约，并对长期无进展的任务执行
`finalizeTask` 收敛状态（避免任务永久停在 `running`）。

## 9. 升级

1. 停worker
2. 换二进制
3. 跑 `researchos migrate`
4. 起 `serve`，再起 `worker`

迁移按文件名顺序执行，`schema_migrations` 记录已应用项；重复执行无副作用。

## 10. 已知部署限制

1. 没有多副本协调，`serve` 只能单实例（Worker 可多实例）
2. 事件表无分区，长期运行需要按 `created_at` 归档
3. 未提供 TLS 终结，生产环境应在前面挂反向代理并只暴露 HTTPS
4. 未做速率限制，公网暴露前必须补
