#!/bin/bash
# Redis 8.10.2 真实操作：每设备游标（Hash）+ 用户变更流水（Stream，MAXLEN≈ + 7 天 TTL）
set -u
R="redis-cli -h ::1 -p 16383"   # 本机沙箱下 IPv4 回环新建连接被拒，用 IPv6 回环 ::1（Redis 默认 bind 127.0.0.1 -::1）
run() { echo "\$ $*"; "$@" 2>&1; echo; }

echo "=== 0) 启动 Redis 8.10.2（端口 16383，纯内存）==="
redis-server --port 16383 --save '' --appendonly no --daemonize yes >/dev/null 2>&1
sleep 0.3
run $R PING

echo "=== 1) 写每设备游标：一个 Hash 存 change_cursor + 每会话 conv_cursor ==="
run $R HSET sync:cursor:u1001:t_phone change_cursor 8842 conv:c1 1200 conv:c2 980 conv:c3 1150
run $R HGETALL sync:cursor:u1001:t_phone
run $R HGET sync:cursor:u1001:t_phone conv:c1

echo "=== 2) 用户变更流水：Redis Stream 追加两条变更（消息瘦条目 + 已读）==="
run $R XADD sync:changes:u1001 '*' kind msg conv c1 seq 1201 msg_id m_c1_001201
run $R XADD sync:changes:u1001 '*' kind read conv c1 read_seq 1201
run $R XLEN sync:changes:u1001
run $R XRANGE sync:changes:u1001 - +

echo "=== 3) 客户端从 change_cursor=8842 拉增量：XREAD 按游标读、后进先出无意义（Stream 天然有序）==="
run $R XREAD COUNT 10 STREAMS sync:changes:u1001 0

echo "=== 4) 变更流水限长 + 7 天 TTL：防内存无限增长，过期后客户端回退全量快照 ==="
run $R XADD sync:changes:u1001 MAXLEN '~' 10000 '*' kind msg conv c2 seq 777 msg_id m_c2_000777
run $R EXPIRE sync:changes:u1001 604800
run $R TTL sync:changes:u1001
run $R EXPIRE sync:cursor:u1001:t_phone 604800
run $R TTL sync:cursor:u1001:t_phone

echo "=== 5) 原子更新：MULTI/EXEC 里同时写游标与变更流水（同实例内不出现半更新）==="
printf 'MULTI\nHSET sync:cursor:u1001:t_phone change_cursor 8843\nXADD sync:changes:u1001 MAXLEN ~ 10000 * kind msg conv c1 seq 1202 msg_id m_c1_001202\nEXEC\n' | $R

echo "=== 6) 收尾清理：删除演示 key 并关停 Redis ==="
run $R DEL sync:cursor:u1001:t_phone sync:changes:u1001
run $R SHUTDOWN NOSAVE
