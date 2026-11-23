# 71.1-geo：地理编码与坐标系统

[71.1 地理编码与坐标系统](../../../v2/L4-组件与技术/71-地理位置服务/71.1-地理编码与坐标系统.md) 的配套代码。

```bash
cd tools/examples/71.1-geo/code
go run ./cmd/offset     # 或 inverse / storage / mercator
```

`internal/geo` 是坐标换算内核；四个命令分别验证偏移、逆地理、存储精度、墨卡托投影。`raw/` 是原始输出。
