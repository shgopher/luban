#!/usr/bin/env python3
"""Build the entry-level architect handbook PDF."""
import subprocess, pathlib, sys, re

ROOT = pathlib.Path(__file__).parent
OUT = ROOT / "build"
OUT.mkdir(exist_ok=True)

ORDER = [
    "00-front.md",
    "01-role.md", "02-requirements.md", "03-solution.md", "04-selection.md", "05-app-arch.md",
    "06-data.md", "07-cache.md", "08-mq.md", "09-api.md", "10-resilience.md",
    "11-deploy.md", "12-release.md", "13-observability.md", "14-ha-dr.md", "15-security.md",
    "16-go.md", "17-performance.md", "18-process.md", "19-evolution.md", "20-runbook.md",
    "appendix-a-checklist.md", "appendix-b-templates.md", "appendix-c-glossary.md",
    "appendix-d-plan.md",
]

parts = []
for name in ORDER:
    p = ROOT / name
    if not p.exists():
        sys.exit(f"missing {name}")
    parts.append(p.read_text(encoding="utf-8").strip())
combined = "\n\n\n".join(parts).replace("\n## 本章自检", "\n### 本章自检").replace("\n▸ 答案：", "\n\n**答：**")
(OUT / "combined.md").write_text(combined, encoding="utf-8")

subprocess.run([
    "pandoc", str(OUT / "combined.md"),
    "-f", "markdown+pipe_tables+raw_html+lists_without_preceding_blankline",
    "-t", "html5",
    "--toc", "--toc-depth=2",
    "--wrap=none",
    "--syntax-highlighting=tango",
    "--template=" + str(ROOT / "pandoc-template.html"),
    "-s",
    "-o", str(OUT / "body.html"),
], check=True)

body = (OUT / "body.html").read_text(encoding="utf-8")

cover = """
<section class="cover">
  <div class="watermark">ARCH</div>
  <div class="rule"></div>
  <div class="kicker">FROM REQUIREMENT TO PRODUCTION</div>
  <h1>入门架构师<br>实战手册</h1>
  <p class="sub">从接需求、写方案，到部署、发布、排障——<br>20 章，把方案做到能上线。</p>
  <div class="tags">
    <span>需求与方案</span><span>技术选型</span><span>数据 · 缓存 · 消息</span>
    <span>稳定性</span><span>部署与发布</span><span>可观测性</span>
    <span>安全</span><span>Go 工程实践</span><span>故障处置</span>
  </div>
  <div class="note">写给第一次承担架构职责的工程师<br>不含：CAP 证明、Raft、GC、内核与调度原理</div>
  <div class="bottom"><span>内部培训材料 · 第 1 版</span><span>2026</span></div>
</section>
"""

html = f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>入门架构师实战手册</title>
<meta name="author" content="内部培训材料">
<meta name="description" content="从接需求、写方案，到部署、发布、排障的入门架构师实战手册">
<link rel="stylesheet" href="style.css">
</head>
<body>
{cover}
<section class="toc">
<h1>目录</h1>
{body.split('<nav id="TOC"')[0] if False else ''}
</section>
</body>
</html>
"""
# body contains nav + content; split it
m = re.search(r'<nav id="TOC".*?</nav>', body, re.S)
toc_html = m.group(0) if m else ""
content_html = body.replace(toc_html, "", 1) if toc_html else body

html = f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>入门架构师实战手册</title>
<meta name="author" content="内部培训材料">
<meta name="description" content="从接需求、写方案，到部署、发布、排障的入门架构师实战手册">
<link rel="stylesheet" href="style.css">
</head>
<body>
{cover}
<section class="toc">
<h1>目录</h1>
{toc_html}
</section>
{content_html}
<section class="backcover">
  <div class="bc-inner">
    <div class="bc-rule"></div>
    <p class="bc-big">先上手，再深入。</p>
    <p class="bc-text">20 章覆盖从接需求、写方案，到部署、发布、排障的完整闭环。<br>原理留给完整教程，这一本只负责让你下一次上线不出事。</p>
  </div>
  <div class="bc-foot">入门架构师实战手册 · 第 1 版 · 2026</div>
</section>
</body>
</html>
"""
(OUT / "book.html").write_text(html, encoding="utf-8")

import weasyprint
weasyprint.HTML(str(OUT / "book.html"), base_url=str(ROOT)).write_pdf(str(OUT / "book.pdf"))
print("wrote", OUT / "book.pdf")
