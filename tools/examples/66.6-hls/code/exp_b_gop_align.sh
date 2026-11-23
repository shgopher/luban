#!/usr/bin/env bash
# 实验 B（核心）：GOP 与切片时长对齐对比
#   B1 2s GOP (-g 60)  + 2s 切片  → 预期整齐
#   B2 1s GOP (-g 30)  + 2s 切片  → 切片点是 GOP 整数倍，仍整齐
#   B3 1.5s GOP(-g 45) + 2s 切片  → 非整数倍，切片被拉到 3s
#   B4 源关键帧抖动（模拟场景切换关键帧）+ -c copy 直通切片 → 时长混合
set -euo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
S=$ROOT/.cache/ch66/s666
OUT=$S/out
SRC=$OUT/source_1080p30.mp4
COMMON="-hide_banner -y -i $SRC -vf scale=1280:720 -c:v libx264 -preset veryfast -profile:v main -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k -an"

mkdir -p "$OUT/gop/g1_2s" "$OUT/gop/g2_1s" "$OUT/gop/g3_1p5s" "$OUT/gop/g4_jitter"
echo "=== B1. 2s GOP（-g 60）+ 2s 切片 ==="
ffmpeg $COMMON -g 60 -keyint_min 60 -sc_threshold 0 \
  -f hls -hls_time 2 -hls_playlist_type vod -hls_segment_type mpegts \
  -hls_segment_filename "$OUT/gop/g1_2s/seg_%03d.ts" "$OUT/gop/g1_2s/index.m3u8" 2>&1 | grep -E 'frame=' | tail -1

echo "=== B2. 1s GOP（-g 30）+ 2s 切片 ==="
ffmpeg $COMMON -g 30 -keyint_min 30 -sc_threshold 0 \
  -f hls -hls_time 2 -hls_playlist_type vod -hls_segment_type mpegts \
  -hls_segment_filename "$OUT/gop/g2_1s/seg_%03d.ts" "$OUT/gop/g2_1s/index.m3u8" 2>&1 | grep -E 'frame=' | tail -1

echo "=== B3. 1.5s GOP（-g 45，非切片整数倍）+ 2s 切片 ==="
ffmpeg $COMMON -g 45 -keyint_min 45 -sc_threshold 0 \
  -f hls -hls_time 2 -hls_playlist_type vod -hls_segment_type mpegts \
  -hls_segment_filename "$OUT/gop/g3_1p5s/seg_%03d.ts" "$OUT/gop/g3_1p5s/index.m3u8" 2>&1 | grep -E 'frame=' | tail -1

echo
echo "=== B4. 造一份关键帧抖动的源（-force_key_frames 不规则插帧 = 场景切换插关键帧）==="
KF=$(python3 - <<'PY'
# 确定性伪随机：每 1.2~2.8s 一次强制关键帧，模拟主播切场景导致编码器插入额外 IDR
import random
random.seed(20261121)
t, out = 0.0, ["0"]
while True:
    t += random.uniform(1.2, 2.8)
    if t >= 59.5:
        break
    out.append(f"{t:.2f}")
print(",".join(out))
PY
)
echo "强制关键帧时间点: $(echo "$KF" | tr ',' ' ')"
ffmpeg $COMMON -g 60 -keyint_min 15 -sc_threshold 40 -force_key_frames "$KF" \
  -f mp4 "$OUT/src_jitter_720p.mp4" 2>&1 | grep -E 'frame=' | tail -1
echo "--- 抖动源的实际关键帧间隔（秒）---"
python3 - "$OUT/src_jitter_720p.mp4" <<'PY'
import subprocess, sys
out = subprocess.run(["ffprobe","-v","error","-select_streams","v:0","-skip_frame","nokey",
                      "-show_entries","frame=pts_time","-of","csv=p=0", sys.argv[1]],
                     capture_output=True, text=True).stdout
ts = [float(x.strip().rstrip(',')) for x in out.splitlines() if x.strip()]
gaps = [round(b - a, 3) for a, b in zip(ts, ts[1:])]
print(f"关键帧数={len(ts)} 间隔 min={min(gaps)}s max={max(gaps)}s 均值={sum(gaps)/len(gaps):.3f}s")
print("前 20 个间隔: " + " ".join(f"{g:g}" for g in gaps[:20]))
PY

echo "--- B4 切片：对抖动源做 -c copy 直通切片（不重编码，切片点只能跟着源关键帧走）---"
ffmpeg -hide_banner -y -i "$OUT/src_jitter_720p.mp4" -c copy \
  -f hls -hls_time 2 -hls_playlist_type vod -hls_segment_type mpegts \
  -hls_segment_filename "$OUT/gop/g4_jitter/seg_%03d.ts" "$OUT/gop/g4_jitter/index.m3u8" 2>&1 | grep -E 'frame=|video:' | tail -1

echo
echo "=== B5. 四组切片时长统计（目标都是 2.000s）==="
python3 "$S/code/parse_hls.py" \
  "$OUT/gop/g1_2s/index.m3u8" "$OUT/gop/g2_1s/index.m3u8" \
  "$OUT/gop/g3_1p5s/index.m3u8" "$OUT/gop/g4_jitter/index.m3u8" --seq

echo
echo "=== B6. 各组 TARGETDURATION 与首 3 片 EXTINF ==="
for g in g1_2s g2_1s g3_1p5s g4_jitter; do
  echo -n "$g: "
  grep -E '^#EXT-X-TARGETDURATION|^#EXTINF' "$OUT/gop/$g/index.m3u8" | head -4 | tr '\n' ' '
  echo
done
