// 最小「词法 → 语法」流水线：把 `a=1;b=hello;` 解析成 map。
// 拆成 Scanner 与 Parser 两段，格式变化只改文法，不改调用方。
package main

import (
	"fmt"
	"strconv"
	"unicode"
)

type tokKind int

const (
	tIdent tokKind = iota
	tNumber
	tAssign
	tSemi
	tEOF
)

type token struct {
	kind tokKind
	lit  string
}

// Scanner：字节流 → Token 流
func scan(src string) []token {
	var ts []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		switch c := rs[i]; {
		case unicode.IsSpace(c):
			i++
		case c == '=':
			ts = append(ts, token{tAssign, "="})
			i++
		case c == ';':
			ts = append(ts, token{tSemi, ";"})
			i++
		case unicode.IsDigit(c):
			j := i
			for j < len(rs) && unicode.IsDigit(rs[j]) {
				j++
			}
			ts = append(ts, token{tNumber, string(rs[i:j])})
			i = j
		case unicode.IsLetter(c):
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			ts = append(ts, token{tIdent, string(rs[i:j])})
			i = j
		default:
			panic("非法字符: " + string(c))
		}
	}
	return append(ts, token{tEOF, ""})
}

// Parser：Token 流 → 结构（DOM）
type kv struct {
	key, val string
	isNum    bool
}

func parse(ts []token) []kv {
	var out []kv
	i := 0
	for ts[i].kind != tEOF {
		key := ts[i].lit
		i++ // ident
		if ts[i].kind != tAssign {
			panic("期望 =")
		}
		i++
		val := ts[i].lit
		num := ts[i].kind == tNumber
		i++
		if ts[i].kind != tSemi {
			panic("期望 ;")
		}
		i++
		out = append(out, kv{key, val, num})
	}
	return out
}

func main() {
	src := "a=1; b=hello; port=8080;"
	for _, e := range parse(scan(src)) {
		if e.isNum {
			n, _ := strconv.Atoi(e.val)
			fmt.Printf("%-6s = %d (int)\n", e.key, n)
		} else {
			fmt.Printf("%-6s = %q (string)\n", e.key, e.val)
		}
	}
}
