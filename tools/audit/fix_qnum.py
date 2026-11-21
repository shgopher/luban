#!/usr/bin/env python3
"""P0-2 修自检 Q 编号：把每节「## 自检」段内的 Q 编号按出现顺序重排为 Q1..Qn。

用法：python3 tools/audit/fix_qnum.py [--write]
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STRUCT = json.load(open(ROOT / "tools/audit/out/structural.json"))
TARGETS = []
for s in STRUCT["sections"]:
    for it in s["issues"]:
        if it["type"] == "q_numbering":
            TARGETS.append(s["num"])
            break
SEC_FILE = {}
for line in (ROOT / "_luban/index.tsv").read_text(encoding="utf-8").splitlines()[1:]:
    n, title, one, chars, out, inn, path = line.split("\t")
    SEC_FILE[n] = path

Q_RE = re.compile(r"^(\s*)(\*\*)?Q(\d+)(\*\*)?(.*)$")


def fix(num):
    p = ROOT / SEC_FILE[num]
    lines = p.read_text(encoding="utf-8").splitlines()
    # 找自检段
    start = None
    for i, l in enumerate(lines):
        if re.match(r"^##\s*自检\s*$", l.strip()):
            start = i
            break
    if start is None:
        return 0, "no selftest"
    end = len(lines)
    for j in range(start + 1, len(lines)):
        if re.match(r"^##\s", lines[j].strip()) or lines[j].strip() == "---":
            end = j
            break
    seq = 0
    changed = 0
    for k in range(start, end):
        m = Q_RE.match(lines[k])
        if not m:
            continue
        seq += 1
        indent, b1, old, b2, rest = m.groups()
        new = f"{indent}{b1 or ''}Q{seq}{b2 or ''}{rest}"
        if new != lines[k]:
            changed += 1
        lines[k] = new
    p.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return changed, seq


def main():
    write = "--write" in sys.argv
    for num in TARGETS:
        if write:
            ch, seq = fix(num)
            print(f"{num}: renumbered {ch} / {seq} questions")
        else:
            print(f"{num}: would renumber")


if __name__ == "__main__":
    main()
