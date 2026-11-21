#!/usr/bin/env python3
"""构建更完整的引用图：解析正文里所有「编号 + 标题 + 相对链接」的引用，
包括关联块里的 `- 前置：[24.4 TCC](24.4-TCC.md)`（make-index 的 refs.tsv 漏了这类）。

用法：python3 tools/audit/lib_graph.py [book_root]
输出：<out>/graph.json（edges / 邻接）
"""
import argparse
import json
import re
from pathlib import Path

SEC_RE = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
FENCE_RE = re.compile(r"^\s*```")
LINK_WITH_NUM = re.compile(r"\[(\d{1,3}\.\d{1,2})[^\]]*\]\(([^)]+)\)")
SEE_NUM = re.compile(r"(?:见|详见|参见|参阅)\s*\[?(\d{1,3}\.\d{1,2})")
PAREN_NUM = re.compile(r"[（(](\d{1,3}\.\d{1,2})[)）]")
DIRECT_PATH = re.compile(r"\]\(([^)]*?(\d{1,3}\.\d{1,2})-[^)]*?\.md)\)")


def numkey(n):
    a, b = n.split(".")
    return int(a), int(b)


def build(root: Path):
    sections = {}
    for p in (root / "v2").rglob("*.md"):
        if "_luban" in p.parts:
            continue
        m = SEC_RE.match(p.name)
        if m:
            sections[m.group(1)] = p
    known = set(sections)
    edges = {}
    for num, p in sections.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        lines = text.splitlines()
        inside = False
        found = set()
        for line in lines:
            if FENCE_RE.match(line):
                inside = not inside
                continue
            if inside:
                continue
            # 1) markdown 链接，label 以编号开头
            for m in LINK_WITH_NUM.finditer(line):
                t = m.group(1)
                if t != num and t in known:
                    found.add(t)
            # 2) [任意标题](path/24.4-xxx.md)
            for m in DIRECT_PATH.finditer(line):
                t = m.group(2)
                if t != num and t in known:
                    found.add(t)
            # 3) 见 24.6 / 详见 [24.6]
            for m in SEE_NUM.finditer(line):
                t = m.group(1)
                if t != num and t in known:
                    found.add(t)
            # 4) 括号裸编号（排除日期/版本之类的误判）
            for m in PAREN_NUM.finditer(line):
                t = m.group(1)
                if t != num and t in known:
                    found.add(t)
        edges[num] = sorted(found, key=numkey)
    return sections, edges


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("root", nargs="?", default=".")
    ap.add_argument("-o", "--out", default="tools/audit/out")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    out = root / args.out
    out.mkdir(parents=True, exist_ok=True)
    sections, edges = build(root)
    json.dump({"edges": edges}, open(out / "graph.json", "w"), ensure_ascii=False, indent=0)
    total = sum(len(v) for v in edges.values())
    print(f"sections={len(sections)} edges={total} (unique pairs)")
    # 与旧图对比
    old = set()
    for line in (root / "_luban" / "refs.tsv").read_text().splitlines()[1:]:
        a, b = line.split("\t")
        old.add((a, b))
    new = {(a, b) for a, bs in edges.items() for b in bs}
    print(f"old={len(old)} new={len(new)} added={len(new - old)} removed={len(old - new)}")


if __name__ == "__main__":
    main()
