# 72.5-sync：跨地域读写与一致性

[72.5 跨地域读写与一致性](../../../v2/L4-组件与技术/72-国际化与多地域/72.5-跨地域读写与一致性.md) 的配套代码（Go + PostgreSQL）。

```bash
cd tools/examples/72.5-sync/code
go mod download
go run . -h        # 参数见 main.go
```

依赖 `github.com/lib/pq`，需要两个地域的 PostgreSQL（`setup.sh` 里有建库命令）。`raw/` 是同步/异步两种模式的延迟 CSV 与陈旧度数据。
