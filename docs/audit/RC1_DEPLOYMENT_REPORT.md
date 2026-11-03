# RC1_DEPLOYMENT_REPORT — 部署验收报告（2026-11-04）

> 结论先行：**Docker 实机部署 BLOCKED**——本机无 Docker 运行时
> （`docker: command not found`，PATH 与常规安装路径均无）。按任务要求不宣称通过，
> 保留静态校验结果并给出解除阻塞的明确步骤。宿主机直跑部署路径完整可用
> （当前 8080 端口服务即为验证主体）。

## 1. Docker 可用性实测

| 检查 | 结果 |
|---|---|
| `docker --version` | command not found（git bash 与 PowerShell 均确认） |
| `docker compose version` | 同上 |
| Docker Desktop 安装痕迹 | 无（`C:\Program Files\Docker` 不存在） |
| 判定 | **BLOCKED：外部环境缺失** |

## 2. 已完成的静态保障（非实机，不作 PASS）

| 项 | 状态 | 依据 |
|---|---|---|
| compose 静态校验 | ✅ 0 错误 0 警告 | `scripts/check-compose.py`（build context 存在、无悬空 depends_on、命名卷已声明、`RESEARCHOS_API_TOKEN:?` 强制必填、127.0.0.1 绑定） |
| Dockerfile 逻辑核对 | ✅ | `Dockerfile.researchos` 多阶段构建（golang 构建 → alpine 运行），二进制自包含内嵌中文字体（go:embed），**无需镜像层安装字体** |
| 字体授权随发行包 | ✅ | `internal/researchos/report/fonts/OFL.txt` 随源码入库，内嵌进二进制 |

## 3. 实机部署前的强制注意项（容器网络拓扑）

本轮已实测确认的约束，Docker 部署时必须处理：

1. **本地模型 API 绑定回环**：`127.0.0.1:18326`（relay `lan.enabled=false`）。
   容器内的 `127.0.0.1` 指容器自身，**必须**改用 `host.docker.internal:18326`，
   且需在容器配置中声明 `extra_hosts: ["host.docker.internal:host-gateway"]`；
   同时中继侧要显式开启 LAN 暴露（`forward.lan.enabled=true`）或使用受控网段——
   **该操作会把本地模型服务暴露到宿主机网卡，属网络安全配置变更，必须先经用户批准**。
2. **PostgreSQL**：compose 已声明命名卷与服务健康检查；宿主机 55432 实例与容器实例端口隔离。
3. **API Token**：compose 中 `RESEARCHOS_API_TOKEN:?` 强制必填，未设置时 compose 拒绝启动（无静默默认）。
4. **数据持久化**：设置中心配置存 PostgreSQL（Web 保存值 source=web），重启容器后需验证
   `GET /api/v1/settings` 的 provider/model/timeout 仍为 web 来源（本轮已在宿主机直跑模式验证该持久化语义）。

## 4. 解除阻塞步骤（用户执行）

```powershell
# 1. 安装 Docker Desktop（或 dockerd）
# 2. 构建并启动
docker compose -f docker-compose.yaml up -d --build
# 3. 迁移在容器启动时自动执行；验收：
docker compose ps            # 全部 healthy
curl http://127.0.0.1:8080/  # 前端 200
# 4. 模型连通：设置中心 Base URL 改为 http://host.docker.internal:18326/v1
# 5. 重启容器验证持久化：docker compose restart && 检查 settings source=web
```

## 5. 宿主机直跑部署路径（本轮已验证）

- 启动：`set -a && . ./.env.researchos && set +a && exec ./bin/researchos.exe all`
- 三次重启（换二进制）均验证：配置持久化（source=web）、数据库连接、前端 200。
- 当前服务即 RC1 候选二进制（UA 修复 + CORS 修复 + 4m LLM 超时配置）。
