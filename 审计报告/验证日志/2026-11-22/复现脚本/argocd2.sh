set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "== 切到另一个 revision 触发第二次 sync"
argocd app sync guestbook --revision e5f4ed89fae9d87d46b79a0ef691537d1ccaf2c4 --timeout 240 2>&1 | tail -6
sleep 3
echo "== 当前 revision"; argocd app get guestbook --output json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"]["sync"].get("revision","")[:12])'
echo "== history"; argocd app history guestbook 2>&1 | head -6
echo "== 回滚到上一个部署（id 0）"; argocd app rollback guestbook 0 --timeout 180 2>&1 | tail -4
sleep 3
echo "== 回滚后 revision"; argocd app get guestbook --output json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"]["sync"].get("revision","")[:12], d["status"]["sync"]["status"], d["status"].get("health",{}).get("status"))'
echo "== pod"; kubectl -n default get pods 2>&1 | head -3
echo "== 资源事件"; kubectl -n default describe pod 2>&1 | grep -A4 "Events:" | head -6
