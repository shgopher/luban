set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n shop delete deploy httpbin --ignore-not-found >/dev/null 2>&1
kubectl -n shop apply -f - <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata: {name: httpbin, namespace: shop}
spec:
  replicas: 1
  selector: {matchLabels: {app: httpbin}}
  template:
    metadata: {labels: {app: httpbin}}
    spec:
      containers:
      - name: httpbin
        image: ghcr.io/mccutchen/go-httpbin:v2.15.0
        ports: [{containerPort: 8080}]
YAML
kubectl -n shop patch svc httpbin -p '{"spec":{"ports":[{"port":80,"targetPort":8080,"name":"http"}]}}' >/dev/null 2>&1 || kubectl -n shop apply -f - <<'YAML'
apiVersion: v1
kind: Service
metadata: {name: httpbin, namespace: shop}
spec:
  selector: {app: httpbin}
  ports: [{port: 80, targetPort: 8080, name: http}]
YAML
kubectl -n shop wait --for=condition=Ready pod -l app=httpbin --timeout=300s 2>&1 | tail -1
sleep 8
echo "== 一次客户端调用 /status/503（VirtualService: attempts 2, retryOn 5xx）"
kubectl -n shop exec client -- sh -c 'wget -qO /dev/null --server-response http://httpbin/status/503 2>&1 | head -2; echo wget_done' 2>&1 | tail -3
sleep 3
echo "== httpbin 入站 sidecar 收到的 /status/503 次数（= 1 次初始 + 2 次重试 应为 3）"
kubectl -n shop logs -l app=httpbin -c istio-proxy --tail=300 2>/dev/null | grep -c "status/503" || true
echo "== 明细"
kubectl -n shop logs -l app=httpbin -c istio-proxy --tail=300 2>/dev/null | grep "status/503" | tail -5
echo "== 对照：改 attempts=0（关闭重试）再调一次"
kubectl -n shop patch virtualservice httpbin --type merge -p '{"spec":{"http":[{"route":[{"destination":{"host":"httpbin"},"retries":{"attempts":0}}]}]}}' >/dev/null 2>&1
sleep 2
kubectl -n shop exec client -- sh -c 'wget -qO /dev/null http://httpbin/status/503; echo wget_done' 2>&1 | tail -1
sleep 3
echo "== 关闭重试后新增次数（应为 1）"
kubectl -n shop logs -l app=httpbin -c istio-proxy --tail=300 2>/dev/null | grep -c "status/503" || true
kubectl -n shop logs -l app=httpbin -c istio-proxy --tail=300 2>/dev/null | grep "status/503" | tail -3
