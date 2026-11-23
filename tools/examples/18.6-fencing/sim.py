#!/usr/bin/env python3
"""本机仿真：一次切换里旧主的「重复写」次数——无 fencing vs 有 fencing。

时间线（离散秒，3 节点 A/B/C）：
  t=0   A 为旧主，租约 T=10s，fencing token=1
  t=10  注入网络分区，A 无法续约；A 的本地租约检查被 STW 暂停 3 秒
  t=11  B/C 达到多数派，提升 B 为新主，存储层当前 epoch 升到 2
  t=13  A 的租约检查恢复，停写（比租约到期晚 3 秒）
  A 每秒写一笔 key=k_<t>（+100 元），B 从 t=11 起也写同一批 key
  「重复写」= B 可写之后，同一个 key 又被 A 写成功
"""
import unicodedata

LEASE_TTL, PROMOTE, T_END, TOKEN_NEW = 10, 11, 20, 2
A_STOP = LEASE_TTL + 3          # 租约到期后，本地检查还被 STW 拖了 3 秒


def w(s):                       # 终端显示宽度：中文按 2 列算
    return sum(2 if unicodedata.east_asian_width(c) in "WF" else 1 for c in s)


def pad(s, n):
    return s + " " * (n - w(s))


def simulate(name, fencing, epoch_bump_at=PROMOTE):
    r = {"name": name, "sent": 0, "accepted": 0, "rejected": 0, "dup": []}
    epoch, b_keys = 1, set()
    for t in range(1, T_END + 1):
        if t == epoch_bump_at:
            epoch = TOKEN_NEW                     # 先递增 epoch，再允许新主写
        if t >= PROMOTE:                          # 新主每秒写一笔
            b_keys.add(f"k_{t}")
        if t <= A_STOP:                           # 旧主在停写前每秒也写一笔
            r["sent"] += 1
            if fencing and 1 < epoch:             # 旧主 token=1，存储层只认 2
                r["rejected"] += 1
            else:
                r["accepted"] += 1
                if f"k_{t}" in b_keys:            # 新主已写过同一个 key
                    r["dup"].append(f"k_{t}")
    return r


rows = [
    simulate("无 fencing（到达即接受）", fencing=False),
    simulate("有 fencing（先 +token 再提升）", fencing=True),
    simulate("有 fencing（顺序写反：先提升）", fencing=True, epoch_bump_at=12),
]
COLS = [("实现", 30), ("旧主写请求", 10), ("被接受", 8), ("被拒", 6),
        ("重复写次数", 10), ("重复的 key", 18), ("重复入账", 8)]
print("  ".join(pad(c, n) for c, n in COLS))
for r in rows:
    dup = len(r["dup"])
    cells = [r["name"], r["sent"], r["accepted"], r["rejected"],
             dup, ",".join(r["dup"]) or "—", f"{dup * 100} 元"]
    print("  ".join(pad(str(c), n) for c, (_, n) in zip(cells, COLS)))

overlap = A_STOP - PROMOTE + 1           # 新主可写 与 旧主仍在写 的重叠秒数
print(f"\n校验：重叠窗口 = A_STOP({A_STOP}) - PROMOTE({PROMOTE}) + 1 = {overlap} 秒")
print(f"      无 fencing 的重复写应等于重叠窗口：{len(rows[0]['dup'])}")
assert len(rows[0]["dup"]) == overlap
assert len(rows[1]["dup"]) == 0 and len(rows[2]["dup"]) == 1
print("断言全部通过（3 行结果可复算）")
