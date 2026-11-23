# 66.2-delivery：消息可靠投递与离线

[66.2 消息可靠投递与离线](../../../v2/L4-组件与技术/66-即时通信与音视频/66.2-消息可靠投递与离线.md) 的配套代码（Go + Redis + MySQL）。

```bash
cd tools/examples/66.2-delivery/code
go mod download
go run ./cmd/inbox       # 或 ackstrategy / relay
```

`code/mysql/schema.sql`、`code/redis/dedupe.lua` 是建表与去重脚本；`redis-data/` 是运行时目录（不入库，起 Redis 前建好即可）。
