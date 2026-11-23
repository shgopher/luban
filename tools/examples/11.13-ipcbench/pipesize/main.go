package main

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	const F_GETPIPE_SZ = 1032
	sz, _, errno := syscall.Syscall(syscall.SYS_FCNTL, r.Fd(), F_GETPIPE_SZ, 0)
	if errno != 0 {
		fmt.Println("fcntl err:", errno)
		return
	}
	fmt.Printf("pipe capacity = %d bytes (F_GETPIPE_SZ)\n", sz)
	st, _ := r.Stat()
	fmt.Printf("pipe inode st_blksize = %d\n", st.Sys().(*syscall.Stat_t).Blksize)
	_ = w
}
