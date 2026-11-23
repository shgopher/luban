#!/usr/bin/env python3
"""轮询 HLS 媒体播放列表，打印每次"切片数变化"的时刻（相对 + 绝对 epoch，便于和推流起点对齐）。

用法: python3 poll_hls.py <index.m3u8> <观察秒数>
"""
import re
import sys
import time

path, dur = sys.argv[1], float(sys.argv[2])
t0 = time.time()
last = -1
while time.time() - t0 < dur:
    try:
        txt = open(path, encoding="utf-8").read()
    except FileNotFoundError:
        txt = ""
    durs = [float(x) for x in re.findall(r"^#EXTINF:([0-9.]+)", txt, re.M)]
    n, total = len(durs), sum(durs)
    if n != last:
        print(f"t={time.time() - t0:6.2f}s  epoch={time.time():.2f}  切片数={n:2d}  累计时长={total:5.1f}s")
        last = n
    time.sleep(0.05)
