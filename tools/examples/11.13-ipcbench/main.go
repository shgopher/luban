package main

// IPC 基准：pipe / unix socket / TCP loopback / 共享内存
// 全部是「两个真实进程」，父进程计时，子进程做对端。
//   ipcbench -kind pipe|unix|tcp|shm -mode lat|tput -size N -n N
// 子进程由父进程自己拉起（-child），不手工调用。

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// shmPath 优先用 /dev/shm（Linux tmpfs），没有就退到临时目录（macOS）。
func shmPath() string {
	if st, err := os.Stat("/dev/shm"); err == nil && st.IsDir() {
		return "/dev/shm/ipcbench"
	}
	return filepath.Join(os.TempDir(), "ipcbench-shm")
}

var (
	mode  = flag.String("mode", "lat", "lat|tput")
	kind  = flag.String("kind", "pipe", "pipe|unix|tcp|shm")
	size  = flag.Int("size", 1, "每次消息字节数")
	n     = flag.Int("n", 20000, "往返次数或消息数")
	child = flag.Bool("child", false, "子进程模式（内部使用）")
	addr  = flag.String("addr", "", "子进程连接地址（内部使用）")
)

func main() {
	flag.Parse()
	if *child {
		childMain()
		return
	}
	start := time.Now()
	switch *mode {
	case "lat":
		report(latency(*kind))
	case "tput":
		bytes := throughput(*kind)
		el := time.Since(start).Seconds()
		fmt.Printf("kind=%s size=%d n=%d bytes=%d elapsed=%.3fs tput=%.1f MB/s\n",
			*kind, *size, *n, bytes, el, float64(bytes)/el/1e6)
	}
}

func report(ds []time.Duration) {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	p := func(q float64) time.Duration {
		i := int(float64(len(ds)) * q)
		if i >= len(ds) {
			i = len(ds) - 1
		}
		return ds[i]
	}
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	fmt.Printf("kind=%s size=%d n=%d mean=%s p50=%s p90=%s p99=%s\n",
		*kind, *size, len(ds), sum/time.Duration(len(ds)), p(0.50), p(0.90), p(0.99))
}

// ---------- 父进程侧 ----------

type peer struct {
	rw  io.ReadWriter // pipe
	c   net.Conn      // unix/tcp
	cmd *exec.Cmd
	shm []byte
}

func (p *peer) close() {
	if p.cmd != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	if p.c != nil {
		p.c.Close()
	}
}

func (p *peer) write(b []byte) error {
	if p.rw != nil {
		_, err := p.rw.Write(b)
		return err
	}
	_, err := p.c.Write(b)
	return err
}

func (p *peer) read(b []byte) error {
	if p.rw != nil {
		_, err := io.ReadFull(p.rw, b)
		return err
	}
	_, err := io.ReadFull(p.c, b)
	return err
}

func start(k string) *peer {
	p := &peer{}
	switch k {
	case "pipe":
		// 两条单向管道：父写→子读(stdin)，子写(stdout)→父读
		prA, pwA, err := os.Pipe()
		must(err)
		prB, pwB, err := os.Pipe()
		must(err)
		cmd := exec.Command(os.Args[0], "-child", "-kind", "pipe", "-mode", *mode, "-size", fmt.Sprint(*size))
		cmd.Stdin = prA
		cmd.Stdout = pwB
		cmd.Stderr = os.Stderr
		must(cmd.Start())
		prA.Close()
		pwB.Close()
		p.rw = &pipeEnd{r: prB, w: pwA}
		p.cmd = cmd

	case "unix":
		path := "/tmp/ipcbench.sock"
		os.Remove(path)
		ln, err := net.Listen("unix", path)
		must(err)
		cmd := exec.Command(os.Args[0], "-child", "-kind", "unix", "-mode", *mode, "-addr", path, "-size", fmt.Sprint(*size))
		cmd.Stderr = os.Stderr
		must(cmd.Start())
		c, err := ln.Accept()
		must(err)
		ln.Close()
		p.c = c
		p.cmd = cmd

	case "tcp":
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		must(err)
		a := ln.Addr().String()
		cmd := exec.Command(os.Args[0], "-child", "-kind", "tcp", "-mode", *mode, "-addr", a, "-size", fmt.Sprint(*size))
		cmd.Stderr = os.Stderr
		must(cmd.Start())
		c, err := ln.Accept()
		must(err)
		ln.Close()
		p.c = c
		p.cmd = cmd

	case "shm":
		os.Remove(shmPath())
		f, err := os.OpenFile(shmPath(), os.O_RDWR|os.O_CREATE, 0600)
		must(err)
		must(f.Truncate(int64(8 + *size)))
		f.Close()
		cmd := exec.Command(os.Args[0], "-child", "-kind", "shm", "-mode", *mode, "-size", fmt.Sprint(*size))
		cmd.Stderr = os.Stderr
		must(cmd.Start())
		p.shm = openShm(*size)
		p.cmd = cmd
	}
	return p
}

// pipeEnd：写端 w，读端 r
type pipeEnd struct {
	r *os.File
	w *os.File
}

func (p *pipeEnd) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeEnd) Read(b []byte) (int, error)  { return io.ReadFull(p.r, b) }

func latency(k string) []time.Duration {
	msg := make([]byte, *size)
	buf := make([]byte, *size)
	ds := make([]time.Duration, 0, *n)
	p := start(k)
	defer p.close()

	if k == "shm" {
		seq := (*uint32)(unsafe.Pointer(&p.shm[0]))
		ack := (*uint32)(unsafe.Pointer(&p.shm[4]))
		data := p.shm[8:]
		for i := 1; i <= 200; i++ { // 预热：等子进程映射好
			data[0] = byte(i)
			atomic.StoreUint32(seq, uint32(i))
			for atomic.LoadUint32(ack) != uint32(i) {
			}
		}
		for i := 201; i <= *n+200; i++ {
			t0 := time.Now()
			data[0] = byte(i)
			atomic.StoreUint32(seq, uint32(i))
			for atomic.LoadUint32(ack) != uint32(i) {
			}
			ds = append(ds, time.Since(t0))
		}
		return ds
	}

	for i := 0; i < *n; i++ {
		t0 := time.Now()
		p.write(msg)
		p.read(buf)
		ds = append(ds, time.Since(t0))
	}
	return ds
}

func throughput(k string) int64 {
	msg := make([]byte, *size)
	p := start(k)
	defer p.close()

	if k == "shm" {
		seq := (*uint32)(unsafe.Pointer(&p.shm[0]))
		ack := (*uint32)(unsafe.Pointer(&p.shm[4]))
		data := p.shm[8:]
		for i := 1; i <= *n; i++ {
			copy(data, msg) // 真搬 64KB，不是只发通知
			atomic.StoreUint32(seq, uint32(i))
			for atomic.LoadUint32(ack) != uint32(i) {
			}
		}
		// 等子进程确认收完最后一块
		return int64(*size) * int64(*n)
	}

	for i := 0; i < *n; i++ {
		if err := p.write(msg); err != nil {
			break
		}
	}
	return int64(*size) * int64(*n)
}

// ---------- 子进程侧 ----------

func childMain() {
	switch *kind {
	case "pipe":
		r, w := io.Reader(os.Stdin), io.Writer(os.Stdout)
		if *mode == "lat" {
			buf := make([]byte, *size)
			for i := 0; i < *n; i++ {
				if _, err := io.ReadFull(r, buf); err != nil {
					return
				}
				w.Write(buf)
			}
			return
		}
		io.Copy(io.Discard, r) // 一直排空，父进程写完就 kill

	case "unix", "tcp":
		var c net.Conn
		var err error
		for i := 0; i < 100; i++ {
			if *kind == "unix" {
				c, err = net.Dial("unix", *addr)
			} else {
				c, err = net.Dial("tcp", *addr)
			}
			if err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		must(err)
		if *mode == "lat" {
			buf := make([]byte, *size)
			for i := 0; i < *n; i++ {
				if _, err := io.ReadFull(c, buf); err != nil {
					return
				}
				c.Write(buf)
			}
			return
		}
		io.Copy(io.Discard, c) // 一直排空，父进程写完就 kill

	case "shm":
		m := openShm(*size)
		seq := (*uint32)(unsafe.Pointer(&m[0]))
		ack := (*uint32)(unsafe.Pointer(&m[4]))
		data := m[8:]
		buf := make([]byte, *size)
		var last uint32
		for {
			s := atomic.LoadUint32(seq)
			if s != last && s != 0 {
				last = s
				copy(buf, data) // 真读一遍，强制内存流量
				atomic.StoreUint32(ack, s)
			}
		}
	}
}

func openShm(size int) []byte {
	f, err := os.OpenFile(shmPath(), os.O_RDWR, 0600)
	must(err)
	m, err := syscall.Mmap(int(f.Fd()), 0, int(8+size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	must(err)
	return m
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
