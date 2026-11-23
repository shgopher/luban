#!/usr/bin/env python3
"""统计 HLS 媒体播放列表里每个切片的 #EXTINF 时长：数量 / min / max / 均值 / 标准差。

用法: python3 parse_hls.py <playlist.m3u8> [playlist2.m3u8 ...] [--seq]
标准差用总体标准差（分母 N），统计的是"这批切片自身"的离散度。
"""
import re
import statistics
import sys


def stats(path, label="", show_seq=False):
    text = open(path, encoding="utf-8").read()
    durs = [float(x) for x in re.findall(r"^#EXTINF:([0-9.]+)", text, re.M)]
    target = re.search(r"^#EXT-X-TARGETDURATION:(\d+)", text, re.M)
    n = len(durs)
    if n:
        mean = statistics.fmean(durs)
        sd = statistics.pstdev(durs)
        print(f"{label:24s} 切片数={n:3d} min={min(durs):.3f}s max={max(durs):.3f}s "
              f"均值={mean:.3f}s 标准差={sd:.3f} 总时长={sum(durs):.1f}s "
              f"TARGETDURATION={target.group(1) if target else '-'}")
        if show_seq:
            print("    时长序列: " + " ".join(f"{d:g}" for d in durs))
    else:
        print(f"{label:24s} 切片数=0")
    return durs


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if a != "--seq"]
    show_seq = "--seq" in sys.argv
    for p in args:
        stats(p, p.split("/")[-2] + "/" + p.split("/")[-1], show_seq)
