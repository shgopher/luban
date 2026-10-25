#!/usr/bin/env python3
"""把「编号 + 标题」的跨篇引用变成可点击的相对路径链接。

  见 8.2 拥塞控制               → 见 [8.2 拥塞控制](8.2-拥塞控制-Nagle与延迟.md)
  （见 40.2 耦合与内聚度量，待补） → （见 [40.2 耦合与内聚度量](…)，待补）
  （26.3、26.5）                → （[26.3 整洁架构](…)、[26.5 六边形架构](…)）
  本地消息表（24.6）            → 本地消息表（[24.6](…)）  ← 前文已说标题，链接文字只留编号

用法：
  python3 linkify-refs.py <书稿根> --dry-run
  python3 linkify-refs.py <书稿根>
"""
import os
import re
import sys
from pathlib import Path

FILE_NUM = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
H1 = re.compile(r"^#\s+(?:\d{1,3}\.\d{1,2}\s+)?(.+)$")
NUM = r"\d{1,3}\.\d{1,2}"
SEEN = re.compile(r"(见|详见|参见|参阅)([ \u3000]*)(" + NUM + r")")
GROUP = re.compile(r"[（(][^（）()]{1,80}[）)]")
INNER_NUM = re.compile(r"^\s*(?:见\s*)?" + NUM)
STOP = "，。；、）)（(|｜\t \u3000]"
TODO = re.compile(r"^[，,]\s*待补|[（(]待补[）)]")
CHAIN = re.compile(r"(\]\([^)]+\))([ \u3000]*[/、][ \u3000]*)(" + NUM + r")[ \u3000]*([^，。；）)（(|\[\]]{0,20})")


def build_index(root: Path):
    idx = {}
    for p in root.rglob("*.md"):
        if ".git" in p.parts or "_luban" in p.parts:
            continue
        m = FILE_NUM.match(p.name)
        if not m:
            continue
        title = m.group(2)
        for line in p.read_text(encoding="utf-8", errors="ignore").splitlines():
            h = H1.match(line.strip())
            if h:
                title = (h.group(1) or "").strip()
                break
        idx[m.group(1)] = (p, title)
    return idx


def short_title(title: str) -> str:
    return re.sub(r"[（(].*?[）)]", "", title).strip()


def take_title(line: str, pos: int):
    i = pos
    while i < len(line) and line[i] in " \u3000":
        i += 1
    j = i
    while j < len(line) and line[j] not in STOP and (j - i) < 20:
        j += 1
    return line[i:j].strip(), j


CONNECT = "的之里中和与及或"


def make_link(line: str, num: str, title: str, at: int, idx, src: Path) -> str:
    target, full = idx[num]
    rel = os.path.relpath(target, src.parent)
    text = title if title else short_title(full)
    tail = line[max(0, at - 14): at]
    if len(short_title(full)) >= 4 and short_title(full)[:4] in tail:
        text = ""  # 前文已说过标题，链接文字只留编号
    # 「1.1 的 P99」「34.5 的 coordinated omission」：连接词留在链接外面
    if text and (len(text) <= 1 or text[0] in CONNECT):
        return f"[{num} {short_title(full)}]({rel})" + (f" {text}" if text else "")
    label = f"{num} {text}".strip()
    return f"[{label}]({rel})"


def linkify(line: str, idx, src: Path) -> str:
    # ① 「见/详见 + 编号 + 标题」
    out, pos = [], 0
    for m in SEEN.finditer(line):
        if m.start() and line[m.start() - 1] == "[":
            continue
        num = m.group(3)
        if num not in idx or line[m.end(): m.end() + 1] == "]":
            continue
        title, end = take_title(line, m.end())
        tm = TODO.match(line[end:end + 12])  # 目标节已写出 → 「待补」自动作废
        if tm:
            end += tm.end()
        out.append(line[pos: m.start()])
        out.append(m.group(1) + m.group(2) + make_link(line, num, title, m.start(), idx, src))
        pos = end
    out.append(line[pos:])
    line = "".join(out)

    # ② 「括号里以编号开头」的组（可含多个编号）
    def fix_group(gm):
        group, inner, base = gm.group(0), gm.group(0)[1:-1], gm.start() + 1
        if "](" in group or ".md" in inner or "/" in inner or "见" in inner:
            return group
        if not INNER_NUM.match(inner):
            return group
        parts = re.split(r"([、/])", inner)
        new, cursor, changed = [], 0, False
        for part in parts:
            pm = re.match(r"(\s*)(" + NUM + r")(.*)$", part)
            abs_at = base + cursor
            cursor += len(part)
            if not pm or pm.group(2) not in idx:
                new.append(part)
                continue
            num, rest = pm.group(2), pm.group(3).strip()
            if rest and rest[0].isdigit():
                new.append(part)
                continue
            new.append(pm.group(1) + make_link(line, num, rest, abs_at, idx, src))
            changed = True
        return group[0] + "".join(new) + group[-1] if changed else group

    line = GROUP.sub(fix_group, line)

    # ③ 链接链：「[已有链接](…) / 44.9 标题」里的第二个编号
    def fix_chain(cm):
        num, title = cm.group(3), cm.group(4).strip()
        if num not in idx:
            return cm.group(0)
        target, full = idx[num]
        rel = os.path.relpath(target, src.parent)
        label = f"{num} {title or short_title(full)}".strip()
        sep = cm.group(2).strip()
        sep = f" {sep} " if sep == "/" else sep
        return f"{cm.group(1)}{sep}[{label}]({rel})"

    prev = None
    while prev != line:
        prev = line
        line = CHAIN.sub(fix_chain, line)
    return line


def main() -> int:
    dry = "--dry-run" in sys.argv
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    root = Path(args[0] if args else "v2").resolve()
    idx = build_index(root)

    changes, files = [], []
    for p in sorted(root.rglob("*.md")):
        if ".git" in p.parts or "_luban" in p.parts:
            continue
        lines = p.read_text(encoding="utf-8").splitlines(keepends=True)
        dirty = False
        for i, raw in enumerate(lines):
            old = raw.rstrip("\n")
            new = linkify(old, idx, p)
            if new != old:
                changes.append((p, i + 1, old, new))
                lines[i] = new + ("\n" if raw.endswith("\n") else "")
                dirty = True
        if dirty:
            files.append(p)
            if not dry:
                p.write_text("".join(lines), encoding="utf-8")

    print(f"{'[预演] ' if dry else ''}{len(changes)} 处 → 链接，涉及 {len(files)} 个文件\n")
    for p, ln, old, new in changes:
        print(f"{p.relative_to(root)}:{ln}\n  − {old[:200]}\n  + {new[:220]}")
    if not dry:
        print(f"\n✓ 已写入 {len(files)} 个文件")
    return 0


if __name__ == "__main__":
    sys.exit(main())
