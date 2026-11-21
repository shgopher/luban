#!/usr/bin/env python3
"""全书结构审计：目录/文件/索引一致性 + 每节结构完整性 + 表格/围栏/链接。

输出：
  <out>/structural.json   机器可读明细
  <out>/structural.md     人读摘要
用法：python3 tools/audit/structural_audit.py [book_root] [-o out_dir]
"""
import argparse
import csv
import json
import re
import sys
from collections import Counter
from pathlib import Path

SEC_RE = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
H1_RE = re.compile(r"^#\s+(?:(\d{1,3}\.\d{1,2})\s+)?(.+)$")
H2_RE = re.compile(r"^#{2,3}\s+(.+)$")
Q_LINE = re.compile(r"^\s*(?:\*\*)?Q(\d+)(?:\*\*)?\s*[\.、:：]?\s*(.*)$")
ANS_RE = re.compile(r"^\s*▸\s*答案")
FENCE_RE = re.compile(r"^\s*```")
SEP_RE = re.compile(r"^\s*\|[\s:|\-]+\|\s*$")
LINK_RE = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")
TOC_LINE = re.compile(r"^###\s*\[?(\d{1,3}\.\d{1,2})\s+([^\]\)]*)")


def mark_fences(lines):
    """返回每行是否在代码围栏内（围栏标记行本身算内部）。"""
    inside = []
    cur = False
    for l in lines:
        if FENCE_RE.match(l):
            inside.append(True)
            cur = not cur
        else:
            inside.append(cur)
    return inside, cur


def ncols(line: str) -> int:
    s = re.sub(r"`[^`]*`", "CODE", line)
    s = s.replace("\\|", "PIPE")
    return s.count("|")


def check_tables(lines, inside):
    """只检查围栏外的表格：列数不一致、悬空/孤立行。"""
    issues = []
    i, n = 0, len(lines)
    in_table = False
    expect = 0
    while i < n:
        line, stripped = lines[i], lines[i].strip()
        if inside[i]:
            in_table = False
            i += 1
            continue
        if not stripped:
            if in_table:
                j = i + 1
                while j < n and not lines[j].strip():
                    j += 1
                if (j < n and not inside[j] and lines[j].lstrip().startswith("|")):
                    # 空行后新表：必须有 header + 分隔行，否则是断掉的残留行
                    ok_new_table = (j + 1 < n and not inside[j + 1]
                                    and SEP_RE.match(lines[j + 1].strip()))
                    if not ok_new_table:
                        issues.append({"type": "dangling_table_row", "line": j + 1,
                                       "text": lines[j].strip()[:80]})
            in_table = False
            i += 1
            continue
        if stripped.startswith("|"):
            if not in_table:
                if i + 1 < n and not inside[i + 1] and SEP_RE.match(lines[i + 1].strip()):
                    in_table = True
                    expect = ncols(stripped)
                else:
                    issues.append({"type": "orphan_table_row", "line": i + 1,
                                   "text": stripped[:80]})
            elif SEP_RE.match(stripped):
                if ncols(stripped) != expect:
                    issues.append({"type": "table_sep_mismatch", "line": i + 1,
                                   "expect": expect, "got": ncols(stripped)})
            elif ncols(stripped) != expect:
                issues.append({"type": "table_col_mismatch", "line": i + 1,
                               "expect": expect, "got": ncols(stripped),
                               "text": stripped[:60]})
            i += 1
            continue
        in_table = False
        i += 1
    return issues


def section_body(lines, inside, start, stop):
    return [(k, lines[k]) for k in range(start, stop) if not inside[k]]


def check_section(path: Path, root: Path):
    text = path.read_text(encoding="utf-8", errors="ignore")
    lines = text.splitlines()
    inside, fence_odd = mark_fences(lines)
    num = path.name.split("-")[0]
    issues = []
    visible = [(k, l) for k, l in enumerate(lines) if not inside[k]]

    # 标题
    h1s = [(k, l) for k, l in visible if H1_RE.match(l.strip())]
    h1_title = ""
    if len(h1s) != 1:
        issues.append({"type": "h1_count", "n": len(h1s)})
    if h1s:
        m = H1_RE.match(h1s[0][1].strip())
        h1_title = (m.group(2) or "").strip()
    if fence_odd:
        issues.append({"type": "fence_unbalanced"})

    # 表格
    for t in check_tables(lines, inside):
        t["file"] = str(path.relative_to(root))
        issues.append(t)

    # 一句话（## 或 ###）
    yij = [(k, l) for k, l in visible
           if H2_RE.match(l.strip()) and re.match(r"^#{2,3}\s*一句话\s*$", l.strip())]
    if len(yij) != 1:
        issues.append({"type": "yijuhua_count", "n": len(yij)})
    else:
        start = yij[0][0] + 1
        quote = []
        i = start
        while i < len(lines) and not (not inside[i] and lines[i].strip().startswith("#")):
            if not inside[i] and lines[i].strip().startswith(">"):
                quote.append(lines[i].strip())
            elif not inside[i] and lines[i].strip():
                break
            i += 1
        if not quote:
            issues.append({"type": "yijuhua_empty"})
        elif len(quote) > 1:
            issues.append({"type": "yijuhua_multiline", "n": len(quote)})

    # 自检
    zj = [(k, l) for k, l in visible if re.match(r"^#{2,3}\s*自检\s*$", l.strip())]
    if not zj:
        issues.append({"type": "no_selftest"})
    else:
        start = zj[0][0]
        end = len(lines)
        for j in range(start + 1, len(lines)):
            if not inside[j] and (lines[j].strip().startswith("## ") or lines[j].strip() == "---"):
                end = j
                break
        seg = [(k, l) for k, l in lines[start:end] if not inside[start:end][k - start]] if False else \
              [(k, lines[k]) for k in range(start, end) if not inside[k]]
        qs = []
        for k, l in seg:
            m = Q_LINE.match(l)
            if m:
                qs.append((m.group(1), k))
        if not qs:
            issues.append({"type": "no_question"})
        else:
            nums = [int(q[0]) for q in qs]
            if nums != list(range(1, len(nums) + 1)):
                issues.append({"type": "q_numbering", "nums": nums})
            miss = []
            for qi, (qn, k) in enumerate(qs):
                nxt = qs[qi + 1][1] if qi + 1 < len(qs) else end
                if not any(ANS_RE.match(lines[t]) for t in range(k, nxt) if not inside[t]):
                    miss.append(qn)
            if miss:
                issues.append({"type": "q_no_answer", "nums": miss})

    # 关联块
    gl = [(k, l) for k, l in visible if re.match(r"^#{2,3}\s*关联\s*$", l.strip())]
    if not gl:
        issues.append({"type": "no_related"})
    else:
        start = gl[0][0]
        seg = []
        j = start + 1
        while j < len(lines) and not (not inside[j] and (lines[j].strip().startswith("#") or lines[j].strip() == "---")):
            if not inside[j] and lines[j].strip():
                seg.append(lines[j].strip())
            j += 1
        if len(seg) > 4:
            issues.append({"type": "related_too_long", "n": len(seg)})
        for l in seg:
            for label, href in LINK_RE.findall(l):
                if href.startswith(("http", "#", "mailto")):
                    continue
                target = (path.parent / href).resolve()
                if not target.exists():
                    issues.append({"type": "related_link_broken", "href": href})
                tm = SEC_RE.match(target.name)
                if tm and tm.group(1) == num:
                    issues.append({"type": "related_self_link", "label": label})
                m = re.search(r"(\d{1,3}\.\d{1,2})", label)
                if m and m.group(1) == num:
                    issues.append({"type": "related_self_link", "label": label})

    # 反模式
    anti = [(k, l) for k, l in visible if re.match(r"^#{2,4}\s*反模式", l.strip())]
    if not anti:
        issues.append({"type": "no_anti"})

    # 占位/待核实/延后
    for i, l in visible:
        if re.search(r"待补(?![偿充])", l):
            issues.append({"type": "todo_placeholder", "line": i + 1, "text": l.strip()[:100]})
        if re.search(r"以文档为准", l):
            issues.append({"type": "doc_defer", "line": i + 1, "text": l.strip()[:80]})

    # 链接存在性（围栏外）
    for i, l in visible:
        for label, href in LINK_RE.findall(l):
            if href.startswith(("http", "#", "mailto")):
                continue
            if not (path.parent / href).resolve().exists():
                issues.append({"type": "link_broken", "line": i + 1, "href": href})

    chars = len(text)
    if chars < 1500:
        issues.append({"type": "too_short", "chars": chars})
    return {"num": num, "file": str(path.relative_to(root)),
            "title": h1_title or path.stem.split("-", 1)[1], "chars": chars, "issues": issues}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("root", nargs="?", default=".")
    ap.add_argument("-o", "--out", default="tools/audit/out")
    args = ap.parse_args()
    root = Path(args.root).resolve()
    out = root / args.out
    out.mkdir(parents=True, exist_ok=True)

    idx = {}
    idx_path = root / "_luban" / "index.tsv"
    if not idx_path.exists():
        idx_path = root / "v2" / "_luban" / "index.tsv"
    if idx_path.exists():
        for r in csv.DictReader(open(idx_path), delimiter="\t"):
            idx[r["编号"]] = r

    secs, guo = {}, []
    for p in (root / "v2").rglob("*.md"):
        if "_luban" in p.parts or "tools" in p.parts:
            continue
        m = SEC_RE.match(p.name)
        if m:
            secs[m.group(1)] = p
        elif p.name == "0-概论.md":
            guo.append(p)

    results = [check_section(p, root) for n, p in sorted(secs.items())]
    for r in results:
        if idx.get(r["num"]) and idx[r["num"]]["标题"].strip() != r["title"]:
            r["issues"].append({"type": "h1_vs_index", "h1": r["title"],
                                "index": idx[r["num"]]["标题"]})

    # README 目录
    readme_issues, toc = [], {}
    rp = root / "README.md"
    if rp.exists():
        for line in rp.read_text(encoding="utf-8").splitlines():
            m = TOC_LINE.match(line.strip())
            if m:
                toc.setdefault(m.group(1), m.group(2).strip())
            lk = re.match(r"^###\s*\[([^\]]*)\]\(([^)]+)\)", line.strip())
            if lk:
                label, href = lk.group(1), lk.group(2)
                if not (root / href).resolve().exists():
                    readme_issues.append({"type": "toc_broken", "section": label, "href": href})
                hm = SEC_RE.match(Path(href).name)
                if hm:
                    lt = re.sub(r"^\d{1,3}\.\d{1,2}\s*", "", label).strip()
                    ft = (idx.get(hm.group(1)) or {}).get("标题", hm.group(2)).strip()
                    if lt and ft and lt != ft and lt.replace("(", "（").replace(")", "）") != ft.replace("(", "（").replace(")", "）"):
                        readme_issues.append({"type": "toc_title_mismatch", "num": hm.group(1),
                                              "toc": lt, "file": ft})
                    elif lt != ft:
                        readme_issues.append({"type": "toc_paren_width", "num": hm.group(1),
                                              "toc": lt, "file": ft})

    toc_missing = [n for n in secs if n not in toc]
    toc_extra = [n for n in toc if n not in secs]
    ch_dirs = {}
    for n, p in secs.items():
        ch_dirs.setdefault(n.split(".")[0], p.parent)
    guo_missing = [c for c, d in ch_dirs.items() if not (d / "0-概论.md").exists()]
    guo_chars = {p.parent.name: len(p.read_text(encoding='utf-8')) for p in guo}

    cnt, per_sec = Counter(), {}
    for r in results:
        for it in r["issues"]:
            cnt[it["type"]] += 1
            per_sec.setdefault(it["type"], []).append(r["num"])

    summary = {
        "sections": len(secs), "chapters": len(ch_dirs), "guo": len(guo),
        "total_chars": sum(r["chars"] for r in results),
        "issue_counts": dict(cnt.most_common()),
        "readme_issues": readme_issues, "toc_missing": toc_missing,
        "toc_extra": toc_extra, "guo_missing": guo_missing,
        "guo_chars": guo_chars,
        "per_type_sections": {k: sorted(v, key=lambda s: tuple(int(x) for x in s.split(".")))
                              for k, v in per_sec.items()},
    }
    (out / "structural.json").write_text(
        json.dumps({"summary": summary, "sections": results}, ensure_ascii=False, indent=1),
        encoding="utf-8")

    lines = ["# 结构审计", "",
             f"- 节：{len(secs)}；章：{len(ch_dirs)}；概论：{len(guo)}；总字数：{summary['total_chars']:,}",
             f"- README 目录：问题 {len(readme_issues)}；目录缺节 {len(toc_missing)}；目录多节 {len(toc_extra)}",
             f"- 缺概论章：{guo_missing}", "", "## 问题类型统计", ""]
    for k, v in cnt.most_common():
        lines.append(f"- {k}: {v}  {per_sec[k][:20]}{' …' if len(per_sec[k]) > 20 else ''}")
    lines += ["", "## README 目录问题", ""]
    for x in readme_issues:
        lines.append(f"- {x}")
    lines += ["", "## 每节问题明细", ""]
    for r in results:
        if not r["issues"]:
            continue
        types = {}
        for it in r["issues"]:
            types.setdefault(it["type"], []).append(it)
        desc = []
        for t, its in types.items():
            if t in ("q_numbering",):
                desc.append(f"{t}:{its[0]['nums']}")
            elif "line" in its[0]:
                vals = "；".join(str(x.get("text", x.get("href", x.get("label", ""))))[:35] for x in its[:3])
                desc.append(f"{t}@{its[0]['line']}:{vals}")
            else:
                desc.append(t)
        lines.append(f"- {r['num']} {r['title']} 〔{r['chars']}字〕 — {'；'.join(desc)}")
    (out / "structural.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    print("\n".join(lines[:45]))
    print(f"\n→ {out}/structural.json / structural.md")
    return 0


if __name__ == "__main__":
    sys.exit(main())
