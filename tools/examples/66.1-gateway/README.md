# 66.1-gateway：长连接网关

[66.1 长连接网关](../../../v2/L4-组件与技术/66-即时通信与音视频/66.1-长连接网关.md) 的配套代码（Go + WebSocket）。

```bash
cd tools/examples/66.1-gateway/code
go mod download
go run ./cmd/...        # 子命令见 cmd/ 目录
```

依赖 `gorilla/websocket`；`internal/` 是网关内核，`scripts/` 是压测脚本，`raw/` 与 `logs/` 是原始输出。
