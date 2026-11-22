export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl apply -f /tmp/cm-v1.yaml
kubectl apply -f /tmp/cm-pod.yaml
kubectl -n lab wait --for=condition=Ready pod/cm-demo --timeout=90s
echo "initial normal=$(kubectl -n lab exec cm-demo -- cat /etc/app/app.conf | tr '\n' '|')"
echo "initial subpath=$(kubectl -n lab exec cm-demo -- cat /etc/sub/app.conf | tr '\n' '|')"
sleep 5
T0=$(date +%s.%N); echo "T0=$T0 $(date -Is)"
kubectl apply -f /tmp/cm-v2.yaml
echo "apply-returned at +$(echo "$(date +%s.%N) - $T0" | bc)s"
UPD=""
for i in $(seq 1 200); do
  V=$(kubectl -n lab exec cm-demo -- cat /etc/app/app.conf 2>/dev/null | tr -d '\n')
  S=$(kubectl -n lab exec cm-demo -- cat /etc/sub/app.conf 2>/dev/null | tr -d '\n')
  EL=$(echo "$(date +%s.%N) - $T0" | bc)
  printf 't+%6.2fs normal=%-10s subpath=%s\n' "$EL" "$V" "$S"
  if [ "$V" = "version=2" ] && [ -z "$UPD" ]; then UPD=$EL; echo "NORMAL_UPDATED_AT=${EL}s"; break; fi
  sleep 1
done
echo "--- now watch subPath for another 90s (expect: unchanged) ---"
SD=$(date +%s)
for i in $(seq 1 9); do
  S=$(kubectl -n lab exec cm-demo -- cat /etc/sub/app.conf 2>/dev/null | tr -d '\n')
  echo "subpath t+$(( $(date +%s) - SD ))s after normal-update: $S"
  sleep 10
done
echo "final normal=$(kubectl -n lab exec cm-demo -- cat /etc/app/app.conf | tr '\n' '|')"
echo "final subpath=$(kubectl -n lab exec cm-demo -- cat /etc/sub/app.conf | tr '\n' '|')"
