#!/usr/bin/env python3
"""把新章节合入 SKILL.md。

用法：
  python3 build.py            生成 SKILL.patched.md（A 装法：整章塞进 SKILL.md）
  python3 build.py --lean     生成 SKILL.lean.md（B 装法：摘要进 SKILL.md，全文进 references/）
  可选第二个参数指定源 SKILL.md 路径。

两套装法的差别只在「新章节放哪」：
  A 全塞    每次加载 skill 多付 ~7.4k token（与是否跨篇写作无关）
  B 摘要版  每次多付 ~0.9k，完整规则按需读 references/cross-refs.md
其余改动（铁律 6 / 关联块 / QA / 交付）两套完全一致。
"""
import sys
from pathlib import Path

HERE = Path(__file__).parent
LEAN = "--lean" in sys.argv
args = [a for a in sys.argv[1:] if not a.startswith("--")]
_SKILL = Path.home() / ".dsh/skills/luban-writing/SKILL.md"
# 源永远是「未打补丁的原版」：装完之后 SKILL.md 已经是产物，重跑要从备份起
SRC = Path(args[0]) if args else (Path(str(_SKILL) + ".bak") if Path(str(_SKILL) + ".bak").exists() else _SKILL)

s = SRC.read_text(encoding="utf-8")
full = (HERE / "二-全书视角-定位与跨篇协作.md").read_text(encoding="utf-8").strip() + "\n"
short = (HERE / "二-摘要.md").read_text(encoding="utf-8").strip() + "\n"
chapter = short if LEAN else full

if LEAN:
    (HERE / "references-cross-refs.md").write_text(
        "# 跨篇协作详解（定位 / 引用 / 索引 / 配合）\n\n"
        "> 这是 `SKILL.md` 第二节的完整版：SKILL.md 只留可执行的摘要，"
        "判主场、切边界、层间分工、反模式与自检题都在这里。\n\n"
        + full.replace("## 二、全书视角：定位与跨篇协作\n\n", "", 1),
        encoding="utf-8",
    )

# 1) 顶部一句话
s = s.replace(
    "> 命名寓意：像鲁班做木工一样",
    "> 单篇按此法写；多篇按「铁律 6」拼——**先读目录，主场讲透，客场只引用**。\n"
    "> 命名寓意：像鲁班做木工一样",
)

# 2) 参考区加脚本
s = s.replace("## 参考范文（写作前先读，内化结构）", "## 参考范文与工具（写作前先读，内化结构）")
s = s.replace(
    "- `references/example-release-strategy.md` —— 演示：四方案对比、决策表、铁律、自检题（滚动 / 蓝绿 / 金丝雀）",
    "- `references/example-release-strategy.md` —— 演示：四方案对比、决策表、铁律、自检题（滚动 / 蓝绿 / 金丝雀）\n"
    "- `references/cross-refs.md` —— 跨篇协作详解：判主场、切边界、层间分工、接缝、反模式、自检题（摘要不够时读）\n"
    "- `scripts/make-index.py` —— 抽每节「一句话」+ 引用图：辅助读目录后的筛选与下游查询\n"
    "- `scripts/linkify-refs.py` —— 把「编号 + 标题」批量转成相对路径链接（跑完再校验）\n"
    "- `scripts/check-refs.py` —— 全书引用校验器：死链 / 裸编号 / 方向代词 / 悬空 / 编号错配，交付前必跑",
)

# 3) 五条 -> 六条铁律
s = s.replace("## 一、五条铁律（按优先级，不可违反）", "## 一、六条铁律（按优先级，不可违反）")
s = s.replace(
    "但要转述得更利落，不硬凑谐音梗。",
    "但要转述得更利落，不硬凑谐音梗。\n"
    "6. **全书一体化：先读目录，再动笔；主场讲透，客场只给结论 + 编号。** 同一知识点全书只设一个主场。"
    "目录必读（10k，占全书 4.7%），正文只精读相关的那几篇——**读什么由目录定，读多深由一句话定**。"
    "铁律 1 管「不掉肉」（相对原文），本条管「不重复」（相对全书）——**零丢失 ≠ 到处重复**。",
)

# 4) 插入新章节
anchor = "## 二、每节的标准结构"
assert anchor in s, "找不到插入锚点"
s = s.replace(anchor, chapter + "\n---\n\n" + anchor, 1)

# 5) 重编号（自下而上）
for a, b in [
    ("## 八、交付", "## 九、交付"),
    ("## 七、一个完整示例（速览）", "## 八、一个完整示例（速览）"),
    ("## 六、写完后的 QA 自检清单", "## 七、写完后的 QA 自检清单"),
    ("## 五、压缩与深度标准", "## 六、压缩与深度标准"),
    ("## 四、语言风格", "## 五、语言风格"),
    ("## 三、八种写作元素（工具箱，按需选用）", "## 四、八种写作元素（工具箱，按需选用）"),
    ("## 二、每节的标准结构", "## 三、每节的标准结构"),
]:
    assert a in s, a
    s = s.replace(a, b, 1)

s = s.replace("### 正文（按需组合下面第三节的元素）", "### 正文（按需组合下面第四节的元素）")

# 6) 标准结构加「关联」块
s = s.replace(
    '### 反模式 / 坑\n- 明确写"别这么做" + "后果"\n\n### 自检',
    '### 反模式 / 坑\n- 明确写"别这么做" + "后果"\n\n'
    "### 关联（跨篇协作，见第二节）\n"
    "- 前置：[24.4 TCC](24.4-TCC.md)、[24.6 本地消息表](24.6-本地消息表.md)\n"
    "- 并列：[24.5 Saga](24.5-Saga.md)（补偿型，无冻结）\n"
    "- 下游：[24.9 事务模型选型](24.9-事务模型选型.md)\n"
    "- 待补：44.8 索引原理（未写，无链接）\n\n"
    "### 自检",
)
s = s.replace(
    '不要出"复述正文"的题。',
    '不要出"复述正文"的题。\n'
    "- 文末「关联」块 ≤ 4 行，且与正文行内引用一一对应（正文没引的，不许写进块里）。",
)

# 7) QA 清单
s = s.replace(
    "- [ ] 表格列数一致，代码围栏成对",
    "- [ ] 表格列数一致，代码围栏成对\n"
    "- [ ] 动笔前读完全文目录，圈出相关节，并精读过其中相关的 1–3 篇\n"
    "- [ ] 别节主场的内容没有第二次展开（客场 ≤ 1 段）\n"
    "- [ ] 引用一律「编号 + 短标题 + 相对路径链接」，无「前文 / 下文 / 后面会讲」\n"
    "- [ ] 已写节的引用全部带链接且可达；未写节标「待补」不写死链\n"
    "- [ ] `check-refs.py . --todo` 里的待补都是真的还没写；已写的待补全部删干净（行内由 linkify 删，关联块手工删）\n"
    "- [ ] 悬空引用（该节未写）都带「（待补）」\n"
    "- [ ] 文末「关联」块与行内引用一一对应\n"
    "- [ ] 每节跨篇引用 3–8 处（现状均值 1.47，差 2–5 倍）\n"
    "- [ ] `python3 scripts/check-refs.py .` 零硬错误\n"
    "- [ ] `python3 scripts/make-index.py . -q <核心术语>` 反查过一遍，没有漏掉的既有节",
)

# 8) 交付
s = s.replace(
    "- 先写 Markdown 源文件（UTF-8）。",
    "- 先写 Markdown 源文件（UTF-8）。\n"
    "- 交付前跑 `python3 scripts/make-index.py <书稿目录>` 更新索引，再跑 `check-refs.py`，硬错误清零再交付。\n"
    "- 本轮新增了编号，同步更新目录（README / TOC）——**目录永远先于正文**。",
)

out = HERE / ("SKILL.lean.md" if LEAN else "SKILL.patched.md")
out.write_text(s, encoding="utf-8")
print(f"✓ {out}  {len(s.splitlines())} 行 / {len(s):,} 字（约 {len(s)*0.7/1000:.1f}k token）")
