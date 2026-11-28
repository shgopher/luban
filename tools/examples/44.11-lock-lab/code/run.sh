#!/usr/bin/env bash
# 44.11 锁实验：一条命令起 MySQL 8.0、灌数、跑 7 个并发场景、打印 data_locks、清理容器。
# 用法：bash code/run.sh            # 默认端口 33199
#       PORT=33200 bash code/run.sh # 换端口
set -uo pipefail

PORT=${PORT:-33199}
NAME="mysqllab-lock-$PORT"
DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="mysql -h127.0.0.1 -P$PORT -uroot --protocol=TCP -N -B --unbuffered"
MYSQL="$ROOT lab"

# Docker Desktop 的二进制与 socket 不在默认 PATH 里（macOS）
export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=${DOCKER_HOST:-unix://$HOME/.docker/run/docker.sock}

cleanup() {
  # 实验会话的 fifo 一并清掉
  rm -f "$DIR"/fifo_* "$DIR"/*.out 2>/dev/null
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "== 起容器 $NAME（mysql:8.0，buffer pool 128M）=="
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes \
  -p "127.0.0.1:$PORT:3306" mysql:8.0 \
  --innodb-buffer-pool-size=128M --max-connections=200 >/dev/null
for _ in $(seq 1 90); do
  if mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP -e "select 1" >/dev/null 2>&1; then break; fi
  sleep 1
done
$ROOT -e "select @@version, @@transaction_isolation, @@binlog_format, @@innodb_buffer_pool_size/1024/1024"

$ROOT < "$DIR/schema.sql"

q() { $MYSQL -e "$1"; }
locks() {
  q "SELECT ENGINE_TRANSACTION_ID trx, INDEX_NAME idx, LOCK_MODE, LOCK_STATUS, LOCK_DATA
     FROM performance_schema.data_locks
     WHERE OBJECT_NAME IN ('t','tu') AND LOCK_TYPE='RECORD'
     ORDER BY INDEX_NAME, LOCK_DATA"
}
countlocks() {
  q "SELECT COUNT(*) FROM performance_schema.data_locks
     WHERE OBJECT_NAME='t' AND LOCK_TYPE='RECORD'"
}
reset_t() { $ROOT < "$DIR/schema.sql"; }

# 起一个交互式会话（fifo 喂 SQL，输出进文件）
open_sess() {
  local tag=$1
  rm -f "$DIR/fifo_$tag"
  mkfifo "$DIR/fifo_$tag"
  $MYSQL < "$DIR/fifo_$tag" > "$DIR/$tag.out" 2>&1 &
  eval "exec $2>\"$DIR/fifo_$tag\""
}

echo
echo "########## 场景 1：select ... for update 在非唯一索引 c 上 ##########"
reset_t
open_sess a 3
open_sess b 4
printf 'begin;\nselect * from t where c=5 for update;\nselect "A-done";\n' >&3
sleep 0.6
echo "-- A 的查询结果（当前读：只有 c=5 的 id=5 一行）--"; cat "$DIR/a.out"
echo "-- A 持有的行锁 --"; locks
printf 'begin;\ninsert into t values(8,8,8);\nselect "B-done";\n' >&4
sleep 0.8
echo "-- B 插入 (8,8,8)（应为空：落在 c 索引的间隙 (5,10) 里）--"; cat "$DIR/b.out"
echo "-- 谁在等谁 --"
q "SELECT r.ENGINE_TRANSACTION_ID req, r.INDEX_NAME, r.LOCK_MODE req_mode,
          b.ENGINE_TRANSACTION_ID blk, b.LOCK_MODE blk_mode, b.LOCK_DATA blk_data
   FROM performance_schema.data_lock_waits w
   JOIN performance_schema.data_locks r ON w.REQUESTING_ENGINE_LOCK_ID=r.ENGINE_LOCK_ID
   JOIN performance_schema.data_locks b ON w.BLOCKING_ENGINE_LOCK_ID=b.ENGINE_LOCK_ID"
printf 'commit;\n' >&3
sleep 0.8
echo "-- A 提交后 B --"; cat "$DIR/b.out"
printf 'commit;\n' >&4
exec 3>&- 4>&-
wait 2>/dev/null

echo
echo "########## 场景 2：update 唯一索引，记录存在 vs 不存在 ##########"
reset_t
( echo 'begin;'; echo 'update t set d=d+1 where id=10;'; echo 'select sleep(1.2);'; echo 'commit;' ) \
  | $MYSQL >/dev/null 2>&1 &
sleep 0.4
echo "-- where id=10（存在）--"; locks
wait 2>/dev/null
reset_t
( echo 'begin;'; echo 'update t set d=d+1 where id=7;'; echo 'select sleep(1.2);'; echo 'commit;' ) \
  | $MYSQL >/dev/null 2>&1 &
sleep 0.4
echo "-- where id=7（不存在）--"; locks
wait 2>/dev/null

echo
echo "########## 场景 3：insert 唯一键冲突 → 两个等待者死锁 ##########"
reset_t
q "DELETE FROM tu; INSERT INTO tu(c,d) VALUES (1,1),(2,2),(3,3),(4,4);"
open_sess a 3
open_sess b 4
open_sess c 5
printf 'begin;\ninsert into tu(c,d) values(5,5);\nselect "A-inserted";\n' >&3
sleep 0.5
printf 'begin;\ninsert into tu(c,d) values(5,5);\n' >&4
sleep 0.5
printf 'begin;\ninsert into tu(c,d) values(5,5);\n' >&5
sleep 0.7
echo "-- 三方的锁（S WAITING 就是等着的那把共享 next-key lock）--"
q "SELECT ENGINE_TRANSACTION_ID trx, INDEX_NAME, LOCK_MODE, LOCK_STATUS, LOCK_DATA
   FROM performance_schema.data_locks WHERE OBJECT_NAME='tu' ORDER BY trx, INDEX_NAME"
printf 'rollback;\n' >&3
sleep 1.2
echo "-- A 回滚后 B（成功则无输出）--"; cat "$DIR/b.out"
echo "-- A 回滚后 C（死锁牺牲者）--"; cat "$DIR/c.out"
exec 3>&- 4>&- 5>&-
wait 2>/dev/null

echo
echo "########## 场景 4：delete 带不带 limit 的锁范围 ##########"
reset_t
q "INSERT INTO t VALUES (30,10,30);"   # 让 c=10 有两行：(10,10,10) 与 (30,10,30)
run_del() {
  local sql=$1 tag=$2
  reset_t
  q "INSERT INTO t VALUES (30,10,30);"
  open_sess a 3
  printf 'begin;\n%s\nselect "A-done";\n' "$sql" >&3
  sleep 0.7
  echo "-- [$tag] 记录锁 --"; locks
  echo "-- [$tag] 记录锁行数 --"; countlocks
  printf 'rollback;\n' >&3
  exec 3>&-
  wait 2>/dev/null
}
run_del "delete from t where c=10;" "无 limit"
run_del "delete from t where c=10 limit 2;" "limit 2"

echo
echo "########## 场景 5：insert ... select 锁源表（RR，binlog_format=ROW）##########"
reset_t
q "DROP TABLE IF EXISTS t2; CREATE TABLE t2(id int PRIMARY KEY AUTO_INCREMENT, c int, d int);"
open_sess a 3
open_sess b 4
printf 'begin;\ninsert into t2(c,d) select c,d from t;\nselect "A-done";\n' >&3
sleep 0.8
echo "-- A 在源表 t 上的锁（S = 共享 next-key lock，supremum 也锁）--"
q "SELECT INDEX_NAME, LOCK_MODE, LOCK_DATA FROM performance_schema.data_locks
   WHERE OBJECT_NAME='t' AND LOCK_TYPE='RECORD' ORDER BY LOCK_DATA"
printf 'begin;\ninsert into t values(9,9,9);\nselect "B-done";\n' >&4
sleep 0.8
echo "-- B 往源表插 (9,9,9)（应为空：被 S next-key lock 挡住）--"; cat "$DIR/b.out"
printf 'commit;\n' >&3
sleep 0.8
echo "-- A 提交后 B --"; cat "$DIR/b.out"
printf 'commit;\n' >&4
exec 3>&- 4>&-
wait 2>/dev/null

echo
echo "########## 场景 6：where 条件没索引的 update ##########"
reset_t
open_sess a 3
printf 'begin;\nupdate t set d=d+1 where d=5;\nselect "A-done";\n' >&3
sleep 0.7
echo "-- d 无索引，主键索引上逐行加锁 + supremum --"
q "SELECT INDEX_NAME, LOCK_MODE, LOCK_DATA FROM performance_schema.data_locks
   WHERE OBJECT_NAME='t' AND LOCK_TYPE='RECORD' ORDER BY LOCK_DATA"
echo "-- 记录锁行数 --"; countlocks
printf 'rollback;\n' >&3
exec 3>&-
wait 2>/dev/null

echo
echo "########## 场景 7：两阶段锁——冲突语句放最后，别人的等待从 1.84s 降到 0.07s ##########"
run_order() {
  local order=$1
  reset_t
  open_sess a 3
  open_sess b 4
  if [ "$order" = first ]; then
    printf 'begin;\nupdate t set d=d+1 where id=5;\nselect sleep(2);\nupdate t set d=d+1 where id=10;\ncommit;\n' >&3
  else
    printf 'begin;\nupdate t set d=d+1 where id=10;\nselect sleep(2);\nupdate t set d=d+1 where id=5;\ncommit;\n' >&3
  fi
  sleep 0.3
  local t0 t1
  t0=$(python3 -c 'import time;print(time.time())')
  printf 'begin;\nupdate t set d=d+1 where id=5;\nselect "B-done";\ncommit;\n' >&4
  for _ in $(seq 1 80); do grep -q B-done "$DIR/b.out" && break; sleep 0.05; done
  t1=$(python3 -c 'import time;print(time.time())')
  echo "-- [$order] B 的 update id=5 从发出到返回：$(python3 -c "print(f'{$t1-$t0:.2f}s')")"
  exec 3>&- 4>&-
  wait 2>/dev/null
}
echo "== A 先更新冲突行 id=5（冲突语句在前）=="; run_order first
echo "== A 最后才更新冲突行 id=5（冲突语句在后）=="; run_order last

echo
echo "== 清理：容器 $NAME 删除 =="
