#!/usr/bin/env python3
"""notif_funnel.py —— 本机仿真：业务通知的去重 → 频控/免打扰 → 通道选择 → 降级。

合成数据：5000 用户、24h、16600 条事件；参数是模型假设，不是厂商 SLA。
"""
import random
from collections import defaultdict

random.seed(42)
USERS, EVENTS = 5000, 16600
KINDS = ["transactional", "system", "marketing"]
CHANNELS = ["push", "sms", "email", "inapp"]

# 事件：用户随机、类型按 15%/45%/40%，10% 是重复事件（同一 dedupe key）
events = []
for i in range(EVENTS):
    uid = random.randrange(USERS)
    kind = random.choices(KINDS, weights=[0.15, 0.45, 0.40])[0]
    events.append((uid, kind, f"evt:{i // 10}" if random.random() < 0.10 else f"evt:{i}"))

# 1) 去重
seen = set(); dedup = []
for uid, kind, key in events:
    if key in seen:
        continue
    seen.add(key); dedup.append((uid, kind))
dup_hit = len(events) - len(dedup)

# 2) 频控（营销 2 条/日）+ 免打扰（23:00-07:00：营销延后、系统静默、事务放行）
cap = defaultdict(int); out = []
quiet_marketing = quiet_system = 0
for uid, kind in dedup:
    if kind == "marketing":
        if random.random() < 0.25:      # 25% 事件落在夜间窗口
            quiet_marketing += 1; continue
        if cap[uid] >= 2: continue
        cap[uid] += 1
    elif kind == "system" and random.random() < 0.10:
        quiet_system += 1; continue
    out.append((uid, kind))

# 3) 通道选择 + 降级：营销只走 inapp/email；事务 push 永久失效(5%)转短信
perm_fail = 0; sms = 0; delivered = 0
for uid, kind in out:
    if kind == "marketing":
        delivered += 1                      # 站内信必达
        continue
    if random.random() < 0.05:
        perm_fail += 1
        if kind == "transactional":
            sms += 1; delivered += 1
        continue
    delivered += 1                          # push/inapp 模型按送达计
print(f"事件 {EVENTS} → 去重命中 {dup_hit} → 频控/免打扰后 {len(out)}")
print(f"夜间：营销延后 {quiet_marketing}，系统静默 {quiet_system}")
print(f"push 永久失效 {perm_fail}，事务转短信 {sms} 条，模型送达 {delivered}（{delivered/len(out)*100:.2f}%）")
print(f"短信成本（0.04 元/条）≈ {sms*0.04:.2f} 元")
