"""重建成本模型：全量 vs 增量（本机计算，纯算术，无 I/O）。
T_full = S + N×c_full；T_inc(d) = V + d×c_inc；两者相等处即交叉点。
"""

N = 20_000_000          # 事件总数（条）
C_FULL = 200e-6         # 全量：单条 200µs（批量 COPY 进影子表）
C_INC = 350e-6          # 增量：单条 350µs（活表 upsert + 索引维护）
S_FULL = 900.0          # 全量固定成本（秒）：影子表 + 索引 + 原子切换 + 抽样校验
V_INC = 120.0           # 增量固定成本（秒）：定位位点 + 圈定影响区间 + 校验
WORKERS = 4             # 重建集群 worker 数
YUAN_PER_CORE_H = 0.6   # 机器单价量级（元/核·时），用于说明「机器成本可忽略」
WRITE_RATE = 2000       # 写侧事件速率（条/秒），用于把「条数」换算成「历史时长」
PROJ_ROWS = 50_000_000  # 投影行数，用于算影子表峰值存储
ROW_BYTES = 200         # 单行投影大小（字节）


def t_full():
    return S_FULL + N * C_FULL


def t_inc(d):
    return V_INC + d * C_INC


def money(seconds):
    return seconds * WORKERS / 3600 * YUAN_PER_CORE_H


def fmt(n):
    return f"{n:,.0f}"


def row(name, d, t):
    return (f"{name:<12} {fmt(d):>12} {t:>10,.0f} {t/3600:>8.2f} "
            f"{money(t):>8.2f} {d/WRITE_RATE/60:>10.1f}")


T_FULL = t_full()

print("== 参数（本机计算） ==")
print(f"事件总数 N={fmt(N)}  全量单条={C_FULL*1e6:.0f}µs  增量单条={C_INC*1e6:.0f}µs")
print(f"全量固定 S={S_FULL:.0f}s  增量固定 V={V_INC:.0f}s  worker={WORKERS}  "
      f"写入速率={WRITE_RATE}条/s")
print(f"全量重建耗时 = S + N×c_full = {S_FULL:.0f} + {N*C_FULL:.0f} = {T_FULL:,.0f}s "
      f"({T_FULL/3600:.2f}h)")

print("\n== 方案对比：全量 vs 增量（d = 需要重算的事件条数） ==")
print(f"{'方案':<12} {'回放条数':>12} {'耗时(s)':>10} {'耗时(h)':>8} "
      f"{'机器成本(元)':>8} {'重算历史(分钟)':>10}")
for d in (N, 16_000_000, 13_657_143, 10_000_000, 2_000_000, 200_000):
    name = "全量重建" if d == N else "增量重放"
    print(row(name, d, t_inc(d) if d != N else T_FULL))

# 交叉点：V + d*c_inc = S + N*c_full  =>  d* = (S - V + N*c_full) / c_inc
D_STAR = (S_FULL - V_INC + N * C_FULL) / C_INC
P_STAR = N - D_STAR
print(f"\n== 交叉点 ==")
print(f"d* = (S - V + N×c_full) / c_inc = ({S_FULL:.0f} - {V_INC:.0f} + {N*C_FULL:.0f}) / {C_INC*1e6:.0f}µs")
print(f"d* = {D_STAR:,.0f} 条（{D_STAR/N*100:.2f}% 的历史事件）→ 需重算超过它时，全量更省")
print(f"对应错误位点 p* = N - d* = {P_STAR:,.0f} 条，即 bug 上线早于 {P_STAR/WRITE_RATE/60:.1f} 分钟前")
print(f"交叉点存在的条件：S - V < N×(c_inc - c_full)；"
      f"本例 {S_FULL-V_INC:.0f}s < {N*(C_INC-C_FULL):,.0f}s → 成立"
      if S_FULL - V_INC < N * (C_INC - C_FULL) else
      f"交叉点存在的条件不成立 → 增量恒更省：{S_FULL-V_INC:.0f}s vs {N*(C_INC-C_FULL):,.0f}s")

print("\n== 敏感性：增量单条成本是交叉点的主因 ==")
print(f"{'c_inc/c_full':>12} {'增量单条(µs)':>12} {'d*(条)':>14} {'触发全量的影响比例':>18}")
for r in (1.00, 1.25, 1.50, 1.75, 2.00, 3.00):
    c_inc = C_FULL * r
    d = (S_FULL - V_INC + N * C_FULL) / c_inc
    if d >= N:
        print(f"{r:>12.2f} {c_inc*1e6:>12.0f} {'不存在':>14} {'增量恒更省':>18}")
    else:
        print(f"{r:>12.2f} {c_inc*1e6:>12.0f} {d:>14,.0f} {d/N*100:>17.2f}%")

print("\n== 存储与机器成本 ==")
proj_gb = PROJ_ROWS * ROW_BYTES / 1e9
print(f"投影 {PROJ_ROWS/1e6:.0f}M 行 × {ROW_BYTES}B = {proj_gb:.0f}GB；"
      f"全量重建峰值 {proj_gb*2:.0f}GB（旧表+影子表），增量 {proj_gb:.0f}GB")
print(f"机器成本全量 {money(T_FULL):.2f} 元、增量(10M) {money(t_inc(10_000_000)):.2f} 元 "
      f"→ 重建的钱不在机器，在窗口内数据陈旧与人力值守")
