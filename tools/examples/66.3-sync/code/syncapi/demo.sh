#!/bin/bash
# 同步 API 真实调用演示（服务端：code/syncapi/main.go，监听 127.0.0.1:18103）
set -u
BASE=http://127.0.0.1:18103
J() { python3 -m json.tool 2>/dev/null || cat; }

echo "=== 0) 健康检查 ==="
echo "\$ curl -s $BASE/health"
curl -s "$BASE/health"; echo

echo
echo "=== 1) 新设备首登：拉快照（snapshot_seq + 会话列表 + 每会话最近 3 条）==="
echo "\$ curl -s '$BASE/sync?device_id=t_phone&snapshot=1'"
curl -s "$BASE/sync?device_id=t_phone&snapshot=1" | J

echo
echo "=== 2) 服务端产生一条新消息（模拟对方发来）==="
echo "\$ curl -s '$BASE/send?conv=c1&sender=u1002&text=hello-sync'"
curl -s "$BASE/send?conv=c1&sender=u1002&text=hello-sync" | J
sleep 0.4

echo
echo "=== 3) 增量拉取：从上次游标 (change_cursor=120, c1:40,c2:40,c3:40) 开始 ==="
echo "\$ curl -s '$BASE/sync?device_id=t_phone&change_cursor=120&conv_cursors=c1:40,c2:40,c3:40'"
curl -s "$BASE/sync?device_id=t_phone&change_cursor=120&conv_cursors=c1:40,c2:40,c3:40" | J
sleep 0.4

echo
echo "=== 4) 已读位置 max 合并：先报 41（生效），再报 20（更旧，直接丢弃）==="
echo "\$ curl -s '$BASE/read?conv=c1&read_seq=41'"
curl -s "$BASE/read?conv=c1&read_seq=41" | J
echo "\$ curl -s '$BASE/read?conv=c1&read_seq=20'"
curl -s "$BASE/read?conv=c1&read_seq=20" | J
sleep 0.4

echo
echo "=== 5) 游标过期（超出 change 流水保留窗口）：返回 reset_required，客户端回退全量快照 ==="
echo "\$ curl -s '$BASE/sync?device_id=t_pc&change_cursor=3&conv_cursors=c1:0'"
curl -s "$BASE/sync?device_id=t_pc&change_cursor=3&conv_cursors=c1:0" | J
sleep 0.4

echo
echo "=== 6) 设备管理：列出设备 → 踢下线 → 被踢设备再同步返回 401 ==="
echo "\$ curl -s $BASE/devices"
curl -s "$BASE/devices" | J
echo "\$ curl -s '$BASE/kick?device_id=t_pad'"
curl -s "$BASE/kick?device_id=t_pad" | J
echo "\$ curl -s -w ' [HTTP %{http_code}]' '$BASE/sync?device_id=t_pad&change_cursor=0'"
curl -s -w ' [HTTP %{http_code}]' "$BASE/sync?device_id=t_pad&change_cursor=0"; echo
sleep 0.4

echo
echo "=== 7) 同步接口限流：同一设备 1 秒内 5 次请求，只放行 3 次 ==="
for i in 1 2 3 4 5; do
  code=$(curl -s -o /tmp/rl_body_$i -w '%{http_code}' "$BASE/sync?device_id=t_burst&change_cursor=120")
  echo "请求 $i => HTTP $code $(head -c 120 /tmp/rl_body_$i)"
done
rm -f /tmp/rl_body_*
