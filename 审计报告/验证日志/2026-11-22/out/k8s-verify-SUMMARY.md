# 书稿 Kubernetes 案例实机验证报告

- 环境: 单节点 k3s `v1.37.2+k3s1`，节点 `decn2jol49vce` (189.24.91.75)，Ubuntu 24.04.1 LTS，内核 6.8.0-48-generic，containerd 2.3.7-k3s1，**1 vCPU / 1968 MiB**，kube-system 内 metrics-server、coredns 正常，**无 IngressClass（未装 Traefik）**。
- 所有资源建在 namespace `lab`，验证后已 `kubectl delete ns lab` 清理；kube-system 未做任何改动。
- 时间基准：远端 UTC（2026-11-22 08:34–08:41）。
- 日志目录: `/tmp/luban-verify/out/`，原始清单与脚本: `/tmp/luban-verify/out/k8s/`。

## 结果总览

| # | 项目 | 结论 | 关键实测数字 | 日志 |
|---|---|---|---|---|
| 1 | HPA 扩容 + `--dry-run=server` | 通过 | 1→3 副本，35 s（扩容决策 ~29 s）；TARGETS `398%/50%` | `1-hpa-expand.log`、`1-hpa-dryrun-server.log`、`1-hpa-precheck.log` |
| 2 | ConfigMap 卷更新延迟 | 通过 | 61.91 s 生效；subPath 150 s 仍未更新 | `2-configmap-delay.log` |
| 3 | 坏镜像发布 + `rollout undo` | 通过 | 20 s 仍卡住；undo 后 1.91 s 完成 | `3-rollback.log` |
| 4 | stable+canary + Service/Endpoints | 通过（按 Pod 数随机，非权重） | Endpoints 3 个（2 stable : 1 canary）；20 次 11/9，100 次 66/34 | `4-canary-endpoints.log`、`5-chaos.log` |
| 5 | 故障演练（自愈 / 依赖不可用） | 通过 | Pod 自愈 4.29 s；断依赖 0.27 s 失败、恢复 0.75 s | `5-chaos.log` |
| 0 | 环境与清理 | — | 1 vCPU / 1968 MiB / 852 MiB available | `0-env-and-cleanup.log` |

## 1) HPA

应用: busybox:1.36 死循环，requests cpu=50m mem=64Mi，limits cpu=200m mem=128Mi，replicas=1；HPA autoscaling/v2，min=1 max=3 target=50%。

```
t+0s    TARGETS cpu: <unknown>/50%   REPLICAS 1   (metrics 尚未就绪)
t+12s   top: 196m；TARGETS 仍 <unknown>
t+35s   TARGETS cpu: 398%/50%        Deployment 3/3 ready  → 达到 max 3
Event   SuccessfulRescale  New size: 3; reason: cpu resource utilization above target (~t+29s)
        ScalingLimited True / TooManyReplicas
```
初次 `<unknown>` 持续约 30 s（`no metrics returned from resource metrics API`、`pods might be unready`），属 metrics-server 采集窗口，不是故障。

server dry-run（书里没跑的那条）：
- `kubectl apply --dry-run=client -f hpa.yaml` → `horizontalpodautoscaler.autoscaling/order-api created (dry run)`（书里说本机无集群会报 localhost:8080 refused，在真集群上 client dry-run 正常）
- `kubectl apply --dry-run=server -f hpa.yaml`（与已存在对象完全一致）→ `horizontalpodautoscaler.autoscaling/order-api unchanged (server dry run)`
- 改 `maxReplicas: 40→30` 后再跑 → `horizontalpodautoscaler.autoscaling/order-api configured (server dry run)`，与书里预期文案一致

书稿可直接写：在 k3s v1.37.2 单节点（1C/1.9G）上，50m CPU request 的 busybox 死循环把 HPA 从 1 个副本扩到 maxReplicas=3 用了 35 秒，HPA 读数 398%/50%，事件 `SuccessfulRescale New size: 3`；`kubectl apply --dry-run=server -f hpa.yaml` 对已存在且无变化的对象返回 `unchanged (server dry run)`，改动字段后返回 `configured (server dry run)`。

## 2) ConfigMap 卷更新延迟

Pod 同时挂载：普通 configMap 卷 `/etc/app`，subPath 挂载 `/etc/sub/app.conf`。08:36:23 执行 `kubectl apply` 把 `version=1` 改为 `version=2`（apply 返回 +1.04 s）。

- 普通卷文件在 **t+61.91 s** 变为 `version=2`（每秒 `kubectl exec cat` 采样），逐秒日志见文件，变化前最后一条 t+58.42 s 仍为 version=1。
- subPath 文件在更新后继续观察 90 s（距 apply 约 150 s）仍为 `version=1`，不改。

书稿可直接写：在 k3s v1.37.2 单节点上实测，改 ConfigMap 到容器内普通卷文件变化耗时 61.9 秒（kubelet 1 分钟同步周期内），落在“最长约 2 分钟”内；同一 ConfigMap 用 subPath 挂载的文件 150 秒后仍是旧值，证实 subPath 不接收更新。

## 3) 回滚

Deployment `rollback-demo` nginx:alpine ×2。08:39:30 `kubectl set image ... nginx=nginx:1.27.0-doesnotexist`：

- 20 s 后：新 RS `5cbb6fbc46` 1 desired / 0 ready，Pod `ImagePullBackOff`；旧 RS `64dd44bc89` 2/2 Running；Deployment `2/2 AVAILABLE`（默认 maxUnavailable 25%，只起 1 个新 Pod，旧副本未停）。
- `kubectl rollout status --timeout=20s` 真实卡住：`Waiting for deployment ... 1 out of 2 new replicas have been updated...` → `error: timed out waiting for the condition`。
- `kubectl rollout undo` 返回 +0.80 s，**1.91 s 后** `rollout status` 成功 `deployment "rollback-demo" successfully rolled out`；旧 Pod 未被替换过，属“热回滚”。
- history：坏版本前 `REVISION 1`；undo 后显示 `2, 3`（undo 到 revision 1 会生成新 revision 3，模板相同的 1 被合并）。

书稿可直接写：坏镜像发布 20 秒后新 ReplicaSet 的 Pod 停在 ImagePullBackOff、旧 2 个副本仍 2/2 可用，`rollout status` 20 秒超时报 `error: timed out waiting for the condition`；`kubectl rollout undo` 后 1.9 秒完成恢复（旧 ReplicaSet 仍在，属热回滚，冷启动场景应计入拉镜像时间）。

## 4) 灰度 / 流量染色

`web-stable`（2 副本，返回 `stable-v1`）+ `web-canary`（1 副本，返回 `canary-v2`），共用 Service `canary-demo`（NodePort 30080，selector `app=canary-demo`）。

Endpoints（v1 Endpoints 已 deprecated，实际看 EndpointSlice 同结果）：
```
10.42.0.16:8080 pod=web-canary-5d5c5bb54b-x74nz
10.42.0.17:8080 pod=web-stable-66cd86cc9c-w2q88
10.42.0.18:8080 pod=web-stable-66cd86cc9c-bwmnb
```
curl 20 次：11 stable / 9 canary（小样本噪声大）；补充 100 次：66 stable / 34 canary ≈ 2:1，正好等于 Pod 数比例。`kubectl get ingressclass` → `No resources found`，无 Ingress，书里基于 Istio header 的灰度本机无法复现，Service 只能按 Pod 数随机、不能配置权重。

书稿可直接写：k3s 无 Ingress 时用 stable 2 副本 + canary 1 副本的 Service，`get endpoints` 能看到 3 个后端（2 个 stable、1 个 canary），100 次请求命中 66:34（≈2:1），比例由 Pod 数量决定而非 Service 权重；要用 5%/10% 这类精确比例必须有 Ingress/网格按 header 或权重路由。

## 5) 故障演练

(a) 自愈：08:41:06 删除 `rollback-demo` 的一个 Pod（Deployment 2 副本），新 Pod 4.29 s 后 Ready，事件序列 `Killing → SuccessfulCreate → Pulled(already present) → Created → Started`。
(b) 依赖不可用：08:41:11 patch Service selector 为 `app=does-not-exist`，patch 返回 +0.22 s，**0.27 s 后** curl exit 7（连接被拒，http_code=000），Endpoints 变 `<none>`；08:41:12 恢复 selector，**0.75 s 后** curl 返回 200，Endpoints 恢复 3 个。

书稿可直接写：k3s v1.37.2 单节点上删除 Deployment 的 1 个 Pod，4.3 秒后新 Pod Ready（镜像已在节点缓存）；把 Service selector 改成不匹配标签后 0.3 秒内请求即失败（curl exit 7）、Endpoints 为空，改回后 0.8 秒恢复正常，可作为“依赖不可用”演练的最小复现。

## 与书中预期不符 / 未跑通

1. 书里预期 server dry-run 输出 `configured (server dry run)`；对**完全一致**的清单实际返回 `unchanged (server dry run)`，只有带差异时才返回 `configured`——建议书里改文案。
2. 书中“`--dry-run=client` 需要 API server、本机报 localhost:8080 refused”的说法本次未复现：真集群上 client dry-run 正常返回 `created (dry run)`（本机无集群时的行为未再验证）。
3. 该 k3s 未安装 Traefik/IngressClass，书中 Istio `VirtualService` 按 header 灰度、Chaos Mesh CRD 均无对应组件，未执行；第 4、5 项用原生 Service/Deployment 等价复现。
4. 回滚 1.9 s 是热回滚（旧 RS 未缩容），不能代表“旧镜像需重新拉取”的生产冷回滚耗时。
5. 20 次请求的 11:9 与 2:1 的期望偏差较大，是样本量问题；100 次为 66:34，结论以 100 次为准。
6. HPA 前 ~30 秒 `TARGETS` 为 `<unknown>`（metrics-server 未采到新 Pod），书里若写“apply 即读指标”需补这个窗口。
