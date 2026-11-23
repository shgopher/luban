// 名单匹配：黑名单拒绝、白名单放行、灰名单降额/复核；优先级高于规则。
package main

import "fmt"

func main() {
	black := map[string]bool{"u1001": true, "dev-9f2a": true}
	white := map[string]bool{"u2001": true}
	grey := map[string]bool{"ip-10.0.0.7": true}
	req := struct{ user, device, ip string }{"u1001", "dev-1234", "ip-10.0.0.7"}
	switch {
	case black[req.user] || black[req.device]:
		fmt.Println("命中黑名单 → 直接拒绝")
	case white[req.user]:
		fmt.Println("命中白名单 → 放行（跳过后续规则）")
	case grey[req.ip]:
		fmt.Println("命中灰名单 → 降额或人工复核")
	default:
		fmt.Println("未命中名单 → 进入规则引擎")
	}
}
