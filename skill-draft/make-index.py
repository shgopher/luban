#!/usr/bin/env python3
"""
make-index.py —— 为「鲁班写作法 · 铁律 6」生成全书索引，用查询替代通读。

为什么要有它：通读全书的成本随书稿线性增长（31 万字的书 ≈ 22 万 token），
而定位需要的只是「哪几节和我有关」。索引把这笔钱一次性付掉：

    python3 make-index.py .            # 建索引，写入 _luban/
    python3 make-index.py . -q 幂等 去重   # 查：哪些节在讲这件事
    python3 make-index.py . -d 24.4     # 查下游：谁引用了我
    python3 make-index.py . -c          # 只看章骨架（每次动笔前读这 83 行）

产物（全部自动生成，不要手改）：
    _luban/index.tsv     编号 / 标题 / 一句话 / 字数 / 出度 / 路径
    _luban/chapters.md   篇 → 章 骨架，每章一行（≈0.9k token，用于防漏）
    _luban/refs.tsv      引用图（from → to），算入度用
    _luban/terms.tsv     术语主场表：同一术语在哪些节出现、谁该是主场、比值多少
"""

import argparse
import re
import sys
from pathlib import Path

FILE_NUM = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
H1 = re.compile(r"^#\s+(?:(\d{1,3}\.\d{1,2})\s+)?(.+)$")
ONE_LINE = re.compile(r"^#{2,4}\s*一句话\s*$")
NUM_REF = re.compile(r"(?:见|详见|参见|参阅)\s*\[?(\d{1,3}\.\d{1,2})")
PAREN_REF = re.compile(r"[（(](\d{1,3}\.\d{1,2})[)）]")
CJK_TERM = re.compile(r"[\u4e00-\u9fff]{2,}")
STOP = {
    "什么", "怎么", "如何", "为什么", "哪些", "一个", "两个", "三种", "四种",
    "五种", "实现", "设计", "问题", "方案", "选型", "对比", "本质", "原则",
}


def toc_numbers(root: Path):
    """目录里已规划的节编号（README*.md 的 ### 行），用于判断括号里的编号是不是真引用"""
    nums = set()
    for base in (root, root.parent):
        for readme in sorted(base.glob("README*.md")):
            for line in readme.read_text(encoding="utf-8", errors="ignore").splitlines():
                m = re.match(r"^###\s*\[?(\d{1,3}\.\d{1,2})\b", line.strip())
                if m:
                    nums.add(m.group(1))
        if nums:
            break
    return nums


def num_key(n: str):
    a, b = n.split(".")
    return (int(a), int(b))


def parse(path: Path):
    """抽出一个节的元信息：标题、一句话、字数、出站引用"""
    text = path.read_text(encoding="utf-8", errors="ignore")
    lines = text.splitlines()

    title = FILE_NUM.match(path.name).group(2)
    for line in lines:
        h = H1.match(line.strip())
        if h:
            title = (h.group(2) or "").strip()
            break

    summary, in_fence = "", False
    for i, line in enumerate(lines):
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
        if in_fence or not ONE_LINE.match(line.strip()):
            continue
        for nxt in lines[i + 1: i + 6]:
            nxt = nxt.strip()
            if not nxt:
                continue
            if nxt.startswith(">"):
                summary = nxt.lstrip("> ").strip()
                summary = re.sub(r"^\*\*一句话\*\*[：:]\s*", "", summary)
                summary = re.sub(r"^一句话[：:]\s*", "", summary)
            break
        break

    # 代码块里的引用（决策树、注释）也是真引用，照收
    refs = []
    for line in lines:
        for m in NUM_REF.finditer(line):
            if m.start() and line[m.start() - 1] in "「“":
                continue
            refs.append(m.group(1))
        for m in PAREN_REF.finditer(line):
            refs.append(m.group(1))  # 括号式引用，稍后用「已知编号」过滤

    return {
        "num": FILE_NUM.match(path.name).group(1),
        "title": title,
        "summary": " ".join(summary.split()),
        "chars": len(text),
        "out": sorted(set(refs), key=num_key),
        "refs": refs,
        "path": path,
    }


def clean(tsv_field: str) -> str:
    return tsv_field.replace("\t", " ").replace("\n", " ").strip()


def build(root: Path, out_dir: Path):
    sections = []
    for p in sorted(root.rglob("*.md")):
        if ".git" in p.parts or out_dir in p.parents:
            continue
        if FILE_NUM.match(p.name):
            sections.append(parse(p))
    sections.sort(key=lambda s: num_key(s["num"]))

    known = {s["num"] for s in sections}
    known |= toc_numbers(root)
    for s in sections:
        s["refs"] = [r for r in s["refs"] if r in known and r != s["num"]]
        s["out"] = sorted(set(s["refs"]), key=num_key)

    incoming = {s["num"]: [] for s in sections}
    for s in sections:
        for t in s["out"]:
            incoming.setdefault(t, []).append(s["num"])

    out_dir.mkdir(parents=True, exist_ok=True)

    # index.tsv
    rows = ["编号\t标题\t一句话\t字数\t出度\t被引\t路径"]
    for s in sections:
        rows.append("\t".join([
            s["num"], clean(s["title"]), clean(s["summary"]),
            str(s["chars"]), str(len(s["out"])), str(len(incoming[s["num"]])),
            str(s["path"].relative_to(root)),
        ]))
    (out_dir / "index.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8")

    # refs.tsv
    rows = ["from\tto"] + [f"{s['num']}\t{t}" for s in sections for t in s["out"]]
    (out_dir / "refs.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8")

    # chapters.md（骨架：每次动笔读这一份）
    groups = {}
    for s in sections:
        rel = s["path"].relative_to(root)
        part = rel.parts[0] if len(rel.parts) > 2 else "（未分篇）"
        chap = rel.parts[1] if len(rel.parts) > 2 else rel.parts[0]
        groups.setdefault((part, chap), []).append(s["num"])

    total_chars = sum(s["chars"] for s in sections)
    lines = [
        f"# 章骨架 · {len(groups)} 章 / {len(sections)} 节 / {total_chars:,} 字",
        "",
        "> 自动生成。动笔前只读这一份（防漏），具体节用 `make-index.py -q 关键词` 查。",
        "",
    ]
    last_part = None
    for (part, chap), nums in sorted(groups.items(), key=lambda kv: num_key(min(kv[1], key=num_key))):
        if part != last_part:
            lines.append(f"\n## {part}")
            last_part = part
        nums = sorted(nums, key=num_key)
        name = re.sub(r"^\d+-", "", chap)
        rng = nums[0] if len(nums) == 1 else f"{nums[0]}–{nums[-1]}"
        lines.append(f"- {name}　{rng}（{len(nums)} 节）")
    (out_dir / "chapters.md").write_text("\n".join(lines) + "\n", encoding="utf-8")

    # terms.tsv（主场表）
    bodies = {s["num"]: s["path"].read_text(encoding="utf-8", errors="ignore") for s in sections}
    candidates = {}
    for s in sections:
        for t in CJK_TERM.findall(s["title"]):
            t = t.strip("与和的在是有及或")
            if len(t) >= 2 and t not in STOP:
                candidates[t] = candidates.get(t, 0) + 1
    top = sorted(candidates.items(), key=lambda kv: -kv[1])[:120]

    rows = ["术语\t主场\t主场次数\t次客场\t次客次数\t比值\t结论"]
    for term, _ in top:
        hits = sorted(
            ((n, bodies[n].count(term)) for n in bodies),
            key=lambda kv: -kv[1],
        )[:2]
        if len(hits) < 2 or hits[0][1] < 3 or hits[1][1] == 0:
            continue
        home, hc = hits[0]
        second, sc = hits[1]
        rows.append("\t".join([
            term, home, str(hc), second, str(sc), f"{hc / sc:.1f}",
            "⚠ 主场不明，需显式分工" if hc < sc * 3 else "✓",
        ]))
    (out_dir / "terms.tsv").write_text("\n".join(rows) + "\n", encoding="utf-8")

    print(
        f"✓ 索引已写入 {out_dir}/ ：{len(sections)} 节 · {len(groups)} 章 · {total_chars:,} 字\n"
        f"  index.tsv / chapters.md / refs.tsv / terms.tsv"
    )
    return sections


def load(root: Path, out_dir: Path):
    idx = out_dir / "index.tsv"
    if not idx.exists():
        sys.exit(f"没有索引，先跑：python3 {Path(__file__).name} {root}")
    rows = [l.split("\t") for l in idx.read_text(encoding="utf-8").splitlines()[1:] if l]
    return [dict(zip(["num", "title", "summary", "chars", "out", "in", "path"], r)) for r in rows]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("root", nargs="?", default=".")
    ap.add_argument("-o", "--out", default="_luban", help="索引输出目录（默认 _luban）")
    ap.add_argument("-q", "--query", nargs="+", help="查关键词（编号/标题/一句话，任一命中）")
    ap.add_argument("-d", "--downstream", help="查下游：谁引用了这一节")
    ap.add_argument("-c", "--chapters", action="store_true", help="只打印章骨架")
    args = ap.parse_args()

    root = Path(args.root).resolve()
    out_dir = root / args.out

    if args.chapters and (out_dir / "chapters.md").exists():
        print((out_dir / "chapters.md").read_text(encoding="utf-8"))
        return 0
    if not (args.query or args.downstream) or not (out_dir / "index.tsv").exists():
        build(root, out_dir)

    rows = load(root, out_dir)

    if args.query:
        terms = [t.lower() for t in args.query]
        hit = [
            r for r in rows
            if any(t in (r["num"] + r["title"] + r["summary"]).lower() for t in terms)
        ]
        print(f"命中 {len(hit)} 节：\n")
        for r in hit:
            print(f"{r['num']:>6}  {r['title']}  〔{r['chars']} 字 · 出度 {r['out']} · 被引 {r['in']}〕")
            if r["summary"]:
                print(f"        {r['summary'][:90]}")
        if not hit:
            print("  （无命中）换同义词再查；仍无 → 这一节在目录里缺位，先补目录")

    if args.downstream:
        tgt = args.downstream
        refs = [l.split("\t") for l in (out_dir / "refs.tsv").read_text(encoding="utf-8").splitlines()[1:] if l]
        ups = [f for f, t in refs if t == tgt]
        title = {r["num"]: r["title"] for r in rows}
        print(f"\n引用 {tgt} 的节：{len(ups)} 个" + ("" if ups else " —— 入度为 0，结论没被接住，或位置太靠后"))
        for u in ups:
            print(f"  {u:>6}  {title.get(u, '')}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
