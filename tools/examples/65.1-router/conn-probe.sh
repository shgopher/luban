#!/bin/sh
# 连接数上限与每连接代价实测（容器内执行）
# $1 = VmRSS / RssAnon / RssShmem：求和口径会重复计入共享内存，所以分开看
mem() {
  for d in /proc/[0-9]*; do
    [ -r "$d/comm" ] || continue
    if [ "$(cat $d/comm 2>/dev/null)" = "postgres" ]; then
      awk -v k="$1" '$1==k":"{print $2}' "$d/status" 2>/dev/null
    fi
  done | awk '{s+=$1} END{printf "%d", s+0}'
}
q() { psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -tAc "$1"; }

q "DROP ROLE IF EXISTS app" >/dev/null 2>&1
q "CREATE ROLE app LOGIN PASSWORD 'app'" >/dev/null 2>&1
q "select set_config('x','1',false)" >/dev/null 2>&1  # 预热一次连接

BASE=$(mem VmRSS); BASE_A=$(mem RssAnon); BASE_S=$(mem RssShmem)
echo "基线：postgres 进程总 RSS=${BASE} KB，当前连接数=$(q 'select count(*) from pg_stat_activity')，max_connections=$(q 'show max_connections')"

# 90 条常驻空闲连接（模拟“每租户 N 条连接”的池化后果）
for i in $(seq 1 90); do
  PGPASSWORD=app psql -h 127.0.0.1 -p 5432 -U app -d postgres -tAc 'select pg_sleep(20)' >/dev/null 2>&1 &
done
sleep 3
MID=$(mem VmRSS); MID_A=$(mem RssAnon); MID_S=$(mem RssShmem)
N=$(q "select count(*) from pg_stat_activity where usename='app'")
echo "打开 90 条应用连接后：VmRSS 合计=${MID} KB（含共享内存重复计入），RssAnon 合计=${MID_A} KB，RssShmem 合计=${MID_S} KB，app 连接数=${N}"
echo "单连接私有内存 ≈ $(( (MID_A-BASE_A)/90 )) KB（RssAnon 增量 / 90）"

# 在此基础上再申请 130 条：观察撞墙位置与报错原文
rm -f /tmp/ok /tmp/err /tmp/sample_err
for i in $(seq 1 130); do
  (
    if PGPASSWORD=app psql -h 127.0.0.1 -p 5432 -U app -d postgres -tAc 'select pg_sleep(4)' >/dev/null 2>/tmp/e$i; then
      echo ok >>/tmp/ok
    else
      echo err >>/tmp/err
      cat /tmp/e$i >/tmp/sample_err 2>/dev/null
    fi
  ) &
done
wait
echo "再申请 130 条连接：成功 $(wc -l </tmp/ok 2>/dev/null || echo 0) 条，失败 $(wc -l </tmp/err 2>/dev/null || echo 0) 条"
echo "失败报错原文：$(head -1 /tmp/sample_err 2>/dev/null)"
echo "此刻连接数：$(q 'select count(*) from pg_stat_activity')"
sleep 3
echo "连接释放后：VmRSS=$(mem VmRSS) KB，RssAnon=$(mem RssAnon) KB"

echo "===== 同一份 information_schema 查询，换成无授权的 app 角色 ====="
PGPASSWORD=app psql -h 127.0.0.1 -p 5432 -U app -d postgres -tAc "\timing on" -c "select count(*) from information_schema.tables" 2>&1 | tail -3
echo "--- 超级用户 postgres 口径 ---"
psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -tAc "select count(*) from information_schema.tables" 2>&1 | tail -2
