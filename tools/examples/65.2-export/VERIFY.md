# 65.2 数据隔离与合规 · 验证记录

环境：macOS darwin/arm64，go version go1.27.1 darwin/arm64，sqlite3 3.43.2，
Docker 本地仅 alpine:3.20（docker pull 因 Keychain Error -67674 失败，未起任何容器）。

本机 Go 的 module index 损坏：不加 `GODEBUG=goindex=0` 时，标准库全部报
`package crypto/aes is not in std (...)`；加上后 `go list std` 恢复 381 个包。这是环境问题，
不是程序问题，正文环境说明里已注明。

## 1. crypto-shredding（AES-256-GCM envelope + 销毁 DEK）

```bash
cd <root>/.cache/t65/65.2 && GODEBUG=goindex=0 go run shred/main.go
```

完整输出见 `shred.log`，关键行：

```
租户     DEK 指纹(sha256前8)   包裹后长度        状态
t-001  90c6ba96957995c9   60           active
t-002  96df10d50e6ca390   60           active
t-003  595b396a4ad3e367   60           active

-- 对 t-002 执行 crypto-shredding：DEK 清零 + 删除包裹记录 --
读 t-002 加密列      → 失败：cipher: message authentication failed
读 t-002 未加密金额  → 成功：300（加密边界之外的数据没被删）
解密破坏前的备份密文 → 失败：cipher: message authentication failed（备份里的密文一起失效）
读 t-001 加密列      → 成功：alice@a.example（密钥隔离 = 爆炸半径单租户）
t-001 密文配错 AAD   → 失败：cipher: message authentication failed

-- nonce 复用实验（同一 key、同一 12B nonce）--
C1 xor C2 = 00000000000000080909
P1 xor P2 = 00000000000000080909
两者相等 = true（GCM 不报警）
```

## 2. 按租户导出（keyset 分页 + manifest 续传 + 幂等 + 快照上限）

```bash
cd <root>/.cache/t65/65.2 && GODEBUG=goindex=0 go run export/main.go
```

完整输出见 `export.log`，关键行：

```
中断于 part2 后，从 lastID=10000 续传：rows=2347 sha256[:8]=28da184021032be9 与全量 part3 一致=true
同一分页请求重投两次 sha256[:8]=28da184021032be9 / 28da184021032be9 一致=true
offset 分页重取 part3：重复行=3、漏行=0（回填行把游标往后挤）
keyset(带 snapshotMax) 重取 part3：重复行=0、漏行=0
```

## 3. 删除 ≠ 删干净（SQLite 3.43.2 实测）

```bash
sqlite3 :memory: "CREATE TABLE u(tenant TEXT,uid TEXT); INSERT INTO u VALUES('t-001','u1'),('t-002','u2'),('t-002','u3'); CREATE TABLE bak AS SELECT * FROM u; DELETE FROM u WHERE tenant='t-002'; SELECT '主库='||count(*) FROM u WHERE tenant='t-002'; SELECT '备份副本='||count(*) FROM bak WHERE tenant='t-002';"
主库=0
备份副本=2
```

## 未验证项

- 按租户 PITR 恢复：缺 PostgreSQL 实例；`docker pull postgres:16-alpine` 因
  `Keychain Error. (-67674)` 失败（本地只有 alpine:3.20），未在真实库上跑恢复演练。
  正文的 87.4 分钟/1 TiB 是 10.4 的换算口径（1 TiB ÷ 200 MB/s），已标「约」与算式。
- crypto-shredding 的 KEK 在程序里是内存字节，未接真实 KMS/HSM；备份密文是内存副本，
  未验证对象存储/离线备份端的密钥分离。
- 未起任何 docker 容器；`docker ps -a` 为空，t65pg 不是本次任务创建的。
