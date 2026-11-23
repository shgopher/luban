# 66.4-media：音视频信令与媒体传输

[66.4 音视频信令与媒体传输](../../../v2/L4-组件与技术/66-即时通信与音视频/66.4-音视频信令与媒体传输.md) 的配套代码（Go + WebRTC）。

```bash
cd tools/examples/66.4-media/code
go mod download
go run ./cmd/turnserver  # 或 expA / expC
```

依赖 `pion/webrtc`、`pion/ice`；编译产物（`s664-expA` 等）未入库。`raw/` 是实验输出。
