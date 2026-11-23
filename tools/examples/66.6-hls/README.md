# 66.6-hls：流媒体处理（HLS/DASH/FLV）

[66.6 流媒体处理](../../../v2/L4-组件与技术/66-即时通信与音视频/66.6-流媒体处理.md) 的配套脚本（ffmpeg + Python）。

```bash
cd tools/examples/66.6-hls/code
bash gen_source.sh      # 生成 1080p30 片源
bash exp_a_ladder_hls.sh
python3 parse_hls.py
```

`out/`（HLS 切片、录制 flv 等几百 MB 媒体文件）未入库，按脚本重新生成；正文引用的 708,397B / 22,020,096B 可在生成后 `ls -l` 复核。
