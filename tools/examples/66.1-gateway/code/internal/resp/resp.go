// Package resp 是一个最小 RESP2 客户端。
// 只实现本实验用到的命令：SET/GET/DEL/GETDEL/PUBLISH/SUBSCRIBE/INFO/EVAL。
// 自己写而不是引 go-redis，是为了让「每次路由查询就是一次 Redis 往返」这件事
// 在代码里显式可见，也便于精确统计命令次数。
package resp

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// Conn 一条到 Redis 的连接。Do 是串行的请求-应答；订阅连接用 Send+ReadValue。
type Conn struct {
	c  net.Conn
	r  *bufio.Reader
	w  *bufio.Writer
	mu sync.Mutex
}

// Dial 建立连接并设置超时。
func Dial(addr string, timeout time.Duration) (*Conn, error) {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	return &Conn{c: c, r: bufio.NewReaderSize(c, 64<<10), w: bufio.NewWriterSize(c, 4<<10)}, nil
}

// Close 关闭连接。
func (c *Conn) Close() error { return c.c.Close() }

// Do 串行执行一条命令，返回解析后的值。
func (c *Conn) Do(args ...any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.c.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, err
	}
	defer func() { _ = c.c.SetDeadline(time.Time{}) }()
	if err := c.writeCommand(args); err != nil {
		return nil, err
	}
	if err := c.w.Flush(); err != nil {
		return nil, err
	}
	return c.readReply()
}

// Send 只写不读，用于 SUBSCRIBE 这类持续推送的命令。
func (c *Conn) Send(args ...any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.writeCommand(args); err != nil {
		return err
	}
	return c.w.Flush()
}

// ReadValue 读一个回复，供订阅连接循环使用。
func (c *Conn) ReadValue() (any, error) { return c.readReply() }

func (c *Conn) writeCommand(args []any) error {
	if _, err := c.w.WriteString("*" + strconv.Itoa(len(args)) + "\r\n"); err != nil {
		return err
	}
	for _, a := range args {
		s := toStr(a)
		if _, err := c.w.WriteString("$" + strconv.Itoa(len(s)) + "\r\n" + s + "\r\n"); err != nil {
			return err
		}
	}
	return nil
}

func toStr(a any) string {
	switch v := a.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}

func (c *Conn) readReply() (any, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, fmt.Errorf("resp: short line %q", line)
	}
	body := line[1 : len(line)-2]
	switch line[0] {
	case '+':
		return body, nil
	case '-':
		return nil, errors.New(body)
	case ':':
		n, err := strconv.ParseInt(body, 10, 64)
		return n, err
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil // nil bulk
		}
		buf := make([]byte, n+2)
		if _, err := readFull(c.r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := c.readReply()
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("resp: unknown prefix %q", line[0])
	}
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
