set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n shop apply -f - <<'YAML'
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
      retryOn: "5xx"
    timeout: 10s
YAML
sleep 12
count() { kubectl -n shop logs -l app=httpbin -c httpbin --tail=800 2>/dev/null | grep -c "status/503"; }
B=$(count)
kubectl -n shop exec client -- sh -c 'wget -qO /dev/null http://httpbin/status/503; true' >/dev/null 2>&1
sleep 4
A=$(count)
echo "retries=2: 一次调用 -> 上游请求 $((A-B)) 次"
kubectl -n shop logs -l app=httpbin -c httpbin --tail=20 2>/dev/null | grep "status/503" | tail -3 | awk '{print $1}'
kubectl -n shop apply -f - <<'YAML'
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata: {name: httpbin, namespace: shop}
spec:
  hosts: [httpbin]
  http:
  - route:
    - destination: {host: httpbin}
    retries:
      attempts: 0
    timeout: 10s
YAML
sleep 12
B=$(count)
kubectl -n shop exec client -- sh -c 'wget -qO /dev/null http://httpbin/status/503; true' >/dev/null 2>&1
sleep 4
A=$(count)
echo "retries=0: 一次调用 -> 上游请求 $((A-B)) 次"
kubectl -n shop logs -l app=httpbin -c httpbin --tail=20 2>/dev/null | grep "status/503" | tail -2 | awk '{print $1}'
