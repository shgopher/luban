set -u
export PATH=$PATH:/root/.dapr/bin
echo "== 1) dapr list"
dapr list 2>&1 | head -6
echo "== 2) 组件（默认 statestore 指向 localhost:6379）"
cat ~/.dapr/components/statestore.yaml
echo "== 3) state API: 写读删"
curl -s -o /dev/null -w "save_http=%{http_code}\n" -X POST localhost:3500/v1.0/state/statestore -H 'Content-Type: application/json' -d '[{"key":"foo","value":{"n":1,"s":"bar"}}]'
echo -n "get: "; curl -s localhost:3500/v1.0/state/statestore/foo; echo
curl -s -o /dev/null -w "delete_http=%{http_code}\n" -X DELETE localhost:3500/v1.0/state/statestore/foo
echo -n "get_after_delete_http="; curl -s -o /dev/null -w "%{http_code}\n" localhost:3500/v1.0/state/statestore/foo
echo "== 4) service invocation: order -> inventory（经 sidecar）"
curl -s -w "\ninvoke_http=%{http_code}\n" localhost:3500/v1.0/invoke/inventory/method/reserve
echo "== 5) metadata"
curl -s localhost:3500/v1.0/metadata | python3 -c 'import json,sys; d=json.load(sys.stdin); print("app_id:",d.get("id"),"| runtime:",d.get("runtimeVersion"),"| components:",[c["name"] for c in d.get("components",[])])'
echo "== 6) 可退：停 order sidecar"
dapr stop --app-id order 2>&1 | tail -2
sleep 3
echo -n "order sidecar 端口状态: "; curl -s -o /dev/null -w "%{http_code}\n" --max-time 3 localhost:3500/v1.0/metadata || echo "connection refused"
echo "== 7) inventory 仍在"
curl -s -o /dev/null -w "inventory_metadata_http=%{http_code}\n" --max-time 3 localhost:3501/v1.0/metadata
echo "== 8) 运行中的容器"
docker ps --format "{{.Names}} {{.Image}}" | head -6
