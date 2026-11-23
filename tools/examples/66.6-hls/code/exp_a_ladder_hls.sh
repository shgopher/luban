#!/usr/bin/env bash
# 实验 A：一条命令、一次解码、两档输出（1080p 5000k / 720p 2500k）→ HLS TS 2s 切片 + master playlist
# 关键点：两路共用同一次解码（filter_complex split），直播里"解码 1 次、编码 N 次"是省钱的前提
set -euo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
S=$ROOT/.cache/ch66/s666
OUT=$S/out
SRC=$OUT/source_1080p30.mp4
mkdir -p "$OUT/hls_ts/1080p" "$OUT/hls_ts/720p"

echo "=== A1. 码率阶梯 + HLS(TS) 打包（2s 切片）==="
ffmpeg -hide_banner -y -i "$SRC" \
  -filter_complex "[0:v]split=2[v1080][v720src];[v720src]scale=1280:720[v720]" \
  -map "[v1080]" -map 0:a:0 -map "[v720]" -map 0:a:0 \
  -c:v:0 libx264 -preset veryfast -profile:v:0 main -pix_fmt yuv420p \
  -b:v:0 5000k -maxrate:v:0 5000k -bufsize:v:0 10000k \
  -c:v:1 libx264 -preset veryfast -profile:v:1 main -pix_fmt yuv420p \
  -b:v:1 2500k -maxrate:v:1 2500k -bufsize:v:1 5000k \
  -g 60 -keyint_min 60 -sc_threshold 0 \
  -c:a aac -b:a 128k -ac 2 \
  -f hls -hls_time 2 -hls_playlist_type vod -hls_segment_type mpegts \
  -hls_segment_filename "$OUT/hls_ts/%v/seg_%03d.ts" \
  -master_pl_name master.m3u8 \
  -var_stream_map "v:0,a:0,name:1080p v:1,a:1,name:720p" \
  -benchmark "$OUT/hls_ts/%v/index.m3u8" 2>&1 | grep -E 'bench: (utime|rtime)|frame=' | tail -3

echo
echo "=== A2. master playlist ==="
cat "$OUT/hls_ts/master.m3u8"
echo
echo "=== A3. 1080p 媒体播放列表（前 14 行）==="
head -14 "$OUT/hls_ts/1080p/index.m3u8"
echo
echo "=== A4. 切片清单（两档各数一数）==="
echo "1080p: $(ls "$OUT/hls_ts/1080p" | wc -l | tr -d ' ') 个文件, $(du -sh "$OUT/hls_ts/1080p" | cut -f1)"
echo "  $(ls "$OUT/hls_ts/1080p" | head -3 | tr '\n' ' ')...$(ls "$OUT/hls_ts/1080p" | tail -2 | tr '\n' ' ')"
echo "720p:  $(ls "$OUT/hls_ts/720p" | wc -l | tr -d ' ') 个文件, $(du -sh "$OUT/hls_ts/720p" | cut -f1)"

echo
echo "=== A5. #EXTINF 时长统计 ==="
python3 "$S/code/parse_hls.py" "$OUT/hls_ts/1080p/index.m3u8" "$OUT/hls_ts/720p/index.m3u8"

echo
echo "=== A6. ffprobe 校验：整条播放列表 / 单个切片 ==="
for r in 1080p 720p; do
  echo "-- $r 播放列表 --"
  ffprobe -v error -show_entries format=duration,format_name -of default=noprint_wrappers=1 \
    "$OUT/hls_ts/$r/index.m3u8"
  echo "-- $r 第一个切片 --"
  ffprobe -v error -show_entries stream=codec_name,width,height,avg_frame_rate \
    -show_entries format=duration,size,bit_rate -of default=noprint_wrappers=1 \
    "$OUT/hls_ts/$r/seg_000.ts"
done

echo
echo "=== A7. CMAF/fMP4 变体（同一源，720p 2500k，2s 切片，单个 init 段）==="
mkdir -p "$OUT/hls_fmp4"
ffmpeg -hide_banner -y -i "$SRC" -vf "scale=1280:720" \
  -c:v libx264 -preset veryfast -profile:v main -pix_fmt yuv420p \
  -b:v 2500k -maxrate 2500k -bufsize 5000k \
  -g 60 -keyint_min 60 -sc_threshold 0 -an \
  -f hls -hls_time 2 -hls_playlist_type vod \
  -hls_segment_type fmp4 -hls_fmp4_init_filename init.mp4 \
  -hls_segment_filename "$OUT/hls_fmp4/seg_%03d.m4s" \
  "$OUT/hls_fmp4/index.m3u8" 2>&1 | grep -E 'frame=' | tail -1
head -6 "$OUT/hls_fmp4/index.m3u8"
echo "fMP4 目录: $(ls "$OUT/hls_fmp4" | wc -l | tr -d ' ') 个文件, $(du -sh "$OUT/hls_fmp4" | cut -f1)"
echo "init.mp4: $(ffprobe -v error -show_entries format=duration,size -of csv=p=0 "$OUT/hls_fmp4/init.mp4")"
echo "第一个 m4s: $(ffprobe -v error -show_entries stream=codec_name,width,height -show_entries format=duration,size -of csv=p=0 "$OUT/hls_fmp4/seg_000.m4s")"
