#!/usr/bin/env python3
"""合并深度审读结果：校验原文引用、去重、按严重度排序，输出 Markdown + JSON。

用法：python3 tools/audit/consolidate.py <results.json> [-o 审计报告/深度审读问题清单.md]
results.json 结构：[{unit, ok, result:{unit, chapters:[{ch,score,verdict,issues:[...],gaps:[...],relations:[...],strongest,weakest}], top_issues:[...]}}]
"""
import argparse
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SEC_FILE = {}


def load_sections():
    import csv
    for r in csv.DictReader(open(ROOT / "_luban" / "index.tsv"), delimiter="\t"):
        SEC_FILE[r["编号"]] = r["路径"]
    # 概论目录
    for p in (ROOT / "v2").rglob("0-概论.md"):
        m = re.match(r"(\d{1,3})-", p.parent.name)
        if m:
            SEC_FILE.setdefault(m.group(1) + ".0", str(p))


def norm(s: str) -> str:
    s = re.sub(r"\s+", "", s)
    s = re.sub(r"[“”\"'‘’「」『』`]", "", s)
    return s


def verify(section: str, quote: str):
    """返回 (exists, matched_ratio)。"""
    path = SEC_FILE.get(section)
    if not path:
        return False, 0.0
    text = norm((ROOT / path).read_text(encoding="utf-8", errors="ignore"))
    q = norm(quote)
    if not q:
        return False, 0.0
    if q in text:
        return True, 1.0
    # 取最长 15 字子串判断
    best = 0.0
    for n in range(min(len(q), 40), 7, -1):
        for i in range(0, len(q) - n + 1, max(1, n // 4)):
            if q[i:i + n] in text:
                best = n / len(q)
                break
        if best:
            break
    return best >= 0.6, best


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("results")
    ap.add_argument("-o", "--out", default=str(ROOT / "审计报告" / "深度审读问题清单.md"))
    args = ap.parse_args()
    load_sections()
    data = json.load(open(args.results))
    if isinstance(data, dict):
        data = data.get("results", [])

    rows = []
    gaps = []
    rels = []
    scores = {}
    top = []
    for item in data:
        if not item or not item.get("ok"):
            continue
        res = item.get("result") or {}
        for ch in res.get("chapters", []):
            c = str(ch.get("ch", "")).strip()
            scores[c] = (ch.get("score"), (ch.get("verdict") or "")[:160])
            for iss in ch.get("issues", []):
                rows.append({"ch": c, "section": str(iss.get("section", "")).strip(),
                             "severity": iss.get("severity", "P1"), "type": iss.get("type", ""),
                             "quote": (iss.get("quote") or "").strip(),
                             "detail": (iss.get("detail") or "").strip(),
                             "fix": (iss.get("fix") or "").strip(),
                             "source": res.get("unit", "")})
            for g in ch.get("gaps", []):
                gaps.append({"ch": c, "section": str(g.get("section", "")),
                             "missing": g.get("missing", ""), "why": g.get("why", ""),
                             "suggest": g.get("suggest", "")})
            for r in ch.get("relations", []):
                rels.append({"ch": c, "section": str(r.get("section", "")),
                             "target": str(r.get("target", "")), "issue": r.get("issue", "")})
        for t in res.get("top_issues", []):
            top.append({"section": str(t.get("section", "")), "severity": t.get("severity", "P1"),
                        "quote": t.get("quote", ""), "issue": t.get("issue", ""),
                        "source": res.get("unit", "")})

    # 核验引用
    for r in rows:
        ok, ratio = verify(r["section"], r["quote"])
        r["verified"] = ok
        r["quote_ratio"] = round(ratio, 2)
    for t in top:
        ok, ratio = verify(t["section"], t["quote"])
        t["verified"] = ok
        t["quote_ratio"] = round(ratio, 2)

    # 去重
    def key(x):
        return (x["section"], norm(x["quote"])[:25])
    seen = {}
    dedup = []
    for r in sorted(rows, key=lambda x: ({"P0": 0, "P1": 1, "P2": 2}.get(x["severity"], 3), x["section"])):
        k = key(r)
        if k in seen:
            continue
        seen[k] = 1
        dedup.append(r)
    rows = dedup

    md = ["# 深度审读问题清单（分单元逐节审读汇总）", "",
          f"- 原始问题 {sum(1 for _ in data)} 单元 / 去重后 {len(rows)} 条（P0 {sum(1 for r in rows if r['severity']=='P0')} / "
          f"P1 {sum(1 for r in rows if r['severity']=='P1')} / P2 {sum(1 for r in rows if r['severity']=='P2')}）",
          f"- 原文引用核验通过 {sum(1 for r in rows if r['verified'])} / {len(rows)}（未通过的多为跨行引用或转述，需人工复核）",
          "", "## 章评分", "", "| 章 | 分 | 评语 |", "|---|---|---|"]
    for c in sorted(scores, key=lambda x: (int(x) if x.isdigit() else 99)):
        s, v = scores[c]
        md.append(f"| {c} | {s} | {v} |")
    md += ["", "## P0", ""]
    for r in [x for x in rows if x["severity"] == "P0"]:
        md.append(f"### {r['section']} [{r['type']}] {'✅' if r['verified'] else '⚠️未核到'}")
        md.append(f"- 原文：「{r['quote']}」")
        md.append(f"- 问题：{r['detail']}")
        md.append(f"- 建议：{r['fix']}")
        md.append("")
    md += ["## P1", ""]
    for r in [x for x in rows if x["severity"] == "P1"]:
        md.append(f"### {r['section']} [{r['type']}] {'✅' if r['verified'] else '⚠️未核到'}")
        md.append(f"- 原文：「{r['quote']}」")
        md.append(f"- 问题：{r['detail']}")
        md.append(f"- 建议：{r['fix']}")
        md.append("")
    md += ["## P2（附）", ""]
    for r in [x for x in rows if x["severity"] == "P2"]:
        md.append(f"- {r['section']}：{r['detail'][:120]}（原文：「{r['quote'][:40]}」）")
    md += ["", "## 查缺补漏", ""]
    for g in gaps:
        md.append(f"- **{g['section']}** 缺：{g['missing']}；为什么：{g['why']}；建议：{g['suggest']}")
    md += ["", "## 跨章关系建议", ""]
    for r in rels:
        md.append(f"- {r['section']} → {r['target']}：{r['issue']}")
    md += ["", "## 单元 top issues", ""]
    for t in top:
        md.append(f"- {t['section']} [{t['severity']}] {'✅' if t['verified'] else '⚠️'} 「{t['quote']}」—— {t['issue']}")
    Path(args.out).write_text("\n".join(md) + "\n", encoding="utf-8")
    print(f"issues={len(rows)} (P0={sum(1 for r in rows if r['severity']=='P0')}, P1={sum(1 for r in rows if r['severity']=='P1')}) gaps={len(gaps)} rels={len(rels)} verified={sum(1 for r in rows if r['verified'])}")
    print("→", args.out)


if __name__ == "__main__":
    main()
