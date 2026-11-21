#!/usr/bin/env python3
"""概论承诺 vs 正文事实：概论「前置/下游/相关」里写的关系，正文引用图里是否成立。

用法：python3 tools/audit/guo_promise.py [book_root]
输出：tools/audit/out/guo_promise.md / .json
"""
import argparse
import json
import re
from pathlib import Path

LINK_RE = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")
NUM_IN_LINK = re.compile(r"(\d{1,3}\.\d{1,2})")


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

    edges = json.load(open(out / "graph.json"))["edges"]
    out_edges = {a: set(b) for a, b in edges.items()}
    in_edges = {}
    for a, bs in out_edges.items():
        for b in bs:
            in_edges.setdefault(b, set()).add(a)

    # chapter dirs
    chap_dirs = {}
    for p in (root / "v2").rglob("0-概论.md"):
        # 章号来自目录名
        m = re.match(r"(\d{1,3})-", p.parent.name)
        if m:
            chap_dirs[m.group(1)] = p

    issues = []
    stats = {"pre": 0, "down": 0, "rel": 0, "ok": 0, "miss": 0}
    for ch, p in sorted(chap_dirs.items(), key=lambda kv: int(kv[0])):
        text = p.read_text(encoding="utf-8")
        # 解析位置表：找含 前置/下游/相关 的行
        direction = None
        for line in text.splitlines():
            s = line.strip()
            if not s.startswith("|"):
                direction = None
                continue
            if "**前置**" in s or "前置（" in s:
                direction = "pre"
            elif "**下游**" in s or "下游（" in s:
                direction = "down"
            elif "**相关**" in s or "相关（" in s:
                direction = "rel"
            if direction is None:
                continue
            for label, href in LINK_RE.findall(s):
                m = NUM_IN_LINK.search(href)
                if not m:
                    continue
                target = m.group(1)
                tch = target.split(".")[0]
                if tch == ch:
                    continue  # 同章不查
                stats[direction] += 1
                # 事实检查
                if direction == "pre":
                    # 本章节应该引用 target
                    ok = any(target in out_edges.get(n, set()) for n in out_edges if n.split(".")[0] == ch)
                elif direction == "down":
                    # 下游 target 应该引用本章任意节
                    ok = any(x.split(".")[0] == ch for x in out_edges.get(target, set()))
                else:
                    ok = (any(target in out_edges.get(n, set()) for n in out_edges if n.split(".")[0] == ch)
                          or any(x.split(".")[0] == ch for x in in_edges.get(target, set())))
                if ok:
                    stats["ok"] += 1
                else:
                    stats["miss"] += 1
                    issues.append({"ch": ch, "direction": direction, "target": target,
                                   "label": label.strip()})

    md = ["# 概论承诺 vs 正文引用", "",
          f"检查 {stats['pre'] + stats['down'] + stats['rel']} 条跨章承诺，成立 {stats['ok']}，"
          f"不成立 {stats['miss']}（前置 {stats['pre']} / 下游 {stats['down']} / 相关 {stats['rel']}）", ""]
    by_ch = {}
    for it in issues:
        by_ch.setdefault(it["ch"], []).append(it)
    for ch in sorted(by_ch, key=int):
        md.append(f"## 第 {ch} 章（{len(by_ch[ch])} 条不成立）")
        for it in by_ch[ch]:
            md.append(f"- [{it['direction']}] {it['target']} {it['label'][:30]}")
        md.append("")
    (out / "guo_promise.md").write_text("\n".join(md) + "\n", encoding="utf-8")
    json.dump({"issues": issues, "stats": stats}, open(out / "guo_promise.json", "w"),
              ensure_ascii=False, indent=1)
    print("\n".join(md[:40]))
    print("\n→", out / "guo_promise.md")


if __name__ == "__main__":
    main()
