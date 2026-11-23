#!/usr/bin/env python3
# expD：群视频带宽账（纯算式，不含实测）。
# 假设：1080p 单路 3 Mbps；N 人，每人上行 1 路、下行 N-1 路（SFU）。
BW = 3.0  # Mbps，1080p30 单路码率（量级假设，见正文说明）
CLOUD_EGRESS_CNY_PER_GB = 0.70  # 国内云公网出流量量级，元/GB（公开报价量级）

def sfu(n, r=BW):
    up = n * r                 # 所有客户端上行，SFU 入口
    egress = n * (n - 1) * r   # SFU 出口 = 每人 N-1 路
    return up, egress

def mesh(n, r=BW):
    per_client_up = (n - 1) * r
    return per_client_up, n * per_client_up

def mcu(n, r_in=BW, r_out=BW):
    # 服务端解 N 路、混流后每客户端 1 路
    return n * r_in, n * r_out

def hour_gb(mbps):
    return mbps * 1e6 / 8 * 3600 / 1e9

print("=== 单路 %s Mbps，N 人，SFU 出口 = N*(N-1)*r ===" % BW)
print("%3s | %12s | %14s | %12s | %10s" % ("N", "SFU入口Mbps", "SFU出口Mbps", "服务器合计", "出口GB/小时"))
for n in (2, 3, 4, 6, 9, 16, 25, 50):
    up, eg = sfu(n)
    print("%3d | %12.1f | %14.1f | %12.1f | %10.2f" % (n, up, eg, up + eg, hour_gb(eg)))

print()
print("=== 1v1（N=2）对比 ===")
up2, eg2 = sfu(2)
print("1v1 经 SFU：入口 %.1f Mbps，出口 %.1f Mbps，合计 %.1f Mbps" % (up2, eg2, up2 + eg2))
up9, eg9 = sfu(9)
print("9 人经 SFU：入口 %.1f Mbps，出口 %.1f Mbps，合计 %.1f Mbps" % (up9, eg9, up9 + eg9))
print("服务器流量倍数（N=9 vs N=2）：%.1fx" % ((up9 + eg9) / (up2 + eg2)))
print("服务器出口倍数：%.1fx" % (eg9 / eg2))

print()
print("=== P2P 全互联（无服务器媒体） ===")
for n in (2, 3, 4, 6, 9):
    per, total = mesh(n)
    print("N=%2d：每客户端上行 %.1f Mbps、下行 %.1f Mbps；全网客户端上行合计 %.1f Mbps" % (n, per, per, total))

print()
print("=== 9 人三种拓扑对比（单路 %.1f Mbps）===" % BW)
n = 9
for name, (up, eg) in (("P2P 全互联", (mesh(n)[1], 0.0)), ("SFU", sfu(n)), ("MCU(1 路混流)", mcu(n))):
    tot = up + eg
    print("%-12s 服务器入口 %7.1f  出口 %7.1f  合计 %7.1f Mbps  出口流量成本 %.2f 元/小时"
          % (name, up, eg, tot, hour_gb(eg) * CLOUD_EGRESS_CNY_PER_GB))

print()
print("=== TURN 中继成本（1v1 双方都经 TURN）===")
r = BW
print("媒体码率 %.1f Mbps/方向；TURN 入口 %.1f、出口 %.1f，TURN 流量合计 %.1f Mbps = %.1fx 媒体码率"
      % (r, 2 * r, 2 * r, 4 * r, 4.0))
print("若 1v1 直连 P2P：服务器流量 0；若只有一方经 TURN：TURN 合计 %.1f Mbps" % (2 * r))
