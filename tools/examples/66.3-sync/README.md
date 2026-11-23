# 66.3-sync：多端同步

[66.3 多端同步](../../../v2/L4-组件与技术/66-即时通信与音视频/66.3-多端同步.md) 的配套代码（Go + Redis）。

```bash
cd tools/examples/66.3-sync/code
go mod download
go run ./cmd/sim         # 或 proto / cost / syncapi
```

四个命令分别对应协议选型、同步成本、同步 API、端到端模拟；`raw/` 是原始输出。
