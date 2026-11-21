#!/usr/bin/env python3
"""P1-a：把「跨篇相关：」段落/正文链接规范为「## 关联」块。

- 62 节有「跨篇相关：」段落：整段删除，按书内顺序拆成 前置/并列/下游 三行；
- 26 节无段落但有正文链接：取不超过 6 条，生成同样三行；
- 9 节没有任何链接：由人工清单（MANUAL）补。

用法：python3 tools/audit/fix_related.py [--write]
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STRUCT = json.load(open(ROOT / "tools/audit/out/structural.json"))
PATHS = {x["num"]: x["file"] for x in STRUCT["sections"]}
NO_REL = STRUCT["summary"]["per_type_sections"].get("no_related", [])

ORDER = {}
IDX = {}
for i, line in enumerate((ROOT / "_luban" / "index.tsv").read_text(encoding="utf-8").splitlines()[1:]):
    n, title, one, chars, out, inn, path = line.split("\t")
    ORDER[n] = i
    IDX[n] = title

LINK = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")
TARGET = re.compile(r"(\d{1,3}\.\d{1,2})-")


def num(href):
    m = TARGET.search(href)
    return m.group(1) if m else None


def classify(cur, tgt):
    if tgt.split(".")[0] == cur.split(".")[0]:
        return "并列"
    return "前置" if ORDER.get(tgt, 0) < ORDER.get(cur, 0) else "下游"


def block_for(cur, pairs):
    """pairs: [(label, href, target)] -> 3 行块。"""
    groups = {"前置": [], "并列": [], "下游": []}
    seen = set()
    for label, href, tgt in pairs:
        if not tgt or tgt == cur or tgt in seen:
            continue
        seen.add(tgt)
        href = href[2:] if href.startswith("./") else href
        groups[classify(cur, tgt)].append(f"[{tgt} {IDX.get(tgt, label)}]({href})")
    lines = ["## 关联", ""]
    for k in ("前置", "并列", "下游"):
        if groups[k]:
            lines.append(f"- {k}：" + "、".join(groups[k]))
    return lines


def take_links(text, limit=6, prefer_cross=True):
    pairs = []
    for label, href in LINK.findall(text):
        tgt = num(href)
        if not tgt:
            continue
        pairs.append((label.strip(), href, tgt))
    # 去重、跨章优先、保持原顺序
    seen, out = set(), []
    if prefer_cross:
        pairs.sort(key=lambda p: (p[2].split(".")[0] == "0",))
    for p in pairs:
        if p[2] in seen:
            continue
        seen.add(p[2])
        out.append(p)
        if len(out) >= limit:
            break
    return out


MANUAL = {
    # 无正文链接的节：按内容人工给关系（目标必须是全书已有节）
    "1.1": [("0.1 复杂度分析", "../../内功/0-内功/0.1-复杂度分析.md", "0.1"),
            ("1.5 需求到技术的翻译", "1.5-需求到技术的翻译.md", "1.5")],
    "1.2": [("1.5 需求到技术的翻译", "1.5-需求到技术的翻译.md", "1.5"),
            ("2.4 从业务模型到系统边界", "../2-业务与领域建模/2.4-从业务模型到系统边界.md", "2.4")],
    "4.1": [("4.2 成本-收益分析", "4.2-成本-收益分析.md", "4.2"),
            ("4.6 决策框架与决策记录", "4.6-决策框架与决策记录.md", "4.6")],
    "4.2": [("4.1 技术投入的收益模型", "4.1-技术投入的收益模型.md", "4.1"),
            ("4.6 决策框架与决策记录", "4.6-决策框架与决策记录.md", "4.6")],
    "24.5": [("24.4 TCC", "24.4-TCC.md", "24.4"),
             ("24.9 事务模型选型", "24.9-事务模型选型.md", "24.9")],
    "34.5": [("34.1 性能指标 (延迟 / 吞吐 / P99)", "34.1-性能指标.md", "34.1"),
             ("34.2 性能建模 (排队论 / USL)", "34.2-性能建模.md", "34.2")],
    "53.1": [("53.2 单元测试", "53.2-单元测试.md", "53.2"),
             ("53.3 集成测试", "53.3-集成测试.md", "53.3")],
    "53.3": [("53.1 测试金字塔", "53.1-测试金字塔.md", "53.1"),
             ("53.4 端到端测试", "53.4-端到端测试.md", "53.4")],
}


def fix(num_):
    p = ROOT / PATHS[num_]
    text = p.read_text(encoding="utf-8")
    lines = text.splitlines()
    if re.search(r"^##\s*关联", text, re.M):
        return "already"
    # 1) 跨篇相关段落
    para_idx = [i for i, l in enumerate(lines) if l.strip().startswith("跨篇相关")]
    if para_idx:
        i = para_idx[0]
        pairs = take_links(lines[i], limit=99)
        # 段落可能是多行（到下一个空行/标题为止）
        j = i
        while j < len(lines) and lines[j].strip() and not lines[j].startswith("#"):
            j += 1
        del lines[i:j]
        # 去掉多余空行
        if i < len(lines) and not lines[i].strip() and i > 0 and not lines[i - 1].strip():
            del lines[i]
        block = block_for(num_, pairs)
    else:
        pairs = MANUAL.get(num_)
        if pairs is None:
            pairs = take_links(text, limit=6)
        block = block_for(num_, pairs)
    if len(block) <= 2:
        return "empty"
    # 插在末尾 --- 之前；没有 --- 就追加到末尾
    sep = None
    for i in range(len(lines) - 1, -1, -1):
        if lines[i].strip() == "---":
            sep = i
            break
    if sep is not None:
        lines[sep:sep] = [""] + block + [""]
    else:
        lines += [""] + block + [""]
    # 规范化连续空行
    out = []
    for l in lines:
        if not l.strip() and out and not out[-1].strip():
            continue
        out.append(l)
    p.write_text("\n".join(out) + "\n", encoding="utf-8")
    return "ok"


def main():
    write = "--write" in sys.argv
    counts = {"ok": 0, "already": 0, "empty": 0}
    for n in NO_REL:
        if write:
            r = fix(n)
            counts[r] += 1
            if r == "empty":
                print("EMPTY", n)
        else:
            print(n, "->", "would fix")
    if write:
        print(counts)


if __name__ == "__main__":
    main()
