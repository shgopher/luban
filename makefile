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
