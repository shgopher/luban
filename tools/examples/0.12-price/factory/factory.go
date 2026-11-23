package factory

type Pricer interface{ Price(cents int) int }

type Factory interface{ Create() Pricer }

type ratePricer struct{ rate int }

func (p *ratePricer) Price(c int) int { return c * p.rate / 100 }

type rateFactory struct{ rate int }

func (f *rateFactory) Create() Pricer { return &ratePricer{rate: f.rate} }

var factories = map[string]Factory{
	"none":   &rateFactory{rate: 100},
	"vip":    &rateFactory{rate: 90},
	"coupon": &rateFactory{rate: 80},
	"season": &rateFactory{rate: 85},
}

func Price(kind string, cents int) int {
	f, ok := factories[kind]
	if !ok {
		return cents
	}
	return f.Create().Price(cents)
}
