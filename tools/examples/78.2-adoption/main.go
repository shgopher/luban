// 平台采用率：用平台的服务 / 全部服务；低于阈值说明平台没成为默认路径。
package main

import "fmt"

type svc struct {
	name    string
	onboard bool
	exempt  bool // 有明确豁免理由（如遗留系统）
}

func main() {
	services := []svc{
		{"order", true, false}, {"payment", true, false}, {"risk", true, false},
		{"legacy-billing", false, true}, {"search", false, false}, {"report", false, false},
	}
	total, on, exempt := 0, 0, 0
	for _, s := range services {
		total++
		if s.onboard {
			on++
		}
		if s.exempt {
			exempt++
		}
	}
	rate := 100 * float64(on) / float64(total-exempt)
	fmt.Printf("服务=%d 已接入=%d 豁免=%d 采用率=%.0f%%\n", total, on, exempt, rate)
	if rate < 70 {
		fmt.Println("结论：平台不是默认路径——先修 DX（文档/自助/错误信息），再谈强制")
	} else {
		fmt.Println("结论：平台已是默认路径，可以设准入门槛")
	}
}
