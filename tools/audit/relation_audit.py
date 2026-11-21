#!/usr/bin/env python3
"""章节关系审计：章级引用矩阵、孤岛、孤儿节、概论承诺核对、结论节入度。

用法：python3 tools/audit/relation_audit.py [book_root]
输出：<out>/relation.md + relation.json
"""
import argparse
import csv
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

SEC_RE = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
LINK_RE = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")


def numkey(n):
    a, b = n.split(".")
    return int(a), int(b)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("root", nargs="?", default=".")
    ap.add_argument("-o", "--out", default="tools/audit/out")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    out = root / args.out
    out.mkdir(parents=True, exist_ok=True)

    # 用完整图（含关联块链接）
    import sys as _sys
    _sys.path.insert(0, str(Path(__file__).parent))
    from lib_graph import build as build_graph
    _, emap = build_graph(root)
    idx = {}
    for r in csv.DictReader(open(root / "_luban" / "index.tsv"), delimiter="\t"):
        idx[r["编号"]] = r
    edges = {(a, b) for a, bs in emap.items() for b in bs}
    outd = defaultdict(set)
    ind = defaultdict(set)
    for a, b in edges:
        outd[a].add(b)
        ind[b].add(a)

    # 章级矩阵
    chapters = sorted({n.split(".")[0] for n in idx}, key=int)
    chap_of = lambda n: n.split(".")[0]
    # 章节标题（README 的 "## 第 X 章 ..."）
    chap_title = {}
    rp = root / "README.md"
    if rp.exists():
        for line in rp.read_text(encoding="utf-8").splitlines():
            m = re.match(r"^##\s*第\s*(\d+)\s*章\s*(.+)$", line.strip())
            if m:
                chap_title[m.group(1)] = m.group(2).strip()
    matrix = Counter()
    intra = Counter()
    for a, b in edges:
        ca, cb = chap_of(a), chap_of(b)
        if ca == cb:
            intra[ca] += 1
        else:
            matrix[(ca, cb)] += 1

    # 每章内/外引用
    rows = []
    for c in chapters:
        secs = [n for n in idx if chap_of(n) == c]
        # 章节边：从本章节出发的边
        out_edges = sum(len(outd[n]) for n in secs)
        in_edges = sum(len(ind[n]) for n in secs)
        # 跨章去重对数
        cross_targets = set()
        for n in secs:
            cross_targets |= {chap_of(t) for t in outd[n] if chap_of(t) != c}
        cross_sources = set()
        for n in secs:
            cross_sources |= {chap_of(t) for t in ind[n] if chap_of(t) != c}
        zero_in = [n for n in secs if not ind[n]]
        zero_out = [n for n in secs if not outd[n]]
        rows.append({
            "ch": c, "title": chap_title.get(c, ""), "n": len(secs),
            "out_edges": out_edges, "in_edges": in_edges,
            "cross_targets": len(cross_targets), "cross_sources": len(cross_sources),
            "zero_in": zero_in, "zero_out": zero_out,
        })

    # 孤儿节（全书入度 0）与孤岛章
    orphans = sorted([n for n in idx if not ind[n]], key=numkey)
    orphan_ch = Counter(chap_of(n) for n in orphans)
    # 结论节：每章最后一节（按编号）
    last_sec = {}
    for n in idx:
        c = chap_of(n)
        if c not in last_sec or numkey(n) > numkey(last_sec[c]):
            last_sec[c] = n
    # 概论承诺核对：概论里的链接目标是否被本章节引用过（至少存在）
    guo_issues = []
    for c in chapters:
        p = None
        for n, r in idx.items():
            if chap_of(n) == c:
                d = (root / r["路径"]).parent
                if (d / "0-概论.md").exists():
                    p = d / "0-概论.md"
                    break
        if not p:
            continue
        text = p.read_text(encoding="utf-8")
        links = LINK_RE.findall(text)
        targets = []
        for label, href in links:
            hm = re.search(r"(\d{1,3}\.\d{1,2})-", href)
            if hm:
                targets.append((label, hm.group(1)))
        for label, t in targets:
            if t not in idx:
                guo_issues.append((c, "概论链接目标不存在", label, t))
                continue
            # 本章节是否真的引用它（方向：概论说前置/下游）
            used = any(t in outd[n] for n in idx if chap_of(n) == c)
            if not used:
                guo_issues.append((c, "概论承诺的关系正文未引用", label, t))

    md = ["# 章节关系审计", "", f"节 {len(idx)} · 跨章边 {sum(matrix.values())} · 章内边 {sum(intra.values())}", ""]
    md += ["## 章级外链概况（越少越孤岛）", "",
           "| 章 | 标题 | 节 | 出边 | 入边 | 指向外部章 | 被外部章引用 | 零入节 | 零出节 |",
           "|---|---|---|---|---|---|---|---|---|"]
    for r in rows:
        md.append(f"| {r['ch']} | {r['title'][:18]} | {r['n']} | {r['out_edges']} | {r['in_edges']} | {r['cross_targets']} | {r['cross_sources']} | {len(r['zero_in'])} | {len(r['zero_out'])} |")

    md += ["", "## 孤岛章（外链到 <=2 个章 或 被 <=2 个章引用）", ""]
    for r in rows:
        if r["cross_targets"] <= 2 or r["cross_sources"] <= 2:
            md.append(f"- 第 {r['ch']} 章 {r['title']}：指向 {r['cross_targets']} 章 / 被 {r['cross_sources']} 章引用")

    md += ["", "## 孤儿节（全书入度 0，结论没被接住）", ""]
    for c in chapters:
        ns = [n for n in orphans if chap_of(n) == c]
        if ns:
            md.append(f"- 第 {c} 章（{len(ns)}）：{', '.join(ns)}")

    md += ["", "## 结论节入度（每章最后一节，应该高）", "",
           "| 章 | 结论节 | 标题 | 入度 | 本章均值 |", "|---|---|---|---|---|"]
    for c in chapters:
        n = last_sec[c]
        secs = [x for x in idx if chap_of(x) == c]
        avg = sum(len(ind[x]) for x in secs) / len(secs)
        flag = " ⚠" if len(ind[n]) == 0 else ""
        md.append(f"| {c} | {n} | {idx[n]['标题'][:16]} | {len(ind[n])}{flag} | {avg:.1f} |")

    md += ["", "## 概论承诺未兑现（概论写了关系，本章正文没引）", ""]
    by_ch = defaultdict(list)
    for c, typ, label, t in guo_issues:
        by_ch[c].append((typ, label, t))
    for c in chapters:
        if c in by_ch:
            md.append(f"- 第 {c} 章：{len(by_ch[c])} 条 —— " +
                      "；".join(f"{t}:{lab[:18]}" for typ, lab, t in by_ch[c][:6]))
    if not guo_issues:
        md.append("- 无")

    (out / "relation.md").write_text("\n".join(md) + "\n", encoding="utf-8")
    json.dump({"rows": rows, "orphans": orphans, "guo_issues": guo_issues,
               "matrix": {f"{a}->{b}": v for (a, b), v in matrix.items()}},
              open(out / "relation.json", "w"), ensure_ascii=False, indent=1)
    print("\n".join(md[:60]))
    print("\n→", out / "relation.md")


if __name__ == "__main__":
    main()
