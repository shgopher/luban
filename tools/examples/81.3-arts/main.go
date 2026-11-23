// 统计 ARTS 打卡：每周 A(Algorithm)/R(Review)/T(Tip)/S(Share) 四项，
// 输出完成率与「连续满勤」周数。
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	f, _ := os.Open("arts.txt")
	defer f.Close()
	sc := bufio.NewScanner(f)
	var weeks, full, cur, best int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		weeks++
		done := 0
		for _, x := range fields[1:5] {
			if x != "-" {
				done++
			}
		}
		if done == 4 {
			full++
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	fmt.Printf("周数=%d 满勤=%d 完成率=%.0f%% 最长连续满勤=%d 周\n",
		weeks, full, 100*float64(full)/float64(weeks), best)
}
