# 7.3 Service 调试三板斧：iptables 条数核对

对应书稿：[7.3 Service 与 Ingress](../../../v2/L0-运行时底座/7-容器编排/7.3-Service与Ingress.md) 的「Service 调试三板斧」。

## 跑法

前置：kind 集群（本文在 kind v1.31.0 / kube-proxy iptables 模式上跑过），`nginx:alpine` 已导入节点。

```bash
kind create cluster --name svc-verify
kind load docker-image nginx:alpine --name svc-verify

kubectl apply -f web.yaml
kubectl -n svc-verify rollout status deploy/web --timeout=120s

# 在节点容器里抓 nat 表
docker exec svc-verify-control-plane sh -c 'iptables-save -t nat' > /tmp/iptables-save.txt
```

## 期望输出（真实粘贴）

```text
$ kubectl -n svc-verify get pods
NAME                   READY   STATUS    RESTARTS   AGE
web-cbfbbd684-45r67    1/1     Running   0          4s
web-cbfbbd684-4rlzs    1/1     Running   0          4s
web-cbfbbd684-g4thd    1/1     Running   0          4s

$ kubectl -n svc-verify get endpointslices
NAME                         PORTS   ENDPOINTS
web-gtnjh                    80      10.244.0.6,10.244.0.7,10.244.0.5
web-np-c4462                 80      10.244.0.6,10.244.0.7,10.244.0.5
```

核对规则（`web` 为 ClusterIP，`web-np` 为 NodePort）：

```bash
# 1) 不同 KUBE-SEP 链的个数 = Endpoints 数（注意不是规则条数）
grep 'comment "svc-verify/web"' /tmp/iptables-save.txt \
  | grep -o 'KUBE-SEP-[A-Z0-9]*' | sort -u | wc -l
# 3

# 2) KUBE-SVC 里 -j KUBE-SEP-* 的规则数 = Endpoints 数
grep 'KUBE-SVC-.*comment "svc-verify/web ->' /tmp/iptables-save.txt \
  | grep -c -- '-j KUBE-SEP-'
# 3

# 3) 每个 KUBE-SEP 链里有 2 条规则：MARK-MASQ + DNAT
grep -c -- 'KUBE-SEP-.*comment "svc-verify/web"' /tmp/iptables-save.txt
# 6  ← 3 个 Endpoint × 2 条；不要把 6 当成 6 个后端
```

NodePort 额外两层：

```bash
grep -E 'KUBE-(NODEPORTS|EXT).*svc-verify/web-np' /tmp/iptables-save.txt
# KUBE-NODEPORTS 两条：localhost 与所有地址各一条
# KUBE-EXT-<hash>：做 MASQUERADE（SNAT）
```

## 结论

| 判据 | 正确写法 | 错误写法 |
|---|---|---|
| Endpoints 数 | 不同 `KUBE-SEP-*` **链**的个数 | 把 `KUBE-SEP-*` 规则条数当后端数（会翻倍） |
| 负载均衡 | `KUBE-SVC-*` 里 `-j KUBE-SEP-*` 的规则数 | 数整个 `KUBE-SVC` 链的行数（含 MARK-MASQ 等） |
| NodePort | 额外看 `KUBE-NODEPORTS` 与 `KUBE-EXT-*`/POSTROUTING | 只看 `KUBE-SERVICES` |

> 这条差异是本机实测发现的：书稿最初写“`KUBE-SEP-*` 条数 = Endpoints 数”，实际每个 SEP 链有 2 条规则，必须按“链”计数。

## 清理

```bash
kubectl delete ns svc-verify
kind delete cluster --name svc-verify
```
