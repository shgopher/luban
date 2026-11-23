#!/usr/bin/env python3
"""shard_retry.py —— 本机仿真：任务分片倾斜 + 失败重试的重复率。"""
import random
from collections import Counter

random.seed(42)
TASKS, SHARDS = 100_000, 16
# 0.1% 热键承担 80% 数据（模拟按用户/商家聚集的任务）
keys = []
for i in range(TASKS):
    if random.random() < 0.80:
        keys.append(random.randrange(100))          # 0.1% 热键
    else:
        keys.append(random.randrange(100_000))

def load(fn):
    c = Counter(fn(k) for k in keys)
    avg = TASKS / SHARDS
    return max(c.values()) / avg, min(c.values()) / avg

print("分片倾斜（最大/平均，理想=1.00）")
print("range(task_id):   %.2f×" % load(lambda k: (k * SHARDS) // TASKS)[0])
print("hash(key):        %.2f×" % load(lambda k: hash(k) % SHARDS)[0])

# 失败重试：5% 任务失败、最多重试 3 次；无幂等 vs 有幂等
FAIL, RETRY = 0.05, 3
dup_without = dup_with = 0
for _ in range(TASKS):
    if random.random() >= FAIL:
        continue
    tries = 0
    while tries < RETRY:
        tries += 1
        if random.random() < 0.5:      # 每次重试 50% 成功
            dup_without += tries - 1   # 前几次失败尝试也算“执行过”
            dup_with += 0              # 幂等键吞掉重复
            break
print(f"\n失败率 {FAIL:.0%}、最多重试 {RETRY} 次：无幂等重复执行 {dup_without} 次；有幂等重复 0 次")
