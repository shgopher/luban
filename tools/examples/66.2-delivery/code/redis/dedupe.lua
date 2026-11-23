-- 上行幂等 + 会话 seq 分配：一个 Lua 脚本原子完成，杜绝「先查再插」竞态
-- KEYS[1]=去重键 s662:if:{c66}:dedupe:<client_msg_id>
-- KEYS[2]=会话 seq 计数器 s662:if:{c66}:seq
-- KEYS[3]=消息本体 hash s662:if:{c66}:msgs （field=seq, value=payload）
-- KEYS[4]=收件人离线收件箱 s662:if:inbox:<user_id> （member=msg_id, score=seq）
-- ARGV[1]=消息 payload（JSON）
-- 返回：第一次落库返回新 msg_id；重发命中返回同一条 msg_id
local cur = redis.call('GET', KEYS[1])
if cur then return cur end
local seq = redis.call('INCR', KEYS[2])
local mid = 'm-' .. seq
redis.call('HSET', KEYS[3], seq, ARGV[1])
redis.call('ZADD', KEYS[4], seq, mid)
redis.call('SET', KEYS[1], mid, 'EX', 86400)
return mid
