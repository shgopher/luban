NODES = {"提交": 2, "主管审批": 10, "财务复核": 15,
         "发票查验": 8, "预算校验": 6,
         "法务会签": 25, "退回修改": 30, "打款": 20}


def par(*d):
    return max(d)


def seq(*d):
    return sum(d)


def build(n):
    checks = par(n["发票查验"], n["预算校验"])
    return {
        "正常": dict(p=0.72, t=seq(n["提交"], n["主管审批"], n["财务复核"], checks, n["打款"])),
        "驳回": dict(p=0.18, t=seq(n["提交"], n["主管审批"], n["退回修改"], n["提交"],
                            n["主管审批"], n["财务复核"], checks, n["打款"])),
        "加签": dict(p=0.10, t=seq(n["提交"], n["主管审批"],
                            par(n["法务会签"], n["财务复核"]), checks, n["打款"])),
    }


def expected(n=None):
    return sum(v["p"] * v["t"] for v in build(NODES if n is None else n).values())


base = expected()
print("路径  概率  时长(分)  期望贡献(分)")
for name, v in build(NODES).items():
    print(f"{name}  p={v['p']:.2f}  时长={v['t']:>3}  贡献={v['p'] * v['t']:.2f}")
print(f"期望总时长 E[T] = {base:.2f} 分钟")

print("\n单点优化（其余不变）：")
print("优化项  改后E[T]  期望收益  期望时长降幅")
for label, node, value in [("财务复核 15→5", "财务复核", 5),
                           ("打款 20→10", "打款", 10),
                           ("退回修改 30→10", "退回修改", 10),
                           ("法务会签 25→5", "法务会签", 5),
                           ("发票查验 8→2", "发票查验", 2)]:
    changed = dict(NODES)
    changed[node] = value
    e = expected(changed)
    print(f"{label}  {e:.2f}  {base - e:+.2f}  {(base - e) / base:+.1%}")

print("\n把驳回率从 18% 降到 8%（前置校验拦掉一半错误单据）：")
d = build(NODES)
e = 0.82 * d["正常"]["t"] + 0.08 * d["驳回"]["t"] + 0.10 * d["加签"]["t"]
print(f"E[T] = 0.82×{d['正常']['t']} + 0.08×{d['驳回']['t']} + 0.10×{d['加签']['t']} = {e:.2f} 分钟"
      f"（省 {base - e:.2f} 分钟）")
