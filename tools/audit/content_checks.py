#!/usr/bin/env python3
"""全书内容横切检查：重复段落、重复一句话、重复代码、过期待补、版本声明、实测标记。

用法：python3 tools/audit/content_checks.py [book_root]
输出：<out>/content_checks.json + content_checks.md
"""
import argparse
import hashlib
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

SEC_RE = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
FENCE_RE = re.compile(r"^\s*```")
LINK_RE = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")
NUMREF = re.compile(r"(\d{1,3}\.\d{1,2})")


def norm(s: str) -> str:
    s = re.sub(r"\s+", "", s)
    s = re.sub(r"[“”\"'‘’《》「」]", "", s)
    return s


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("root", nargs="?", default=".")
    ap.add_argument("-o", "--out", default="tools/audit/out")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    out = root / args.out
    out.mkdir(parents=True, exist_ok=True)

    secs = {}
    for p in (root / "v2").rglob("*.md"):
        if "_luban" in p.parts:
            continue
        m = SEC_RE.match(p.name)
        if m:
            secs[m.group(1)] = p

    # 1) 重复段落
    para_map = defaultdict(list)
    one_map = defaultdict(list)
    code_map = defaultdict(list)
    for num, p in secs.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        lines = text.splitlines()
        inside, cur_code, code_start = False, [], 0
        para = []
        for i, l in enumerate(lines):
            if FENCE_RE.match(l):
                if inside and len(cur_code) >= 8:
                    h = hashlib.md5(norm("\n".join(cur_code)).encode()).hexdigest()
                    code_map[h].append((num, i + 1, "\n".join(cur_code[:2])[:80]))
                inside = not inside
                cur_code = []
                continue
            if inside:
                cur_code.append(l)
                continue
            s = l.strip()
            if not s or s.startswith("#") or s.startswith(">") or s.startswith("|") or s == "---":
                if len(para) >= 2:
                    t = "".join(para)
                    if len(norm(t)) >= 80:
                        para_map[norm(t)].append((num, t[:100]))
                para = []
                continue
            para.append(s)
        if len(para) >= 2:
            t = "".join(para)
            if len(norm(t)) >= 80:
                para_map[norm(t)].append((num, t[:100]))
        # 一句话
        m = re.search(r"^#{2,3}\s*一句话\s*$", text, re.M)
        if m:
            tail = text[m.end():]
            q = re.search(r"^\s*>\s*(.+)$", tail, re.M)
            if q:
                one_map[norm(q.group(1))].append(num)

    dup_paras = [v for v in para_map.values() if len(v) >= 2]
    dup_paras.sort(key=lambda v: -len(v))
    dup_ones = [v for v in one_map.values() if len(v) >= 2]
    dup_codes = [v for v in code_map.values() if len(v) >= 2]

    # 2) 过期待补
    stale = []
    for num, p in secs.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        for i, line in enumerate(text.splitlines()):
            if "待补" not in line:
                continue
            # 找最近的编号
            nums = NUMREF.findall(line)
            target = nums[-1] if nums else None
            # 行内链接的目标
            for label, href in LINK_RE.findall(line):
                hm = re.search(r"(\d{1,3}\.\d{1,2})-", href)
                if hm:
                    target = hm.group(1)
            exists = target in secs if target else None
            stale.append({"section": num, "line": i + 1, "target": target,
                          "target_exists": exists, "text": line.strip()[:160]})
    stale_stale = [s for s in stale if s["target_exists"]]

    # 3) 版本声明
    vers = defaultdict(list)
    for num, p in secs.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        for m in re.finditer(r"\b(?:go|Go|golang)\s?1\.\d+(?:\.\d+)?\b", text):
            vers[m.group(0)].append(num)
        for m in re.finditer(r"\b(?:linux|kernel)\s?\d+\.\d+(?:\.\d+)?\b", text, re.I):
            vers[m.group(0)].append(num)
    ver_summary = {k: sorted(set(v)) for k, v in vers.items()}

    # 4) 实测标记与环境说明
    measured = []
    for num, p in secs.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        hits = re.findall(r"本机实测|实测输出|实测：|实测结果|跑出来", text)
        if hits:
            # 抓环境说明
            env = re.findall(r"(go\d+\.\d+(?:\.\d+)?[^\n]{0,40}|Apple M\d[^\n]{0,30}|[Dd]arwin (?:arm64|x86_64)|darwin/[a-z0-9]+|linux/[a-z0-9]+|内核 \d[^\n]{0,20}|\d+ ?核)", text)
            measured.append({"section": num, "hits": len(hits), "env": list(dict.fromkeys(env))[:4]})
    no_env = [m for m in measured if not m["env"]]

    # 5) 可疑空话
    words = ["非常重要", "极其关键", "众所周知", "不言而喻", "显然", "毫无疑问", "极大地", "大大提升"]
    empty_talk = defaultdict(list)
    for num, p in secs.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        for w in words:
            c = text.count(w)
            if c:
                empty_talk[w].append((num, c))

    # 6) 代码块统计
    code_stats = []
    for num, p in secs.items():
        text = p.read_text(encoding="utf-8", errors="ignore")
        blocks = re.findall(r"```([a-zA-Z0-9_+-]*)\n", text)
        langs = Counter(blocks)
        code_stats.append({"section": num, "blocks": len(blocks), "langs": dict(langs)})
    no_code = [c for c in code_stats if c["blocks"] == 0]

    result = {
        "dup_paragraphs": [{"count": len(v), "sections": [x[0] for x in v],
                            "sample": v[0][1]} for v in dup_paras[:60]],
        "dup_ones": [{"count": len(v), "sections": v} for v in dup_ones],
        "dup_codes": [{"count": len(v), "sections": [x[0] for x in v],
                       "sample": v[0][2]} for v in dup_codes[:40]],
        "todo": stale,
        "todo_stale": stale_stale,
        "versions": ver_summary,
        "measured": measured,
        "measured_no_env": no_env,
        "empty_talk": {k: v for k, v in empty_talk.items()},
        "no_code": [c["section"] for c in no_code],
        "lang_dist": dict(Counter(
            lang for c in code_stats for lang in c["langs"] for _ in range(c["langs"][lang])).most_common()),
    }
    json.dump(result, open(out / "content_checks.json", "w"), ensure_ascii=False, indent=1)

    md = ["# 内容横切检查", ""]
    md += ["## 重复段落（跨节完全相同，>=80 字）", ""]
    for d in dup_paras[:40]:
        md.append(f"- {d[0][0]} 与 {', '.join(x[0] for x in d[1:])} —— {d[0][1][:60]}")
    md += ["", "## 重复的一句话", ""]
    for d in dup_ones:
        md.append(f"- {', '.join(d[1])}")
    md += ["", "## 重复代码块（>=8 行且完全相同）", ""]
    for d in dup_codes[:30]:
        md.append(f"- {', '.join(x[0] for x in d[1:])} —— {d[1][0][2][:50]}")
    md += ["", "## 过期待补（目标节已存在却还标待补）", ""]
    for s in stale_stale:
        md.append(f"- {s['section']}:{s['line']} → {s['target']} —— {s['text'][:100]}")
    md += ["", "## 版本声明", ""]
    for k, v in ver_summary.items():
        md.append(f"- {k}: {v[:12]}{' …' if len(v) > 12 else ''}")
    md += ["", "## 实测标记但无环境说明", ""]
    md.append("- " + ", ".join(m["section"] for m in no_env) if no_env else "- 无")
    md += ["", "## 空话统计", ""]
    for k, v in empty_talk.items():
        md.append(f"- {k}: {len(v)} 节  {v[:10]}")
    md += ["", f"## 无代码块的节（{len(no_code)}）", "", "- " + ", ".join(c["section"] for c in no_code)]
    (out / "content_checks.md").write_text("\n".join(md) + "\n", encoding="utf-8")
    print("\n".join(md[:60]))
    print("\n→", out / "content_checks.md")


if __name__ == "__main__":
    main()
