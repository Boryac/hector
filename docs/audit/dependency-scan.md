# 依赖漏洞扫描记录（T6.3）

工具：`govulncheck`（`golang.org/x/vuln/cmd/govulncheck@latest`，基于官方漏洞库 vulndb）
时间：2026-11-03
命令：

```bash
cd upstream/hector
go run golang.org/x/vuln/cmd/govulncheck@latest ./internal/researchos/...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

---

## 1. 研究域（`./internal/researchos/...`）

**结论：No vulnerabilities found.**

扫描范围覆盖 ResearchOS 的全部 13 个包（api / config / db / dag / domain / exec /
fetch / llm / planner / report / search / store / textutil / worker）。

### 1.1 从「有洞」到「清零」的过程

首扫命中 3 条**可达**漏洞，全部经 `pgx.ConnectConfig` 路径触达：

| 编号 | 模块 | 原版本 | 修复版本 | 说明 |
|---|---|---|---|---|
| GO-2026-5004 | `github.com/jackc/pgx/v5` | v5.8.0 | v5.9.2 | dollar-quoted 字符串导致的占位符混淆，可致 SQL 注入 |
| GO-2026-5970 | `golang.org/x/text` | v0.30.0 | v0.39.0 | 非法输入触发死循环 |
| GO-2026-6629 | `golang.org/x/text` | v0.39.0 | v0.41.0 | 升级到 v0.39.0 后暴露的同族问题，继续升到 v0.41.0 |

处理：`go get github.com/jackc/pgx/v5@v5.9.2 golang.org/x/text@v0.41.0` → `go mod tidy`。
复扫结果：`No vulnerabilities found.`；`go build ./...` 与
`go test ./internal/researchos/...`（10 个包）全绿。

**副作用（已在 DECISIONS.md 记录）**：`golang.org/x/text` v0.41.0 要求
`go >= 1.25`，因此 `go.mod` 的 `go` 指令由 `1.24.4` 提升到 `1.25.0`，
`toolchain go1.24.8` 一行被移除，`Dockerfile.researchos` 的基础镜像同步改为
`golang:1.25-alpine`。这是安全修复换来的最低版本抬升，不是随手升级。

---

## 2. 全模块（`./...`，含上游 `pkg/`）

首扫：**20 条可达漏洞，来自 8 个模块**。

| 模块 | 条目数 | 触达位置 |
|---|---|---|
| `github.com/xuri/excelize/v2` | 6 | 上游 Excel 导出能力 |
| `golang.org/x/net` | 6 | HTTP/2、IDNA 处理 |
| `go.opentelemetry.io/otel*` | 3 | 上游可观测性导出器 |
| `google.golang.org/grpc` | 2 | 上游 gRPC 传输 |
| `github.com/aws/aws-sdk-go-v2/*` | 1 | 上游 S3 快照存储 |
| `github.com/go-jose/go-jose/v4` | 1 | 上游 JWT/JOSE |
| 其他 | 1 | — |

处理：对上述模块批量 `go get -u`（不修改上游业务代码，只抬依赖版本）：

```bash
go get -u golang.org/x/net github.com/xuri/excelize/v2 \
  go.opentelemetry.io/otel go.opentelemetry.io/otel/sdk \
  google.golang.org/grpc github.com/go-jose/go-jose/v4
go get -u go.opentelemetry.io/otel/exporters/otlp/otlptrace \
  github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream
```

结果：**20 → 2**。`go build ./...` 通过，`researchos` 的 10 个测试包全绿。

### 2.1 剩余 2 条（未修复，附原因）

| 编号 | 模块 | 触发条件 | 为什么不修 |
|---|---|---|---|
| GO-2026-6505 | `otel/exporters/otlp/otlptrace` | OTLP 导出器在 info 级日志里打印 endpoint URL | 该导出器只在配置了 OTLP 采集端时启用；ResearchOS 的 `researchos` 子命令不初始化它。`go get -u` 已取到当前最新版，仍标记受影响（上游尚无发布修复版本） |
| GO-2026-5764 | `aws-sdk-go-v2/aws/protocol/eventstream` | 解析 S3 EventStream 时 panic 导致 DoS | 只有走 S3 blob 快照路径才会解码 EventStream；本机部署用本地文件存储，不启用 S3。同上，当前最新版仍受影响 |

两条都只通过包 `init` 出现在调用图里（`govulncheck` 的 trace 显示
`pkg/rag/blob_source.go:15:2 rag.init calls s3blob.init`、
`pkg/observability/tracer.go:11:2 observability.init calls stdouttrace.init`），
不在 ResearchOS 运行时的执行路径上。

**没有把这两条写成「已修复」。**它们属于上游依赖尚未发版修复的限制，
升级到修复版后应重跑 `govulncheck ./...` 确认。

---

## 3. 复现

```bash
cd upstream/hector

# 研究域（应为 No vulnerabilities found）
go run golang.org/x/vuln/cmd/govulncheck@latest ./internal/researchos/...

# 全模块（当前 2 条，见 §2.1）
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

> 全模块扫描耗时较长（约 5 分钟），在 280 秒超时下会被截断；
> 上面的结论来自完整跑完的那次扫描。
