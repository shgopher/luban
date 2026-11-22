#!/bin/bash
set -u
U=http://localhost:14242
T='*:*.unleash-insecure-admin-api-token'
echo "== 版本"
curl -s "$U/api/admin/features" -H "Authorization: $T" | head -c 200; echo
docker exec unleash cat /app/package.json 2>/dev/null | jq -r .version 2>/dev/null || true
echo "== 1) 创建开关（带 owner / 退出日期）"
curl -s -X POST "$U/api/admin/projects/default/features" -H "Authorization: $T" -H 'Content-Type: application/json' \
  -d '{"name":"checkout.v2","type":"release","description":"owner:team-order expires:2027-01-31"}' -w "\ncreate_http=%{http_code}\n" | tail -4
echo "== 2) 生产环境打开"
curl -s -X POST "$U/api/admin/projects/default/features/checkout.v2/environments/production/toggle" -H "Authorization: $T" -H 'Content-Type: application/json' \
  -d '{"enabled":true}' -w "\ntoggle_http=%{http_code}\n" | tail -3
echo "== 3) 客户端 API 求值（真实读开关）"
curl -s "$U/api/client/features" -H "Authorization: $T" | jq -c '.features[] | {name,enabled,type}'
echo "== 4) 关闭开关后再求值"
curl -s -X POST "$U/api/admin/projects/default/features/checkout.v2/environments/production/toggle" -H "Authorization: $T" -H 'Content-Type: application/json' \
  -d '{"enabled":false}' -w "\ntoggle_off_http=%{http_code}\n" | tail -2
curl -s "$U/api/client/features" -H "Authorization: $T" | jq -c '.features[] | {name,enabled}'
echo "== 5) 环境列表"
curl -s "$U/api/admin/environments" -H "Authorization: $T" | jq -c '.[]? | {name,type}'
