# 审计报告目录

> 生成日期：2026-11-21。本目录是一次性审计产物，不属于书稿正文；确认后可整体删除，或保留 `全书记审报告.md` 作为交付记录。

| 文件 | 内容 |
|---|---|
| `全书记审报告.md` | **主报告**：结论摘要、结构一致性、质量门禁、内容正确性、章节关系、查缺补漏、修复队列 |
| `问题登记表.md` | 285 节逐节问题（门禁失败 / 断表 / 缺关联 / 零入出度 / 过期待补），按节号排序 |
| `深度审读问题清单.md` | 分单元深度审读的全部问题（带原文引用与核验标记）、章节评分、缺口与跨章关系建议 |
| `修复记录.md` | **P0+P1 修复记录**：断表/Q 编号/待补/目录/81 章概论/34 条事实错误/97 节关联块/109 节门禁 |
| `机械修复清单.md` | 首轮审计时的可脚本化修复项基线（P0 部分已完成，见修复记录） |
| `高并发的哲学原理-借鉴评估.md` | 外部书目评估（261 页逐页精读）：12 章逐章价值图、可吸收的 Nginx/网关/L4/DB 机制、7 处书内矛盾、40 条引用前纠错、2 新节 + 15 处补写清单 |
| `SQL必知必会-精读与融入清单.md` | 外部课程评估（51 个 PDF / 48 讲逐讲精读）：逐讲去向、刻意不写清单、4 处原文纠错、MySQL 8.0.46 验证记录 |

数据与脚本在 `tools/audit/`：

```bash
python3 tools/audit/structural_audit.py .     # 结构与体例
python3 tools/audit/lib_graph.py .            # 引用图（修复 make-index 漏算）
python3 tools/audit/relation_audit.py .       # 章节关系
python3 tools/audit/content_checks.py .       # 重复/待补/版本
python3 tools/audit/guo_promise.py .          # 概论承诺核对
python3 tools/audit/chapter_scorecard.py .    # 章级风险记分卡
python3 ~/.dsh/skills/luban-writing/scripts/check-depth.py v2 --json > /tmp/depth.json
```

修复后建议重跑：

```bash
python3 tools/audit/structural_audit.py .
python3 ~/.dsh/skills/luban-writing/scripts/check-depth.py v2
python3 ~/.dsh/skills/luban-writing/scripts/check-refs.py v2 --quiet
```
