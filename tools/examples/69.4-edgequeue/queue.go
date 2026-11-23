// Package edgequeue 演示「断网续传」的最小正确实现：追加写 WAL + 独立 ack 水位 + 重放。
// 三条不变量：
//  1. 只有 CRC 校验通过的完整帧才算一条记录（尾部半帧自动忽略）；
//  2. ack 水位只增不减，且独立落盘（原子 rename）；
//  3. 重放从 ack+1 开始，顺序不变、不重不漏。
package edgequeue

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type Record struct {
	Seq     uint64 `json:"seq"`
	TS      int64  `json:"ts"` // 设备本地采集时间（ms）；云端还要记自己的接收时间
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
}

const (
	walName = "edge.wal"
	ackName = "edge.ack"
	// 帧格式：4B 长度 | 4B CRC32(payload) | payload(json)
	frameHeader = 8
)

type Queue struct {
	mu      sync.Mutex
	f       *os.File
	w       *bufio.Writer
	ackPath string
	lastSeq uint64
	ack     uint64
	fsync   bool
	valid   int64 // WAL 里已校验通过的字节数
}

// Open 打开（或新建）队列目录。fsync=true 表示每条记录都 fsync（最坏崩溃丢 0 条，吞吐最低）。
func Open(dir string, fsync bool) (*Queue, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	q := &Queue{ackPath: filepath.Join(dir, ackName), fsync: fsync}
	if b, err := os.ReadFile(q.ackPath); err == nil {
		if len(b) != 8 {
			return nil, fmt.Errorf("ack 文件损坏：%d 字节", len(b))
		}
		q.ack = binary.BigEndian.Uint64(b)
	}
	f, err := os.OpenFile(filepath.Join(dir, walName), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	q.f = f
	// 恢复：顺序扫描，遇到半帧/CRC 错的尾部就截断（这些记录从未被 ack，截断安全）
	if err := q.recover(); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := q.f.Seek(q.valid, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	q.w = bufio.NewWriterSize(q.f, 64*1024)
	return q, nil
}

func (q *Queue) recover() error {
	if _, err := q.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReaderSize(q.f, 64*1024)
	var off, last uint64
	for {
		var hdr [frameHeader]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			break // EOF 或半帧
		}
		n := binary.BigEndian.Uint32(hdr[0:4])
		if n == 0 || n > 16<<20 {
			break
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			break
		}
		if crc32.ChecksumIEEE(body) != binary.BigEndian.Uint32(hdr[4:8]) {
			break
		}
		var rec Record
		if err := json.Unmarshal(body, &rec); err != nil {
			break
		}
		off += uint64(frameHeader) + uint64(n)
		last = rec.Seq
	}
	q.valid = int64(off)
	q.lastSeq = last
	if err := q.f.Truncate(q.valid); err != nil {
		return err
	}
	return nil
}

// Append 写一条记录：先写 WAL 再返回 seq；seq 是云端去重键。
func (q *Queue) Append(ts int64, topic string, payload []byte) (uint64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.lastSeq++
	rec := Record{Seq: q.lastSeq, TS: ts, Topic: topic, Payload: payload}
	body, err := json.Marshal(rec)
	if err != nil {
		return 0, err
	}
	var hdr [frameHeader]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(body)))
	binary.BigEndian.PutUint32(hdr[4:8], crc32.ChecksumIEEE(body))
	if _, err := q.w.Write(hdr[:]); err != nil {
		return 0, err
	}
	if _, err := q.w.Write(body); err != nil {
		return 0, err
	}
	if err := q.w.Flush(); err != nil {
		return 0, err
	}
	if q.fsync {
		if err := q.f.Sync(); err != nil {
			return 0, err
		}
	}
	q.valid += int64(frameHeader + len(body))
	return rec.Seq, nil
}

// Ack 推进水位：只增不减；原子 rename 保证要么旧值要么新值，不会写出半个文件。
// 注意 ack 只表示「云端已确认」，记录本身由 Compact 删除——先 ack 后删，
// 反过来的话崩溃窗口里会丢数据。
func (q *Queue) Ack(seq uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if seq <= q.ack {
		return nil // 幂等：重复 ack 不倒退
	}
	if seq > q.lastSeq {
		return fmt.Errorf("ack %d 超过已写入的 %d", seq, q.lastSeq)
	}
	tmp := q.ackPath + ".tmp"
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], seq)
	// ① 写临时文件并 fsync（数据落盘）② rename（原子替换）③ fsync 目录（目录项落盘）
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b[:]); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, q.ackPath); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(q.ackPath)); err == nil {
		dir.Sync()
		dir.Close()
	}
	q.ack = seq
	return nil
}

func (q *Queue) Acked() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.ack
}

// Replay 从 ack+1 开始顺序回调；回调返回错误会立刻停（保留水位，下次重试）。
func (q *Queue) Replay(fn func(Record) error) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.w.Flush(); err != nil {
		return 0, err
	}
	f, err := os.Open(q.f.Name())
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	var off int64
	n := 0
	for off < q.valid {
		var hdr [frameHeader]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return n, err
		}
		size := binary.BigEndian.Uint32(hdr[0:4])
		body := make([]byte, size)
		if _, err := io.ReadFull(r, body); err != nil {
			return n, err
		}
		off += int64(frameHeader) + int64(size)
		if crc32.ChecksumIEEE(body) != binary.BigEndian.Uint32(hdr[4:8]) {
			return n, errors.New("重放时 CRC 校验失败")
		}
		var rec Record
		if err := json.Unmarshal(body, &rec); err != nil {
			return n, err
		}
		if rec.Seq <= q.ack {
			continue
		}
		if err := fn(rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Compact 丢掉所有 seq <= ack 的记录：写新文件 → fsync → rename → 重置偏移。
// 只在 ack 之后做；任何时刻崩溃，要么是旧 WAL，要么是新 WAL，两者都含全部未 ack 记录。
func (q *Queue) Compact() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.w.Flush(); err != nil {
		return err
	}
	tmp := q.f.Name() + ".compact"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := q.ReplayLocked(func(rec Record) error {
		body, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		var hdr [frameHeader]byte
		binary.BigEndian.PutUint32(hdr[0:4], uint32(len(body)))
		binary.BigEndian.PutUint32(hdr[4:8], crc32.ChecksumIEEE(body))
		if _, err := out.Write(hdr[:]); err != nil {
			return err
		}
		_, err = out.Write(body)
		return err
	}); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	out.Close()
	q.f.Close()
	if err := os.Rename(tmp, q.f.Name()); err != nil {
		return err
	}
	f, err := os.OpenFile(q.f.Name(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	q.f = f
	if err := q.recover(); err != nil {
		return err
	}
	if _, err := q.f.Seek(q.valid, io.SeekStart); err != nil {
		return err
	}
	q.w = bufio.NewWriterSize(q.f, 64*1024)
	return nil
}

func (q *Queue) ReplayLocked(fn func(Record) error) (int, error) {
	if err := q.w.Flush(); err != nil {
		return 0, err
	}
	f, err := os.Open(q.f.Name())
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	var off int64
	n := 0
	for off < q.valid {
		var hdr [frameHeader]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return n, err
		}
		size := binary.BigEndian.Uint32(hdr[0:4])
		body := make([]byte, size)
		if _, err := io.ReadFull(r, body); err != nil {
			return n, err
		}
		off += int64(frameHeader) + int64(size)
		var rec Record
		if err := json.Unmarshal(body, &rec); err != nil {
			return n, err
		}
		if rec.Seq <= q.ack {
			continue
		}
		if err := fn(rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (q *Queue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.w != nil {
		if err := q.w.Flush(); err != nil {
			return err
		}
	}
	return q.f.Close()
}

func (q *Queue) Stats() (last, ack uint64, bytes int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lastSeq, q.ack, q.valid
}
