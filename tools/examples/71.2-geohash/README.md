# 71.2-geohash：GeoHash 与空间索引

[71.2 GeoHash 与空间索引](../../../v2/L4-组件与技术/71-地理位置服务/71.2-GeoHash与空间索引.md) 的配套代码。

```bash
cd tools/examples/71.2-geohash/code
go mod download
go run .
```

依赖 `h3-go`、`golang/geo`、`go-redis`；实验覆盖 GeoHash/R-Tree/S2 与 Redis GEO 的精度与查询代价。`raw/` 是原始输出。
