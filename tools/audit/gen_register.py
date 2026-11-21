#!/usr/bin/env python3
"""生成「问题登记表.md」：当前剩余的逐节问题（P0 修复后基线）。

用法：python3 tools/audit/gen_register.py
"""
import json
import re
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tools" / "audit" / "out"
struct = json.load(open(OUT / "structural.json"))
depth = json.load(open("/tmp/depth6.json"))
graph = json.load(open(OUT / "graph.json"))["edges"]
dates = json.load(open(OUT / "dates.json"))

secs = {s["num"]: s for s in struct["sections"]}
ind = defaultdict(int)
for a, bs in graph.items():
    for b in bs:
        ind[b] += 1
reg = defaultdict(lambda: defaultdict(list))
for s in struct["sections"]:
    for it in s["issues"]:
        reg[s["num"]][it["type"]].append(it)
for d in depth:
    if not d["errors"]:
        continue
    m = re.search(r"/(\d{1,3}\.\d{1,2})-", d["file"])
    if m:
        reg[m.group(1)]["depth"] = d["errors"]
for n in secs:
    if not graph.get(n):
        reg[n]["zero_out"] = ["出度 0"]
    if ind[n] == 0:
        reg[n]["zero_in"] = ["入度 0"]

lines = ["# 全节问题登记表（P0 修复后 · 按节号排序）", "",
         "说明：P0 已修（断表/自检编号/过期待补/目录标题/81 章概论/已核实事实错误）。本表是**剩余问题**，主要属于 P1/P2：",
         "缺「关联」块、深度门禁（多为自检题数 4<5）、关联过长、一句话多行、零入/出度。", "",
         "| 节 | 标题 | 字数 | 日期 | 门禁 | 结构 | 关系 | 其它 |", "|---|---|---|---|---|---|---|---|"]
order = sorted(secs, key=lambda x: tuple(int(y) for y in x.split(".")))
for n in order:
    r = reg.get(n)
    if not r:
        continue
    s = secs[n]
    gate = "; ".join(r.get("depth", []))
    fmt = []
    if r.get("yijuhua_multiline") or r.get("yijuhua_count"):
        fmt.append("一句话多行/多入口")
    if r.get("no_anti"):
        fmt.append("缺反模式")
    if r.get("no_question"):
        fmt.append("缺自检")
    rel = []
    if r.get("no_related"):
        rel.append("缺关联块")
    if r.get("related_too_long"):
        rel.append("关联过长")
    if r.get("related_self_link"):
        rel.append("关联自引")
    if r.get("zero_out"):
        rel.append("出度0")
    if r.get("zero_in"):
        rel.append("入度0")
    other = []
    if r.get("todo_placeholder"):
        other.append("待补占位")
    if r.get("link_broken"):
        other.append("死链")
    d = dates.get(n, {}).get("date", "")
    lines.append(f"| {n} | {s['title'][:22]} | {s['chars']} | {d} | {gate} | {'; '.join(fmt)} | {'; '.join(rel)} | {'; '.join(other)} |")
(ROOT / "审计报告" / "问题登记表.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
print("rows:", sum(1 for n in order if n in reg))
