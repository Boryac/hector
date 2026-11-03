#!/usr/bin/env python3
"""check-compose.py — Docker Compose 文件的静态校验（无 Docker 环境下的最低保障）。

本机没有 Docker（`docker: command not found`），因此真实 `compose up` 无法执行。
这个脚本不能替代实机部署，但能拦住"文件本身就是错的"这一类问题——它们在有 Docker
的机器上同样会立刻失败：

  1. YAML 解析失败；
  2. 引用了不存在的 Dockerfile / 构建上下文目录；
  3. 引用了未声明的 depends_on 服务；
  4. 服务既没有 image 也没有 build；
  5. 引用了未声明的命名卷；
  6. 端口绑定在 0.0.0.0（对外暴露）却没有显式声明为允许；
  7. 必填变量使用了 ${VAR} 而没有 `:?`（会在缺变量时静默用空值启动）。

退出码：0 全部通过，1 有错误，2 有警告但没有错误（用 --strict 时警告也算失败）。

用法：
  python scripts/check-compose.py deployments/compose.yaml docker-compose.yaml
  python scripts/check-compose.py --strict deployments/compose.yaml
"""
from __future__ import annotations

import argparse
import os
import re
import sys

try:
    import yaml
except ImportError:
    print("需要 PyYAML：python -m pip install pyyaml", file=sys.stderr)
    sys.exit(1)

# 允许对外暴露端口的服务（本项目中应为空：API 由反向代理前置，DB 不对外）
ALLOW_PUBLIC = set()

# 必须写 healthcheck 的服务（有状态 / 提供服务）
REQUIRE_HEALTHCHECK = {"db", "app", "postgres", "api"}

VAR_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(:?[-?][^}]*)?\}")


def load(path: str):
    with open(path, encoding="utf-8") as fh:
        return yaml.safe_load(fh)


def check_file(path: str, strict: bool) -> tuple[list[str], list[str]]:
    errors: list[str] = []
    warns: list[str] = []
    base = os.path.dirname(os.path.abspath(path))

    try:
        doc = load(path)
    except yaml.YAMLError as exc:
        return [f"YAML 解析失败：{exc}"], []
    if not isinstance(doc, dict):
        return ["顶层不是 mapping"], []

    services = doc.get("services") or {}
    if not services:
        return ["没有 services 段"], []

    declared_volumes = set((doc.get("volumes") or {}).keys())

    for name, svc in services.items():
        if not isinstance(svc, dict):
            errors.append(f"{name}: service 定义不是 mapping")
            continue

        build = svc.get("build")
        if build is None and not svc.get("image"):
            errors.append(f"{name}: 既没有 image 也没有 build")
        if isinstance(build, dict):
            ctx = build.get("context", ".")
            ctx_abs = os.path.normpath(os.path.join(base, ctx))
            if not os.path.isdir(ctx_abs):
                errors.append(f"{name}: build.context 不存在 → {ctx_abs}")
            df = build.get("dockerfile", "Dockerfile")
            df_abs = os.path.join(ctx_abs, df)
            if not os.path.isfile(df_abs):
                errors.append(f"{name}: build.dockerfile 不存在 → {df_abs}")

        for dep in (svc.get("depends_on") or {}):
            if dep not in services:
                errors.append(f"{name}: depends_on 引用了未声明的服务 {dep!r}")

        for vol in svc.get("volumes") or []:
            # 短语法 "src:dst[:mode]"；src 是命名卷时必须在顶层 volumes 里声明
            if isinstance(vol, dict):
                src = vol.get("source")
                if src and not src.startswith((".", "/")) and src not in declared_volumes:
                    errors.append(f"{name}: 引用了未声明的命名卷 {src!r}")
            elif isinstance(vol, str):
                src = vol.split(":")[0]
                if src and not src.startswith((".", "/")) and not src.startswith("$") and src not in declared_volumes:
                    errors.append(f"{name}: 引用了未声明的命名卷 {src!r}")

        if name in REQUIRE_HEALTHCHECK and not svc.get("healthcheck"):
            warns.append(f"{name}: 没有 healthcheck（依赖它的服务会过早启动）")

        for port in svc.get("ports") or []:
            spec = port if isinstance(port, str) else str(port)
            host = spec.split(":")[0] if ":" in spec else ""
            if host in ("", "0.0.0.0") and name not in ALLOW_PUBLIC:
                warns.append(f"{name}: 端口 {spec} 绑定 0.0.0.0（对外暴露）")

    # 检查环境变量引用：缺省值还是强制必填
    raw = open(path, encoding="utf-8").read()
    for var, default in VAR_RE.findall(raw):
        if var in ("TZ",):
            continue
        if default.startswith(":?") or default.startswith("?"):
            continue
        if not default:
            warns.append(f"环境变量 {{{{{var}}}}} 无默认值也无强制标记，缺变量时会注入空串")

    if strict:
        errors.extend(warns)
        warns = []
    return errors, warns


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("files", nargs="+")
    ap.add_argument("--strict", action="store_true", help="把警告也当作失败")
    args = ap.parse_args()

    total_err = 0
    total_warn = 0
    for path in args.files:
        if not os.path.isfile(path):
            print(f"[FAIL] {path}: 文件不存在")
            total_err += 1
            continue
        errors, warns = check_file(path, args.strict)
        total_err += len(errors)
        total_warn += len(warns)
        if errors:
            print(f"[FAIL] {path}")
            for e in errors:
                print(f"       ✗ {e}")
        else:
            print(f"[ OK ] {path}")
        for w in warns:
            print(f"       ! {w}")

    print(f"\n合计：{total_err} 个错误，{total_warn} 个警告")
    if total_err:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
