// 命令 fx：汇率报价方向、三角交叉换算、往返损失、as_of 漂移的本机实测。
// 输入是 raw/ 下真实抓取的 ECB 每日 XML 与 open.er-api.com 的 USD 基准 JSON，
// 代码里不做任何「假设汇率」——所有数字都能从 raw 文件复算。
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// ecbDay 一天的 ECB 参考价：全部以 EUR 为 base（1 EUR = rate 个 quote 币种）。
type ecbDay struct {
	asOf  string
	rates map[string]float64 // quote -> rate
}

// parseECB 解析 eurofxref XML。这里手写扫描而不用 encoding/xml：
// 报文结构固定（<Cube time=...><Cube currency=... rate=.../></Cube>），
// 30 行字符串处理足够；换用 XML 库只是多一层依赖，结果一样。
func parseECB(path string) []ecbDay {
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	body := string(raw)
	var days []ecbDay
	// 先按 "<Cube time=" 切开：每段的第一行是日期，后面跟着当天的 <Cube currency=.../>
	segs := strings.Split(body, "<Cube time=")
	for _, seg := range segs[1:] {
		q := 0
		if len(seg) > 0 && (seg[0] == '"' || seg[0] == '\'') {
			q = 1
		}
		quote := byte('"')
		if q == 1 {
			quote = seg[0]
		}
		end := strings.IndexByte(seg[q:], quote)
		if end < 0 {
			continue
		}
		d := ecbDay{asOf: seg[q : q+end], rates: map[string]float64{}}
		rest := seg[q+end+1:]
		if next := strings.Index(rest, "<Cube time="); next >= 0 {
			rest = rest[:next]
		}
		for _, c := range strings.Split(rest, "<Cube currency=")[1:] {
			cq := c[0]
			ce := strings.IndexByte(c[1:], cq)
			if ce < 0 {
				continue
			}
			ccy := c[1 : 1+ce]
			ri := strings.Index(c, "rate=")
			if ri < 0 {
				continue
			}
			rq := c[ri+5]
			re := strings.IndexByte(c[ri+6:], rq)
			if re < 0 {
				continue
			}
			r, err := strconv.ParseFloat(c[ri+6:ri+6+re], 64)
			if err == nil {
				d.rates[ccy] = r
			}
		}
		if len(d.rates) > 0 {
			days = append(days, d)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].asOf > days[j].asOf })
	return days
}

type erAPI struct {
	Result     string             `json:"result"`
	UpdatedUTC string             `json:"time_last_update_utc"`
	NextUpdate string             `json:"time_next_update_utc"`
	Rates      map[string]float64 `json:"rates"`
}

// cross 返回 quote per base 的汇率。ECB 的报价是 EUR base，
// 所以任意两币种都要先「除以同一 base」：A/B = (EUR->B) / (EUR->A)。
func (d ecbDay) cross(base, quote string) float64 {
	if base == "EUR" {
		return d.rates[quote]
	}
	if quote == "EUR" {
		return 1 / d.rates[base]
	}
	return d.rates[quote] / d.rates[base]
}

func bps(a, b float64) float64 { return (a - b) / b * 10000 }

func main() {
	days := parseECB(os.Args[1])
	raw, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	var er erAPI
	if err := json.Unmarshal(raw, &er); err != nil {
		panic(err)
	}

	fmt.Println("## B0 数据来源与 as_of")
	fmt.Println()
	byDate := map[string]ecbDay{}
	for _, d := range days {
		byDate[d.asOf] = d
	}
	fmt.Printf("ECB 90 天文件共 %d 个交易日；最新 as_of = %s，次新 = %s\n", len(days), days[0].asOf, days[1].asOf)
	fmt.Printf("er-api USD 基准：time_last_update_utc = %s\n", er.UpdatedUTC)
	weekend := 0
	for _, d := range days {
		// 2026-11-21 是本机抓取日（周六）；检查文件里有没有周六/周日
		if len(d.asOf) == 10 {
			wd := d.asOf
			if wd == "2026-11-14" || wd == "2026-11-15" || wd == "2026-11-21" || wd == "2026-11-22" {
				weekend++
			}
		}
	}
	fmt.Printf("90 天文件里 2026-11-14/15（周六日）与 2026-11-21/22 的出现次数 = %d\n", weekend)
	fmt.Println()

	fri13 := byDate["2026-11-13"] // 周五
	mon16 := byDate["2026-11-16"] // 下周一
	fri20 := byDate["2026-11-20"] // 最新（周五）

	fmt.Println("## B1 报价方向：ECB 全部是 EUR base")
	fmt.Println()
	fmt.Printf("ECB %s: EUR/USD=%.5f, EUR/CNY=%.4f, EUR/JPY=%.2f（读法：1 EUR 换 %.5f USD）\n",
		fri20.asOf, fri20.rates["USD"], fri20.rates["CNY"], fri20.rates["JPY"], fri20.rates["USD"])
	fmt.Printf("同一组数换 USD base：USD/CNY=%.6f, USD/JPY=%.6f（推算：EUR/CNY ÷ EUR/USD）\n",
		fri20.cross("USD", "CNY"), fri20.cross("USD", "JPY"))
	fmt.Printf("er-api 直接报 USD base：USD/CNY=%.6f, USD/JPY=%.6f\n", er.Rates["CNY"], er.Rates["JPY"])
	fmt.Println()

	fmt.Println("## B2 三角交叉换算 vs 直接报价（同一 as_of = 2026-11-20）")
	fmt.Println()
	// (a) 同一来源内部三角：USD->CNY->JPY 对比 USD->JPY
	triECB := fri20.cross("USD", "CNY") * fri20.cross("CNY", "JPY")
	directECB := fri20.cross("USD", "JPY")
	// (b) 跨来源三角：USD->CNY 取 er-api，CNY->JPY 取 ECB
	triMixed := er.Rates["CNY"] * fri20.cross("CNY", "JPY")
	directMixed := er.Rates["JPY"]
	// (c) 跨来源直接对比：ECB 推导的 USD/JPY vs er-api 的 USD/JPY
	fmt.Printf("(a) ECB 内部三角：USD→CNY(%.6f)→JPY = %.6f；ECB 直接 USD/JPY = %.6f；差 %.4f bps\n",
		fri20.cross("USD", "CNY"), triECB, directECB, bps(triECB, directECB))
	fmt.Printf("(b) 跨源三角：USD→CNY(er-api %.6f)→JPY(ECB %.6f) = %.6f；er-api 直接 USD/JPY = %.6f；差 %+.2f bps\n",
		er.Rates["CNY"], fri20.cross("CNY", "JPY"), triMixed, directMixed, bps(triMixed, directMixed))
	fmt.Printf("(c) 两个来源同日 USD/JPY：ECB 推导 %.6f vs er-api %.6f；差 %+.2f bps\n",
		directECB, directMixed, bps(directMixed, directECB))
	fmt.Println()

	fmt.Println("## B3 换算往返损失")
	fmt.Println()
	// 单一来源 mid：USD -> CNY -> USD（两条腿用同一个价，mid 无点差）
	y := fri20.cross("USD", "CNY") // 6.6777 CNY per USD
	cny := 1 * y
	usdBack := cny / y
	fmt.Printf("单一来源 mid：1 USD → %.6f CNY → %.10f USD，损失 %+.4f bps\n", cny, usdBack, bps(usdBack, 1))
	// 跨来源：买 CNY 用 er-api 的价，卖回 USD 用 ECB 的价
	yERP := er.Rates["CNY"]           // er-api: 6.69198
	yECB := fri20.cross("USD", "CNY") // ECB: 6.677707
	cny2 := 1 * yERP
	usdBack2 := cny2 / yECB
	fmt.Printf("跨来源：1 USD → %.6f CNY(er-api) → %.10f USD(ECB, 1 USD = %.6f CNY)，损益 %+.2f bps\n",
		cny2, usdBack2, yECB, bps(usdBack2, 1))
	cnyBack2 := (1 / yECB) * yERP
	fmt.Printf("跨来源反向：1 CNY → %.10f USD(ECB) → %.10f CNY(er-api)，损益 %+.2f bps\n",
		1/yECB, cnyBack2, bps(cnyBack2, 1))
	fmt.Println()

	fmt.Println("## B4 同一订单在周五 / 周一两个 as_of 下换算")
	fmt.Println()
	orderJPY := 1000000.0 // 订单金额：100 万日元，以 USD 结算
	rFri := fri13.cross("USD", "JPY")
	rMon := mon16.cross("USD", "JPY")
	usdFri := orderJPY / rFri
	usdMon := orderJPY / rMon
	fmt.Printf("2026-11-13（周五）USD/JPY=%.6f → %.2f JPY 折 %.4f USD\n", rFri, orderJPY, usdFri)
	fmt.Printf("2026-11-16（周一）USD/JPY=%.6f → %.2f JPY 折 %.4f USD\n", rMon, orderJPY, usdMon)
	fmt.Printf("汇率变动 %+.4f bps；同一订单收入差 %+.4f USD（%+.4f%%）\n",
		bps(rMon, rFri), usdMon-usdFri, (usdMon-usdFri)/usdFri*100)
	fmt.Println()

	fmt.Println("## B5 推算：1000 笔订单跨周末的收入差（用 B4 实测汇率推算，非实测收入）")
	n := 1000.0
	fmt.Printf("1000 笔 × 100 万 JPY = %.0f JPY；按周五汇率收入 %.2f USD，按周一 %.2f USD，差 %.2f USD\n",
		n*orderJPY, n*usdFri, n*usdMon, n*(usdMon-usdFri))

	fmt.Println()
	fmt.Println("## B6 一周内 as_of 漂移（ECB 90 天文件里 11-13 ~ 11-20 的 USD/JPY）")
	fmt.Println()
	fmt.Println("| as_of | USD/JPY（ECB 推导） | 相对 11-13 变动 bps |")
	fmt.Println("|---|---|---|")
	base := fri13.cross("USD", "JPY")
	for i := len(days) - 1; i >= 0; i-- {
		d := days[i]
		if d.asOf < "2026-11-13" || d.asOf > "2026-11-20" {
			continue
		}
		r := d.cross("USD", "JPY")
		fmt.Printf("| %s | %.6f | %+.2f |\n", d.asOf, r, bps(r, base))
	}
	var mn, mx float64
	first := true
	for _, d := range days {
		if d.asOf < "2026-11-13" || d.asOf > "2026-11-20" {
			continue
		}
		r := d.cross("USD", "JPY")
		if first {
			mn, mx, first = r, r, false
		}
		mn = math.Min(mn, r)
		mx = math.Max(mx, r)
	}
	fmt.Printf("\n11-13~11-20 区间：USD/JPY 最低 %.6f、最高 %.6f，极差 %.2f bps（推算：极差 ÷ 最低值）\n",
		mn, mx, (mx-mn)/mn*10000)
}
