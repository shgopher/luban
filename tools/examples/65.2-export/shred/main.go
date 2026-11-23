// crypto-shredding 实测：每租户一把 DEK，KEK 包裹；销毁 DEK 即让该租户密文永久不可解。
// 纯标准库，go run 直接跑。原始输出落 .cache/t65/65.2/shred.log
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

func mustGCM(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key) // 32 字节 = AES-256
	if err != nil {
		panic(err)
	}
	g, err := cipher.NewGCM(block) // 默认 12 字节 nonce、16 字节 tag
	if err != nil {
		panic(err)
	}
	return g
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil { // nonce / 密钥只许来自 CSPRNG
		panic(err)
	}
	return b
}

// seal 把 nonce 前置拼到密文里：nonce(12) || ciphertext || tag(16)
func seal(g cipher.AEAD, plain, aad []byte) []byte {
	nonce := randBytes(g.NonceSize())
	return g.Seal(nonce, nonce, plain, aad)
}

func open(g cipher.AEAD, blob, aad []byte) ([]byte, error) {
	ns := g.NonceSize()
	if len(blob) < ns+16 {
		return nil, errors.New("密文太短")
	}
	return g.Open(nil, blob[:ns], blob[ns:], aad)
}

type tenant struct {
	id      string
	dek     []byte // 只在内存/受控 KMS 中明文存在
	wrapped []byte // KEK 包裹后的 DEK，存进租户元数据表
	shred   bool
}

type record struct {
	tenant string
	email  []byte // 加密列
	amount int    // 未加密列（故意的对照）
}

func aad(tid string) []byte { return []byte(tid + "|users.email") }

func main() {
	kek := randBytes(32)
	kekGCM := mustGCM(kek)
	fmt.Println("== 环境：Go 标准库 AES-256-GCM，KEK 32B，每租户 DEK 32B ==")

	// ① 建租户：每租户生成一把 DEK，用 KEK 包裹后落库
	ts := map[string]*tenant{}
	for _, id := range []string{"t-001", "t-002", "t-003"} {
		t := &tenant{id: id, dek: randBytes(32)}
		t.wrapped = seal(kekGCM, t.dek, []byte("dek/"+id)) // KEK 只加密"密钥"，不加密业务数据
		ts[id] = t
	}
	fmt.Printf("%-6s %-18s %-12s %s\n", "租户", "DEK 指纹(sha256前8)", "包裹后长度", "状态")
	for _, t := range ts {
		fp := sha256.Sum256(t.dek)
		fmt.Printf("%-6s %-18s %-12d %s\n", t.id, hex.EncodeToString(fp[:8]), len(t.wrapped), "active")
	}

	// ② 写业务数据：加密列用本租户 DEK + 租户绑定的 AAD
	recs := []record{
		{"t-001", nil, 100}, {"t-001", nil, 200},
		{"t-002", nil, 300}, {"t-002", nil, 400},
		{"t-003", nil, 500},
	}
	emails := []string{"alice@a.example", "bob@a.example", "carol@b.example", "dave@b.example", "erin@c.example"}
	for i := range recs {
		g := mustGCM(ts[recs[i].tenant].dek)
		recs[i].email = seal(g, []byte(emails[i]), aad(recs[i].tenant))
	}
	fmt.Printf("\n%-6s %-18s %-12s %s\n", "租户", "加密列(前8B hex)", "密文长度", "未加密金额")
	for i, r := range recs {
		fmt.Printf("%-6s %-18s %-12d %d\n", r.tenant, hex.EncodeToString(r.email[:8]), len(r.email), r.amount)
		_ = i
	}

	// ③ 销毁 t-002 的 DEK：先备份一份密文，证明"备份里的密文同样解不开"
	backup := append([]byte(nil), recs[2].email...)
	victim := ts["t-002"]
	for i := range victim.dek { // 覆写内存中的密钥材料
		victim.dek[i] = 0
	}
	victim.wrapped = nil // 元数据表里删除被包裹的 DEK
	victim.shred = true
	fmt.Println("\n-- 对 t-002 执行 crypto-shredding：DEK 清零 + 删除包裹记录 --")

	// ④ 销毁后逐个验证
	g2 := mustGCM(victim.dek) // 即便拿到全零密钥，也解不开原密文
	_, err := open(g2, recs[2].email, aad("t-002"))
	fmt.Printf("读 t-002 加密列      → 失败：%v\n", err)
	fmt.Printf("读 t-002 未加密金额  → 成功：%d（加密边界之外的数据没被删）\n", recs[2].amount)
	outside := mustGCM(ts["t-002"].dek)
	_, err = open(outside, backup, aad("t-002"))
	fmt.Printf("解密破坏前的备份密文 → 失败：%v（备份里的密文一起失效）\n", err)

	// 其他租户不受影响：t-001 正常解
	p, err := open(mustGCM(ts["t-001"].dek), recs[0].email, aad("t-001"))
	fmt.Printf("读 t-001 加密列      → 成功：%s（密钥隔离 = 爆炸半径单租户）\n", p)

	// ⑤ AAD 绑定租户：拿对的 DEK、错的租户上下文也解不开
	_, err = open(mustGCM(ts["t-001"].dek), recs[0].email, aad("t-999"))
	fmt.Printf("t-001 密文配错 AAD   → 失败：%v\n", err)

	// ⑥ nonce 复用：GCM 不会报错，但两段密文异或 = 两段明文异或（密钥流重复）
	key := randBytes(32)
	g := mustGCM(key)
	fixed := make([]byte, g.NonceSize()) // 全零 nonce，模拟"写死 nonce"
	p1, p2 := []byte("amount=100"), []byte("amount=999")
	c1 := g.Seal(nil, fixed, p1, nil)
	c2 := g.Seal(nil, fixed, p2, nil)
	tag := 16                                     // GCM tag 长度，Seal 输出 = 密文 || tag
	x1 := xor(c1[:len(c1)-tag], c2[:len(c2)-tag]) // 去掉 tag 只比密文
	x2 := xor(p1, p2)
	fmt.Printf("\n-- nonce 复用实验（同一 key、同一 12B nonce）--\n")
	fmt.Printf("C1 xor C2 = %s\n", hex.EncodeToString(x1))
	fmt.Printf("P1 xor P2 = %s\n", hex.EncodeToString(x2))
	fmt.Printf("两者相等 = %v（密钥流重复，攻击者由 C1 xor C2 直接得到 P1 xor P2，GCM 不报警）\n", bytes.Equal(x1, x2))
}

func xor(a, b []byte) []byte {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}
