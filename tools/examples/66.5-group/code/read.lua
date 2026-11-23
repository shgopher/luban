-- 66.5 群已读聚合：一次 Lua 调用完成「去重 + 计数 + 返回最新已读人数」
-- KEYS[1] = 已读集合 hash（field = 成员 uid，天然去重，也支持按需取明细）
-- KEYS[2] = 聚合计数 hash（只存一个数字，发送者只读它）
-- ARGV[1] = 成员 uid
-- 返回：>0 = 新已读人数；-1 = 该成员之前已读过（幂等，不重复计数）
local added = redis.call('HSETNX', KEYS[1], ARGV[1], '1')
if added == 1 then
  return redis.call('HINCRBY', KEYS[2], 'read', 1)
end
return -1
