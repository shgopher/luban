#!/usr/bin/env python3
"""章节质量记分卡：合并 depth 门禁、结构审计、关系图、横切检查，按章排序。

用法：python3 tools/audit/chapter_scorecard.py [book_root]
依赖：tools/audit/out/{structural.json,relation.json,content_checks.json} 与 /tmp/depth6.json
输出：tools/audit/out/scorecard.md + scorecard.json
"""
import argparse
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

SEC_RE = re.compile(r"^(\d{1,3}\.\d{1,2})-.*\.md$")


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

    struct = json.load(open(out / "structural.json"))
    rel = json.load(open(out / "relation.json"))
    checks = json.load(open(out / "content_checks.json"))
    depth = json.load(open("/tmp/depth6.json"))

    # depth by section
    depth_by = {}
    for d in depth:
        m = SEC_RE.match(Path(d["file"]).name)
        if m:
            depth_by[m.group(1)] = d

    # 结构问题按节
    struct_by = {}
    for s in struct["sections"]:
        struct_by[s["num"]] = s
    issue_secs = defaultdict(set)
    for t, secs in struct["summary"]["per_type_sections"].items():
        for n in secs:
            issue_secs[n].add(t)

    # 关系：入度/出度
    edges = json.load(open(out / "graph.json"))["edges"]
    ind = Counter()
    for a, bs in edges.items():
        for b in bs:
            ind[b] += 1

    # 章节汇总
    chapters = defaultdict(lambda: {"secs": [], "chars": 0})
    for n in struct_by:
        c = n.split(".")[0]
        chapters[c]["secs"].append(n)
        chapters[c]["chars"] += struct_by[n]["chars"]

    readme = (root / "README.md").read_text(encoding="utf-8")
    titles = {}
    for line in readme.splitlines():
        m = re.match(r"^##\s*第\s*(\d+)\s*章\s*(.+)$", line.strip())
        if m:
            titles[m.group(1)] = m.group(2).strip()

    rows = []
    for c in sorted(chapters, key=int):
        secs = sorted(chapters[c]["secs"], key=numkey)
        depth_fail = [n for n in secs if depth_by.get(n, {}).get("errors")]
        struct_issues = [n for n in secs if issue_secs.get(n)]
        stale = [x for x in checks["todo_stale"] if x["section"] in secs]
        zero_in = [n for n in secs if ind[n] == 0]
        # 严重度加权
        score = 100
        score -= len(depth_fail) * 4
        score -= len(struct_issues) * 3
        score -= len(stale) * 3
        score -= len(zero_in) * 2
        # 章均字数低于 5000 扣分
        avg = chapters[c]["chars"] / len(secs)
        if avg < 5000:
            score -= 8
        rows.append({
            "ch": c, "title": titles.get(c, ""), "n": len(secs),
            "chars": chapters[c]["chars"], "avg": round(avg),
            "depth_fail": depth_fail, "struct_issues": struct_issues,
            "stale_todo": len(stale), "zero_in": zero_in, "score": score,
        })
    rows.sort(key=lambda r: (r["score"], r["ch"]))

    md = ["# 章节质量记分卡（分越低风险越高）", "",
          "| 章 | 标题 | 节 | 字数 | 均字 | 门禁失败 | 结构问题节 | 过期待补 | 零入度 | 风险分 |",
          "|---|---|---|---|---|---|---|---|---|---|"]
    for r in rows:
        md.append(f"| {r['ch']} | {r['title'][:16]} | {r['n']} | {r['chars']:,} | {r['avg']} | {len(r['depth_fail'])} | {len(r['struct_issues'])} | {r['stale_todo']} | {len(r['zero_in'])} | {r['score']} |")
    md += ["", "## 门禁失败明细", ""]
    for r in rows:
        if r["depth_fail"]:
            md.append(f"- 第 {r['ch']} 章（{len(r['depth_fail'])}）：{', '.join(r['depth_fail'])}")
    (out / "scorecard.md").write_text("\n".join(md) + "\n", encoding="utf-8")
    json.dump(rows, open(out / "scorecard.json", "w"), ensure_ascii=False, indent=1)
    print("\n".join(md[:30]))
    print("\n→", out / "scorecard.md")


if __name__ == "__main__":
    main()
