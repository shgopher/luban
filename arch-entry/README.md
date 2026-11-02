# 《入门架构师实战手册》源文件

- `00-front.md` ～ `20-runbook.md`：正文 20 章
- `appendix-a` ～ `appendix-d`：附录
- `BRIEF.md` / `OUTLINE.md`：写作规范与目录（不进入正文）
- `style.css`：排版样式（A4、中文字体、目录点线、代码块）
- `pandoc-template.html`：Pandoc 模板（生成目录）
- `build.py`：构建脚本，输出 `build/book.pdf`

## 重新构建

依赖：pandoc 3.x、Python 3、WeasyPrint（`pip install weasyprint`）。

```bash
python3 -m venv .venv && .venv/bin/pip install weasyprint
.venv/bin/python build.py
```

字体：正文使用 HarmonyOS Sans SC（回退 PingFang SC / Heiti SC），代码使用 Menlo。
若在其他机器构建，请先安装对应中文字体，否则会回退到系统默认字体。

## 修改约定

- 每章一个文件，章节编号与文件名前缀一致；
- 小节用 `## N.x`，不要用 `###` 做小节（会撑爆目录）；
- 每章末尾 `## 本章自检`（构建时会自动降级为 `###`，不进目录）；
- 代码块标注语言，命令写清版本前提。
