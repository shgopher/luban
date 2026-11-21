#!/usr/bin/env python3
"""P1-c：规范「## 关联」块。

1) 同一类（前置/并列/下游）多行合并为一行，去重；
2) 关联块若出现在最后一个自检题之前（后期追加 Q 造成），移动到文末（--- 之前）；
3) 删除过期的「（未写，无链接）」。
目标：每个关联块 ≤4 行（前置/并列/下游各一行）。

用法：python3 tools/audit/fix_related_blocks.py [--write]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BULLET = re.compile(r"^-\s*(前置|并列|下游|待补)[：:]\s*(.*)$")
LINK = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")
TARGET = re.compile(r"(\d{1,3}\.\d{1,2})[-)]")


def extract_block(lines, start):
    """从 ## 关联 行开始，返回 (end_index, items[list of (cat, text)])."""
    items = []
    j = start + 1
    seen = False
    while j < len(lines):
        s = lines[j].strip()
        if s.startswith("#") or s == "---":
            break
        if not s:
            j += 1
            continue
        m = BULLET.match(s)
        if m:
            seen = True
            items.append((m.group(1), m.group(2).strip()))
            j += 1
            continue
        if seen:
            break  # 关联块结束后跟了自检题/正文：不吞
        j += 1
    return j, items


def normalize_items(items):
    """合并同类、按链接目标去重、删除无链接的过期项。"""
    groups = {"前置": [], "并列": [], "下游": []}
    seen = set()
    for cat, text in items:
        if cat not in groups:
            continue
        links = LINK.findall(text)
        if not links:
            continue
        for label, href in links:
            m = TARGET.search(href) or TARGET.search(label)
            key = m.group(1) if m else label
            if key in seen:
                continue
            seen.add(key)
            groups[cat].append(f"[{label}]({href})")
    return groups


def process(num_path):
    p = ROOT / num_path
    lines = p.read_text(encoding="utf-8").splitlines()
    starts = [i for i, l in enumerate(lines) if re.match(r"^##\s*关联\s*$", l.strip())]
    if not starts:
        return 0
    start = starts[0]
    end, items = extract_block(lines, start)
    groups = normalize_items(items)
    block = ["## 关联", ""]
    for k in ("前置", "并列", "下游"):
        if groups[k]:
            block.append(f"- {k}：" + "、".join(groups[k]))
    # 删除旧块（连同块后空行）
    del lines[start:end]
    # 清理尾部空行
    while lines and not lines[-1].strip():
        lines.pop()
    # 放到末尾 --- 之前
    sep = None
    for i in range(len(lines) - 1, -1, -1):
        if lines[i].strip() == "---":
            sep = i
            break
    if sep is not None:
        lines[sep:sep] = [""] + block + [""]
    else:
        lines += [""] + block
    # 规范空行
    out = []
    for l in lines:
        if not l.strip() and out and not out[-1].strip():
            continue
        out.append(l)
    p.write_text("\n".join(out) + "\n", encoding="utf-8")
    return 1


def main():
    write = "--write" in sys.argv
    changed = 0
    for p in (ROOT / "v2").rglob("*.md"):
        if not re.match(r"^\d{1,3}\.\d{1,2}-", p.name):
            continue
        if write:
            changed += process(p.relative_to(ROOT))
        else:
            t = p.read_text(encoding="utf-8")
            if re.search(r"^##\s*关联\s*$", t, re.M):
                changed += 1
    print(("changed" if write else "has block"), changed)


if __name__ == "__main__":
    main()
