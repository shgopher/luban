// 衡量「一个周边功能对核心系统的伤害值」：
// 伤害值 = Σ over 每一处修改 log2(该处修改行数 + 1)
// 「一处修改」= git diff -U0 里一个 hunk；修改行数 = 删除行 + 新增行。
//
// 默认 -demo=true：读 testdata/ 下四个固定补丁，不依赖 git 历史；
// -demo=false 时对真实仓库跑（-repo 指定路径），需要 HEAD~3..HEAD 四段提交。
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type hunk struct{ del, add int }

func hunks(diff string) []hunk {
	var out []hunk
	for _, ln := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(ln, "@@") {
			continue
		}
		// @@ -a,b +c,d @@
		f := strings.Fields(ln)
		if len(f) < 3 {
			continue
		}
		del := part(f[1]) // -a,b
		add := part(f[2]) // +c,d
		out = append(out, hunk{del, add})
	}
	return out
}

func part(s string) int {
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	if i := strings.Index(s, ","); i >= 0 {
		s = s[i+1:]
	} else {
		return 1 // 省略 ,行数 时默认 1 行
	}
	n, _ := strconv.Atoi(s)
	return n
}

// score 返回 (处数, 修改行, 伤害值)。
func score(diff string) (int, float64, float64) {
	hs := hunks(diff)
	var lines, sum float64
	for _, h := range hs {
		n := float64(h.del)
		if h.add > h.del { // 替换算 1 行：取增删较大值
			n = float64(h.add)
		}
		lines += n
		sum += math.Log2(n + 1)
	}
	return len(hs), lines, sum
}

func main() {
	demo := flag.Bool("demo", true, "用 testdata 固定补丁复现")
	repo := flag.String("repo", "repo", "真实仓库路径（-demo=false 时使用）")
	td := flag.String("testdata", "testdata", "补丁目录")
	flag.Parse()

	names := []string{"f1 优惠券", "f2 税", "f3 物流费", "f4 风控打点"}

	if *demo {
		for i, name := range names {
			b, err := os.ReadFile(filepath.Join(*td, fmt.Sprintf("f%d.patch", i+1)))
			if err != nil {
				fmt.Fprintln(os.Stderr, "读补丁失败:", err)
				os.Exit(1)
			}
			n, lines, sum := score(string(b))
			fmt.Printf("%-12s 处数=%d 修改行=%-4.0f 伤害值=%.2f  明细=%v\n", name, n, lines, sum, hunks(string(b)))
		}
		return
	}

	for i, c := range []string{"HEAD~3", "HEAD~2", "HEAD~1", "HEAD"} {
		out, err := exec.Command("git", "-C", *repo, "diff", "-U0", c+"^", c, "--", "core").Output()
		if err != nil {
			panic(err)
		}
		n, lines, sum := score(string(out))
		fmt.Printf("%-12s 处数=%d 修改行=%-4.0f 伤害值=%.2f  明细=%v\n", names[i], n, lines, sum, hunks(string(out)))
	}
}
