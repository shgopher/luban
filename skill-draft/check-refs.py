#!/usr/bin/env python3
"""
check-refs.py —— 全书跨篇引用校验器（鲁班写作法 · 铁律 6）

查五件事：
  1. 悬空引用：`见 44.8` 指向的节没写正文，且没标「待补」
  2. 错编号  ：`见 44.99` 在目录里也不存在（纯笔误）
  3. 裸编号  ：`见 24.6` 后面没跟标题（读者要先翻目录）
  4. 方向代词：`详见上文` / `见前文` / `后面会讲`（不可定位、导出即失效）
  5. 标题不符：引用里写的标题与目标节的一级标题不一致（接口被改过）

用法：
  python3 check-refs.py .                      # 校验整个项目
  python3 check-refs.py v2                     # 只校验 v2（自动向上找 README*.md 当目录）
  python3 check-refs.py . --min-refs 3         # 额外要求每篇至少 3 个跨篇引用
  python3 check-refs.py . --quiet              # 只报硬错误

约定：书稿里的代码块（决策树、注释）中的引用同样校验——那些也是真引用；
非书稿目录（默认 _luban、skill-draft）与 SKILL*.md / README*.md 跳过；
被「」/引号包起来的反面示例不算引用。

退出码：0 = 通过；1 = 有硬错误
"""

import argparse
import re
import sys
from pathlib import Path

FILE_NUM = re.compile(r"^(\d{1,3}\.\d{1,2})-(.+)\.md$")
H1 = re.compile(r"^#\s+(?:(\d{1,3}\.\d{1,2})\s+)?(.+)$")
TOC_LINE = re.compile(r"^###\s*\[?(\d{1,3}\.\d{1,2})\s+([^\]\)]*)")
NUM_REF = re.compile(r"(?:见|详见|参见|参阅)\s*(\[?)(\d{1,3}\.\d{1,2})")
LINK_TAIL = re.compile(r"\]\(([^)]+)\)")
DIRECTIONAL = re.compile(
    r"详见(?:上文|前文|下文|后文)|见(?:上文|前文|下文|后文)|"
    r"前面(?:提到|讲过|说过)|后面(?:会讲|再说|讲到)|后文(?:会|再)"
)
PAREN_NUM = re.compile(r"[（(](\d{1,3}\.\d{1,2})[)）]")
LINK_ANY = re.compile(r"\[(\d{1,3}\.\d{1,2})[^\]]*\]\(([^)]+)\)")
TITLED_NO_LINK = re.compile(r"(?:见|详见|参见|参阅)\s*(\d{1,3}\.\d{1,2})\s*[\u4e00-\u9fff]")
CJK_OR_WORD = re.compile(r"[\u4e00-\u9fffA-Za-z]")
TERM = re.compile(r"[\u4e00-\u9fff]{2,}|[A-Za-z][A-Za-z0-9+#.]{1,}")


QUOTE_L, QUOTE_R = "「“\"", "」”\""


def quoted(line: str, m) -> bool:
    """被「」/引号包起来的，是「反面示例」或引用别人的说法，不算真引用。"""
    i = m.start() - 1
    return i >= 0 and line[i] in QUOTE_L


def named_in(title: str, line: str) -> bool:
    """这一行是否已经说出了目标节的标题（如「本地消息表 (24.6)」）
    —— 标题在句子里出现时，括号/裸编号是可读的，不算裸编号。
    「连接复用与 TIME_WAIT」这种标题，句子里只写「连接复用」也算已命名。"""
    for tok in TERM.findall(title or ""):
        tok = tok.strip("与和的在是及或")
        if len(tok) < 2:
            continue
        if tok in line:
            return True
        if len(tok) >= 4:  # 允许只写出标题的前缀，至少 3 个字
            for n in range(len(tok) - 1, 2, -1):
                if tok[:n] in line:
                    return True
    return False


def build_index(root: Path):
    """编号 -> (路径, 标题)，标题取正文一级标题（接口签名）"""
    sections = {}
    for p in root.rglob("*.md"):
        if ".git" in p.parts or "_luban" in p.parts or "skill-draft" in p.parts:
            continue
        m = FILE_NUM.match(p.name)
        if not m:
            continue
        title = m.group(2)
        try:
            for line in p.read_text(encoding="utf-8", errors="ignore").splitlines():
                h = H1.match(line.strip())
                if h:
                    title = (h.group(2) or "").strip()
                    break
        except OSError:
            pass
        sections[m.group(1)] = (p, title)
    return sections


def build_toc(root: Path):
    """目录里已规划的节编号 -> 标题（README*.md 的 ### 行）"""
    planned = {}
    for base in [root, root.parent, root.parent.parent]:
        for readme in sorted(base.glob("README*.md")):
            for line in readme.read_text(encoding="utf-8", errors="ignore").splitlines():
                m = TOC_LINE.match(line.strip())
                if m:
                    planned.setdefault(m.group(1), m.group(2).strip())
        if planned:
            break
    return planned


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("root", nargs="?", default=".")
    ap.add_argument("--quiet", action="store_true")
    ap.add_argument("--min-refs", type=int, default=0)
    ap.add_argument("--exclude", default="_luban,skill-draft", help="跳过的目录，逗号分隔")
    ap.add_argument("--todo", action="store_true", help="列出所有「待补」引用及目标文件该叫什么")
    args = ap.parse_args()

    root = Path(args.root).resolve()
    skip_dirs = {x.strip() for x in args.exclude.split(",") if x.strip()}
    sections, planned = build_index(root), build_toc(root)

    hard, soft = [], []
    todo = [] if args.todo else None
    per_file = {}

    for p in sorted(root.rglob("*.md")):
        if ".git" in p.parts or p.name.startswith(("README", "SKILL")):
            continue
        if skip_dirs & set(p.parts):
            continue
        rel = p.relative_to(root)
        count = 0
        for lineno, raw in enumerate(p.read_text(encoding="utf-8", errors="ignore").splitlines(), 1):
            line = raw.rstrip()
            if not line.strip():
                continue
            where = f"{rel}:{lineno}"

            for m in DIRECTIONAL.finditer(line):
                if quoted(line, m):
                    continue
                hard.append(f"{where}  方向代词「{m.group(0)}」→ 改成「见 N.N 标题」")

            links = list(LINK_TAIL.finditer(line))
            for m in NUM_REF.finditer(line):
                is_link = False
                rest = ""
                if m.group(1):  # 形如 `见 [24.6 标题](...)`
                    close = line.find("]", m.end())
                    tail = LINK_TAIL.match(line, close) if close != -1 else None
                    if tail:
                        is_link = True
                        target = tail.group(1)
                        if "…" in target or target.startswith("#"):
                            is_link = True
                        elif not (p.parent / target).exists() and not (root / target).exists():
                            hard.append(f"{where}  链接失效「{target}」")
                num = m.group(2)
                if quoted(line, m):
                    continue
                count += 1
                rest = line[m.end():].lstrip(" 　")
                if num in sections:
                    real = sections[num][1]
                    if not is_link and not CJK_OR_WORD.match(rest) and not named_in(real, line):
                        soft.append(f"{where}  裸编号「见 {num}」→ 补标题 + 链接「{real}」")
                    elif rest and CJK_OR_WORD.match(rest):
                        tok = re.search(r"[\u4e00-\u9fff]{3,}", rest)
                        written = tok.group(0) if tok else ""
                        if written and real and written not in real and real not in line:
                            soft.append(f"{where}  标题不符：正文为「{real}」，引用写成「{written}」")
                elif num in planned:
                    if "待补" not in line:
                        hard.append(
                            f"{where}  悬空引用「见 {num} {planned[num]}」→ 该节未写，加「（待补）」"
                        )
                else:
                    where_hint = "目录里也没有" if planned else "（未找到目录，无法判断）"
                    msg = f"{where}  未知编号「见 {num}」→ {where_hint}"
                    (hard if planned else soft).append(msg)

            if "待补" in line:  # 待补的两个方向：还该等 / 已经可以补了
                bare = re.sub(r"\[[^\]]*\]\([^)]*\)", "", line)  # 去掉已有链接
                for m in re.finditer("待补", bare):
                    # 取编号有两种写法，按优先级认：
                    #   A. 待补在编号后：「见 44.8 索引原理（待补）」
                    #   B. 待补在编号前：「- 待补：44.8 索引原理」
                    left = bare[max(0, m.start() - 50): m.start()]
                    right = bare[m.end(): m.end() + 50]
                    num = None
                    rn = re.match(r"[）)】]?\s*[：:]?\s*(\d{1,3}\.\d{1,2})", right)
                    if rn:
                        num = rn.group(1)
                    else:
                        ln = re.search(r"(\d{1,3}\.\d{1,2})[^\d]{0,14}$", left)
                        if ln:
                            num = ln.group(1)
                    if num is None:
                        continue
                    is_block = bare.strip().startswith(("- 待补", "-待补", "* 待补"))
                    if num in sections:
                        if is_block:
                            hard.append(
                                f"{where}  待补清单没删：{num} 已经写了 → 把这一项并入"
                                f"「前置 / 下游」并删掉这行待补"
                            )
                        else:
                            hard.append(
                                f"{where}  待补过期：「{num} {sections[num][1]}」已经写了，"
                                f"去掉「待补」并补链接（跑 linkify-refs.py）"
                            )
                    elif todo is not None and num in planned:
                        todo.append((num, planned[num], str(rel)))

            for m in LINK_ANY.finditer(line):
                target = m.group(2)
                if "…" in target or target.startswith("#"):
                    continue
                if not (p.parent / target).exists() and not (root / target).exists():
                    hard.append(f"{where}  编号链接失效「[{m.group(1)}…]({target})」")

            for m in TITLED_NO_LINK.finditer(line):
                if m.group(1) not in sections:
                    continue  # 节还没写，只能标「待补」，无链接可加
                if "](" in line[m.start(): m.start() + 120]:
                    continue
                soft.append(
                    f"{where}  引用「{m.group(1)}」有标题没链接 → 补相对路径，点击就能跳"
                )

            for m in PAREN_NUM.finditer(line):
                num = m.group(1)
                if num in sections and not named_in(sections[num][1], line):
                    soft.append(
                        f"{where}  括号裸编号「({num})」→ 改成「见 {num} {sections[num][1]}」"
                    )
        per_file[rel] = count

    if args.min_refs:
        for rel, n in sorted(per_file.items()):
            if n < args.min_refs:
                soft.append(f"{rel}  只有 {n} 个跨篇引用（下限 {args.min_refs}）")

    linked = sum(1 for n in per_file.values() if n)
    print(
        f"扫描 {len(per_file)} 个文件 · {linked} 个有跨篇引用 · 共 {sum(per_file.values())} 处引用"
        f" · 目录收录 {len(planned)} 节 · 已有正文 {len(sections)} 节"
    )

    if hard:
        print(f"\n✗ 硬错误 {len(hard)} 条：")
        for x in hard:
            print("  " + x)
    if soft and not args.quiet:
        print(f"\n⚠ 提醒 {len(soft)} 条：")
        for x in soft[:40]:
            print("  " + x)
        if len(soft) > 40:
            print(f"  …… 另有 {len(soft) - 40} 条（加 --quiet 只看硬错误）")

    if args.todo:
        rows = sorted(set(todo or []), key=lambda r: (int(r[0].split('.')[0]), int(r[0].split('.')[1])))
        print(f"\n待补引用 {len(rows)} 处（目录有规划、正文还没写）：")
        for num, title, where in rows:
            print(f"  {num}  {title}   ← {where}")
        print("  那一节写完后跑 `linkify-refs.py`：链接、标题、「待补」会自动补齐。")

    print("\n✓ 无硬错误" if not hard else "\n先修硬错误，再看提醒。")
    return 1 if hard else 0


if __name__ == "__main__":
    sys.exit(main())
