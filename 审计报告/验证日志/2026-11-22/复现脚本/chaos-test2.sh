set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n chaos-lab get pod web -o jsonpath='{.metadata.labels}{"\n"}'
measure() { kubectl -n chaos-lab exec client -- sh -c 'for i in $(seq 1 10); do curl -s -o /dev/null -w "%{time_total}\n" http://web/; done' 2>/dev/null; }
echo "== 基线（无注入）"; measure | awk '{s+=$1;n++} END{printf "requests=%d avg=%.3fs\n", n, s/n}'
cat <<'YAML' | kubectl apply -f - 2>&1 | tail -1
apiVersion: chaos-mesh.org/v1alpha1
kind: NetworkChaos
metadata: {name: web-delay, namespace: chaos-lab}
spec:
  action: delay
  mode: all
  selector:
    namespaces: [chaos-lab]
    labelSelectors: {run: web}
  delay: {latency: "200ms", jitter: "50ms", correlation: "100"}
  duration: "180s"
YAML
sleep 25
echo "== CR 状态"
kubectl -n chaos-lab get networkchaos web-delay -o jsonpath='{.status.conditions[*].type}={.status.conditions[*].status}{"\n"}' 2>/dev/null
kubectl -n chaos-lab get networkchaos 2>&1 | tail -2
echo "== 注入后"; measure | awk '{s+=$1;n++} END{printf "requests=%d avg=%.3fs\n", n, s/n}'
kubectl -n chaos-lab delete networkchaos web-delay >/dev/null 2>&1
sleep 10
echo "== 删除 CR 后"; measure | awk '{s+=$1;n++} END{printf "requests=%d avg=%.3fs\n", n, s/n}'
