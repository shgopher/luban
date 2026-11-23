# 71.3-nearby：附近的人与距离计算

[71.3 附近的人与距离计算](../../../v2/L4-组件与技术/71-地理位置服务/71.3-附近的人与距离计算.md) 的配套代码。

```bash
cd tools/examples/71.3-nearby/code
go mod download
go run ./cmd/dist      # 或 privacy / redisstream / writeamp / rank
```

五个命令分别对应距离精度、隐私栅格、Redis Stream、写放大与排序。`internal/geoform` 是公共换算；`raw/` 是原始输出。
