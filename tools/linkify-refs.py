#!/usr/bin/env python3
"""linkify-refs.py —— 把「编号 + 标题」的跨篇引用变成链接，且**只在确定是引用时才动**。

为什么这么保守：上一版把任意 `(N.N)` 都当引用，在真实书稿上把
`lint(1.5)`、`（47.4%）`、`（16.7 分钟）`、ASCII 谱系图里的 `（16.2）`
都改成了链接——26 处修改里 6 类是误判，且它直接原地落盘。

三条接受规则（其余一律跳过并给出原因，不猜）：
  R1  见 / 详见 / 参见 / 参阅 + 编号 + 标题      ← 必须有引用动词
  R2  编号 + 标题 + （待补），且该节已经写出      ← 顺手删掉过期的「待补」
  R3  （编号）单独成组，且前 20 字里出现目标标题的 ≥4 字片段

四道否决闸（任一命中就跳过）：
  ① 在代码块 / 行内代码里        ← ASCII 谱系图、表格里的 `lint(1.5)` 全在这里
  ② 括号组内带单位（% 分钟 秒 人日 QPS 倍 行 条 台 …）
  ③ 编号紧邻 ASCII 字母/数字     ← `lint(1.5)`、`build(4)`
  ④ 取到的标题与目标节标题对不上  ← 防跨行取错标题

用法：
  python3 scripts/linkify-refs.py .                   # 预演：只打印 diff（默认）
  python3 scripts/linkify-refs.py . --write           # 落盘（先看预演！）
  python3 scripts/linkify-refs.py . --check           # 门禁：需要改动就退出 1
  python3 scripts/linkify-refs.py --selftest          # 自检：3 条该改 + 9 条不该改 + 幂等
"""
import os
import re
import sys
import tempfile
from pathlib import Path

FILE_NUM = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
H1 = re.compile(r"^#\s+(?:\d{1,3}\.\d{1,2}\s+)?(.+)$")
NUM = r"\d{1,3}\.\d{1,2}"

R1 = re.compile(r"(见|详见|参见|参阅)([ 　]*)(" + NUM + r")")
R2 = re.compile(r"(" + NUM + r")([ 　]+)([^，。；、）)\n]{1,20}?)(（待补）|\(待补\))")
R3 = re.compile(r"[（(](\s*)(" + NUM + r")(\s*)[）)]")
TODO_MARK = re.compile(r"[ 　]*[，,]?[ 　]*[（(]待补[）)]")

UNITS = ("%", "％", "分钟", "小时", "秒", "毫秒", "人日", "人天", "人时",
         "QPS", "RPS", "TPS", "倍", "行", "条", "台", "个", "次", "页", "字节")
SKIP_DIRS = {"_luban", ".git", "node_modules", ".linkify-bak", "skill-draft"}
STOP = "，。；、）)（(|｜\t 　]"


def build_index(root: Path):
    """编号 -> (路径, 标题)；标题取一级标题（没有则取文件名）"""
    idx = {}
    for p in root.rglob("*.md"):
        if SKIP_DIRS & set(p.parts) or p.name.startswith(("README", "SKILL")):
            continue
        m = FILE_NUM.match(p.name)
        if not m:
            continue
        title = m.group(2)
        for line in p.read_text(encoding="utf-8", errors="ignore").splitlines():
            h = H1.match(line.strip())
            if h:
                title = h.group(1).strip()
                break
        idx[m.group(1)] = (p, title)
    return idx


def in_fence(line_no: int, spans: list) -> bool:
    """代码块区间内（不含围栏行本身）"""
    return any(a < line_no <= b for a, b in spans)


def in_inline_code(line: str, pos: int) -> bool:
    """pos 之前出现奇数个反引号 → 落在行内代码里"""
    return line[:pos].count("`") % 2 == 1


def in_link(line: str, pos: int) -> bool:
    """编号是否已经落在某个 [..] 里（已在链接中，不再处理）"""
    left = line.rfind("[", 0, pos)
    if left == -1:
        return False
    close = line.find("]", left)
    return close != -1 and pos < close


def take_title(line: str, pos: int):
    """从 pos 起取标题片段，遇到标点/空白为止，最多 20 字"""
    i = pos
    while i < len(line) and line[i] in " 　":
        i += 1
    j = i
    while j < len(line) and line[j] not in STOP and (j - i) < 20:
        j += 1
    return line[i:j].strip(), j


def title_ok(taken: str, full: str) -> bool:
    """取到的标题必须能对上目标标题，防跨行取错"""
    if not taken:
        return True
    if taken in full or full.startswith(taken):
        return True
    return len(taken) >= 2 and taken[:2] in full


def fragment_ok(line: str, pos: int, full: str) -> bool:
    """R3 用：前 20 字里要有目标标题的 ≥4 字片段（如「分布式单体」→「（30.3）」）"""
    window = line[max(0, pos - 20): pos]
    for n in range(min(len(full), 8), 3, -1):
        for s in range(0, len(full) - n + 1):
            if full[s:s + n] in window:
                return True
    return False


def bare_guard(line: str, pos: int, group: str) -> bool:
    """R3 的否决闸：前一个字符不能是 ASCII 字母/数字；组内不能带单位"""
    if pos > 0 and re.match(r"[A-Za-z0-9_]", line[pos - 1]):
        return False
    return not any(u in group for u in UNITS)


def todo_span(line: str, start: int, num: str, limit: int = 60):
    """找本行里属于这条链接的「（待补）」，返回 (起, 止)；找不到或属于别人则 None。
    中间夹了别的编号就放弃——那多半是下一句里另一节的待补。"""
    seg = line[start:start + limit]
    m = TODO_MARK.search(seg)
    if not m:
        return None
    for other in re.findall(NUM, seg[:m.start()]):
        if other != num:
            return None
    return start + m.start(), start + m.end()


def rel_link(src: Path, target: Path) -> str:
    return os.path.relpath(target, src.parent)


def rewrite_line(line: str, idx, src: Path, lineno: int, report: list):
    """逐字符扫描：命中规则且过闸 → 换成链接；否则原样保留并记下原因"""
    out, i = [], 0
    while i < len(line):
        m1 = R1.match(line, i)
        if m1 and m1.group(3) in idx:
            num, at = m1.group(3), m1.start()
            title, end = take_title(line, m1.end())
            full = idx[num][1]
            if in_link(line, at) or in_inline_code(line, at) or not title_ok(title, full):
                report.append((lineno, num, "标题对不上/在代码或链接里，跳过"))
                out.append(line[i:m1.end()]); i = m1.end(); continue
            new = f"{m1.group(1)}{m1.group(2)}[{num} {title or full}]({rel_link(src, idx[num][0])})"
            span = todo_span(line, end, num)
            if span:                     # 目标节已写出 → 删掉过期的「待补」，正文照留
                out.append(line[i:at] + new + line[end:span[0]])
                line = line[:span[0]] + line[span[1]:]
                i = span[0]
            else:
                out.append(line[i:at] + new)
                i = end
            continue

        m2 = R2.match(line, i)
        if m2 and m2.group(1) in idx:
            num, at, title = m2.group(1), m2.start(), m2.group(3).strip()
            full = idx[num][1]
            if in_link(line, at) or in_inline_code(line, at) or not title_ok(title, full):
                report.append((lineno, num, "标题对不上/在代码或链接里，跳过"))
                out.append(line[i:m2.end()]); i = m2.end(); continue
            out.append(line[i:at] + f"[{num} {title}]({rel_link(src, idx[num][0])})")
            i = m2.end(); continue

        m3 = R3.match(line, i)
        if m3 and m3.group(2) in idx:
            num, at = m3.group(2), m3.start()
            full = idx[num][1]
            if (in_inline_code(line, at) or not bare_guard(line, at, m3.group(0))
                    or not fragment_ok(line, at, full)):
                report.append((lineno, num, "括号裸编号但上下文不足，跳过"))
                out.append(line[i:m3.end()]); i = m3.end(); continue
            out.append(line[i:at] + f"（[{num}]({rel_link(src, idx[num][0])})）")
            i = m3.end(); continue

        out.append(line[i]); i += 1
    return "".join(out)


def fences_of(lines: list) -> list:
    """代码块区间 [(起始行, 结束行)]，行号从 1 起"""
    spans, open_at = [], None
    for n, line in enumerate(lines, 1):
        if line.lstrip().startswith("```"):
            if open_at is None:
                open_at = n
            else:
                spans.append((open_at, n)); open_at = None
    if open_at is not None:
        spans.append((open_at, len(lines)))
    return spans


def show_pair(old: str, new: str, ctx: int = 26) -> tuple:
    """只显示改动那一段（前后各留 ctx 字），长行也能一眼看出改了哪几个字"""
    a, b = old.strip(), new.strip()
    p = 0
    while p < min(len(a), len(b)) and a[p] == b[p]:
        p += 1
    k = 0
    while k < min(len(a), len(b)) - p and a[-1 - k] == b[-1 - k]:
        k += 1
    head = a[max(0, p - ctx): p]
    a_mid = a[p: len(a) - k if k else len(a)]
    b_mid = b[p: len(b) - k if k else len(b)]
    tail = a[len(a) - k:][:ctx] if k else ""
    return (f"…{head}【{a_mid}】{tail}…", f"…{head}【{b_mid}】{tail}…")


def scan(root: Path):
    """只读扫描：返回 (diff 列表, 被跳过项, 待写入清单)"""
    idx = build_index(root)
    diff, skipped, plans = [], [], []
    for p in sorted(root.rglob("*.md")):
        if SKIP_DIRS & set(p.parts) or p.name.startswith(("README", "SKILL")):
            continue
        text = p.read_text(encoding="utf-8", errors="ignore")
        lines = text.splitlines()
        spans = fences_of(lines)
        out, dirty = [], False
        for n, line in enumerate(lines, 1):
            if in_fence(n, spans):       # 代码块里的编号是图/表内容，不动
                out.append(line); continue
            report = []
            new = rewrite_line(line, idx, p, n, report)
            skipped += [(str(p.relative_to(root)), r[0], r[1], r[2]) for r in report]
            if new != line:
                dirty = True
                diff.append((str(p.relative_to(root)), n, line, new))
            out.append(new)
        if dirty:
            new_text = "\n".join(out) + ("\n" if text.endswith("\n") else "")
            plans.append((p, text, new_text))
    return diff, skipped, plans


def apply_changes(root: Path, plans: list) -> Path:
    """落盘前先备份：备份放 .linkify-bak/，且**不叫 .md**（否则会被当正文重新扫进来）"""
    bak = root / ".linkify-bak"
    for p, old, new in plans:
        rel = p.relative_to(root)
        dst = (bak / rel).with_suffix(rel.suffix + ".bak")
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_text(old, encoding="utf-8")
        p.write_text(new, encoding="utf-8")
    return bak


FIXTURE = """# 99.1 测试

OK1 见 1.1 甲（待补）
OK2 、2.1 丙（待补）
OK3 得到"分布式单体"（30.3）

NO1 | `lint(1.5) → build(4)` | 17.14 |
NO2 直接相加是 128,000（47.4%），高估明显。
NO3 排空需要 1000s（16.7 分钟）就够。
NO4 收敛 669 次 (33.5%)，平均 1 tick。
NO5 命中 109,589 行（1.1%），走索引反而更慢。
NO6 见 [1.1 甲](1.1-甲.md)。
NO7 见 1.1 分钟
NO8 版本 build(1.5) 的耗时

```
NO9 线性一致（16.2）            实时顺序 + 全序 + 单对象原子
```
"""


def _line_of(text: str, tag: str) -> str:
    """取以某标记开头的那一行（用于「必须原样」的逐字对比）"""
    for line in text.splitlines():
        if line.startswith(tag):
            return line
    return ""


def selftest() -> int:
    """回归样本：3 条必须被改 + 9 条必须原样 + 幂等。规则一旦被改坏，这里先红。"""
    checks = []
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        for name, title in [("1.1-甲.md", "# 1.1 甲"),
                            ("1.5-戊.md", "# 1.5 戊"),
                            ("2.1-丙.md", "# 2.1 丙"),
                            ("30.3-分布式单体反模式.md", "# 30.3 分布式单体反模式")]:
            (root / name).write_text(title + "\n", encoding="utf-8")
        doc = root / "99.1-测试.md"
        doc.write_text(FIXTURE, encoding="utf-8")

        diff, _, plans = scan(root)
        checks.append(("3 条真引用被识别为待改", len(diff) == 3, f"实际 {len(diff)} 处"))

        apply_changes(root, plans)
        text = doc.read_text(encoding="utf-8")

        for tag in ("OK1", "OK2", "OK3"):
            line = _line_of(text, tag)
            ok = "](" in line and "待补" not in line
            checks.append((f"{tag} 加了链接且删掉待补", ok, line[:60]))
        for tag in ("NO1", "NO2", "NO3", "NO4", "NO5", "NO6", "NO7", "NO8", "NO9"):
            ok = _line_of(text, tag) == _line_of(FIXTURE, tag)
            checks.append((f"{tag} 一字未动", ok, _line_of(text, tag)[:60]))

        again, _, _ = scan(root)
        checks.append(("幂等：第二遍 0 处", len(again) == 0, f"第二遍仍想改 {len(again)} 处"))

    for name, ok, detail in checks:
        print(f"  {'✓' if ok else '✗'} {name}" + ("" if ok else f" —— {detail}"))
    bad = sum(1 for _, ok, _ in checks if not ok)
    print(f"自检结果：{len(checks) - bad}/{len(checks)} 通过")
    return 0 if bad == 0 else 1


def main() -> int:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    flags = {a for a in sys.argv[1:] if a.startswith("--")}

    if "--selftest" in flags:
        return selftest()

    root = Path(args[0] if args else ".").resolve()
    write = "--write" in flags
    check = "--check" in flags
    quiet = "--quiet" in flags
    # --dry-run 保留为兼容旧写法（默认就是预演）

    diff, skipped, plans = scan(root)

    for rel, n, old, new in diff:
        minus, plus = show_pair(old, new)
        print(f"{rel}:{n}\n  − {minus}\n  + {plus}")
    print(f"\n{len(diff)} 处 → 链接，涉及 {len(plans)} 个文件；跳过 {len(skipped)} 处可疑")
    if not quiet and not check:          # 门禁模式只报结论，不刷可疑清单
        for f, n, num, why in skipped[:20]:
            print(f"  · {f}:{n} 〔{num}〕 {why}")

    if len(diff) > 200:
        print("✗ 单次超过 200 处改动，拒绝执行（先人工看看是不是规则太宽）")
        return 2
    if check:
        return 1 if diff else 0
    if not write:
        print("（预演，未写入；逐条看过 diff 后加 --write）")
        return 0
    if not diff:
        return 0

    bak = apply_changes(root, plans)
    print(f"✓ 已写入 {len(plans)} 个文件；原文件备份在 {bak}")
    print("  回滚：git checkout -- <文件>   或   cp -r .linkify-bak/* .")
    return 0


if __name__ == "__main__":
    sys.exit(main())
