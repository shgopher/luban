// 真 Redis 上的配额预扣：先 GET 再 INCR 会不会超卖？INCR/DECR 与 Lua 有没有超卖？
// 手写最小 RESP 客户端，零第三方依赖：go run redis_quota.go -addr 127.0.0.1:16382
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	quotaKey = "quota:t-001:2026-11:api_calls"
	limit    = 100
	reqs     = 1000
)

type conn struct {
	c net.Conn
	r *bufio.Reader
}

func dial(addr string) (*conn, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &conn{c: c, r: bufio.NewReader(c)}, nil
}

// 发 RESP 数组命令（inline 命令不支持带空格的 Lua 脚本）
func (cn *conn) send(args ...string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	_, err := cn.c.Write([]byte(b.String()))
	return err
}

func (cn *conn) reply() (interface{}, error) {
	line, err := cn.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, fmt.Errorf("redis error: %s", line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, _ := strconv.Atoi(line[1:])
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io_ReadFull(cn.r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, _ := strconv.Atoi(line[1:])
		out := make([]interface{}, 0, n)
		for i := 0; i < n; i++ {
			v, err := cn.reply()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown reply %q", line)
}

func io_ReadFull(r *bufio.Reader, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := r.Read(buf[n:])
		if err != nil {
			return n, err
		}
		n += m
	}
	return n, nil
}

func (cn *conn) do(args ...string) (interface{}, error) {
	if err := cn.send(args...); err != nil {
		return nil, err
	}
	return cn.reply()
}

func (cn *conn) close() { cn.c.Close() }

func mustDial(addr string) *conn {
	cn, err := dial(addr)
	if err != nil {
		panic(err)
	}
	return cn
}

func reset(addr string) {
	cn := mustDial(addr)
	defer cn.close()
	cn.do("SET", quotaKey, "0")
}

func finalUsed(addr string) int64 {
	cn := mustDial(addr)
	defer cn.close()
	v, err := cn.do("GET", quotaKey)
	if err != nil || v == nil {
		return 0
	}
	n, _ := strconv.ParseInt(v.(string), 10, 64)
	return n
}

// 错误：先 GET 判定，再 INCR 扣减。判定用的是旧值，窗口内全体通过。
func wrongGetThenIncr(addr string) (admitted int64) {
	reset(addr)
	var wg sync.WaitGroup
	for i := 0; i < reqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cn := mustDial(addr)
			defer cn.close()
			v, err := cn.do("GET", quotaKey)
			if err != nil {
				return
			}
			cur := int64(0)
			if v != nil {
				cur, _ = strconv.ParseInt(v.(string), 10, 64)
			}
			if cur >= limit {
				return
			}
			if _, err := cn.do("INCR", quotaKey); err == nil {
				atomic.AddInt64(&admitted, 1)
			}
		}()
	}
	wg.Wait()
	return
}

// 正确：INCR 先扣，再看返回值；超限立刻 DECR 回退。判定与扣减是同一条原子命令。
func correctIncrDecr(addr string) (admitted, decr int64) {
	reset(addr)
	var wg sync.WaitGroup
	for i := 0; i < reqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cn := mustDial(addr)
			defer cn.close()
			v, err := cn.do("INCR", quotaKey)
			if err != nil {
				return
			}
			n := v.(int64)
			if n > limit {
				cn.do("DECR", quotaKey) // 回退
				atomic.AddInt64(&decr, 1)
				return
			}
			atomic.AddInt64(&admitted, 1)
		}()
	}
	wg.Wait()
	return
}

const luaScript = `local v = redis.call('INCR', KEYS[1])
if v > tonumber(ARGV[1]) then redis.call('DECR', KEYS[1]); return 0 end
return v`

// 正确且只花一次往返：Lua 里完成「扣 + 判 + 回退」。
func correctLua(addr string) (admitted, denied int64) {
	reset(addr)
	var wg sync.WaitGroup
	for i := 0; i < reqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cn := mustDial(addr)
			defer cn.close()
			v, err := cn.do("EVAL", luaScript, "1", quotaKey, strconv.Itoa(limit))
			if err != nil {
				return
			}
			if v.(int64) > 0 {
				atomic.AddInt64(&admitted, 1)
			} else {
				atomic.AddInt64(&denied, 1)
			}
		}()
	}
	wg.Wait()
	return
}

func main() {
	addr := flag.String("addr", "127.0.0.1:16382", "redis addr")
	flag.Parse()
	cn := mustDial(*addr)
	ver, _ := cn.do("INFO", "server")
	cn.close()
	verLine := ""
	for _, l := range strings.Split(fmt.Sprint(ver), "\n") {
		if strings.HasPrefix(l, "redis_version") {
			verLine = strings.TrimSpace(l)
		}
	}
	fmt.Printf("环境：%s/%s，Redis %s（docker alpine:3.20，127.0.0.1:16382）\n", "darwin", "arm64", strings.TrimPrefix(verLine, "redis_version:"))
	fmt.Printf("场景：配额上限 %d，%d 个并发请求（每请求独立连接）\n\n", limit, reqs)

	a := wrongGetThenIncr(*addr)
	fmt.Println("== 错误：GET 判定后再 INCR ==")
	fmt.Printf("放行 %d 个，超卖 %d 个，Redis 最终计数 %d（配额的 %.0f%%）\n\n",
		a, a-limit, finalUsed(*addr), float64(finalUsed(*addr))*100/limit)

	b, d := correctIncrDecr(*addr)
	fmt.Println("== 正确：INCR 先扣，超限 DECR 回退 ==")
	fmt.Printf("放行 %d 个，拒绝 %d 个，超卖 %d 个，Redis 最终计数 %d（客户端多花 %d 次 DECR）\n\n",
		b, reqs-b, b-limit, finalUsed(*addr), d)

	c, e := correctLua(*addr)
	fmt.Println("== 正确且单次往返：Lua 里「扣 + 判 + 回退」==")
	fmt.Printf("放行 %d 个，拒绝 %d 个，超卖 %d 个，Redis 最终计数 %d（客户端只 1 次往返）\n",
		c, e, c-limit, finalUsed(*addr))
}
