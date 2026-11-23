#!/usr/bin/env python3
"""本机仿真：简化版本链 + 快照读，看清 RC / RR 的可见性差别。

两个概念：
  版本链：每条版本 = (值, 提交时刻, 写入事务)；未提交的写不落链，COMMIT 才落
  快照点：一个提交时刻上限，只可见 commit_ts <= 快照点 的版本
    RC（读已提交）：快照点 = 语句执行时刻，每条语句重新取
    RR（可重复读）：快照点 = 事务开始时刻，整个事务复用

时间线（秒）：
  t=0  基线 A=1
  t=1  T1 开始；t=2 T1 当前读 A=1，写 A=2（悬挂）
  t=3  T3 开始（RR 的快照点 = 3）
  t=5  T1 提交 -> v2
  t=6  T2 开始，当前读 A=2，写 A=3（悬挂）；T5 也在 t=6 开始
  t=8  T2 提交 -> v3
"""
import unicodedata

CHAIN = [(1, 0, "基线")]          # (值, 提交时刻, 写入事务)


def commit(value, ts, txn):
    CHAIN.append((value, ts, txn))


def visible(snapshot):
    """版本链里 commit_ts <= 快照点 的最新一版。"""
    best = None
    for val, ts, txn in CHAIN:
        if ts <= snapshot:
            best = (val, ts, txn)
    return best


def w(s):                          # 终端显示宽度：中文按 2 列算
    return sum(2 if unicodedata.east_asian_width(c) in "WF" else 1 for c in s)


def pad(s, n):
    return s + " " * (n - w(s))


def row(cells, widths):
    return "  ".join(pad(str(c), n) for c, n in zip(cells, widths))


# ---- 写事务：先当前读（写前提），再悬挂写，最后提交 ----
t1_base = visible(2)                    # T1 在 t=2 的当前读
commit(2, 5, "T1")
t2_base = visible(6)                    # T2 在 t=6 的当前读（T1 已提交）
commit(3, 8, "T2")

# ---- 读事务：T3 在 t=3 开始读 3 次；T5 在 t=6 开始读 2 次 ----
READS = [("T3", 3, [4, 6, 9]), ("T5", 6, [7, 9])]

W1 = [8, 6, 12, 10]
print("版本链（按提交时刻）：")
print("  " + row(["版本", "值", "提交时刻", "写入事务"], W1))
for i, (val, ts, txn) in enumerate(CHAIN, start=1):
    print("  " + row([f"v{i}", val, ts, txn], W1))

print("\n写事务的当前读（写的前提，不是本事务的快照读）：")
print(f"  T1 在 t=2 当前读 A={t1_base[0]} -> 写 A=2（悬挂，t=5 提交）")
print(f"  T2 在 t=6 当前读 A={t2_base[0]} -> 写 A=3（悬挂，t=8 提交）")

W2 = [6, 6, 8, 12, 12, 8, 8]
print("\n读事务的可见性（快照点相同则读数相同）：")
print("  " + row(["事务", "开始", "读时刻", "RC 快照点", "RR 快照点",
                  "RC 读到", "RR 读到"], W2))
for txn, start, ticks in READS:
    for t in ticks:
        rc, rr = visible(t), visible(start)   # RC 按语句取，RR 按事务开始取
        print("  " + row([txn, start, t, t, start, rc[0], rr[0]], W2))

t3_rc = sorted({visible(t)[0] for t in [4, 6, 9]})
t3_rr = sorted({visible(3)[0] for _ in [4, 6, 9]})
print(f"\nT3 一个事务里看到的值的集合：RC={t3_rc}，RR={t3_rr}")

# ---- 长事务的清理代价：版本链长度 -> 清理延迟 ----
WRITE_TPS, BYTES_PER_VER = 5000, 200      # 假设：热行 5000 写/s，每版本约 200B
PURGE_RATES = {"清理@20万/s": 200_000, "清理@5万/s": 50_000}

print("\n长事务的清理代价（本机计算：写入 5000 版本/s、每版本 200B）：")
W3 = [16, 18, 16, 15, 12]
print("  " + row(["版本链长度", "对应长事务时长", "清理@20万/s", "清理@5万/s", "占用磁盘"], W3))
for n in [10_000, 100_000, 1_000_000, 10_000_000]:
    disk = n * BYTES_PER_VER / 1e9
    disk_s = f"{disk:.2f} GB" if disk >= 1 else f"{disk * 1000:.0f} MB"
    print("  " + row([f"{n:,}", f"{n / WRITE_TPS:.0f} s",
                      f"{n / PURGE_RATES['清理@20万/s']:.2f} s",
                      f"{n / PURGE_RATES['清理@5万/s']:.2f} s", disk_s], W3))

assert (1_000_000 / 200_000) == 5.0 and (10_000_000 / 5000) == 2000.0
print("\n断言通过：清理耗时 = 版本链长度 ÷ 清理速率；长事务时长 = 版本链长度 ÷ 写入速率")
