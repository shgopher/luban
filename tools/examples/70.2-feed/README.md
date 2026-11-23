# 70.2-feed：Feed 流的拉取与混合

[70.2 Feed 流架构](../../../v2/L4-组件与技术/70-内容与社交/70.2-Feed流架构.md) 的配套代码（Go + Redis + MySQL）。

```bash
cd tools/examples/70.2-feed/code
go mod download
go run ./cmd/paging        # 或 mixrt / contentdb / zsetcursor / zsetmem
```

五个命令对应：翻页方式、推拉混合、内容库回填、ZSet 游标、ZSet 内存。依赖 Redis 与 MySQL，连接参数见各 `main.go` 顶部；`scripts/redis-evidence.sh` 采集 Redis 侧证据。
