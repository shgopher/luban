lint:
	@bash ./lint.sh

# 引用链接：默认只预演（dry-run），确认 diff 后才加 --write
# tools/linkify-refs.py 同步自 ~/.dsh/skills/luban-writing/scripts/linkify-refs.py
# 改规则请先改 skill 里的上游，再覆盖本文件；两者应逐字节一致（diff 一下即可）
refs:
	python3 tools/linkify-refs.py v2

refs-write:
	python3 tools/linkify-refs.py v2 --write

refs-check:
	python3 tools/linkify-refs.py v2 --check

# 深度门禁：一句话 / 自检数 / 反模式行数 / 代码或实测 / 数字表
SKILL_SCRIPTS ?= $(HOME)/.dsh/skills/luban-writing/scripts
depth:
	python3 $(SKILL_SCRIPTS)/check-depth.py v2 --top 20
depth-strict:
	python3 $(SKILL_SCRIPTS)/check-depth.py v2 --strict --top 20
