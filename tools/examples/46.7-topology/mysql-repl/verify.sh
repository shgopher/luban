#!/usr/bin/env bash
set -uo pipefail
export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=unix://$HOME/.docker/run/docker.sock
NET=luban-repl-net; P=db-primary; R=db-replica
M(){ docker exec "$1" mysql -h127.0.0.1 -uroot -prootpw "${@:2}"; }
cleanup(){ docker rm -f $P $R >/dev/null 2>&1; docker network rm $NET >/dev/null 2>&1; echo "[cleanup] containers removed"; }
trap cleanup EXIT
docker network create $NET >/dev/null || exit 1
docker run -d --name $P --network $NET -e MYSQL_ROOT_PASSWORD=rootpw -e MYSQL_DATABASE=app \
  mysql:8.0 --server-id=1 --log-bin=mysql-bin --binlog-format=ROW \
  --gtid-mode=ON --enforce-gtid-consistency=ON >/dev/null
docker run -d --name $R --network $NET -e MYSQL_ROOT_PASSWORD=rootpw \
  mysql:8.0 --server-id=2 --relay-log=relay-bin --read-only=ON \
  --gtid-mode=ON --enforce-gtid-consistency=ON >/dev/null
wait_tcp(){ for i in $(seq 1 120); do docker exec "$1" mysql -h127.0.0.1 -uroot -prootpw -Nse "SELECT 1" >/dev/null 2>&1 && return 0; sleep 1; done; return 1; }
wait_tcp $P && wait_tcp $R || { echo "not ready"; exit 1; }
echo "== 实例 =="
M $P -Nse "SELECT CONCAT('primary server_id=',@@server_id,' gtid=',@@gtid_mode,' binlog=',@@binlog_format,' sync_binlog=',@@sync_binlog)" 2>/dev/null
M $R -Nse "SELECT CONCAT('replica server_id=',@@server_id,' read_only=',@@read_only,' super_read_only=',@@super_read_only)" 2>/dev/null
M $R -e "CREATE DATABASE IF NOT EXISTS app" 2>/dev/null
M $P -e "CREATE USER 'repl'@'%' IDENTIFIED WITH mysql_native_password BY 'replpw'; GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%';" 2>/dev/null
M $R -e "CHANGE REPLICATION SOURCE TO SOURCE_HOST='$P',SOURCE_USER='repl',SOURCE_PASSWORD='replpw',SOURCE_AUTO_POSITION=1,GET_SOURCE_PUBLIC_KEY=1; START REPLICA;" 2>/dev/null
for i in $(seq 1 30); do
  st=$(M $R -e "SHOW REPLICA STATUS\G" 2>/dev/null | sed -n 's/^ *\(Replica_IO_Running\|Replica_SQL_Running\): //p' | tr '\n' '/')
  [ "$st" = "Yes/Yes/" ] && break; sleep 1
done
echo "== 复制链路 =="
M $R -e "SHOW REPLICA STATUS\G" 2>/dev/null | grep -E "Source_Host|Replica_IO_Running:|Replica_SQL_Running:|Auto_Position|Seconds_Behind_Source"
M $P app -e "CREATE TABLE orders(id BIGINT PRIMARY KEY AUTO_INCREMENT, amount DECIMAL(10,2), created_at DATETIME(6) NOT NULL DEFAULT NOW(6));" 2>/dev/null

echo "== A. 本机回环：写后立刻读从库 20 次（正常复制）=="
docker exec -i $P bash -s <<'SH'
q(){ mysql -h127.0.0.1 -uroot -prootpw "$@" 2>/dev/null; }
qr(){ mysql -hdb-replica -uroot -prootpw "$@" 2>/dev/null; }
miss=0
for i in $(seq 1 20); do
  id=$(q app -Nse "INSERT INTO orders(amount) VALUES ($i.01); SELECT LAST_INSERT_ID();")
  hit=$(qr app -Nse "SELECT COUNT(*) FROM orders WHERE id=$id")
  age=$(qr app -Nse "SELECT IFNULL(ROUND(TIMESTAMPDIFF(MICROSECOND,(SELECT created_at FROM orders WHERE id=$id),NOW(6))/1000,1),-1)")
  [ "$hit" = "0" ] && miss=$((miss+1))
  printf "  #%02d 可见=%s 读到的那一刻该行已存在 %.1f ms\n" "$i" "$hit" "$age"
done
echo "  → 20 次里写完立刻读不到的次数：$miss"
SH

echo "== B. 人为停掉回放线程：制造滞后，看「传输到了但看不见」=="
M $R -e "STOP REPLICA SQL_THREAD;" 2>/dev/null
docker exec -i $P bash -s <<'SH'
q(){ mysql -h127.0.0.1 -uroot -prootpw "$@" 2>/dev/null; }
qr(){ mysql -hdb-replica -uroot -prootpw "$@" 2>/dev/null; }
q app -e "INSERT INTO orders(amount) VALUES (1001.00),(1002.00),(1003.00),(1004.00),(1005.00);"
sleep 0.5
echo "  主库最新 5 条：$(q app -Nse "SELECT GROUP_CONCAT(amount) FROM (SELECT amount FROM orders WHERE amount>=1001 ORDER BY id) t")"
echo "  从库读到：      $(qr app -Nse "SELECT IFNULL(GROUP_CONCAT(amount),'(空)') FROM (SELECT amount FROM orders WHERE amount>=1001 ORDER BY id) t")"
echo "  从库 IO 线程：  $(qr -Nse "SELECT CONCAT('Retrieved=',SUBSTRING_INDEX(SUBSTRING_INDEX(@@gtid_executed,':',-1),',',-1))")"
echo "  --- 恢复回放 ---"
t0=$(date +%s%3N)
qr -e "START REPLICA SQL_THREAD;"
while :; do
  n=$(qr app -Nse "SELECT COUNT(*) FROM orders WHERE amount>=1001")
  [ "${n:-0}" -ge 5 ] && break; sleep 0.01
done
t1=$(date +%s%3N)
echo "  回放 5 条积压事务耗时：$((t1-t0)) ms，之后从库读到 $(qr app -Nse "SELECT COUNT(*) FROM orders WHERE amount>=1001") 条"
SH

echo "== C. 大事务：一次提交 20 万行，测后续提交的可见延迟 =="
M $P app -e "SET SESSION cte_max_recursion_depth=300000; INSERT INTO orders(amount) WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<200000) SELECT 88.88 FROM seq;" 2>/dev/null
M $P app -Nse "SELECT CONCAT('  主库行数=',COUNT(*)) FROM orders" 2>/dev/null
docker exec -i $P bash -s <<'SH'
q(){ mysql -h127.0.0.1 -uroot -prootpw "$@" 2>/dev/null; }
qr(){ mysql -hdb-replica -uroot -prootpw "$@" 2>/dev/null; }
t0=$(date +%s%3N)
q app -e "INSERT INTO orders(amount) VALUES (777.77);"
while :; do
  n=$(qr app -Nse "SELECT COUNT(*) FROM orders WHERE amount=777.77")
  [ "${n:-0}" -ge 1 ] && break
  sleep 0.02
done
t1=$(date +%s%3N)
echo "  大事务之后那条插入：commit → 从库可见 = $((t1-t0)) ms"
for i in 1 2 3 4 5 6; do
  echo "  Seconds_Behind_Source=$(qr -e 'SHOW REPLICA STATUS\G' | sed -n 's/^ *Seconds_Behind_Source: //p')"
  sleep 0.3
done
SH

echo "== D. read_only / super_read_only 挡不挡得住手写 =="
M $R app -e "INSERT INTO orders(amount) VALUES (1.00);" >/dev/null 2>&1 && echo "  read_only=ON：root 写入成功（照写不误）"
M $R -Nse "SELECT CONCAT('  从库多出的本地 GTID 段：',SUBSTRING_INDEX(@@gtid_executed,',',-1))" 2>/dev/null
M $R -e "SET GLOBAL super_read_only=ON;" 2>/dev/null
M $R app -e "INSERT INTO orders(amount) VALUES (2.00);" 2>&1 | grep -v Warning | sed 's/^/  super_read_only=ON：/'
M $R -e "SET GLOBAL super_read_only=OFF;" 2>/dev/null

echo "== E. 从库不是备份：主库 DROP TABLE，从库跟着没 =="
M $P app -e "CREATE TABLE t_drop(id INT PRIMARY KEY); INSERT INTO t_drop VALUES (1),(2);" 2>/dev/null
sleep 1
M $R app -Nse "SELECT CONCAT('  删表前从库行数=',COUNT(*)) FROM t_drop" 2>/dev/null
M $P app -e "DROP TABLE t_drop;" 2>/dev/null
sleep 1
M $R app -e "SELECT COUNT(*) FROM t_drop;" 2>&1 | grep -v Warning | sed 's/^/  删表后从库：/'

echo "== F. 切换演练：追平 + 提升 + 可写（数据侧 RTO）=="
docker exec -i $P bash -s <<'SH'
qr(){ mysql -hdb-replica -uroot -prootpw "$@" 2>/dev/null; }
t0=$(date +%s%3N)
qr -e "STOP REPLICA; RESET REPLICA ALL; SET GLOBAL super_read_only=OFF; SET GLOBAL read_only=OFF;"
qr -e "INSERT INTO app.orders(amount) VALUES (999.99);"
t1=$(date +%s%3N)
echo "  停止复制+清配置+解除只读+写入成功 = $((t1-t0)) ms"
qr app -Nse "SELECT CONCAT('  新主已可写，查到行数=',COUNT(*)) FROM orders WHERE amount=999.99"
SH
