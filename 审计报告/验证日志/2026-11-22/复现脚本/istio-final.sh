set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "===== A) 注入与 sidecar 形态 ====="
echo "-- namespace label:"; kubectl get ns shop -o jsonpath='{.metadata.labels.istio-injection}{"\n"}'
echo "-- 常规容器:"; kubectl -n shop get pod web-v1-67b9cbc799-tmdg6 -o jsonpath='{.spec.containers[*].name}{"\n"}'
echo "-- init 容器（native sidecar）:"; kubectl -n shop get pod web-v1-67b9cbc799-tmdg6 -o jsonpath='{range .spec.initContainers[*]}{.name}{" restartPolicy="}{.restartPolicy}{"\n"}{end}'
echo "-- READY:"; kubectl -n shop get pods -l app=web --no-headers 2>&1 | awk '{print $1, $2}'
echo "-- istioctl analyze:"; istioctl analyze -n shop 2>&1 | tail -2
echo "===== B) 重试策略 ====="
kubectl -n shop patch virtualservice httpbin --type merge -p '{"spec":{"http":[{"route":[{"destination":{"host":"httpbin"},"retries":{"attempts":2,"perTryTimeout":"2s","retryOn":"5xx"}}]}]}}' >/dev/null 2>&1
sleep 2
B=$(kubectl -n shop logs -l app=httpbin -c httpbin --tail=500 2>/dev/null | grep -c "status/503")
kubectl -n shop exec client -- sh -c 'wget -qO /dev/null http://httpbin/status/503; true' >/dev/null 2>&1
sleep 3
A=$(kubectl -n shop logs -l app=httpbin -c httpbin --tail=500 2>/dev/null | grep -c "status/503")
echo "retries=2 时，一次客户端调用产生的上游请求数 = $((A-B))"
kubectl -n shop patch virtualservice httpbin --type merge -p '{"spec":{"http":[{"route":[{"destination":{"host":"httpbin"},"retries":{"attempts":0}}]}]}}' >/dev/null 2>&1
sleep 2
B=$(kubectl -n shop logs -l app=httpbin -c httpbin --tail=500 2>/dev/null | grep -c "status/503")
kubectl -n shop exec client -- sh -c 'wget -qO /dev/null http://httpbin/status/503; true' >/dev/null 2>&1
sleep 3
A=$(kubectl -n shop logs -l app=httpbin -c httpbin --tail=500 2>/dev/null | grep -c "status/503")
echo "retries=0 时，一次客户端调用产生的上游请求数 = $((A-B))"
echo "-- 三条请求的时间戳（retries=2 那一次）"
kubectl -n shop logs -l app=httpbin -c httpbin --tail=500 2>/dev/null | grep "status/503" | tail -4 | awk '{print $1}'
echo "===== C) 可退：去掉注入标签并重启 ====="
kubectl label namespace shop istio-injection- >/dev/null 2>&1
kubectl -n shop rollout restart deployment web-v1 web-v2 >/dev/null 2>&1
sleep 40
echo "-- 重启后容器列表:"; kubectl -n shop get pods -l app=web --no-headers 2>&1 | awk '{print $1, $2}'
kubectl -n shop get pod -l app=web -o jsonpath='{range .items[*]}{.metadata.name}{" init="}{.spec.initContainers[*].name}{" containers="}{.spec.containers[*].name}{"\n"}{end}'
echo "===== D) 恢复注入标签 ====="
kubectl label namespace shop istio-injection=enabled --overwrite >/dev/null 2>&1
kubectl -n shop rollout restart deployment web-v1 web-v2 >/dev/null 2>&1
sleep 40
kubectl -n shop get pods -l app=web --no-headers 2>&1 | awk '{print $1, $2}'
kubectl -n shop get pod -l app=web -o jsonpath='{range .items[*]}{.metadata.name}{" init="}{.spec.initContainers[*].name}{"\n"}{end}'
