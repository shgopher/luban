package plain

func Price(kind string, cents int) int {
	if kind == "none" {
		return cents
	}
	if kind == "vip" {
		return cents * 90 / 100
	}
	if kind == "coupon" {
		return cents * 80 / 100
	}
	if kind == "season" {
		return cents * 85 / 100
	}
	return cents
}
