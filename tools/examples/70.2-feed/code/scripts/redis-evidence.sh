#!/usr/bin/env bash
# 实测 B 的数据准备 + redis-cli 真实命令证据（只用 16392 本节实例）
# 不重启、不 kill 任何进程；只对自己实例做 FLUSHALL。
# 用法：./redis-evidence.sh load   # 灌数据 + 写入/翻页证据 + 命令耗时
#       ./redis-evidence.sh trim   # 裁剪证据（放 Go 计时之后跑，避免动到数据集）
set -uo pipefail
R="redis-cli -p 16392"
MODE="${1:-load}"

if [ "$MODE" = "trim" ]; then
  echo "== 6. 可抄之三：裁剪（收件箱只留最近 1000 条，控制内存）=="
  set -x
  $R ZCARD inbox:u1001
  $R ZREMRANGEBYRANK inbox:u1001 0 -1001
  $R ZCARD inbox:u1001
  set +x
  echo "-- 裁剪后最老的一条（第 1000 条）："
  $R ZREVRANGEBYSCORE inbox:u1001 +inf -inf WITHSCORES LIMIT 999 1
  echo
  echo "DONE"
  exit 0
fi

echo "== 0. 环境 =="
$R INFO server | grep -E 'redis_version|os:|arch_bits|process_id'
echo -n "PING: "; $R PING

echo
echo "== 1. 清空本节实例（16392 只有本节的 key）=="
$R FLUSHALL

echo
echo "== 2. 可抄之一：单条写入（收件箱 = ZSET，score 用毫秒时间戳，member = feed:+12 位补零 id）=="
set -x
$R ZADD inbox:u1001 1730000000000 feed:000000009001
$R ZADD inbox:u1001 1730000000123 feed:000000009002
$R ZADD inbox:u1001 1730000000123 feed:000000009003
set +x
echo -n "ZCARD inbox:u1001 -> "; $R ZCARD inbox:u1001
echo "同分（1730000000123）两条 member：feed:000000009002 < feed:000000009003，ZREVRANGE 降序先出 9003"
$R ZREVRANGE inbox:u1001 0 -1 WITHSCORES

echo
echo "== 3. 灌入两套基线（各 3000 条，每 3 条共用一个 score）=="
echo "   inbox:u1001    member = feed:012d（生产格式，字符串序 = 数值序）"
echo "   inbox:u1001raw member = feed:%d（对照，字符串序 != 数值序）"
python3 - > /tmp/ch70_2_zset.txt <<'PY'
base = 1_700_000_000_000
for n in range(1, 3001):
    score = base + (n // 3) * 1000
    print(f"ZADD inbox:u1001 {score} feed:{n:012d}")
    print(f"ZADD inbox:u1001raw {score} feed:{n}")
PY
$R DEL inbox:u1001 inbox:u1001raw
$R --pipe < /tmp/ch70_2_zset.txt | tail -1
echo -n "ZCARD inbox:u1001    -> "; $R ZCARD inbox:u1001
echo -n "ZCARD inbox:u1001raw -> "; $R ZCARD inbox:u1001raw

echo
echo "== 4. 可抄之二：真实游标翻页（ZREVRANGEBYSCORE + 同分 id 兜底）=="
echo "-- 第 1 页：score 从 +inf 往下取 20 条"
set -x
$R ZREVRANGEBYSCORE inbox:u1001 +inf -inf WITHSCORES LIMIT 0 20
set +x
echo "-- 第 1 页末条 = (score, member)，作为下一页游标"
$R ZREVRANGEBYSCORE inbox:u1001 +inf -inf WITHSCORES LIMIT 19 1

echo
echo "-- 错误写法：下一页直接 (score 排掉整个同分组，同分剩余成员一起丢"
LAST_SCORE=$($R ZREVRANGEBYSCORE inbox:u1001 +inf -inf WITHSCORES LIMIT 19 1 | tail -1)
echo "   第 20 名的 score = $LAST_SCORE"
$R ZREVRANGEBYSCORE inbox:u1001 "($LAST_SCORE" -inf WITHSCORES LIMIT 0 4

echo
echo "-- 正确写法：先把同一 score 组里 member < 游标的补出来，再从更小 score 取"
$R ZREVRANGEBYSCORE inbox:u1001 "$LAST_SCORE" "$LAST_SCORE" WITHSCORES LIMIT 0 10

echo
echo "== 5. redis-cli 侧单命令耗时（CONFIG RESETSTAT + INFO commandstats，服务端口径）=="
$R CONFIG RESETSTAT > /dev/null
python3 - > /tmp/ch70_2_pages.txt <<'PY'
base = 1_700_000_000_000
for i in range(2000):
    sc = base + (i % 1000) * 1000
    print(f"ZREVRANGEBYSCORE inbox:u1001 {sc} {sc} LIMIT 0 20")
    print(f"ZREVRANGEBYSCORE inbox:u1001 ({sc} -inf LIMIT 0 20")
PY
$R < /tmp/ch70_2_pages.txt > /dev/null
$R INFO commandstats | grep -E 'cmdstat_zrevrangebyscore|cmdstat_zscore'

echo
echo "LOAD_DONE"
