#!/usr/bin/env bash
# 实验 C 补充：
#   S1 CPU 计账：utime/stime/rtime → "省 CPU" 到底省多少
#   S2 SSIM 重复性：同码率 10s 片段各 3 次取中位
#   S3 低码率判别：码率越低两种编码器差距越大（合成源在 2.5M 已接近透明）
#   S4 加噪变体：叠均匀噪声模拟摄像头颗粒，比合成源更接近真实采集
set -euo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
S=$ROOT/.cache/ch66/s666
OUT=$S/out
SRC=$OUT/source_1080p30.mp4
mkdir -p "$OUT/speed" "$OUT/ssim"

enc() { # $1=x264|vt  $2=out  $3=dur  $4=bitratek  $5=vf
  local vf=$5
  if [ "$1" = "x264" ]; then
    ffmpeg -hide_banner -y -t "$3" -i "$SRC" -vf "$vf" \
      -c:v libx264 -preset veryfast -profile:v main -pix_fmt yuv420p \
      -b:v "$4" -maxrate "$4" -bufsize "$((${4%k} * 2))k" \
      -g 60 -keyint_min 60 -sc_threshold 0 -an -f mp4 "$2" 2>&1
  else
    ffmpeg -hide_banner -y -t "$3" -i "$SRC" -vf "$vf" \
      -c:v h264_videotoolbox -profile:v main -pix_fmt yuv420p \
      -b:v "$4" -maxrate "$4" -bufsize "$((${4%k} * 2))k" \
      -g 60 -keyint_min 60 -sc_threshold 0 -an -f mp4 "$2" 2>&1
  fi
}
ssim() { # $1=enc.mp4 $2=ref
  ffmpeg -hide_banner -i "$1" -i "$2" -lavfi "[0:v][1:v]ssim" -f null - 2>&1 \
    | grep -oE 'All:[0-9.]+' | head -1 | cut -d: -f2
}
abr() { ffprobe -v error -show_entries format=bit_rate -of csv=p=0 "$1"; }

echo "=== S1. CPU 计账（60s 1080p30 → 720p@2500k，单次）==="
for b in x264 vt; do
  if [ "$b" = "x264" ]; then
    line=$(ffmpeg -hide_banner -y -i "$SRC" -vf scale=1280:720 -c:v libx264 -preset veryfast \
      -profile:v main -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k \
      -g 60 -keyint_min 60 -sc_threshold 0 -an -f mp4 "$OUT/speed/cpu_$b.mp4" -benchmark 2>&1 \
      | grep -E '^bench:')
  else
    line=$(ffmpeg -hide_banner -y -i "$SRC" -vf scale=1280:720 -c:v h264_videotoolbox \
      -profile:v main -pix_fmt yuv420p -b:v 2500k -maxrate 2500k -bufsize 5000k \
      -g 60 -keyint_min 60 -sc_threshold 0 -an -f mp4 "$OUT/speed/cpu_$b.mp4" -benchmark 2>&1 \
      | grep -E '^bench:')
  fi
  echo "  $b: $line"
done

echo
echo "=== S2. SSIM 重复性（clean 源，名义 2500k，10s，各 3 次）==="
VF_CLEAN="scale=1280:720"
ffmpeg -hide_banner -y -t 10 -i "$SRC" -vf "$VF_CLEAN" -c:v libx264 -qp 0 -preset veryfast \
  -pix_fmt yuv420p -an "$OUT/ssim/ref_clean.mkv" 2>/dev/null
for b in x264 vt; do
  vals=()
  for i in 1 2 3; do
    enc "$b" "$OUT/ssim/${b}_clean_run$i.mp4" 10 2500k "$VF_CLEAN" >/dev/null 2>&1
    vals+=("$(ssim "$OUT/ssim/${b}_clean_run$i.mp4" "$OUT/ssim/ref_clean.mkv")")
  done
  echo "  $b 名义2500k 实际码率=$(abr "$OUT/ssim/${b}_clean_run1.mp4") SSIM 3次=${vals[*]}"
done

echo
echo "=== S3. 低码率判别（clean 源，10s，单次）==="
for br in 1000k 1500k; do
  for b in x264 vt; do
    enc "$b" "$OUT/ssim/${b}_clean_$br.mp4" 10 "$br" "$VF_CLEAN" >/dev/null 2>&1
    echo "  $b 名义$br 实际码率=$(abr "$OUT/ssim/${b}_clean_$br.mp4") SSIM=$(ssim "$OUT/ssim/${b}_clean_$br.mp4" "$OUT/ssim/ref_clean.mkv")"
  done
done

echo
echo "=== S4. 加噪变体（noise=alls=15:allf=t+u:all_seed=42，模拟摄像头颗粒，名义 2500k）==="
VF_NOISE="scale=1280:720,noise=alls=15:allf=t+u:all_seed=42"
ffmpeg -hide_banner -y -t 10 -i "$SRC" -vf "$VF_NOISE" -c:v libx264 -qp 0 -preset veryfast \
  -pix_fmt yuv420p -an "$OUT/ssim/ref_noise.mkv" 2>/dev/null
for b in x264 vt; do
  enc "$b" "$OUT/ssim/${b}_noise.mp4" 10 2500k "$VF_NOISE" >/dev/null 2>&1
  echo "  $b 名义2500k 实际码率=$(abr "$OUT/ssim/${b}_noise.mp4") SSIM=$(ssim "$OUT/ssim/${b}_noise.mp4" "$OUT/ssim/ref_noise.mkv")"
done
echo
echo "参考(无损)体积: clean=$(ffprobe -v error -show_entries format=size -of csv=p=0 "$OUT/ssim/ref_clean.mkv") noise=$(ffprobe -v error -show_entries format=size -of csv=p=0 "$OUT/ssim/ref_noise.mkv")"
