# 70.3-fanout：关注关系与扩散

[70.3 关注关系与扩散](../../../v2/L4-组件与技术/70-内容与社交/70.3-关注关系与扩散.md) 的配套代码（Go + Redis + MySQL）。

```bash
cd tools/examples/70.3-fanout/code
go mod download
go run ./cmd/fanout        # 或 redisbench / mysqlbench
```

写扩散与读扩散的写入放大、Redis 与 MySQL 各自的基准。依赖 Redis/MySQL，连接参数见 `main.go` 顶部。
