export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n lab delete deploy cpu-burn --wait=false >/dev/null 2>&1
kubectl -n lab delete hpa cpu-burn --wait=false >/dev/null 2>&1
kubectl apply -f /tmp/canary.yaml
kubectl -n lab rollout status deploy/web-stable --timeout=120s
kubectl -n lab rollout status deploy/web-canary --timeout=120s
sleep 3
kubectl -n lab get deploy,svc -o wide
echo "=== endpoints ==="
kubectl -n lab get endpoints canary-demo -o wide
echo "=== endpoints yaml addresses ==="
kubectl -n lab get endpoints canary-demo -o jsonpath='{range .subsets[*].addresses[*]}{.ip}{" pod="}{.targetRef.name}{"\n"}{end}'
echo "=== ingressclass ==="
kubectl get ingressclass 2>&1
echo "=== 20 curl via NodePort 30080 ==="
for i in $(seq 1 20); do
  R=$(curl -s -m 3 http://127.0.0.1:30080/ 2>&1)
  echo "req$i: $R"
done | tee /tmp/curl20.txt
echo "=== tally ==="
grep -o 'stable-v1\|canary-v2' /tmp/curl20.txt | sort | uniq -c
