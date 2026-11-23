package main

import "fmt"

func main() {
	total := 0
	for i := 0; i < 10; i++ {
		total += i
	}
	fmt.Println(total)
}
