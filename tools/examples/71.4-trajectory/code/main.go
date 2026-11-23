package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: exp_a|exp_b|exp_d|exp_c")
		return
	}
	switch os.Args[1] {
	case "exp_a":
		expA()
	case "exp_b":
		expB()
	case "exp_d":
		expD()
	case "st":
		stTest()
	case "dbg":
		dbg()
	case "exp_c":
		expC()
	default:
		fmt.Println("unknown", os.Args[1])
	}
}
