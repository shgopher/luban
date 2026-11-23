package edgequeue

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// 模拟「进程被 kill -9」：数据已 write+fsync 到 WAL，但还没来得及 ack。
// 重开队列后必须从 ack+1 精确重放，一条不多一条不少。
func TestCrashResumeExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	q, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 1000; i++ {
		if _, err := q.Append(int64(1700000000000+i), "dev/1/temp", []byte(fmt.Sprintf(`{"t":%d}`, i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Ack(500); err != nil { // 云端只确认到 500
		t.Fatal(err)
	}
	// 这里不 Close：等价于进程直接被杀（fsync=true 时数据已落盘）
	last, ack, _ := q.Stats()
	t.Logf("崩溃前：lastSeq=%d ack=%d", last, ack)

	q2, err := Open(dir, true) // 重启
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Close()
	last, ack, _ = q2.Stats()
	if last != 1000 || ack != 500 {
		t.Fatalf("恢复后 last=%d ack=%d，期望 1000/500", last, ack)
	}
	got := make([]uint64, 0, 500)
	n, err := q2.Replay(func(r Record) error {
		got = append(got, r.Seq)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 500 || got[0] != 501 || got[len(got)-1] != 1000 {
		t.Fatalf("重放 %d 条，首 %d 尾 %d；期望 500 条 501..1000", n, got[0], got[len(got)-1])
	}
	for i := 1; i < len(got); i++ {
		if got[i] != got[i-1]+1 {
			t.Fatalf("序号不连续：%d → %d", got[i-1], got[i])
		}
	}
	// 续写必须接着 1001，不能回绕重复
	seq, err := q2.Append(1700000002000, "dev/1/temp", []byte(`{"t":1001}`))
	if err != nil || seq != 1001 {
		t.Fatalf("续写 seq=%d err=%v，期望 1001", seq, err)
	}
}

// 尾部半帧（写到一半断电）：CRC/长度校验应忽略它，回收后还能继续写。
func TestTornTail(t *testing.T) {
	dir := t.TempDir()
	q, _ := Open(dir, true)
	for i := 0; i < 100; i++ {
		q.Append(1, "t", []byte(`{"v":1}`))
	}
	q.Close()
	wal := filepath.Join(dir, walName)
	st, _ := os.Stat(wal)
	if err := os.Truncate(wal, st.Size()-5); err != nil { // 砍掉最后 5 字节
		t.Fatal(err)
	}
	q2, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Close()
	n, _ := q2.Replay(func(Record) error { return nil })
	last, _, valid := q2.Stats()
	t.Logf("半帧恢复：valid=%d 字节，可重放 %d 条，lastSeq=%d", valid, n, last)
	if n != 99 || last != 99 {
		t.Fatalf("期望 99 条有效记录，得到 n=%d last=%d", n, last)
	}
	seq, err := q2.Append(2, "t", []byte(`{"v":2}`))
	if err != nil || seq != 100 {
		t.Fatalf("恢复后追加 seq=%d err=%v，期望 100", seq, err)
	}
}

func TestAckMonotonic(t *testing.T) {
	q, _ := Open(t.TempDir(), false)
	defer q.Close()
	for i := 0; i < 10; i++ {
		q.Append(1, "t", []byte("x"))
	}
	if err := q.Ack(10); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(5); err != nil { // 旧 ack 幂等，不倒退
		t.Fatal(err)
	}
	if q.Acked() != 10 {
		t.Fatalf("ack 倒退了：%d", q.Acked())
	}
	if err := q.Ack(11); err == nil {
		t.Fatal("ack 超过 lastSeq 应报错")
	}
}

// 压缩：ack 之后丢掉已确认记录，文件变小，未确认记录仍完整。
func TestCompact(t *testing.T) {
	dir := t.TempDir()
	q, _ := Open(dir, false)
	payload := make([]byte, 200)
	for i := 0; i < 1000; i++ {
		q.Append(1, "t", payload)
	}
	q.Ack(900)
	wal := filepath.Join(dir, walName)
	before, _ := os.Stat(wal)
	if err := q.Compact(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(wal)
	n, _ := q.Replay(func(Record) error { return nil })
	t.Logf("压缩：%d B → %d B（%.1f%%），剩余未确认 %d 条", before.Size(), after.Size(),
		100*float64(after.Size())/float64(before.Size()), n)
	if n != 100 {
		t.Fatalf("压缩后应剩 100 条，得到 %d", n)
	}
	q.Close()
	q2, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Close()
	n2, _ := q2.Replay(func(Record) error { return nil })
	if n2 != 100 {
		t.Fatalf("重开后应剩 100 条，得到 %d", n2)
	}
}

// 量一下「每条都 fsync」的代价：这是断网续传最关键的选型点（丢 0 条 vs 吞吐）。
func BenchmarkAppendFsync(b *testing.B) {
	q, _ := Open(b.TempDir(), true)
	defer q.Close()
	payload := make([]byte, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Append(1, "dev/1/temp", payload)
	}
}

func BenchmarkAppendNoFsync(b *testing.B) {
	q, _ := Open(b.TempDir(), false)
	defer q.Close()
	payload := make([]byte, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Append(1, "dev/1/temp", payload)
	}
}
