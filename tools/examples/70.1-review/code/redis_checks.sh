#!/usr/bin/env bash
# Redis 8.10.2 @ 127.0.0.1:16391（本节专用实例，可 FLUSHALL）
# 用途：回调短窗口去重 + mock 机审调用限流（数据库去重表仍是最终真相，Redis 只挡前面一层）
set -u
R="redis-cli -p 16391"
$R FLUSHALL >/dev/null

echo "# 1) 回调短窗口去重：同一 audit_id 第二次 SET NX 不再落键"
echo -n "SET NX (第1次): "; $R SET 'audit:dedup:callback:req-001' 1 NX PX 86400000
echo -n "SET NX (第2次): "; $R SET 'audit:dedup:callback:req-001' 1 NX PX 86400000
echo -n "EXISTS:        "; $R EXISTS 'audit:dedup:callback:req-001'

echo "# 2) mock 机审调用限流：分钟窗口 INCR + TTL"
echo -n "INCR (第1次):  "; $R INCR 'audit:limit:mockaudit:202611211730'
echo -n "INCR (第2次):  "; $R INCR 'audit:limit:mockaudit:202611211730'
echo -n "EXPIRE 60:     "; $R EXPIRE 'audit:limit:mockaudit:202611211730' 60
echo -n "TTL:           "; $R TTL 'audit:limit:mockaudit:202611211730'
