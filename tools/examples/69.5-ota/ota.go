// Package ota 演示固件升级的三件保命事：清单签名、断点续传、A/B 回滚。
// 不实现真正的镜像刷写（依赖设备 bootloader），只做云端能控制、且能测试的部分。
package ota

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Manifest 是「一句话把固件钉死」的东西：哪个产品、哪个版本、多大、哈希多少、从哪下。
// 注意：签名覆盖的是 Manifest 的规范序列化，不覆盖固件本体；固件本体由 SHA256 钉住。
type Manifest struct {
	Product  string `json:"product"`
	HwRev    string `json:"hw_rev"` // 硬件版本：A 型号固件不能推给 B 型号
	Version  string `json:"version"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	URL      string `json:"url"`
	NotAfter int64  `json:"not_after"`             // 清单过期时间，防多年后重放旧签名
	Rollback string `json:"rollback_to,omitempty"` // 失败时回到哪个版本
}

func canonical(m Manifest) ([]byte, error) {
	// struct 字段顺序固定 + json.Marshal 不排序 map（这里没有 map），签名双方得到同一串字节。
	return json.Marshal(m)
}

func Sign(priv ed25519.PrivateKey, m Manifest) (string, error) {
	b, err := canonical(m)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, b)), nil
}

func Verify(pub ed25519.PublicKey, m Manifest, sigB64 string) error {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("签名不是合法 base64：%w", err)
	}
	b, err := canonical(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, b, sig) {
		return errors.New("清单签名校验失败：拒绝升级")
	}
	return nil
}

var ErrInterrupted = errors.New("模拟网络中断")

type DownloadStat struct {
	TotalBytes  int64 // 固件总大小
	Fetched     int64 // 本次真正从网络读到的字节
	ResumedFrom int64 // 本次从哪个偏移续传（0 = 从头）
	Requests    int
	Elapsed     time.Duration
}

// Downloader 做「先写 .part，校验通过再原子改名」：升级失败时旧固件始终可用。
type Downloader struct {
	Client      *http.Client
	InterruptAt int64 // 测试用：读到该字节数后主动断开，模拟断网；0 = 不中断
	Timeout     time.Duration
}

func (d *Downloader) Fetch(ctx context.Context, m Manifest, dest string) (DownloadStat, error) {
	st := DownloadStat{TotalBytes: m.Size}
	part := dest + ".part"
	if d.Client == nil {
		d.Client = &http.Client{Timeout: 30 * time.Second}
	}
	start := time.Now()
	defer func() { st.Elapsed = time.Since(start) }()

	var offset int64
	if fi, err := os.Stat(part); err == nil {
		offset = fi.Size()
	}
	if offset > m.Size { // 本地残留比目标还大：不可信，从头来
		os.Remove(part)
		offset = 0
	}
	// 已经下完：只做校验，不重新下载（掉电重试的关键优化）
	if offset == m.Size && m.Size > 0 {
		if err := verifyFile(part, m.SHA256); err == nil {
			return st, os.Rename(part, dest)
		}
		os.Remove(part)
		offset = 0
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return st, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	st.ResumedFrom = offset
	resp, err := d.Client.Do(req)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	st.Requests = 1

	var f *os.File
	switch resp.StatusCode {
	case http.StatusPartialContent: // 206：服务端支持断点续传
		f, err = os.OpenFile(part, os.O_WRONLY|os.O_APPEND, 0o644)
	case http.StatusOK: // 200：服务端忽略 Range，只能从头重下，必须截断否则文件会拼坏
		offset = 0
		st.ResumedFrom = 0
		f, err = os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	default:
		return st, fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	if err != nil {
		return st, err
	}
	defer f.Close()

	limit := int64(-1)
	if d.InterruptAt > 0 {
		limit = d.InterruptAt
	}
	buf := make([]byte, 64*1024)
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if limit >= 0 && written+int64(n) > limit {
				n = int(limit - written)
			}
			if n > 0 {
				if _, err := f.Write(buf[:n]); err != nil {
					return st, err
				}
				written += int64(n)
			}
			if limit >= 0 && written >= limit {
				f.Sync()
				st.Fetched = written
				return st, ErrInterrupted
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			st.Fetched = written
			return st, rerr
		}
	}
	st.Fetched = written
	if err := f.Sync(); err != nil {
		return st, err
	}
	if err := verifyFile(part, m.SHA256); err != nil {
		os.Remove(part)
		return st, err
	}
	if err := os.Rename(part, dest); err != nil {
		return st, err
	}
	return st, nil
}

func verifyFile(path, wantHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(got), []byte(strings.ToLower(wantHex))) != 1 {
		return fmt.Errorf("固件 SHA256 不匹配：期望 %s…，实际 %s…", wantHex[:8], got[:8])
	}
	return nil
}

// ---- A/B 分区状态机：升级最怕的是「新固件起不来」，所以必须能自己回滚 ----

type Slot string

const (
	SlotA Slot = "A"
	SlotB Slot = "B"
)

type State struct {
	Active       Slot   `json:"active"`
	Pending      Slot   `json:"pending,omitempty"`
	BootAttempts int    `json:"boot_attempts"`
	Version      string `json:"version"`
	PrevVersion  string `json:"prev_version,omitempty"`
}

var (
	ErrNoPending = errors.New("没有待启动的分区")
	ErrSameSlot  = errors.New("不能把待升级分区设成当前分区")
)

// Stage 把新固件写到「非活动分区」，并标记待启动。此时活动分区与旧固件完全不动。
func (s *State) Stage(to Slot, version string) error {
	if to == s.Active {
		return ErrSameSlot
	}
	s.Pending = to
	s.BootAttempts = 0
	s.PrevVersion = s.Version
	s.Version = version
	return nil
}

// MarkBootOK：新固件起来了，提交。
func (s *State) MarkBootOK() {
	if s.Pending == "" {
		return
	}
	s.Active = s.Pending
	s.Pending = ""
	s.BootAttempts = 0
}

// MarkBootFailed：起不来就累计；达到阈值回滚到旧分区（旧固件一直在活动分区，回滚=改回指针）。
// 判据来自 bootloader 的「试启动次数」，不是云端的「升级失败次数」——云端连不上时设备也要能自救。
func (s *State) MarkBootFailed(maxAttempts int) (rolledBack bool) {
	if s.Pending == "" {
		return false
	}
	s.BootAttempts++
	if s.BootAttempts >= maxAttempts {
		s.Pending = ""
		s.BootAttempts = 0
		s.Version = s.PrevVersion
		return true
	}
	return false
}
