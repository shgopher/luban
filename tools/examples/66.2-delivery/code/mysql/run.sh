#!/bin/bash
# 66.2 MySQL 幂等验证：唯一索引 vs 先查再插（并发抢插同一 client_msg_id）
MYSQL="mysql -S /tmp/mysql-ch66-2/mysql.sock -uroot"
$MYSQL -e "DROP DATABASE IF EXISTS im66; CREATE DATABASE im66;"
$MYSQL im66 < mysql/schema.sql

echo "--- D1 正常落库（首次上行）"
$MYSQL im66 -e "INSERT INTO message(conv_id,conv_seq,msg_id,sender_id,client_msg_id,body) VALUES (66,1,'m-1',1001,'cmid-000001','hello'); SELECT LAST_INSERT_ID() AS new_id;"

echo "--- D2 同一 client_msg_id 重发：唯一索引报 1062"
$MYSQL im66 -e "INSERT INTO message(conv_id,conv_seq,msg_id,sender_id,client_msg_id,body) VALUES (66,1,'m-1',1001,'cmid-000001','hello');" 2>&1 | tail -2

echo "--- D2b 服务端已分配新 seq 才发现是重复：uk_sender_cmid 单独报 1062"
$MYSQL im66 -e "INSERT INTO message(conv_id,conv_seq,msg_id,sender_id,client_msg_id,body) VALUES (66,2,'m-9',1001,'cmid-000001','hello')" 2>&1 | tail -2

echo "--- D3 服务端「重发也返回同一条 msg_id」的写法"
$MYSQL im66 -e "INSERT INTO message(conv_id,conv_seq,msg_id,sender_id,client_msg_id,body) VALUES (66,1,'m-1',1001,'cmid-000001','hello') ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id); SELECT LAST_INSERT_ID() AS same_id;"

echo "--- D4 20 个并发客户端抢插同一个 client_msg_id：唯一索引下只落 1 行"
rm -f /tmp/race_exits.txt
for i in $(seq 1 20); do
  ( $MYSQL im66 -e "INSERT INTO message(conv_id,conv_seq,msg_id,sender_id,client_msg_id,body) VALUES (67,1,'m-2',1001,'cmid-000002','race')" >/dev/null 2>&1; echo $? >> /tmp/race_exits.txt ) &
done
wait
$MYSQL im66 -e "SELECT COUNT(*) AS rows_for_cmid2 FROM message WHERE sender_id=1001 AND client_msg_id='cmid-000002';"
echo -n "并发退出码统计（0=插入成功, 1=被挡住）: "; sort /tmp/race_exits.txt | uniq -c | tr '\n' ' '; echo
rm -f /tmp/race_exits.txt

echo "--- D5 反例：没有唯一索引、用「先查再插」，20 个并发客户端会插出多少行？"
$MYSQL im66 -e "CREATE TABLE message_noidx (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, sender_id BIGINT UNSIGNED NOT NULL, client_msg_id VARCHAR(64) NOT NULL, body VARCHAR(64) NOT NULL) ENGINE=InnoDB;"
for i in $(seq 1 20); do
  # 经典 TOCTOU：先非锁定读判断「不存在」，再插入；SLEEP 模拟校验与插入之间的业务处理耗时
  ( $MYSQL im66 -e "SET @c=(SELECT COUNT(*) FROM message_noidx WHERE sender_id=1001 AND client_msg_id='cmid-000003'); SELECT SLEEP(0.2); INSERT INTO message_noidx(sender_id,client_msg_id,body) SELECT 1001,'cmid-000003','race' FROM DUAL WHERE @c=0;" >/dev/null 2>&1 ) &
done
wait
$MYSQL im66 -e "SELECT COUNT(*) AS duplicate_rows FROM message_noidx WHERE sender_id=1001 AND client_msg_id='cmid-000003';"

echo "--- D6 离线分页：索引扫描而不是全表扫"
$MYSQL im66 -e "EXPLAIN SELECT id,conv_seq,msg_id,body FROM message WHERE conv_id=66 AND conv_seq > 0 ORDER BY conv_seq LIMIT 200;"

echo "--- D7 状态机 CAS：UPDATE ... WHERE id=? AND status<? 的 row_count 才是竞争裁决"
$MYSQL im66 -e "UPDATE message SET status=1 WHERE id=1 AND status<1; SELECT ROW_COUNT() AS first_win; UPDATE message SET status=1 WHERE id=1 AND status<1; SELECT ROW_COUNT() AS second_lose;"

echo "--- D8 服务端版本与隔离级别"
$MYSQL -e "SELECT VERSION() AS version, @@transaction_isolation AS isolation, @@innodb_buffer_pool_size AS buffer_pool;"
