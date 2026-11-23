// 13.3 端口矩阵检查器：把“服务 → 端口 → 可达范围 → 认证/加密”的登记表，跑成一张分级暴露面清单。
// 输入：每行一条暴露记录，逗号分隔：服务,端口,协议,可达范围(internet|bastion|private),认证(true|false),加密(true|false),类型(public|business|management|data)
// 输出：逐条等级、按等级汇总、互联网可达数量、P0/P1 高危项与处置动作。
package main

import (
	"fmt"
	"strconv"
	"strings"
)

type item struct {
	service string
	port    int
	proto   string
	reach   string // internet / bastion / private
	auth    bool   // 是否强制认证
	tls     bool   // 是否传输加密
	kind    string // public / business / management / data
}

type finding struct {
	it     item
	level  int // 0=P0 ... 3=P3
	reason string
	action string
}

// inventory 模拟从 CMDB / 云控制台导出的暴露面登记表（14 条）
const inventory = `edge-lb,443,tcp,internet,true,true,public
admin-console,8443,tcp,internet,true,true,management
ssh-prod,22,tcp,internet,true,false,management
mysql-primary,3306,tcp,private,true,true,data
redis-cache,6379,tcp,private,false,false,data
kube-apiserver,6443,tcp,internet,true,true,management
node-exporter,9100,tcp,private,false,false,management
prometheus,9090,tcp,private,true,false,management
internal-api,8080,tcp,private,true,false,business
job-worker,9000,tcp,private,true,false,business
backup-agent,22,tcp,bastion,true,true,management
dns-recursive,53,udp,internet,false,false,public
legacy-ftp,21,tcp,internet,false,false,business
smtp-relay,25,tcp,internet,false,false,public`

func parse(s string) []item {
	var out []item
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		f := strings.Split(strings.TrimSpace(line), ",")
		if len(f) != 7 {
			panic("列数不对：" + line)
		}
		port, err := strconv.Atoi(f[1])
		if err != nil {
			panic("端口不是数字：" + line)
		}
		out = append(out, item{
			service: f[0], port: port, proto: f[2], reach: f[3],
			auth: f[4] == "true", tls: f[5] == "true", kind: f[6],
		})
	}
	return out
}

// grade：按“可达范围 → 认证/加密 → 数据类型”依次判，先命中先定级
func grade(it item) finding {
	f := finding{it: it}
	switch {
	case it.reach == "internet" && !it.auth:
		f.level, f.reason, f.action = 0, "互联网可达且未强制认证", "立即关闭公网入口或前置统一认证"
	case it.reach == "internet" && it.kind == "management":
		f.level, f.reason, f.action = 1, "管理面上公网（有认证）", "收敛到堡垒机或 IP 白名单"
	case it.reach == "internet" && !it.tls:
		f.level, f.reason, f.action = 1, "公网明文传输", "启用 TLS，否则下线"
	case it.reach == "internet":
		f.level, f.reason, f.action = 2, "受控公网业务入口", "保持 WAF / 限流 / 证书巡检"
	case !it.auth && it.kind == "data":
		f.level, f.reason, f.action = 1, "数据面未认证，内网可达即拖库", "加认证并限定来源到 app 网段"
	case !it.auth && it.kind == "management":
		f.level, f.reason, f.action = 1, "管理面未认证", "加认证或改为仅本机监听"
	case it.reach == "bastion" && !it.auth:
		f.level, f.reason, f.action = 1, "跳板可达且未认证", "补认证，跳板侧加双人审批"
	case it.kind == "data" && !it.tls:
		f.level, f.reason, f.action = 2, "数据面明文", "启用内网 TLS"
	case it.kind == "data":
		f.level, f.reason, f.action = 2, "数据面可达，需最小权限", "来源限定到 app 网段并审计"
	case it.kind == "management":
		f.level, f.reason, f.action = 2, "内网管理面", "限制到堡垒机网段"
	case it.reach == "bastion":
		f.level, f.reason, f.action = 2, "经堡垒机可达", "纳入清单，按季度复核"
	default:
		f.level, f.reason, f.action = 3, "内网业务面（已认证）", "保持现状，纳入清单"
	}
	return f
}

func main() {
	items := parse(inventory)
	findings := make([]finding, 0, len(items))
	internet, perLevel := 0, map[int]int{}
	for _, it := range items {
		if it.reach == "internet" {
			internet++
		}
		f := grade(it)
		perLevel[f.level]++
		findings = append(findings, f)
	}

	fmt.Printf("== 暴露面清单（%d 条）==\n", len(items))
	fmt.Printf("%-16s %-6s %-5s %-10s %-6s %-6s %-12s %-5s %s\n",
		"服务", "端口", "协议", "可达范围", "认证", "加密", "类型", "等级", "理由")
	for _, f := range findings {
		fmt.Printf("%-16s %-6d %-5s %-10s %-6s %-6s %-12s %-5s %s\n",
			f.it.service, f.it.port, f.it.proto, f.it.reach,
			yn(f.it.auth), yn(f.it.tls), f.it.kind, levelName(f.level), f.reason)
	}

	fmt.Printf("\n== 汇总 ==\n")
	fmt.Printf("  条目总数 %d；互联网可达 %d 条（%.0f%%）\n", len(items), internet, 100*float64(internet)/float64(len(items)))
	fmt.Printf("  P0 %d 条 / P1 %d 条 / P2 %d 条 / P3 %d 条\n",
		perLevel[0], perLevel[1], perLevel[2], perLevel[3])

	fmt.Printf("\n== P0（24 小时内处置）==\n")
	for _, f := range findings {
		if f.level == 0 {
			fmt.Printf("  %s %d/%s：%s → %s\n", f.it.service, f.it.port, f.it.proto, f.reason, f.action)
		}
	}
	fmt.Printf("\n== P1（本迭代内处置）==\n")
	for _, f := range findings {
		if f.level == 1 {
			fmt.Printf("  %s %d/%s：%s → %s\n", f.it.service, f.it.port, f.it.proto, f.reason, f.action)
		}
	}
}

func yn(b bool) string {
	if b {
		return "有"
	}
	return "无"
}

func levelName(n int) string {
	return [...]string{"P0", "P1", "P2", "P3"}[n]
}
