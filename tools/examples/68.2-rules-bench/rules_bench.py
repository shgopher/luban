# 本机计算：10 条风控规则，决策表驱动 vs if/else 硬编码，各跑 100 万次
import random, statistics, time

# >>> RULES（决策表：规则是数据，顺序即优先级）
RULES = [
    ("R1  黑名单",     (("is_black", "==", True),),                         "reject"),
    ("R2  白名单",     (("is_white", "==", True),),                         "pass"),
    ("R3  单笔超限",   (("amount", ">", 50000),),                           "reject"),
    ("R4  新设备大额", (("new_device", "==", True), ("amount", ">", 10000)), "verify"),
    ("R5  频次超限",   (("cnt_1h", ">", 20),),                              "reject"),
    ("R6  小时累计",   (("amt_1h", ">", 100000),),                          "verify"),
    ("R7  境外 IP",    (("foreign_ip", "==", True),),                       "verify"),
    ("R8  夜间大额",   (("night", "==", True), ("amount", ">", 5000)),       "verify"),
    ("R9  新户大额",   (("age_days", "<", 1), ("amount", ">", 500)),         "verify"),
    ("R10 模型高分",   (("score", ">", 0.8),),                              "verify"),
]
# <<< RULES_END

# >>> TABLE_IMPL（解释器：一次写对，之后只改数据）
def decide_table(r):
    """通用解释器：顺序匹配 + 命中短路；规则不写死在代码里。"""
    for _id, conds, action in RULES:
        for f, op, v in conds:
            x = r[f]
            if not (x > v if op == ">" else x < v if op == "<" else x == v):
                break
        else:
            return action
    return "pass"
# <<< TABLE_IMPL_END

# >>> CODE_IMPL（硬编码：同样 10 条、同样优先级；改阈值 = 改函数体 + 发版）
def decide_code(r):
    if r["is_black"]:
        return "reject"
    if r["is_white"]:
        return "pass"
    if r["amount"] > 50000:
        return "reject"
    if r["new_device"] and r["amount"] > 10000:
        return "verify"
    if r["cnt_1h"] > 20:
        return "reject"
    if r["amt_1h"] > 100000:
        return "verify"
    if r["foreign_ip"]:
        return "verify"
    if r["night"] and r["amount"] > 5000:
        return "verify"
    if r["age_days"] < 1 and r["amount"] > 500:
        return "verify"
    if r["score"] > 0.8:
        return "verify"
    return "pass"
# <<< CODE_IMPL_END

POOL, ITER, ROUNDS, QPS = 1000, 1_000_000, 5, 20_000

def make_pool(n, seed=42):                      # 合成请求：让 10 条规则都有命中
    rnd, pool = random.Random(seed), []
    for _ in range(n):
        pool.append({"is_black": rnd.random() < 0.01, "is_white": rnd.random() < 0.02,
            "amount": rnd.choice([50, 300, 800, 3000, 9000, 30000, 80000]),
            "new_device": rnd.random() < 0.15, "cnt_1h": rnd.choice([0, 3, 8, 15, 25, 40]),
            "amt_1h": rnd.choice([0, 5000, 60000, 150000]), "foreign_ip": rnd.random() < 0.05,
            "night": rnd.random() < 0.30, "age_days": rnd.choice([0, 1, 30, 365]),
            "score": rnd.choice([0.1, 0.4, 0.7, 0.85, 0.95])})
    return pool

def bench(fn, pool):
    for i in range(20_000):
        fn(pool[i % POOL])                      # 预热，避免首轮解释器特化影响
    ts = []
    for _ in range(ROUNDS):
        t0 = time.perf_counter()
        for i in range(ITER):
            fn(pool[i % POOL])
        ts.append(time.perf_counter() - t0)
    return statistics.median(ts)

def loc(begin, end):                            # 数标记区间的有效行（非空非注释）
    inside, n = False, 0
    for ln in open(__file__, encoding="utf-8"):
        if begin in ln: inside = True; continue
        if end in ln: break
        s = ln.strip()
        if inside and s and not s.startswith("#"): n += 1
    return n

pool = make_pool(POOL)
t_code, t_table = bench(decide_code, pool), bench(decide_table, pool)
diff = sum(1 for r in pool if decide_code(r) != decide_table(r))
dist = {}
for r in pool:
    dist[decide_code(r)] = dist.get(decide_code(r), 0) + 1

us_code, us_table = t_code / ITER * 1e6, t_table / ITER * 1e6
extra = us_table - us_code
fixed_days, save_days = 10.0, 0.75 - 0.15

print(f"请求池 {POOL} 条 × {ITER:,} 次 × {ROUNDS} 轮取中位（本机计算，纯解释执行、无网络/DB）")
print(f"硬编码 {t_code:.3f}s  {us_code:.2f}µs/次")
print(f"决策表 {t_table:.3f}s  {us_table:.2f}µs/次  = {t_table/t_code:.2f}×（每次多 {extra:.2f}µs）")
print(f"决策一致性：{POOL-diff}/{POOL} 相同，差异 {diff} 条；分布 "
      + "，".join(f"{k}={v}" for k, v in sorted(dist.items())))
print(f"QPS={QPS:,} 时额外 CPU ≈ {extra*1e-6*QPS:.2f} 核")
n_rules = loc('>>> RULES', '<<< RULES_END')
n_eng = loc('>>> TABLE_IMPL', '<<< TABLE_IMPL_END')
n_code = loc('>>> CODE_IMPL', '<<< CODE_IMPL_END')
print(f"有效代码行：硬编码 {n_code} 行；决策表 = 规则数据 {n_rules} 行 + 解释器 {n_eng} 行 "
      f"= {n_rules + n_eng} 行（{((n_rules+n_eng)/n_code):.2f}×）")
print(f"回本变更次数 = 引擎固定 {fixed_days:.0f} 人日 ÷ 每次省 {save_days:.2f} 人日 "
      f"= {fixed_days/save_days:.1f} 次")
for pm in (0.5, 1, 2, 4, 12):
    print(f"  月变更 {pm:>4} 次 → 年 {pm*12:>3.0f} 次 → 回本 {fixed_days/save_days/pm:>5.1f} 个月")
