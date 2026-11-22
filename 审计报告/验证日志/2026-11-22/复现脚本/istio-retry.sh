set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "== 容器名（单 Pod）"
kubectl -n shop get pod web-v1-67b9cbc799-tmdg6 -o jsonpath='{.spec.containers[*].name}{"\n"}'
kubectl -n shop get pod web-v1-67b9cbc799-tmdg6 -o jsonpath='{.status.containerStatuses[*].name}{"\n"}'
echo "== 部署 httpbin（用于重试实验）"
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
        image: hashicorp/httpbin:latest
        ports: [{containerPort: 80}]
---
apiVersion: v1
kind: Service
metadata: {name: httpbin, namespace: shop}
spec:
  selector: {app: httpbin}
  ports: [{port: 80, targetPort: 80, name: http}]
---
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata: {name: httpbin, namespace: shop}
spec:
  hosts: [httpbin]
  http:
  - route:
    - destination: {host: httpbin}
    retries:
      attempts: 2
      perTryTimeout: 2s
      retryOn: 5xx
    timeout: 10s
YAML
kubectl -n shop wait --for=condition=Ready pod -l app=httpbin --timeout=240s 2>&1 | tail -1
sleep 8
echo "== 一次客户端调用 /status/503"
kubectl -n shop exec client -- sh -c 'wget -qO- --server-response http://httpbin/status/503 2>&1 | head -3; echo "wget_exit=$?"' 2>&1 | tail -5
sleep 2
echo "== httpbin 侧收到的请求数（sidecar 日志中的 upstream 请求）"
kubectl -n shop logs -l app=httpbin -c istio-proxy --tail=200 2>/dev/null | grep -c "POST\|GET /status/503" || true
kubectl -n shop logs -l app=httpbin -c httpbin --tail=100 2>/dev/null | tail -6
