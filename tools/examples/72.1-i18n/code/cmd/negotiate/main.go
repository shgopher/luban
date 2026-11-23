// 实测 A：BCP 47 协商矩阵。真实请求头 -> MatchStrings -> 支持 locale + 内容回退链。
package main

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

// 服务端实际能提供文案的 locale（UI 语言包），不是「用户能发的所有 tag」。
var supported = []language.Tag{
	language.MustParse("en"),
	language.MustParse("zh-Hans"),
	language.MustParse("zh-Hant"),
	language.MustParse("sr-Latn"),
	language.MustParse("es-419"),
	language.MustParse("de"),
	language.MustParse("fr"),
	language.MustParse("ja"),
	language.MustParse("ko"),
	language.MustParse("ru"),
	language.MustParse("ar"),
	language.MustParse("pl"),
	language.MustParse("pt-BR"),
}

var headers = []string{
	"zh-CN,zh;q=0.9,en;q=0.8",                 // 大陆用户常见
	"zh-Hans-CN,zh;q=0.9",                     // 显式 script
	"zh-Hant-HK",                              // 港澳繁体、无 q
	"sr-Latn-RS",                              // 同一语言两种字母
	"sr-Cyrl-RS",                              // 对照：西里尔
	"es-419",                                  // 拉美西语：只有地区码，没国家
	"es-ES,es;q=0.9",                          // 对照：欧洲西语
	"en-US;q=0,fr;q=0.9",                      // q=0 明确拒绝 en-US
	"*",                                       // 只要任意
	"de-DE, *;q=0.5",                          // 通配兜底，权重更低
	"zh-Hant-TW,zh;q=0.8",                     // 台湾繁体
	"en-GB;q=0.8,en-US;q=0.7",                 // 同语言不同地区
	"pt-BR,pt;q=0.9,en;q=0.5",                 // 巴西葡语
	"ja-JP, ja;q=0.9, en;q=0.8",               // 带空格的真实写法
	"xx-YY,zh-Hans;q=0.5",                     // 非法/未知语言 tag + 合法备选
	";;;,q=0.9",                               // 完全非法的头
	"zh-CN;q=abc,en;q=0.4",                    // q 值非法
	"en-US;q=0.9,zh-Hant;q=0.8,zh-Hans;q=0.7", // 三语权重
}

// fallbackChain 返回内容包查找顺序：越具体越先查，最后永远落到 en。
// 关键：BCP 47 的「具体度」不等于字符串长度，而是 script/region 的有无。
func fallbackChain(t language.Tag) []string {
	base, _ := t.Base()
	script, sc := t.Script()
	var out []string
	add := func(s string) {
		for _, v := range out {
			if v == s {
				return
			}
		}
		out = append(out, s)
	}
	add(t.String())
	if sc == language.Exact {
		add(base.String() + "-" + script.String())
	}
	add(base.String())
	add("en")
	return out
}

func main() {
	m := language.NewMatcher(supported)
	fmt.Println("## 支持 locale 集（13 个）:", tagsToStrings(supported))
	fmt.Println()
	for i, h := range headers {
		parsed, qs, err := language.ParseAcceptLanguage(h)
		matched, idx := language.MatchStrings(m, h)
		chain := fallbackChain(matched)
		note := ""
		if err != nil {
			note = "err=" + err.Error()
		}
		fmt.Printf("%02d\theader=%q\n", i+1, h)
		fmt.Printf("    parsed=%v q=%v %s\n", tagsToStrings(parsed), qs, note)
		fmt.Printf("    matched=%s idx=%d supported[idx]=%s\n", matched, idx, supported[idx])
		fmt.Printf("    chain=%s\n", strings.Join(chain, " -> "))
	}

	// 统计：原始头 -> 匹配 tag 字符串（带 -u-rg 扩展） vs 支持索引（真正的内容 locale）
	rawTag := map[string]bool{}
	byIdx := map[string]bool{}
	for _, h := range headers {
		tag, idx := language.MatchStrings(m, h)
		rawTag[tag.String()] = true
		byIdx[supported[idx].String()] = true
	}
	fmt.Printf("\n## 归一化：%d 个请求头 -> matched tag 字符串 %d 种；按 supported 索引只有 %d 种\n",
		len(headers), len(rawTag), len(byIdx))
	fmt.Printf("## matched 字符串集合=%s\n", mapKeys(rawTag))
	fmt.Printf("## 真正的内容 locale（缓存键应取这个）=%s\n", mapKeys(byIdx))
}

func tagsToStrings(ts []language.Tag) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.String()
	}
	return out
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
