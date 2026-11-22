export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "### apply HPA"
kubectl apply -f /tmp/hpa.yaml
T0=$(date +%s)
echo "T0=$(date -Is) epoch=$T0"
for i in $(seq 1 40); do
  EL=$(( $(date +%s) - T0 ))
  echo "----- t+${EL}s ($(date -Is)) -----"
  kubectl top pods -n lab 2>&1
  kubectl get hpa -n lab 2>&1
  kubectl get deploy -n lab -o wide 2>&1
  R=$(kubectl -n lab get deploy cpu-burn -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  echo "readyReplicas=${R:-0}"
  if [ "$R" = "3" ]; then echo "REACHED_MAX(3) at t+${EL}s"; break; fi
  sleep 10
done
echo "### describe hpa (tail)"
kubectl -n lab describe hpa cpu-burn 2>&1 | tail -25
