#!/usr/bin/env python3
"""verify_pdf_content.py — 验证导出的 PDF 没有丢内容。

用法：
    python scripts/verify_pdf_content.py <report.md> <report.pdf>

原理：把 Markdown 中的标记语法剥掉（**、_、[]() 等）得到期望文本集合，
再从 PDF 提取全文（去除空白）逐项比对。三类对象必须全部出现：
  1. 每个章节标题（^## ...）；
  2. 每个来源 URL（(...) 里的 http(s) 链接）；
  3. 每段正文的前 24 个有效字符（列表项、表格单元格同样抽样）。

退出码 0 = 全部通过；1 = 有缺失（打印每一处）。
依赖：pip install pymupdf
"""
from __future__ import annotations

import io
import re
import sys

import pymupdf

LINK = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")
BOLD = re.compile(r"\*\*([^*]+)\*\*")
ITALIC = re.compile(r"_([^_]+)_")


def plain(line: str) -> str:
    """剥掉 Markdown 标记，得到 PDF 里应出现的文本形态。"""

    def link_sub(m: re.Match) -> str:
        text, url = m.group(1), m.group(2)
        if url.startswith(("http://", "https://")):
            # PDF 端约定：链接渲染为「文本（URL）」
            return f"{text}（{url}）"
        return text

    s = LINK.sub(link_sub, line)
    s = BOLD.sub(r"\1", s)
    s = ITALIC.sub(r"\1", s)
    return s


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__)
        return 2
    md = io.open(sys.argv[1], encoding="utf-8").read()
    doc = pymupdf.open(sys.argv[2])
    pdf_flat = re.sub(r"\s+", "", "".join(p.get_text() for p in doc))

    problems: list[str] = []

    # 结构检查
    if len(doc) < 1:
        problems.append("PDF 没有任何页")
    page1 = doc[0].get_text() if len(doc) else ""
    if "第 1 页 / 共" not in page1:
        problems.append("首页缺少页码页脚")
    last = doc[len(doc) - 1].get_text() if len(doc) else ""
    if f"第 {len(doc)} 页 / 共 {len(doc)} 页" not in last:
        problems.append("末页页码与总页数不符")

    def flat_expected(line: str) -> str:
        return re.sub(r"\s+", "", plain(line))

    # 章节标题
    headings = re.findall(r"^## (.+)$", md, re.M)
    for h in headings:
        if flat_expected(h) not in pdf_flat:
            problems.append(f"缺章节标题: {h}")

    # 来源 URL
    urls = re.findall(r"\((https?://[^)]+)\)", md)
    for u in sorted(set(urls)):
        if re.sub(r"\s+", "", u) not in pdf_flat:
            problems.append(f"缺来源 URL: {u}")

    # 正文 / 列表 / 表格单元格抽样
    samples = 0
    for line in md.split("\n"):
        t = line.strip()
        if len(t) < 20 or t.startswith("#"):
            continue
        if t.startswith("- "):
            t = t[2:]  # 列表标记在 PDF 里渲染为 • / 编号，比对正文部分
        if t.startswith("|"):
            # 表格行：PDF 按「列宽等比 + 单元格内换行」重排，整句无法 flat 匹配。
            # 改为比对第一列（结论列）的前 24 个有效字符；分隔行与空列跳过。
            cols = [c.strip() for c in t.strip("|").split("|")]
            if not cols or set(t.replace("|", "").replace("-", "").replace(" ", "").replace(":", "")) == set():
                continue  # 分隔行 |---|---|
            t = cols[0] if len(cols[0]) >= 12 else (cols[-1] if len(cols) > 1 and len(cols[-1]) >= 12 else "")
            if not t:
                continue
        key = flat_expected(t)
        if len(key) < 12:
            continue
        samples += 1
        if key[:24] not in pdf_flat:
            problems.append(f"缺正文: {t[:48]}")

    print(f"PDF {len(doc)} 页；比对章节 {len(headings)}、URL {len(set(urls))}、正文抽样 {samples}")
    if problems:
        print(f"发现 {len(problems)} 处缺失：")
        for p in problems:
            print(f"  ✗ {p}")
        return 1
    print("[OK] PDF 内容与 Markdown 一致，无丢失")
    return 0


if __name__ == "__main__":
    sys.exit(main())
