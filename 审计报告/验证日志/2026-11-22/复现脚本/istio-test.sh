set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "== 1) 四个动作之注入：Pod 内容器名"
kubectl -n shop get pod -o jsonpath='{range .items[*]}{.metadata.name}{": "}{.spec.containers[*].name}{"\n"}{end}'
echo "== istioctl analyze"
istioctl analyze -n shop 2>&1 | tail -4
echo "== proxy-status"
istioctl proxy-status 2>&1 | head -6
echo "== 2) 起客户端"
kubectl -n shop run client --image=busybox:1.36 --restart=Never -- sleep 3600 >/dev/null 2>&1
kubectl -n shop wait --for=condition=Ready pod/client --timeout=180s 2>&1 | tail -1
sleep 5
echo "== 3) 100 次请求（无 header，按 95/5 权重）"
kubectl -n shop exec client -- sh -c 'for i in $(seq 1 100); do wget -qO- http://web/ | grep "^Hostname:"; done' 2>/dev/null | awk '{print $2}' | sed 's/-[a-z0-9]*-[a-z0-9]*$//' | sort | uniq -c
echo "== 4) 10 次带 x-canary: true（应全落 v2）"
kubectl -n shop exec client -- sh -c 'for i in $(seq 1 10); do wget -qO- --header="x-canary: true" http://web/ | grep "^Hostname:"; done' 2>/dev/null | awk '{print $2}' | sed 's/-[a-z0-9]*-[a-z0-9]*$//' | sort | uniq -c
echo "== 5) 10 次带 x-canary: false（应落 v1/v2 权重）"
kubectl -n shop exec client -- sh -c 'for i in $(seq 1 10); do wget -qO- --header="x-canary: false" http://web/ | grep "^Hostname:"; done' 2>/dev/null | awk '{print $2}' | sed 's/-[a-z0-9]*-[a-z0-9]*$//' | sort | uniq -c
