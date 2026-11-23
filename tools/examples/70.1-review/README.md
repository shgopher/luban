# 70.1-review：内容审核状态机与回调乱序

[70.1 内容生产与审核](../../../v2/L4-组件与技术/70-内容与社交/70.1-内容生产与审核.md) 的配套代码（Go）。

```bash
cd tools/examples/70.1-review/code
go run .
```

同进程对比正确版（签名 + 去重表 + 条件更新）与错误版（无条件 UPDATE / last-write-wins）：800 条内容、5000 次回调（含重复、乱序、伪造签名）。`mysql_concurrency.sh`、`redis_checks.sh` 是 MySQL/Redis 侧的并发证据采集脚本。`raw/` 是原始输出。
