export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "=== supplementary: 100 curl (2 stable pods : 1 canary pod) ==="
for i in $(seq 1 100); do curl -s -m 3 http://127.0.0.1:30080/; echo; done > /tmp/curl100.txt
grep -o 'stable-v1\|canary-v2' /tmp/curl100.txt | sort | uniq -c
echo
echo "########## (a) self-heal: delete one pod ##########"
kubectl -n lab get pods -l app=rollback-demo -o wide
VICTIM=$(kubectl -n lab get pods -l app=rollback-demo -o jsonpath='{.items[0].metadata.name}')
echo "victim=$VICTIM"
T0=$(date +%s.%N); echo "DELETE_T0=$(date -Is) epoch=$T0"
kubectl -n lab delete pod "$VICTIM" --wait=false
for i in $(seq 1 240); do
  READY=$(kubectl -n lab get pods -l app=rollback-demo --no-headers 2>/dev/null | awk '$2=="1/1"{c++} END{print c+0}')
  NEW=$(kubectl -n lab get pods -l app=rollback-demo --no-headers 2>/dev/null | grep -c .)
  if [ "$READY" = "2" ] && [ "$NEW" = "2" ]; then
    EL=$(echo "$(date +%s.%N) - $T0" | bc)
    echo "SELF_HEAL_DONE after ${EL}s (ready=$READY total=$NEW)"
    break
  fi
  sleep 0.5
done
kubectl -n lab get pods -l app=rollback-demo -o wide
kubectl -n lab get events --sort-by=.lastTimestamp 2>/dev/null | tail -8
echo
echo "########## (b) break Service selector ##########"
kubectl -n lab get endpoints canary-demo
T1=$(date +%s.%N); echo "BREAK_T0=$(date -Is) epoch=$T1"
kubectl -n lab patch svc canary-demo -p '{"spec":{"selector":{"app":"does-not-exist"}}}'
echo "patch returned at +$(echo "$(date +%s.%N) - $T1" | bc)s"
for i in $(seq 1 15); do
  CODE=$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:30080/ 2>/dev/null); RC=$?
  echo "t+$(echo "$(date +%s.%N) - $T1"|bc)s curl_exit=$RC http_code=$CODE"
  [ "$CODE" = "000" ] && break
  sleep 1
done
kubectl -n lab get endpoints canary-demo 2>&1
kubectl -n lab get endpointslices -l kubernetes.io/service-name=canary-demo 2>&1
T2=$(date +%s.%N); echo "RESTORE_T0=$(date -Is) epoch=$T2"
kubectl -n lab patch svc canary-demo -p '{"spec":{"selector":{"app":"canary-demo"}}}'
for i in $(seq 1 120); do
  CODE=$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:30080/ 2>/dev/null)
  if [ "$CODE" = "200" ]; then
    EL=$(echo "$(date +%s.%N) - $T2" | bc)
    echo "RECOVERED after ${EL}s http_code=$CODE"
    break
  fi
  sleep 0.5
done
kubectl -n lab get endpoints canary-demo 2>&1
