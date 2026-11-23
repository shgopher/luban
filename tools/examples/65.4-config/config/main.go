package main

// 租户配置治理算例：四级继承 + schema 校验（fail-closed）+ config_version 迁移链
// 输入是假设的场景（算例），不是任何真实客户数据。

import (
	"fmt"
	"sort"
)

type kv map[string]any

// 平台默认 → 套餐 → 租户 → 用户，后者覆盖前者
type layer struct {
	name string
	kv   kv
}

var layers = []layer{
	{"平台默认", kv{"region": "cn", "timeout_s": float64(30), "sso": false, "retention_days": float64(90)}},
	{"套餐(pro)", kv{"timeout_s": float64(60), "sso": true, "sso_meta": "https://idp.example.com"}},
	{"租户(T1042)", kv{"region": "eu"}},
	{"用户(u7)", kv{"timeout_s": float64(120)}},
}

// 生效值 + 来源层
func resolve() (map[string]any, map[string]string) {
	val := map[string]any{}
	from := map[string]string{}
	for _, l := range layers {
		for k, v := range l.kv {
			val[k] = v
			from[k] = l.name
		}
	}
	return val, from
}

// 校验：枚举、范围、跨字段依赖；失败返回错误列表（fail-closed，不回落默认）
func validate(c kv) []string {
	var errs []string
	region, ok := c["region"].(string)
	if !ok {
		errs = append(errs, "region 缺失或不是字符串")
	} else {
		switch region {
		case "cn", "eu", "us":
		default:
			errs = append(errs, fmt.Sprintf("region=%q 不在枚举 {cn,eu,us}", region))
		}
	}
	if t, ok := c["timeout_s"].(float64); ok {
		if t < 1 || t > 300 {
			errs = append(errs, fmt.Sprintf("timeout_s=%.0f 越界 [1,300]", t))
		}
	} else {
		errs = append(errs, "timeout_s 缺失或不是数字")
	}
	if sso, _ := c["sso"].(bool); sso {
		if m, _ := c["sso_meta"].(string); m == "" {
			errs = append(errs, "sso=true 但缺 sso_meta（跨字段依赖）")
		}
	}
	if r, ok := c["retention_days"].(float64); ok {
		if r < 30 || r > 365 {
			errs = append(errs, fmt.Sprintf("retention_days=%.0f 越界 [30,365]", r))
		}
	}
	return errs
}

// 迁移函数链：v1→v2→v3，每一步只做一件事，幂等（重跑不会重复加字段）
type migrator struct {
	from, to int
	apply    func(kv) (kv, string) // 返回新配置 + 本步说明
}

func clone(c kv) kv {
	n := kv{}
	for k, v := range c {
		n[k] = v
	}
	return n
}

var chain = []migrator{
	{1, 2, func(c kv) (kv, string) {
		n := clone(c)
		rename := map[string]string{"cn-north": "cn", "cn-east": "cn", "eu-west": "eu"}
		old, _ := n["region"].(string)
		if r, ok := rename[old]; ok {
			n["region"] = r
		} else if old != "cn" && old != "eu" && old != "us" {
			return nil, fmt.Sprintf("region=%q 没有映射规则，停链", old)
		}
		// 字符串数字转数字（v1 的 timeout_s 是字符串）
		if s, ok := n["timeout_s"].(string); ok {
			var f float64
			fmt.Sscanf(s, "%f", &f)
			n["timeout_s"] = f
		}
		if _, ok := n["sso"]; !ok {
			n["sso"] = false
		}
		return n, "region 重命名映射 + timeout_s 字符串转数字 + 补 sso=false"
	}},
	{2, 3, func(c kv) (kv, string) {
		n := clone(c)
		if _, ok := n["retention_days"]; !ok {
			n["retention_days"] = float64(90)
		}
		return n, "新增 retention_days=90（默认值，不覆盖已有值）"
	}},
}

func migrate(raw kv, fromVersion int) (kv, []string, error) {
	steps := []string{}
	cur := clone(raw)
	v := fromVersion
	for _, m := range chain {
		if m.from != v {
			continue
		}
		next, desc := m.apply(cur)
		if next == nil {
			return nil, steps, fmt.Errorf("v%d→v%d 无法自动迁移：%s", m.from, m.to, desc)
		}
		cur = next
		steps = append(steps, fmt.Sprintf("v%d→v%d: %s", m.from, m.to, desc))
		v = m.to
	}
	return cur, steps, nil
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func main() {
	fmt.Println("① 四级继承：平台默认 → 套餐 → 租户 → 用户")
	val, from := resolve()
	fmt.Println("配置项           生效值                     来源层")
	for _, k := range keys(val) {
		fmt.Printf("%-16s %-24v %s\n", k, val[k], from[k])
	}
	fmt.Println()

	fmt.Println("② 校验（fail-closed：失败就拒绝落库，不回落平台默认）")
	if errs := validate(val); len(errs) == 0 {
		fmt.Println("   T1042 的生效配置：校验通过")
	}
	bad := kv{"region": "eu-west", "timeout_s": float64(600), "sso": true, "retention_days": float64(90)}
	for _, e := range validate(bad) {
		fmt.Println("   拒绝:", e)
	}
	fmt.Println()

	fmt.Println("③ config_version 迁移链（v1 → v3）")
	tenant := kv{"region": "cn-north", "timeout_s": "45"}
	got, steps, err := migrate(tenant, 1)
	if err != nil {
		fmt.Println("   迁移失败:", err)
	} else {
		fmt.Printf("   T1042 原始 v1: region=%v timeout_s=%v(%T)\n", tenant["region"], tenant["timeout_s"], tenant["timeout_s"])
		for _, s := range steps {
			fmt.Println("  ", s)
		}
		fmt.Printf("   迁移结果 v3: region=%v timeout_s=%v(%T) sso=%v retention_days=%v\n",
			got["region"], got["timeout_s"], got["timeout_s"], got["sso"], got["retention_days"])
		fmt.Println("   校验:", func() string {
			if e := validate(got); len(e) == 0 {
				return "通过"
			} else {
				return fmt.Sprint(e)
			}
		}())
	}
	fmt.Println()

	tenant2 := kv{"region": "xx", "timeout_s": "30"}
	if _, steps, err := migrate(tenant2, 1); err != nil {
		fmt.Println("   T2211 迁移停链:", err)
		fmt.Println("   已执行步骤数:", len(steps), "→ 原配置不动，进人工迁移队列")
	}
	fmt.Println()

	fmt.Println("④ 旧平台遇到新配置：拒绝加载，不猜默认值（fail-closed）")
	platformCfgCap := 2 // 假设某平台版本只认识 config_version ≤ 2
	tenantCfgVersion := 3
	if tenantCfgVersion > platformCfgCap {
		fmt.Printf("   平台认识 config_version ≤ %d，租户是 v%d → 返回错误、租户留在旧平台\n",
			platformCfgCap, tenantCfgVersion)
		fmt.Println("   这解释了版本矩阵里为什么每个平台版本都有一个 C 上限")
	}
}
