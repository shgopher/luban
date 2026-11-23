package main

import (
	"crypto/aes"
	"fmt"
)

func main() { b, _ := aes.NewCipher(make([]byte, 16)); fmt.Println(b.BlockSize()) }
