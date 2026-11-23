#!/usr/bin/env bash
# 实验源：60s 1080p30 合成源，固定 2s GOP（-g 60），后续所有实验复用这一份
# 合成源用 testsrc2（运动纹理+文字+渐变，编码复杂度接近真实画面）+ sine 音频
set -euo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
OUT=$ROOT/.cache/ch66/s666/out
mkdir -p "$OUT"

ffmpeg -hide_banner -y \
  -f lavfi -i "testsrc2=size=1920x1080:rate=30:duration=60" \
  -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=60" \
  -c:v libx264 -preset veryfast -crf 18 -pix_fmt yuv420p \
  -g 60 -keyint_min 60 -sc_threshold 0 \
  -c:a aac -b:a 128k -ac 2 \
  -movflags +faststart -benchmark \
  "$OUT/source_1080p30.mp4" 2>&1 | grep -E 'bench:|frame=|video:|audio:' | tail -4

echo "--- ffprobe source ---"
ffprobe -v error \
  -show_entries format=duration,size,bit_rate \
  -show_entries stream=codec_name,width,height,r_frame_rate,avg_frame_rate,nb_frames \
  -of default=noprint_wrappers=1 "$OUT/source_1080p30.mp4"

echo "--- 关键帧时间戳（应恒为 2s 一座：0,2,4,...,58）---"
ffprobe -v error -select_streams v:0 -skip_frame nokey -show_entries frame=pts_time -of csv=p=0 \
  "$OUT/source_1080p30.mp4" 2>/dev/null | tr -d ',' | head -30 | tr '\n' ' '
echo
echo "关键帧总数: $(ffprobe -v error -select_streams v:0 -skip_frame nokey -show_entries frame=pts_time -of csv=p=0 "$OUT/source_1080p30.mp4" 2>/dev/null | wc -l | tr -d ' ')"
