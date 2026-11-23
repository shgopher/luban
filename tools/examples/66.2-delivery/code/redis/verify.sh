#!/bin/bash
# 66.2 可抄/可验：Redis 上行幂等 + 收件箱（本机 Redis 8.10.2，端口 16382）
R="redis-cli -p 16382"
INBOX='s662:if:inbox:u1002'
$R del 's662:if:{c66}:dedupe:cmid-1' 's662:if:{c66}:seq' 's662:if:{c66}:msgs' "$INBOX" >/dev/null

echo "--- 第 1 次发送（EVAL 同一脚本）"
$R --eval redis/dedupe.lua 's662:if:{c66}:dedupe:cmid-1' 's662:if:{c66}:seq' 's662:if:{c66}:msgs' "$INBOX" , '{"sender":1001,"body":"hello"}'
echo "--- 第 2 次发送（同一 client_msg_id 重发）"
$R --eval redis/dedupe.lua 's662:if:{c66}:dedupe:cmid-1' 's662:if:{c66}:seq' 's662:if:{c66}:msgs' "$INBOX" , '{"sender":1001,"body":"hello"}'
echo "--- 断言：重复发送后 消息条数不变、seq 只分配一次"
echo "hlen msgs = $($R hlen 's662:if:{c66}:msgs')"
echo "zcard inbox = $($R zcard "$INBOX")"
echo "seq 计数器 = $($R get 's662:if:{c66}:seq')"
echo "去重键剩余 TTL = $($R ttl 's662:if:{c66}:dedupe:cmid-1') 秒"
echo "--- 取回邮件箱一条消息（收件人视角）"
echo "zrange inbox 0 0 = $($R zrange "$INBOX" 0 0)"
echo "hget msgs 1 = $($R hget 's662:if:{c66}:msgs' 1)"
