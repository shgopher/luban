#!/usr/bin/env bash
# 实验 D：RTMP 推流 → 接入(ffmpeg -listen 1 当服务端) → 2s HLS 切片 → 拉流校验
# 说明：ffmpeg 的 -listen 1 只接 1 个连接，是"最小 RTMP 服务端"，用来验证推拉流链路本身；
#       生产接入用 SRS / MediaMTX / 云直播（多路分发、鉴权、GOP 缓存、录制）。
# 注意：本机实测 -listen 1 在推流端 EOF 后，推流/服务两端的 RTMP close 握手会互等卡住，
#       所以脚本用 watchdog 强杀收尾（不是链路本身的问题，是玩具服务端的边界）。
set -uo pipefail
ROOT=/Users/shgopher/Desktop/family/luban
S=$ROOT/.cache/ch66/s666
OUT=$S/out
SRC=$OUT/source_1080p30.mp4
PORT=19350
mkdir -p "$OUT/rtmp_hls"
rm -f "$OUT/rtmp_hls"/* "$S/raw/exp_d_poll.txt"
: > "$S/raw/exp_d_server.log"
: > "$S/raw/exp_d_push.log"

wait_or_kill() { # $1=pid $2=秒数；超时则强杀并返回 1
  local i
  for i in $(seq 1 $(($2 * 4))); do
    kill -0 "$1" 2>/dev/null || return 0
    sleep 0.25
  done
  kill -9 "$1" 2>/dev/null || true
  echo "  [watchdog] pid=$1 超过 ${2}s 未退出，已强杀" >&2
  return 1
}

echo "=== D1. RTMP 服务端（-listen 1，端口 $PORT）：收到流后直接切 2s HLS（-c copy，不重编码）==="
ffmpeg -hide_banner -y -listen 1 -i "rtmp://127.0.0.1:$PORT/live/test" \
  -c copy -f hls -hls_time 2 -hls_list_size 0 -hls_segment_type mpegts \
  -hls_segment_filename "$OUT/rtmp_hls/seg_%03d.ts" "$OUT/rtmp_hls/index.m3u8" \
  > "$S/raw/exp_d_server.log" 2>&1 &
SERVER_PID=$!
sleep 1.5

echo "=== D2. 起轮询（每 50ms 读一次播放列表）==="
python3 -u "$S/code/poll_hls.py" "$OUT/rtmp_hls/index.m3u8" 24 > "$S/raw/exp_d_poll.txt" 2>&1 &
POLL_PID=$!
sleep 0.3
python3 -c 'import time; print(f"{time.time():.3f}")' > "$S/raw/exp_d_t0.txt"

echo "=== D3. 推流端：-re 实时推 20s、-c copy（推流不改编码）==="
ffmpeg -hide_banner -y -re -t 20 -i "$SRC" -c copy -f flv \
  "rtmp://127.0.0.1:$PORT/live/test" > "$S/raw/exp_d_push.log" 2>&1 &
PUSH_PID=$!

wait_or_kill "$PUSH_PID" 26 && echo "  推流端正常退出" || true
PUSH_LAST=$(grep -E 'frame=' "$S/raw/exp_d_push.log" | tail -1 | sed 's/.*\(frame=.*\)/\1/')
echo "  推流端最后进度: $PUSH_LAST"
sleep 1.5
wait_or_kill "$SERVER_PID" 8 || true
wait_or_kill "$POLL_PID" 3 || true

echo
echo "=== D4. 服务端收到的流参数（ffmpeg -listen 1 的 Input #0）==="
grep -E 'Input #0|Stream #0:0|Stream #0:1' "$S/raw/exp_d_server.log" | head -4

echo
echo "=== D5. 轮询记录：推流开始后切片何时进入播放列表 ==="
cat "$S/raw/exp_d_poll.txt"
T0=$(cat "$S/raw/exp_d_t0.txt")
python3 - "$S/raw/exp_d_poll.txt" "$T0" <<'PY'
import re, sys
lines = open(sys.argv[1]).read().splitlines()
t0 = float(sys.argv[2])
marks = []
for ln in lines:
    m = re.search(r"切片数=\s*(\d+)", ln)
    ep = re.search(r"epoch=([0-9.]+)", ln)
    if m and ep:
        marks.append((int(m.group(1)), float(ep.group(1)) - t0))
for n, d in marks:
    if n > 0:
        print(f"推流起点 → 播放列表出现第 {n} 个切片: {d:.2f}s")
PY

echo
echo "=== D6. 拉流侧校验：HLS 播放列表与切片 ==="
echo "切片文件数: $(ls "$OUT/rtmp_hls"/*.ts 2>/dev/null | wc -l | tr -d ' ')"
python3 "$S/code/parse_hls.py" "$OUT/rtmp_hls/index.m3u8" --seq
echo "--- 用 ffprobe 像播放器一样读这条 HLS（拉流校验）---"
ffprobe -v error -show_entries format=duration,format_name \
  -show_entries stream=codec_name,width,height,avg_frame_rate \
  -of default=noprint_wrappers=1 "$OUT/rtmp_hls/index.m3u8" 2>&1 | grep -E 'duration|format_name|codec_name|width|height' | head -8
echo "--- 单个切片的实际码率 ---"
ffprobe -v error -show_entries format=duration,size,bit_rate -of default=noprint_wrappers=1 \
  "$OUT/rtmp_hls/seg_001.ts" 2>/dev/null

echo
echo "=== D7. 边界：推流进行中尝试第二个客户端拉流（-listen 1 只能服务 1 个连接）==="
rm -f "$OUT/rtmp_hls"/*
ffmpeg -hide_banner -y -listen 1 -i "rtmp://127.0.0.1:$PORT/live/test" \
  -c copy -f flv /dev/null > "$S/raw/exp_d_server2.log" 2>&1 &
SRV2=$!
sleep 1.2
ffmpeg -hide_banner -y -re -t 8 -i "$SRC" -c copy -f flv \
  "rtmp://127.0.0.1:$PORT/live/test" > "$S/raw/exp_d_push2.log" 2>&1 &
PUSH2=$!
sleep 2
python3 - <<'PY' | tee "$S/raw/exp_d_second_client.log"
import subprocess
try:
    r = subprocess.run(["ffprobe", "-v", "error", "rtmp://127.0.0.1:19350/live/test",
                        "-show_entries", "stream=codec_name", "-of", "csv=p=0"],
                       capture_output=True, text=True, timeout=5)
    print(f"第二个客户端 rc={r.returncode} stdout={r.stdout.strip()!r} stderr={r.stderr.strip()[:160]!r}")
except subprocess.TimeoutExpired:
    print("第二个客户端：5s 超时内既没连上也没拉到流")
PY
sleep 1
kill -9 "$PUSH2" "$SRV2" 2>/dev/null || true
wait 2>/dev/null || true
echo "（已清理本次实验的 ffmpeg 进程）"
