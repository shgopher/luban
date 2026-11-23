#!/bin/bash
# 66.5 生产落地：群聊的 Redis 数据面「可抄 / 可验 / 可退」命令序列
# 前置：redis-server 8.10.2 已起在 16385 端口（见 run_all.sh 头部）
set -u
R="redis-cli -p 16385"
cd "$(dirname "$0")"

echo "## 0) 连通性"
$R ping
echo

echo "## 1) 群元数据：成员数决定扩散策略，群 ID 就是后续分片键"
$R del group:g1 >/dev/null
$R hset group:g1 name "产品评审群" member_count 500 strategy write
$R hgetall group:g1
echo

echo "## 2) 群时间线：单群单 key，score = 群内单调 seq（消息只存一份）"
$R del tl:g1 >/dev/null
$R zadd tl:g1 1001 "msg:1001|hello" 1002 "msg:1002|world" 1003 "msg:1003|again"
$R zrange tl:g1 0 -1 withscores
echo

echo "## 3) 收件箱（写扩散区）：只存 msg id 指针，裁剪到最近 100 条，7 天 TTL"
$R del inbox:u1 >/dev/null
$R zadd inbox:u1 1001 m0000001001 1002 m0000001002 1003 m0000001003
$R zremrangebyrank inbox:u1 0 -101
$R expire inbox:u1 604800
echo -n "zcard="; $R zcard inbox:u1
echo -n "ttl="; $R ttl inbox:u1
echo

echo "## 4) join_seq：新成员 u9 入群时记水位，只能看到 1003 之后的消息"
$R hset group:g1:member:u9 join_seq 1003
$R hget group:g1:member:u9 join_seq
echo

echo "## 5) 已读聚合：Lua 去重 + 计数；先算一次 zrange 拿到待读消息"
$R del readset:msg1 cnt:msg1 >/dev/null
echo -n "u1 首次已读 -> "; $R --eval read.lua readset:msg1 cnt:msg1 , u1
echo -n "u1 重复已读 -> "; $R --eval read.lua readset:msg1 cnt:msg1 , u1
echo -n "u2 首次已读 -> "; $R --eval read.lua readset:msg1 cnt:msg1 , u2
echo -n "发送者看到已读人数 -> "; $R hget cnt:msg1 read
echo

echo "## 6) 可退：大群阈值开关（扇出 P99 > 200ms 或成员数 > 阈值 → 切读扩散）"
$R set group:g1:strategy read
echo -n "当前策略 -> "; $R get group:g1:strategy
echo

echo "## 7) 失败形态：对 string 类型 key 做 ZADD 必须报 WRONGTYPE（不是静默失败）"
$R set tl:bad "not-a-zset"
$R zadd tl:bad 1 x
echo

echo "## 8) 抽样核对：写扩散区里 u1 的收件箱与群时间线尾部"
$R zrange inbox:u1 0 -1 withscores
$R zrange tl:g1 -1 -1 withscores
