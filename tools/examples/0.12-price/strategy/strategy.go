package strategy

type Rule func(cents int) int

var rules = map[string]Rule{
	"none":   func(c int) int { return c },
	"vip":    func(c int) int { return c * 90 / 100 },
	"coupon": func(c int) int { return c * 80 / 100 },
	"season": func(c int) int { return c * 85 / 100 },
}

func Price(kind string, cents int) int {
	if r, ok := rules[kind]; ok {
		return r(cents)
	}
	return cents
}
