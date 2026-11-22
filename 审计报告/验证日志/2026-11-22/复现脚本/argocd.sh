set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo "== argocd 版本"; argocd version --client --short
echo "== 取 admin 初始密码"; PW=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d); echo "pw_len=${#PW}"
pkill -f "port-forward svc/argocd-server" 2>/dev/null; sleep 1
nohup kubectl -n argocd port-forward svc/argocd-server 8080:443 --address 127.0.0.1 >/tmp/argocd-pf.log 2>&1 &
sleep 5
echo "== 登录"; argocd login 127.0.0.1:8080 --insecure --username admin --password "$PW" 2>&1 | tail -3
echo "== 创建 application（public git 仓库）"
argocd app create guestbook --repo https://github.com/argoproj/argocd-example-apps.git --path guestbook --dest-server https://kubernetes.default.svc --dest-namespace default --sync-policy none 2>&1 | tail -2
echo "== app list"; argocd app list -o name 2>&1 | head -3
sleep 8
echo "== 同步前状态"; argocd app get guestbook --output json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print("sync:",d["status"]["sync"]["status"],"health:",d["status"].get("health",{}).get("status"))'
echo "== app diff（前 20 行）"; argocd app diff guestbook 2>&1 | head -20
echo "== sync"; time argocd app sync guestbook --timeout 300 2>&1 | tail -12
echo "== 同步后状态"; sleep 5; argocd app get guestbook --output json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print("sync:",d["status"]["sync"]["status"],"health:",d["status"].get("health",{}).get("status"))'
echo "== 资源"; kubectl -n default get deploy,svc,pods 2>&1 | head -12
echo "== 历史"; argocd app history guestbook 2>&1 | head -5
echo "== 回滚到 1"; argocd app rollback guestbook 1 --timeout 120 2>&1 | tail -4
echo "== 回滚后状态"; argocd app get guestbook --output json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print("sync:",d["status"]["sync"]["status"],"health:",d["status"].get("health",{}).get("status"))'
