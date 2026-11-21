#!/usr/bin/env python3
"""P0-1 修断表：把被空行截断的表行并回表格。

26 节：反模式表末尾 2 行被空行截断 → 删空行并回。
6 节特殊：0.11 行在正文中间、3.4 行缺空行、20.2/24.3/24.8 行漂到章首、23.1 行重复。

用法：python3 tools/audit/fix_tables.py [--write]
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

MERGE = ['19.9', '20.5', '21.10', '24.1', '25.2', '25.3', '27.2', '27.3', '27.4', '27.5',
         '28.1', '28.2', '28.3', '28.4', '28.5', '31.1', '31.2', '31.3', '31.4', '31.5',
         '32.1', '32.2', '32.3', '32.4', '32.5', '32.6']
SEC_FILE = {}
for line in (ROOT / "_luban" / "index.tsv").read_text(encoding="utf-8").splitlines()[1:]:
    n, title, one, chars, out, inn, path = line.split("\t")
    SEC_FILE[n] = path


def load(num):
    return (ROOT / SEC_FILE[num]).read_text(encoding="utf-8").splitlines()


def save(num, lines):
    (ROOT / SEC_FILE[num]).write_text("\n".join(lines) + "\n", encoding="utf-8")


def is_table_row(s: str) -> bool:
    return s.strip().startswith("|")


def is_sep(s: str) -> bool:
    return bool(re.match(r"^\|[\s:\-|]+\|$", s.strip()))


STRUCT = json.load(open(ROOT / "tools" / "audit" / "out" / "structural.json"))
ORPHANS = {}
for s in STRUCT["sections"]:
    for it in s["issues"]:
        if it["type"] in ("orphan_table_row", "dangling_table_row"):
            ORPHANS.setdefault(s["num"], set()).add(it["line"])


def groups(nums):
    """把行号分成连续组，返回每组首行号（1-based）。"""
    nums = sorted(nums)
    out = []
    for n in nums:
        if not out or n != out[-1][-1] + 1:
            out.append([n])
        else:
            out[-1].append(n)
    return [g[0] for g in out]


def merge_tail(num):
    """只处理扫描器标记的断行：删掉其上方空行、并在需要时补空行。"""
    lines = load(num)
    changed = 0
    for L in sorted(groups(ORPHANS[num]), reverse=True):
        i = L - 1  # 1-based 行号 -> 0-based 索引
        if i - 1 >= 0 and not lines[i - 1].strip():
            del lines[i - 1]
            changed += 1
            # 删除后，表格行从 i-1 开始；若后面紧跟标题，补空行
            j = i - 1
            while j < len(lines) and is_table_row(lines[j]):
                j += 1
            if j < len(lines) and lines[j].strip().startswith("#") and lines[j - 1].strip():
                lines.insert(j, "")
    save(num, lines)
    return changed


def move_top_to_anti(num, rows_text):
    """把章首漂出的 rows 移到 ## 反模式 表末尾。"""
    lines = load(num)
    # 删除 rows（以及前面/后面多余空行）
    removed = 0
    for t in rows_text:
        if t in lines:
            k = lines.index(t)
            del lines[k]
            removed += 1
    # 清理连续空行
    out = []
    for l in lines:
        if not l.strip() and out and not out[-1].strip():
            continue
        out.append(l)
    lines = out
    # 找反模式表最后一个表格行
    ai = next(i for i, l in enumerate(lines) if re.match(r"^##\s*反模式", l.strip()))
    j = ai + 1
    last = None
    while j < len(lines) and not lines[j].strip().startswith("#"):
        if is_table_row(lines[j]):
            last = j
        j += 1
    if last is None:
        raise SystemExit(f"{num}: 找不到反模式表")
    for k, t in enumerate(rows_text):
        lines.insert(last + 1 + k, t)
    save(num, lines)
    return removed


def fix_231():
    lines = load("23.1")
    remove = [
        "| 用 `time.Now()` 做去重键 | 时钟回拨导致键重复/失效 |",
        "| 跨机比较 `created_at` 判断先后 | 漂移让“旧写”覆盖“新写” |",
    ]
    for t in remove:
        if t in lines:
            lines.remove(t)
    out = []
    for l in lines:
        if not l.strip() and out and not out[-1].strip():
            continue
        out.append(l)
    save("23.1", out)
    return len(remove)


def fix_011():
    lines = load("0.11")
    row = "| 实现依赖 | **默认直接依赖**，不抽象 | “如无必要，勿增实体”；抽象 = 配置化，可配置性↑、学习成本↑ |"
    lines.remove(row)
    idx = next(i for i, l in enumerate(lines) if "使用界面依赖" in l and l.strip().startswith("|"))
    lines.insert(idx + 1, row)
    save("0.11", lines)
    return 1


def fix_34():
    lines = load("3.4")
    row = "| 把 “行业惯例” 当 “合规要求” | 按旧惯例实现，新规一出就返工 |"
    lines.remove(row)
    # 找反模式表最后一个表格行
    ai = next(i for i, l in enumerate(lines) if re.match(r"^##\s*反模式", l.strip()))
    j = ai + 1
    last = None
    while j < len(lines) and not lines[j].strip().startswith("#"):
        if is_table_row(lines[j]):
            last = j
        j += 1
    lines.insert(last + 1, row)
    # 确保 row 与下一个标题之间有空行
    k = last + 2
    if k < len(lines) and lines[k].strip().startswith("#"):
        lines.insert(k, "")
    save("3.4", lines)
    return 1


def main():
    write = "--write" in sys.argv
    if not write:
        print("dry-run；加 --write 生效")
        return
    for num in MERGE:
        n = merge_tail(num)
        print(num, "merged", n)
    for num, rows in {
        "20.2": ["| 锁服务与业务共用集群 | 锁抖动直接放大为业务故障 |",
                 "| 不监控锁等待时长 | 死锁/长持锁只能靠用户投诉发现 |"],
        "24.3": ["| 把 3PC 当“2PC 升级版”直接替换 | 多一个 RTT，且分区下仍不安全 |",
                 "| 不给 `canCommit` 设超时 | 协调者等不到回复，长时间挂起 |"],
        "24.8": ["| 通知体里带全量业务数据 | 数据泄露 + 体积膨胀，失败重传更贵 |",
                 "| 不同事件共用幂等键 | 后到的事件被误判为重复 |"],
    }.items():
        print(num, "moved", move_top_to_anti(num, rows))
    print("23.1 removed", fix_231())
    print("0.11 fixed", fix_011())
    print("3.4 fixed", fix_34())


if __name__ == "__main__":
    main()
