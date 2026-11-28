#!/usr/bin/env bash
# 44.27 示例：InnoDB 表空间、空间回收与在线 DDL
# 一条命令起 MySQL 8.0 → 灌数 → 跑实验 → 删容器
#   bash code/run.sh              # 默认端口 33199
#   PORT=34199 bash code/run.sh   # 端口被占时换一个
set -euo pipefail

PORT="${PORT:-33199}"
NAME="mysqlex-4427-${PORT}"
IMG="${IMG:-mysql:8.0}"

# macOS 上 docker 常常不在 PATH 里
if ! command -v docker >/dev/null 2>&1; then
  export PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH"
  export DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
fi

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

now_ms() {
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import time; print(int(time.time()*1000))'
  else
    echo $(( $(date +%s) * 1000 ))
  fi
}
elapsed() { echo "$(( $2 - $1 )) ms"; }

echo "== 0. 起容器 ($NAME, 端口 $PORT) =="
cleanup
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p "127.0.0.1:${PORT}:3306" "$IMG" \
  --innodb-buffer-pool-size=128M >/dev/null

# 用宿主机经 TCP 探活，且要求连续两次成功：
# mysql 镜像初始化阶段会先起一个临时实例，只有一次成功会被误判为"已就绪"
ok=0
for _ in $(seq 1 120); do
  if mysql -h127.0.0.1 -P"${PORT}" -uroot --protocol=TCP -e "SELECT @@version" >/dev/null 2>&1; then
    ok=$((ok + 1))
    [ "$ok" -ge 2 ] && break
  else
    ok=0
  fi
  sleep 1
done

M="mysql -h127.0.0.1 -P${PORT} -uroot --protocol=TCP"

echo
echo "== 1. 环境：版本 / 页大小 / 独立表空间 / online log 上限 =="
$M -t -e "SELECT @@version AS version, @@innodb_page_size AS page_size,
                 @@innodb_file_per_table AS file_per_table,
                 @@innodb_online_alter_log_max_size AS online_log_max,
                 @@innodb_sort_buffer_size AS sort_buffer,
                 @@secure_file_priv AS secure_file_priv;"

echo
echo "== 2. DELETE 一半：文件不缩；OPTIMIZE 之后才缩 =="
$M -t < code/01-space.sql
echo "-- TRUNCATE 之后容器里 .ibd 的字节数 --"
docker exec "$NAME" ls -l /var/lib/mysql/sp/ | grep -E 't\.ibd|total'

echo
echo "== 3. 插入顺序决定页数：随机插入的碎片有多少 =="
$M -t < code/05-fragment.sql

echo
echo "== 4. 在线 DDL：INSTANT / INPLACE / COPY 的成功与失败 =="
$M -t --force < code/02-ddl.sql || true

echo
echo "== 5. 分区表：每分区一个 .ibd、裁剪、DROP PARTITION =="
$M -t < code/03-partition.sql
echo "-- 每个分区一个独立表空间文件 --"
docker exec "$NAME" ls -l /var/lib/mysql/sp/ | grep 'pt#p' || echo "(p2023 已被 DROP PARTITION 删掉)"

echo
echo "== 6. 复制一张表：四条路，5 万行量级 =="
$M -t < code/04-copy.sql

echo "-- 路线一计时：CREATE TABLE LIKE + INSERT ... SELECT --"
$M -t -e "USE sp; DROP TABLE IF EXISTS dst_like; CREATE TABLE dst_like LIKE src;"
s=$(now_ms); $M -e "USE sp; INSERT INTO dst_like SELECT * FROM src;"; e=$(now_ms)
echo "耗时 $(elapsed "$s" "$e")，行数 $($M -N -e 'SELECT COUNT(*) FROM sp.dst_like')"

echo "-- SELECT ... INTO OUTFILE + LOAD DATA（在容器内执行）--"
$M -e "USE sp; DROP TABLE IF EXISTS dst_csv; CREATE TABLE dst_csv LIKE src;"
s=$(now_ms); docker exec "$NAME" mysql -uroot -e "SELECT * FROM sp.src INTO OUTFILE '/var/lib/mysql-files/src.csv'"; e=$(now_ms)
echo "导出耗时 $(elapsed "$s" "$e")"
s=$(now_ms); docker exec "$NAME" mysql -uroot -e "LOAD DATA INFILE '/var/lib/mysql-files/src.csv' INTO TABLE sp.dst_csv"; e=$(now_ms)
echo "导入耗时 $(elapsed "$s" "$e")，行数 $($M -N -e 'SELECT COUNT(*) FROM sp.dst_csv')"

if command -v mysqldump >/dev/null 2>&1; then
  echo "-- mysqldump | mysql --"
  $M -e "USE sp; DROP TABLE IF EXISTS dst_dump; CREATE TABLE dst_dump LIKE src;"
  s=$(now_ms)
  mysqldump -h127.0.0.1 -P"${PORT}" -uroot --protocol=TCP \
    --single-transaction --no-create-info --skip-disable-keys \
    --add-locks=0 --set-gtid-purged=off sp src 2>/dev/null \
    | sed 's/INSERT INTO `src`/INSERT INTO `dst_dump`/' | $M sp
  e=$(now_ms)
  echo "耗时 $(elapsed "$s" "$e")，行数 $($M -N -e 'SELECT COUNT(*) FROM sp.dst_dump')"
else
  echo "-- 本机没有 mysqldump，跳过该路线 --"
fi

echo "-- 可传输表空间：cp .ibd，物理拷贝 --"
$M -e "USE sp; DROP TABLE IF EXISTS dst_ts;"
s=$(now_ms)
docker exec -i "$NAME" mysql -uroot sp <<'SQL'
CREATE TABLE dst_ts LIKE src;
ALTER TABLE dst_ts DISCARD TABLESPACE;
FLUSH TABLE src FOR EXPORT;
system cp /var/lib/mysql/sp/src.cfg /var/lib/mysql/sp/dst_ts.cfg
system cp /var/lib/mysql/sp/src.ibd /var/lib/mysql/sp/dst_ts.ibd
system chown mysql:mysql /var/lib/mysql/sp/dst_ts.cfg /var/lib/mysql/sp/dst_ts.ibd
UNLOCK TABLES;
ALTER TABLE dst_ts IMPORT TABLESPACE;
SQL
e=$(now_ms)
echo "耗时 $(elapsed "$s" "$e")，行数 $($M -N -e 'SELECT COUNT(*) FROM sp.dst_ts')"
echo "-- import 不会删掉 .cfg，它会留在库目录里 --"
docker exec "$NAME" ls -l /var/lib/mysql/sp/ | grep 'dst_ts' || true

echo
echo "== 7. 共享表空间：innodb_file_per_table=OFF 时删表也不缩 ibdata1 =="
docker exec "$NAME" ls -l /var/lib/mysql/ibdata1
$M -e "SET GLOBAL innodb_file_per_table=off; USE sp;
       DROP TABLE IF EXISTS shared_t;
       CREATE TABLE shared_t (id INT PRIMARY KEY, pad CHAR(200));
       SET SESSION cte_max_recursion_depth=200000;
       INSERT INTO shared_t
       SELECT n, REPEAT('z',200) FROM
         (WITH RECURSIVE s AS (SELECT 1 n UNION ALL SELECT n+1 FROM s WHERE n<100000) SELECT n FROM s) x;"
echo "-- 灌进 10 万行后 --"
docker exec "$NAME" ls -l /var/lib/mysql/ibdata1
$M -e "USE sp; DROP TABLE shared_t;"
echo "-- DROP TABLE 之后 --"
docker exec "$NAME" ls -l /var/lib/mysql/ibdata1
$M -e "SET GLOBAL innodb_file_per_table=on;"

echo
echo "== 完成。容器 $NAME 随 trap 自动删除 =="
