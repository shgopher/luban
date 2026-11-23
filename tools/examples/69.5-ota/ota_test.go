package ota

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 确定性伪随机：让每次运行的 SHA256 一致，文档里的实测输出才可复现。
func fwData(n int) []byte {
	b := make([]byte, n)
	x := uint32(0x9e3779b9)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return b
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// 服务端：记录 Range 请求次数与实际发出的字节数，验证客户端真的只拉了缺的那段。
type srvStat struct {
	reqs   atomic.Int64
	served atomic.Int64
	mu     sync.Mutex
	ranges []string
}

func (st *srvStat) rangeList() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]string(nil), st.ranges...)
}

func rangeServer(t *testing.T, data []byte, ignoreRange bool) (*httptest.Server, *srvStat) {
	t.Helper()
	st := &srvStat{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.reqs.Add(1)
		st.mu.Lock()
		st.ranges = append(st.ranges, r.Header.Get("Range"))
		st.mu.Unlock()
		if ignoreRange {
			r.Header.Del("Range")
		}
		http.ServeContent(&countWriter{ResponseWriter: w, n: &st.served}, r, "fw.bin", time.Unix(1700000000, 0), newReaderAt(data))
	})
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s, st
}

type countWriter struct {
	http.ResponseWriter
	n *atomic.Int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n.Add(int64(n))
	return n, err
}

// 用例 1：断在 100KB，续传 156KB，总下载量 = 固件大小，SHA256 通过。
func TestResumeDownload(t *testing.T) {
	data := fwData(256 << 10)
	srv, stat := rangeServer(t, data, false)
	m := Manifest{Product: "gw-x1", Version: "2.3.0", Size: int64(len(data)), SHA256: sha(data), URL: srv.URL + "/fw.bin"}
	dest := filepath.Join(t.TempDir(), "fw.bin")

	d := &Downloader{InterruptAt: 100 << 10} // 第一次：读到 100KB 就断
	st1, err := d.Fetch(t.Context(), m, dest)
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("第一次应中断，得到 %v", err)
	}
	t.Logf("第一次：下载 %d B 后中断（ResumedFrom=%d），.part=%d B", st1.Fetched, st1.ResumedFrom, st1.Fetched)

	d2 := &Downloader{} // 第二次：正常续传
	st2, err := d2.Fetch(t.Context(), m, dest)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("第二次：ResumedFrom=%d，本次拉取 %d B / 总 %d B，耗时 %v", st2.ResumedFrom, st2.Fetched, st2.TotalBytes, st2.Elapsed.Round(time.Millisecond))
	t.Logf("服务端：%d 次请求，Range 头=%q，共写出 %d B（固件 %d B）", stat.reqs.Load(), stat.rangeList(), stat.served.Load(), len(data))
	if st2.ResumedFrom != 100<<10 {
		t.Fatalf("应从 100KB 续传，实际 %d", st2.ResumedFrom)
	}
	ranges := stat.rangeList()
	if len(ranges) != 2 || ranges[0] != "" || ranges[1] != "bytes=102400-" {
		t.Fatalf("第二次请求应带 Range: bytes=102400-，实际 %q", ranges)
	}
	if got, _ := os.ReadFile(dest); sha(got) != m.SHA256 {
		t.Fatal("落地固件哈希不对")
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("成功后应删掉 .part")
	}
}

// 用例 2：服务端篡改固件（或 CDN 缓存了旧包）→ 哈希不匹配必须删掉拒装。
func TestTamperRejected(t *testing.T) {
	data := fwData(64 << 10)
	m := Manifest{Product: "gw-x1", Version: "2.3.0", Size: int64(len(data)), SHA256: sha(data), URL: "http://x/fw.bin"}
	bad := append([]byte(nil), data...)
	bad[100] ^= 0xff // 改一个字节
	srv, _ := rangeServer(t, bad, false)
	m.URL = srv.URL + "/fw.bin"
	dest := filepath.Join(t.TempDir(), "fw.bin")
	_, err := (&Downloader{}).Fetch(t.Context(), m, dest)
	if err == nil {
		t.Fatal("篡改的固件应被拒绝")
	}
	t.Logf("拒绝原因：%v", err)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("校验失败不能留下正式文件")
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("校验失败要清掉 .part")
	}
}

// 用例 3：签名覆盖清单（版本/哈希/URL），改任何一个字段都必须验签失败。
func TestManifestSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	m := Manifest{Product: "gw-x1", Version: "2.3.0", Size: 1024, SHA256: sha([]byte("x")), URL: "https://cdn/fw.bin", Rollback: "2.2.9"}
	sig, err := Sign(priv, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, m, sig); err != nil {
		t.Fatalf("原样清单应通过：%v", err)
	}
	for _, tamper := range []func(*Manifest){
		func(m *Manifest) { m.Version = "2.4.0" }, // 升到别的版本
		func(m *Manifest) { m.URL = "https://evil/fw.bin" },
		func(m *Manifest) { m.SHA256 = sha([]byte("y")) },
	} {
		cp := m
		tamper(&cp)
		if err := Verify(pub, cp, sig); err == nil {
			t.Fatal("篡改清单应验签失败")
		}
	}
	// 换一把公钥（例如另一家厂商的固件）也必须失败
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := Verify(pub2, m, sig); err == nil {
		t.Fatal("错误公钥应验签失败")
	}
}

// 用例 4：服务端不支持 Range（返回 200）时，客户端必须截断重下，不能把新内容拼到旧 .part 后面。
func TestServerIgnoresRange(t *testing.T) {
	data := fwData(128 << 10)
	m := Manifest{Product: "gw-x1", Version: "2.3.0", Size: int64(len(data)), SHA256: sha(data)}
	srv, stat := rangeServer(t, data, true)
	m.URL = srv.URL + "/fw.bin"
	dest := filepath.Join(t.TempDir(), "fw.bin")
	d := &Downloader{InterruptAt: 60 << 10}
	if _, err := d.Fetch(t.Context(), m, dest); !errors.Is(err, ErrInterrupted) {
		t.Fatal(err)
	}
	st, err := (&Downloader{}).Fetch(t.Context(), m, dest)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("服务端忽略 Range：ResumedFrom=%d（应被重置为 0），共写出 %d B", st.ResumedFrom, stat.served.Load())
	if st.ResumedFrom != 0 {
		t.Fatal("200 响应必须从头下")
	}
	if got, _ := os.ReadFile(dest); sha(got) != m.SHA256 {
		t.Fatal("重下后哈希不对")
	}
}

// 用例 5：A/B 回滚——新固件连续两次起不来，回到旧分区（旧固件从未被覆盖）。
func TestABRollback(t *testing.T) {
	st := State{Active: SlotA, Version: "2.2.9"}
	if err := st.Stage(SlotB, "2.3.0"); err != nil {
		t.Fatal(err)
	}
	st.MarkBootFailed(2)
	rolled := st.MarkBootFailed(2)
	if !rolled || st.Active != SlotA || st.Pending != "" || st.Version != "2.2.9" {
		t.Fatalf("应回滚到 A/2.2.9，实际 %+v", st)
	}
	// 正常路径：一次启动成功即提交
	st.Stage(SlotB, "2.3.0")
	st.MarkBootOK()
	if st.Active != SlotB || st.Version != "2.3.0" {
		t.Fatalf("应提交到 B，实际 %+v", st)
	}
	t.Logf("回滚后状态=%+v；提交后状态=%+v", State{Active: SlotA, Version: "2.2.9"}, st)
}

func newReaderAt(b []byte) io.ReadSeeker { return bytes.NewReader(b) }
