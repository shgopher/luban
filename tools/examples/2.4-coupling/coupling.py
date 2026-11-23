"""优惠券归属切换（营销域 → 交易域）前后的耦合量化。
频次为业务量估算假设，不是线上采集的数据；结论随假设变，脚本可改参数重算。
"""

# (事件, 每日次数, 单次事件跨域调用跳数, 其中 “必须跨域同事务” 的写对)
BEFORE = [
    ("下单用券", 120_000, 2, 1),   # 交易→营销：查券 1 跳 + 核销 1 跳
    ("发券", 30_000, 0, 0),        # 营销本地：发放 + 预算扣减同一事务
    ("活动规则变更", 40, 0, 0),     # 营销本地发布
    ("日终对账", 1, 2, 0),          # 财务分别取交易流水、营销核销明细
    ("活动 ROI 归因", 4, 0, 0),     # 营销本地算
]
AFTER = [
    ("下单用券", 120_000, 0, 0),   # 交易本地：下单 + 核销同一事务
    ("发券", 30_000, 1, 1),        # 营销→交易：建券/发券；预算扣减仍在营销
    ("活动规则变更", 40, 1, 0),     # 营销→交易：券字段/口径要交易域配合
    ("日终对账", 1, 1, 0),          # 财务→交易：流水与核销同源
    ("活动 ROI 归因", 4, 1, 0),     # 营销→交易：按活动维度取核销归因
]

# 依赖边：(上游, 下游, 用途)
EDGES_BEFORE = [("交易", "营销", "查券"), ("交易", "营销", "核销"),
                ("财务", "交易", "流水"), ("财务", "营销", "核销明细")]
EDGES_AFTER = [("营销", "交易", "建券/发券"), ("营销", "交易", "规则同步"),
               ("营销", "交易", "ROI 取数"), ("财务", "交易", "流水与核销")]

RULES_PER_DAY, CODE_RATIO, CAPACITY_PER_WEEK = 40, 0.05, 5


def total(events):
    calls = sum(n * hops for _, n, hops, _ in events)
    tx = sum(n * pairs for _, n, _, pairs in events)
    return calls, tx


def fan_in(edges):
    counter = {}
    for _, dst, _ in edges:
        counter[dst] = counter.get(dst, 0) + 1
    return counter


cb, tb = total(BEFORE)
ca, ta = total(AFTER)
print(f"每日跨域调用次数     切前 {cb:>7,}  切后 {ca:>6,}  变化 {(ca - cb) / cb:+.1%}")
print(f"每日跨域强一致写入对 切前 {tb:>7,}  切后 {ta:>6,}  变化 {(ta - tb) / tb:+.1%}")

print("\n边界被依赖数（fan-in）：")
fb, fa = fan_in(EDGES_BEFORE), fan_in(EDGES_AFTER)
for dom in ("营销", "交易", "财务"):
    print(f"  {dom}域  切前 {fb.get(dom, 0)}  切后 {fa.get(dom, 0)}")

need = RULES_PER_DAY * 5 * CODE_RATIO
print(f"\n需交易域发版的变更  切前 0 次/周  切后 {need:.0f} 次/周"
      f"（发布能力 {CAPACITY_PER_WEEK} 次/周，缺口 {need - CAPACITY_PER_WEEK:+.0f} 次/周）")

print("\n口径 owner（核销明细由谁定义与出数）：")
print("  切前：营销域（营销自证活动效果；交易域只管支付流水）")
print("  切后：交易域（营销的 ROI 报表要等交易域口径与排期）")
