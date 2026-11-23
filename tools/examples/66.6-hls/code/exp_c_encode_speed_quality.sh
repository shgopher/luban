#!/usr/bin/env bash
# 实验 C：libx264(软编) vs h264_videotoolbox(硬编) 的速度与同码率画质
#   C1 速度：60s 1080p30 → 720p@2500k，各跑 3 次取中位数（-benchmark 的 rtime 才是真实墙钟）
#   C2 画质：10s 片段编 2500k，对 720p 无损参考算 SSIM
#   C3 补码率：VT 逐步加码率，看加多少才能追平 x264@2500k 的 SSIM
set -euo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
S=$ROOT/.cache/ch66/s666
OUT=$S/out
SRC=$OUT/source_1080p30.mp4
mkdir -p "$OUT/speed" "$OUT/ssim"

# 三个编码后端共用的"同码率"约束：CBR 味道的 -b:v/-maxrate/-bufsize，GOP 固定 2s
BITRATE_ARGS=(-b:v 2500k -maxrate 2500k -bufsize 5000k -g 60 -keyint_min 60 -sc_threshold 0)

run_speed() { # $1=backend $2=out ; 输出一行: rtime
  local rtime
  if [ "$1" = "x264" ]; then
    rtime=$(ffmpeg -hide_banner -y -i "$SRC" -vf scale=1280:720 \
      -c:v libx264 -preset veryfast -profile:v main -pix_fmt yuv420p \
      "${BITRATE_ARGS[@]}" -an -f mp4 "$2" -benchmark 2>&1 \
      | grep -oE 'rtime=[0-9.]+' | head -1 | cut -d= -f2)
  else
    rtime=$(ffmpeg -hide_banner -y -i "$SRC" -vf scale=1280:720 \
      -c:v h264_videotoolbox -profile:v main -pix_fmt yuv420p \
      "${BITRATE_ARGS[@]}" -an -f mp4 "$2" -benchmark 2>&1 \
      | grep -oE 'rtime=[0-9.]+' | head -1 | cut -d= -f2)
  fi
  echo "$rtime"
}

echo "=== C1. 速度：60s 1080p30 → 720p@2500k（同机并行任务下测得，3 次取中位）==="
X264_R=(); VT_R=()
for i in 1 2 3; do
  r=$(run_speed x264 "$OUT/speed/x264_run$i.mp4"); X264_R+=("$r")
  echo "  x264 veryfast run$i rtime=${r}s"
done
for i in 1 2 3; do
  r=$(run_speed vt "$OUT/speed/vt_run$i.mp4"); VT_R+=("$r")
  echo "  h264_videotoolbox run$i rtime=${r}s"
done

echo
echo "--- 中位数与实时倍数（60s 素材 / 耗时）---"
python3 - "${X264_R[@]}" --- "${VT_R[@]}" <<'PY'
import statistics, sys
a = sys.argv[1:]
sep = a.index("---")
x = [float(v) for v in a[:sep]]
v = [float(v) for v in a[sep + 1:]]
for name, r in (("libx264 veryfast", x), ("h264_videotoolbox", v)):
    med = statistics.median(r)
    print(f"{name:20s} 3 次={r} 中位={med:.2f}s 实时倍数={60/med:.2f}x 单路 1 小时内容耗时={med*(3600/60)/60:.1f}min")
print(f"硬编/软编耗时比 = {statistics.median(v)/statistics.median(x):.2f}x（<1 表示硬编更快）")
PY

echo
echo "--- 输出文件大小与实际码率（第 1 次产物）---"
for b in x264 vt; do
  echo -n "  $b: "
  ffprobe -v error -show_entries format=size,bit_rate,duration -of default=noprint_wrappers=1 \
    "$OUT/speed/${b}_run1.mp4" | tr '\n' ' '
  echo
done

echo
echo "=== C2. 同码率画质：10s 片段，SSIM 对 720p 无损参考 ==="
echo "--- 无损参考（-qp 0，仅用于当基准，不参与码率对比）---"
ffmpeg -hide_banner -y -t 10 -i "$SRC" -vf scale=1280:720 \
  -c:v libx264 -qp 0 -preset veryfast -pix_fmt yuv420p -an "$OUT/ssim/ref_720p_lossless.mkv" 2>&1 \
  | grep -E 'frame=' | tail -1
ffprobe -v error -show_entries format=size,bit_rate -of default=noprint_wrappers=1 "$OUT/ssim/ref_720p_lossless.mkv"

for b in x264 vt; do
  if [ "$b" = "x264" ]; then
    ffmpeg -hide_banner -y -t 10 -i "$SRC" -vf scale=1280:720 \
      -c:v libx264 -preset veryfast -profile:v main -pix_fmt yuv420p \
      "${BITRATE_ARGS[@]}" -an "$OUT/ssim/${b}_2500k.mp4" 2>&1 | grep -E 'frame=' | tail -1
  else
    ffmpeg -hide_banner -y -t 10 -i "$SRC" -vf scale=1280:720 \
      -c:v h264_videotoolbox -profile:v main -pix_fmt yuv420p \
      "${BITRATE_ARGS[@]}" -an "$OUT/ssim/${b}_2500k.mp4" 2>&1 | grep -E 'frame=' | tail -1
  fi
  echo -n "  ${b}_2500k 实际码率: "
  ffprobe -v error -show_entries format=bit_rate,size -of csv=p=0 "$OUT/ssim/${b}_2500k.mp4"
  ffmpeg -hide_banner -i "$OUT/ssim/${b}_2500k.mp4" -i "$OUT/ssim/ref_720p_lossless.mkv" \
    -lavfi "[0:v][1:v]ssim" -f null - 2>&1 | grep -oE 'SSIM .*' | tail -1 | sed "s/^/  ${b}_2500k /"
done

echo
echo "=== C3. 硬编加码率能不能追平软编@2500k 的画质 ==="
for br in 3000 4000 5000 7000; do
  ffmpeg -hide_banner -y -t 10 -i "$SRC" -vf scale=1280:720 \
    -c:v h264_videotoolbox -profile:v main -pix_fmt yuv420p \
    -b:v ${br}k -maxrate ${br}k -bufsize $((br * 2))k -g 60 -keyint_min 60 -sc_threshold 0 -an \
    "$OUT/ssim/vt_${br}k.mp4" 2>&1 | grep -E 'frame=' | tail -1 >/dev/null
  actual=$(ffprobe -v error -show_entries format=bit_rate -of csv=p=0 "$OUT/ssim/vt_${br}k.mp4")
  ssim=$(ffmpeg -hide_banner -i "$OUT/ssim/vt_${br}k.mp4" -i "$OUT/ssim/ref_720p_lossless.mkv" \
    -lavfi "[0:v][1:v]ssim" -f null - 2>&1 | grep -oE 'All:[0-9.]+' | head -1)
  echo "  vt_${br}k 实际码率=${actual} $ssim"
done
