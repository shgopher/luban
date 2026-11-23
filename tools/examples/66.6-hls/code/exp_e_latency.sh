#!/usr/bin/env bash
# 实验 E：延迟构成（直播 HLS 首片等待 + 编码器 lookahead 延迟）
#   E1 默认 libx264 参数（rc-lookahead=40 帧）从 -re 输入转 2s HLS，测"开始→首片可播"
#   E2 加 -tune zerolatency（关预读），同一测法
#   两者差值 ≈ 编码器预读带来的额外延迟
set -uo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
S=$ROOT/.cache/ch66/s666
OUT=$S/out
SRC=$OUT/source_1080p30.mp4
mkdir -p "$OUT/live_hls"

wait_or_kill() { # $1=pid $2=秒数
  local i
  for i in $(seq 1 $(($2 * 4))); do kill -0 "$1" 2>/dev/null && sleep 0.25 || return 0; done
  kill -9 "$1" 2>/dev/null || true
  return 1
}

measure() { # $1=输出名 $2=额外编码参数
  local name=$1; shift
  local dir="$OUT/live_hls/$name"
  rm -rf "$dir"; mkdir -p "$dir"
  python3 -u "$S/code/poll_hls.py" "$dir/index.m3u8" 14 > "$S/raw/exp_e_poll_$name.txt" 2>&1 &
  local poll=$!
  sleep 0.2
  python3 -c 'import time; print(f"{time.time():.3f}")' > "$S/raw/exp_e_t0_$name.txt"
  ffmpeg -hide_banner -y -re -t 10 -i "$SRC" -vf scale=1280:720 \
    -c:v libx264 -preset veryfast -profile:v main -pix_fmt yuv420p \
    -b:v 2500k -maxrate 2500k -bufsize 5000k -g 60 -keyint_min 60 -sc_threshold 0 -an \
    "$@" -f hls -hls_time 2 -hls_list_size 0 -hls_segment_type mpegts \
    -hls_segment_filename "$dir/seg_%03d.ts" "$dir/index.m3u8" \
    > "$S/raw/exp_e_ffmpeg_$name.log" 2>&1
  wait_or_kill "$poll" 6 || true
  local t0; t0=$(cat "$S/raw/exp_e_t0_$name.txt")
  python3 - "$S/raw/exp_e_poll_$name.txt" "$t0" "$name" <<'PY'
import re, sys
lines = open(sys.argv[1]).read().splitlines()
t0 = float(sys.argv[2]); name = sys.argv[3]
marks = []
for ln in lines:
    m = re.search(r"切片数=\s*(\d+)", ln); ep = re.search(r"epoch=([0-9.]+)", ln)
    if m and ep: marks.append((int(m.group(1)), float(ep.group(1)) - t0))
pos = [(n, d) for n, d in marks if n > 0]
print(f"[{name}] 起播→第 1 个切片可播: {pos[0][1]:.2f}s" if pos else f"[{name}] 没测到切片")
if len(pos) >= 2:
    gaps = [round(pos[i][1] - pos[i-1][1], 2) for i in range(1, len(pos))]
    print(f"[{name}] 之后每个切片的出现间隔: {gaps}")
PY
}

echo "=== E1. 默认参数（x264 默认 rc-lookahead=40 帧 ≈ 1.3s 预读）==="
measure default

echo
echo "=== E2. -tune zerolatency（关预读/B 帧重排，直播必开）==="
measure zerolatency -tune zerolatency

echo
echo "=== E3. 两组的切片时长（-re 实时产出，所以是 2s 一片）==="
python3 "$S/code/parse_hls.py" "$OUT/live_hls/default/index.m3u8" "$OUT/live_hls/zerolatency/index.m3u8"
