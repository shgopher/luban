export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl apply -f /tmp/rollback-deploy.yaml
kubectl -n lab rollout status deploy/rollback-demo --timeout=120s
echo "=== rollout history (before) ==="
kubectl -n lab rollout history deploy/rollback-demo
sleep 3
T0=$(date +%s.%N); echo "SET_IMAGE_T0=$(date -Is) epoch=$T0"
kubectl -n lab set image deploy/rollback-demo nginx=nginx:1.27.0-doesnotexist
echo "set-image returned at +$(echo "$(date +%s.%N) - $T0" | bc)s"
sleep 20
echo "=== after bad image, t+$(( $(date +%s) - ${T0%.*} ))s ==="
kubectl -n lab get rs -o wide
kubectl -n lab get pods -o wide
kubectl -n lab get deploy rollback-demo
echo "=== rollout status with 20s timeout (expect hang/timeout) ==="
time kubectl -n lab rollout status deploy/rollback-demo --timeout=20s 2>&1
echo "=== old pods still serving? (direct pod IP via wget from busybox pod) ==="
kubectl -n lab get pods -l app=rollback-demo -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.status.podIP}{"\n"}{end}'
T1=$(date +%s.%N); echo "UNDO_T0=$(date -Is) epoch=$T1"
kubectl -n lab rollout undo deploy/rollback-demo
echo "undo returned at +$(echo "$(date +%s.%N) - $T1" | bc)s"
for i in $(seq 1 120); do
  if kubectl -n lab rollout status deploy/rollback-demo --timeout=2s >/tmp/rs.out 2>&1; then
    EL=$(echo "$(date +%s.%N) - $T1" | bc)
    echo "ROLLOUT_COMPLETE after ${EL}s"; cat /tmp/rs.out; break
  fi
  sleep 1
done
echo "=== after undo ==="
kubectl -n lab rollout history deploy/rollback-demo
kubectl -n lab get rs -o wide
kubectl -n lab get pods -o wide
kubectl -n lab get deploy rollback-demo
