#!/usr/bin/env python3
"""OpenAPI 契约自检。

做的事只有三件，但每一件都是「YAML 能解析」掩盖不了的问题：

1. 拒绝重复键。YAML 规范里重复键是未定义的，PyYAML/大多数解析器会静默
   保留最后一个，于是「我明明加了这个路径」和「线上就是 404」可以同时成立。
2. 校验所有 `$ref` 指向的组件真实存在。悬空 ref 在生成客户端时才爆炸，
   在契约文件里读起来完全正常。
3. 校验 operationId 唯一、path 参数都已声明。

退出码 0 = 通过，1 = 有问题（逐条打印）。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:  # pragma: no cover
    sys.exit("需要 PyYAML：python -m pip install pyyaml")

HTTP_METHODS = {"get", "put", "post", "delete", "patch", "head", "options", "trace"}


class StrictLoader(yaml.SafeLoader):
    """在构造 mapping 时检查重复键。"""


def _construct_mapping(loader, node, deep=False):
    mapping = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in mapping:
            mark = key_node.start_mark
            raise yaml.YAMLError(
                f"重复的键 {key!r}，位置 第 {mark.line + 1} 行第 {mark.column + 1} 列"
            )
        mapping[key] = loader.construct_object(value_node, deep=deep)
    return mapping


StrictLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _construct_mapping
)


def walk_refs(node, path="$"):
    """产出 (json 指针, ref)。"""
    if isinstance(node, dict):
        for k, v in node.items():
            if k == "$ref" and isinstance(v, str):
                yield path, v
            else:
                yield from walk_refs(v, f"{path}/{k}")
    elif isinstance(node, list):
        for i, v in enumerate(node):
            yield from walk_refs(v, f"{path}[{i}]")


def resolve_ref(doc, ref):
    if not ref.startswith("#/"):
        return None  # 外部引用不在这里管
    cur = doc
    for part in ref[2:].split("/"):
        part = part.replace("~1", "/").replace("~0", "~")
        if not isinstance(cur, dict) or part not in cur:
            return None
        cur = cur[part]
    return cur


def main() -> int:
    target = Path(sys.argv[1] if len(sys.argv) > 1 else "api/openapi.yaml")
    if not target.exists():
        print(f"找不到 {target}")
        return 1

    raw = target.read_text(encoding="utf-8")
    try:
        doc = yaml.load(raw, Loader=StrictLoader)
    except yaml.YAMLError as exc:
        print(f"[FAIL] YAML 解析失败：{exc}")
        return 1

    problems: list[str] = []

    # 1. 悬空 ref
    refs = list(walk_refs(doc))
    for where, ref in refs:
        if resolve_ref(doc, ref) is None:
            problems.append(f"悬空引用 {ref}  <- {where}")

    # 2. 未被任何地方引用且不是根级的组件（提示用，不算失败）
    used = {
        r.rsplit("/", 1)[-1]
        for _, r in refs
        if r.startswith("#/components/schemas/")
    }

    # 3. operationId 唯一
    seen_ops: dict[str, str] = {}
    paths = doc.get("paths", {})
    for p, item in paths.items():
        if not isinstance(item, dict):
            continue
        in_template = set(re.findall(r"\{([^}]+)\}", p))
        # 路径参数既可声明在 path-item 级，也可声明在单个 operation 级，取并集。
        shared = item.get("parameters", []) or []
        for method, op in item.items():
            if method not in HTTP_METHODS or not isinstance(op, dict):
                continue
            declared = set()
            raw_params = list(shared) + list(op.get("parameters", []) or [])
            for prm in raw_params:
                if isinstance(prm, dict) and "$ref" in prm:
                    prm = resolve_ref(doc, prm["$ref"])
                if isinstance(prm, dict) and prm.get("in") == "path" and prm.get("name"):
                    declared.add(prm["name"])
            missing = in_template - declared
            if missing:
                problems.append(
                    f"路径参数未声明 {sorted(missing)}  <- {method.upper()} {p}"
                )
            opid = op.get("operationId")
            if opid:
                if opid in seen_ops:
                    problems.append(
                        f"operationId 重复 {opid!r}：{seen_ops[opid]} 与 {method.upper()} {p}"
                    )
                seen_ops[opid] = f"{method.upper()} {p}"

    if problems:
        for p in problems:
            print(f"[FAIL] {p}")
        print(f"\n共 {len(problems)} 个问题")
        return 1

    npaths = len(paths)
    nops = sum(
        1
        for item in paths.values()
        if isinstance(item, dict)
        for m in item
        if m in HTTP_METHODS
    )
    schemas = doc.get("components", {}).get("schemas", {})
    print(
        f"[OK] {target}: {npaths} 条路径 / {nops} 个操作 / "
        f"{len(schemas)} 个 schema / {len(refs)} 处引用，无重复键、无悬空引用"
    )
    unused = sorted(set(schemas) - used)
    if unused:
        print(f"     未被引用的 schema（可能正常，也可能是漏接线）：{', '.join(unused)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
