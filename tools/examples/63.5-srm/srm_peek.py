#!/usr/bin/env python3
"""ab_platform.py —— 只用标准库：SRM 卡方 + 偷看对假阳性的影响（A/A，真实效应 0）。"""
import math, random
from statistics import NormalDist

def chi2_sf_df1(x):
    return math.erfc(math.sqrt(x / 2.0))  # df=1 卡方尾概率

def srm(a, b):
    n = a + b; e = n / 2
    chi = (a - e) ** 2 / e + (b - e) ** 2 / e
    return chi, chi2_sf_df1(chi)

print("SRM 检查（df=1，p<0.001 视为分流异常）")
for a, b in [(500000, 500000), (505000, 495000), (510000, 490000), (520000, 480000)]:
    chi, p = srm(a, b)
    print(f"{a}:{b}  chi2={chi:8.2f}  p={p:.3e}  {'告警' if p < 0.001 else 'OK'}")

def two_prop_p(x1, n1, x2, n2):
    p1, p2 = x1 / n1, x2 / n2
    p = (x1 + x2) / (n1 + n2)
    se = math.sqrt(p * (1 - p) * (1 / n1 + 1 / n2))
    if se == 0: return 1.0
    z = (p1 - p2) / se
    return 2 * (1 - NormalDist().cdf(abs(z)))

def draw(n):
    x1 = sum(1 for _ in range(n) if random.random() < 0.10)
    x2 = sum(1 for _ in range(n) if random.random() < 0.10)
    return x1, n, x2, n

random.seed(7)
trials, N = 2000, 20000
fixed_sig = 0
for _ in range(trials):
    x1, n1, x2, n2 = draw(N)
    if two_prop_p(x1, n1, x2, n2) < 0.05: fixed_sig += 1

random.seed(7)
peek_sig = 0
for _ in range(trials):
    x1 = x2 = n1 = n2 = 0
    stopped = False
    for _ in range(10):
        a, b = draw(N // 10)[0], draw(N // 10)[2]
        x1 += a; x2 += b; n1 += N // 10; n2 += N // 10
        if two_prop_p(x1, n1, x2, n2) < 0.05:
            peek_sig += 1; stopped = True; break
    if not stopped and two_prop_p(x1, n1, x2, n2) < 0.05:
        peek_sig += 1
print(f"\nA/A 实验（真实效应 0）：固定样本只看一次 假阳性 {fixed_sig/trials*100:.1f}%；")
print(f"每 10% 偷看一次、显著就停 假阳性 {peek_sig/trials*100:.1f}%（{peek_sig/trials*100/(fixed_sig/trials*100):.1f} 倍）")
