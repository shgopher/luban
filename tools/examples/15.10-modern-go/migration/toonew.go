//go:build demo_toonew

package main

import "strings"

// 只在 `go test -tags demo_toonew` 时参与编译：
// strings.CutLast 是 Go 1.27 才有的 API，而本模块声明 go 1.21。
func init() {
	_, _, _ = strings.CutLast("a/b/c", "/")
}
