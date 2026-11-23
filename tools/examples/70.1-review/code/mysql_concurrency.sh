#!/usr/bin/env bash
# 20 个并发连接打同一条审核回调：条件更新 / 唯一键去重，统计 affected rows 之和。
# 环境：MySQL 26.7.0 @ 127.0.0.1:13390，root 无密码；库 ch70_1 仅本实验使用。
set -u

BASE="$(cd "$(dirname "$0")" && pwd)"
OUT="$BASE/../raw"
CONC="$OUT/mysql_conc"
rm -rf "$CONC"
mkdir -p "$CONC"

SRV="mysql --protocol=tcp -h127.0.0.1 -P13390 -uroot"
DB="$SRV -N ch70_1"

$SRV <<'SQL'
CREATE DATABASE IF NOT EXISTS ch70_1 DEFAULT CHARSET utf8mb4;
USE ch70_1;
DROP TABLE IF EXISTS content_state;
CREATE TABLE content_state (
  content_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  status     VARCHAR(32) NOT NULL,
  visibility VARCHAR(16) NOT NULL,
  version    INT UNSIGNED NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB;
DROP TABLE IF EXISTS audit_callback_dedup;
CREATE TABLE audit_callback_dedup (
  audit_id   VARCHAR(64) NOT NULL PRIMARY KEY,
  content_id BIGINT UNSIGNED NOT NULL,
  result     VARCHAR(16) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB;
SQL

# 20 个客户端在同一时刻起跑：先忙等同一个 start 文件，再各自发一条 SQL。
run20() { # $1=输出目录  $2=SQL  $3=前缀
  local dir="$1"
  local sql="$2"
  local pre="$3"
  local start="$1/start_$pre"
  rm -f "$dir"/${pre}_*.txt "$start"
  mkdir -p "$dir"
  for i in $(seq 1 20); do
    (
      while [ ! -f "$start" ]; do :; done
      $DB -e "$sql" > "$dir/${pre}_$i.txt" 2>&1
    ) &
  done
  touch "$start"
  wait
  echo "  单请求 affected rows: $(cat "$dir"/${pre}_*.txt | tr '\n' ' ')"
  awk '{s+=$1} END {printf "  affected rows 之和 = %d\n", s}' "$dir"/${pre}_*.txt
}

echo "===== 实验 B-1：20 并发条件更新同一条内容（published → rejected） ====="
$DB -e "INSERT INTO content_state (content_id,status,visibility,version) VALUES (1,'published','public',0)
        ON DUPLICATE KEY UPDATE status='published',visibility='public',version=0;"
run20 "$CONC" "UPDATE content_state SET status='rejected', visibility='private', version=version+1
               WHERE content_id=1 AND status='published'; SELECT ROW_COUNT();" "cond"
$DB -e "SELECT content_id,status,visibility,version FROM content_state WHERE content_id=1;"

echo
echo "===== 实验 B-2（对照）：20 并发无条件 UPDATE（last-write-wins） ====="
$DB -e "UPDATE content_state SET status='published', visibility='public', version=0 WHERE content_id=1;"
run20 "$CONC" "UPDATE content_state SET status='rejected', visibility='private', version=version+1
               WHERE content_id=1; SELECT ROW_COUNT();" "uncond"
$DB -e "SELECT content_id,status,visibility,version FROM content_state WHERE content_id=1;"

echo
echo "===== 实验 B-3：20 并发唯一键插入同一条回调（同一 audit_id） ====="
$DB -e "TRUNCATE TABLE audit_callback_dedup;"
run20 "$CONC" "INSERT IGNORE INTO audit_callback_dedup (audit_id,content_id,result)
               VALUES ('audit#500',500,'reject'); SELECT ROW_COUNT();" "dup"
$DB -e "SELECT COUNT(*) AS rows_in_dedup FROM audit_callback_dedup;"
