# 第 11 章 部署形态：从一台机器到 K8s

> **本章一句话**：部署形态不是「先进程度」的排名，而是「你的运维能力」和「业务规模」的匹配；选错的代价是每天多花两小时救火。

**读完你会：**
- 用一张表选出现在最合适的部署形态
- 写出一个能上生产的 Dockerfile
- 配齐 K8s 最小可用的 7 个对象
- 把探针、资源、弹性、配置这四件事一次做对

---

## 11.1 五种部署形态怎么选

**结论先行**：年营收千万以下、团队十人以下，大概率只需要「云主机 + Docker Compose」；服务超过 10 个、需要弹性、有专职运维，才轮到 K8s。

| 形态 | 上手成本 | 弹性 | 隔离性 | 运维负担 | 典型场景 |
|---|---|---|---|---|---|
| 物理机 | 高 | 无 | 最强 | 最重 | 数据库、大数据、GPU |
| 云主机 + systemd | 低 | 手动 | 强 | 中 | 单体、内部系统、早期产品 |
| Docker Compose | 低 | 手动 | 中 | 中 | 10 个服务以内的中小项目 |
| Kubernetes | 高 | 自动 | 中 | 重（或买托管） | 多服务、多团队、需要弹性 |
| Serverless / FaaS | 低 | 全自动 | 弱 | 最轻 | 事件驱动、定时任务、流量稀疏 |

选型只看四个问题：

| 你的情况 | 选什么 |
|---|---|
| 1–3 个服务，流量平稳，没有运维 | 云主机 + Docker Compose |
| 5–20 个服务，有 1 个运维，需要滚动发布 | 托管 K8s（ACK / TKE / EKS） |
| 流量波峰波谷差 10 倍以上 | K8s + HPA，或 Serverless |
| 每天请求不到 1 万次、事件驱动 | Serverless，别自己养机器 |

> 🔑 **部署形态的第一原则：你能维护的，才是你能用的。** 自建 K8s 的隐性成本是「一个专职的人 + 一套监控 + 一套升级流程」，这三样没有，K8s 会变成新的故障源。

---

## 11.2 什么时候不该上 K8s

**结论先行**：以下四种情况，上 K8s 是负收益。

| 情况 | 原因 | 更好的选择 |
|---|---|---|
| 团队 < 5 人、服务 < 10 个 | 编排收益抵不过学习与维护成本 | 云主机 + Compose + 托管数据库 |
| 没有专职运维 / SRE | 控制面故障、证书过期、节点驱逐没人处理 | 托管 K8s，或干脆不上 |
| 单体应用、流量稳定 | K8s 解决的调度问题你根本没有 | 2 台云主机 + 负载均衡 |
| 需要强隔离 / 特殊内核 / GPU 直通 | 容器共享内核，隔离弱于虚机 | 虚机或裸金属 |

反过来，出现这些信号说明该上了：

- 服务数量 > 10，手工部署开始出错；
- 需要按流量自动扩缩容；
- 多环境配置漂移严重；
- 发布要停机，业务不能接受；
- 有多个团队共用一套机器，需要资源配额。

---

## 11.3 容器化：一个能上生产的 Dockerfile

**结论先行**：Go 服务用「多阶段构建 + 静态编译 + distroless」三件套，镜像从 800MB 降到 20MB 以内，攻击面同时变小。

```dockerfile
# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
WORKDIR /src

# 先拷依赖清单，利用层缓存：代码变了不用重新下载依赖
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# 静态编译：不依赖 libc，才能放进 distroless/scratch
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot
# 需要时区数据就加这一行；distroless 默认没有 tzdata
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
```

配套的 `.dockerignore`，少一行就多传几百 MB 上下文：

```text
.git
.venv*
*.md
*.pdf
tmp/
dist/
```

验证三件事：

```bash
docker build -t order-api:$(git rev-parse --short HEAD) .
docker images order-api --format '{{.Repository}}:{{.Tag}} {{.Size}}'   # 预期 20MB 上下
docker run --rm -p 8080:8080 order-api:$(git rev-parse --short HEAD)     # 能起来
```

### 坑

| 坑 | 后果 | 正确做法 |
|---|---|---|
| 用 `FROM golang` 做运行镜像 | 镜像 800MB+，漏洞扫出一堆 | 多阶段构建，运行阶段只留二进制 |
| `CGO_ENABLED=1` 放进 distroless | 启动报 `no such file or directory` | 静态编译，或换 debian 基础镜像 |
| 用 `latest` 标签 | 回滚时不知道回哪个版本 | tag 用 git sha，禁止 latest 上生产 |
| 容器里用 root 跑 | 逃逸风险、只读挂载失败 | `USER nonroot`，`securityContext` 限制 |
| 没设时区 | 日志时间差 8 小时，排查全乱 | 拷 tzdata 或统一用 UTC |

---

## 11.4 K8s 最小可用清单

**结论先行**：一个能上生产的服务，最少需要 7 类对象；少一个，都会在某个深夜还债。

| 对象 | 作用 | 少了会怎样 |
|---|---|---|
| Namespace | 环境/团队隔离 | 测试把生产配额吃光 |
| Deployment | 管理副本与滚动更新 | 无法声明式发布、无法回滚 |
| Service | 集群内固定访问入口 | Pod IP 一变就失联 |
| Ingress | 集群外 HTTP 入口 | 只能 NodePort，难做域名与 TLS |
| ConfigMap | 非敏感配置 | 配置写死在镜像里 |
| Secret | 敏感配置 | 密码进 Git |
| HPA | 自动扩缩容 | 流量来了手动扩容，来不及 |

建议再加两个，成本很低：**PDB**（自愿中断保护，节点维护时不断服务）和 **NetworkPolicy**（默认拒绝，只放行必要流量）。

```bash
kubectl -n prod get deploy,svc,ingress,hpa,pdb
kubectl -n prod rollout status deploy/order-api --timeout=120s
kubectl -n prod rollout history deploy/order-api
```

---

## 11.5 探针：两个探针别配反

**结论先行**：`readinessProbe` 决定「要不要给它流量」，`livenessProbe` 决定「要不要重启它」。配反的后果是：依赖一抖，全线重启。

| 探针 | 回答的问题 | 失败动作 | 检查内容 |
|---|---|---|---|
| startupProbe | 启动完了吗 | 继续等，超过阈值才重启 | 端口监听、初始化完成 |
| readinessProbe | 能接流量吗 | 从 Service 摘除，不重启 | 自身健康 + 必要依赖 |
| livenessProbe | 还活着吗 | 重启容器 | 只查自身，**绝不查下游** |

```yaml
startupProbe:            # 慢启动服务（如加载大模型、预热缓存）必须配
  httpGet: { path: /healthz, port: 8080 }
  failureThreshold: 30
  periodSeconds: 2       # 最多给 60 秒启动
readinessProbe:          # 依赖没就绪就不接流量，但不重启
  httpGet: { path: /readyz, port: 8080 }
  periodSeconds: 5
  failureThreshold: 3
livenessProbe:           # 只查进程自身，别查数据库
  httpGet: { path: /healthz, port: 8080 }
  periodSeconds: 10
  failureThreshold: 3
  timeoutSeconds: 2
```

⚠️ **最常见的配错**：把数据库 ping 写进 `livenessProbe`。数据库抖 30 秒 → 所有 Pod 被判死 → 全部重启 → 冷启动又把数据库打挂。**liveness 只查自己。**

---

## 11.6 资源与 QoS：request/limit 怎么设

**结论先行**：`requests` 按 P95 实际用量设，`limits` 按请求的 1.5–2 倍设；CPU limit 设太低会被节流，内存 limit 设太低会被 OOMKill。

| 资源 | requests | limits | 设错的后果 |
|---|---|---|---|
| CPU | P95 用量，如 200m | 1–2 倍，如 500m–1 | 太低 → 持续 throttling，P99 飙升 |
| 内存 | P95 用量，如 256Mi | 1.5 倍，如 512Mi | 太低 → OOMKilled，反复重启 |

QoS 三档，决定了节点资源紧张时谁先被杀：

| QoS | 条件 | 被驱逐优先级 |
|---|---|---|
| Guaranteed | 所有容器 request = limit | 最后 |
| Burstable | request < limit | 中间 |
| BestEffort | 完全没设 | 最先 |

```yaml
resources:
  requests: { cpu: 200m, memory: 256Mi }
  limits:   { cpu: "1",  memory: 512Mi }
```

Go 服务额外两件事：

```yaml
env:
  - name: GOMAXPROCS          # 别让 Go 以为有 64 核
    valueFrom: { resourceFieldRef: { resource: limits.cpu } }
  - name: GOMEMLIMIT          # 给 GC 一个上限，避免顶到 cgroup 上限被 OOMKill
    value: "400MiB"
```

> 💡 经验值：`GOMEMLIMIT` 取内存 limit 的 75%–80%，给运行时和突发留余量。

---

## 11.7 弹性伸缩：HPA 与扩容指标

**结论先行**：HPA 默认按 CPU 扩容，但 CPU 往往滞后；对 IO 密集型服务，按 QPS 或队列长度扩容更准。

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: order-api, namespace: prod }
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: order-api }
  minReplicas: 3
  maxReplicas: 20
  metrics:
    - type: Resource
      resource: { name: cpu, target: { type: Utilization, averageUtilization: 70 } }
    - type: Pods
      pods:
        metric: { name: http_requests_per_second }
        target: { type: AverageValue, averageValue: "300" }
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300   # 缩容等 5 分钟，防抖动
```

三个必须知道的限制：

1. **HPA 扩的是 Pod，不是节点**：节点满了要 Cluster Autoscaler，节点扩容通常 1–3 分钟。
2. **冷启动决定弹性上限**：Go 服务启动 2 秒没问题；要预热缓存的服务，扩出来也接不住。
3. **扩容指标要能反映瓶颈**：CPU 打不满但连接池满了，按 CPU 扩没用。

---

## 11.8 多环境：dev / staging / prod

**结论先行**：环境差异只允许存在于「配置」和「数据」，不允许存在于「镜像」和「部署方式」。

| 环境 | 目的 | 数据 | 规格 | 谁能发 |
|---|---|---|---|---|
| dev | 开发自测 | 假数据 | 1 副本 | 开发 |
| staging | 发布前验证 | 脱敏生产数据 | 生产 1/2 | 开发/测试 |
| prod | 真实用户 | 生产数据 | 生产规格 | 审批后 |

用 Kustomize 管差异，比复制三份 YAML 可靠：

```text
deploy/
  base/
    deployment.yaml
    service.yaml
    kustomization.yaml
  overlays/
    dev/kustomization.yaml
    prod/kustomization.yaml
```

```yaml
# overlays/prod/kustomization.yaml
resources: [../../base]
namespace: prod
images:
  - name: registry.example.com/order-api
    newTag: 9f3c2a1
patches:
  - target: { kind: Deployment, name: order-api }
    patch: |-
      - op: replace
        path: /spec/replicas
        value: 6
```

```bash
kubectl apply -k deploy/overlays/prod
kubectl diff -k deploy/overlays/prod    # 发之前先看差异
```

---

## 11.9 配置与密钥：别写进镜像

**结论先行**：配置走 ConfigMap / 配置中心，密钥走 Secret Manager；K8s 的 Secret 只是 base64，不是加密。

| 类型 | 放哪 | 变更方式 | 注意 |
|---|---|---|---|
| 普通配置 | ConfigMap / Nacos / Apollo | 热更新或重启 | 变更要审计 |
| 密钥 | Vault / KMS / External Secrets | 轮换 | 不要进 Git、不要进镜像 |
| 启动参数 | 环境变量 | 重启 | 别塞几十个变量 |

```yaml
envFrom:
  - configMapRef: { name: order-api-config }
  - secretRef:    { name: order-api-secret }
```

检查有没有密钥泄露，两条命令：

```bash
gitleaks detect --source . --verbose          # 扫 Git 历史
kubectl -n prod get secret order-api-secret -o jsonpath='{.data}' | head -c 200
```

生产建议：用 External Secrets Operator 把 Vault/KMS 的密钥同步成 K8s Secret，Pod 里只引用 Secret 名字，轮换由外部系统负责。

---

## 11.10 一份完整的 deployment.yaml

**结论先行**：下面这份可以直接改镜像名和资源值上线，覆盖了探针、优雅停机、安全上下文、拓扑打散、PDB 五件事。

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: order-api
  namespace: prod
  labels: { app: order-api }
spec:
  replicas: 3
  revisionHistoryLimit: 5
  strategy:
    type: RollingUpdate
    rollingUpdate: { maxSurge: 1, maxUnavailable: 0 }
  selector:
    matchLabels: { app: order-api }
  template:
    metadata:
      labels: { app: order-api }
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      terminationGracePeriodSeconds: 30
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
        seccompProfile: { type: RuntimeDefault }
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels: { app: order-api }
      containers:
        - name: app
          image: registry.example.com/order-api:9f3c2a1
          imagePullPolicy: IfNotPresent
          ports: [{ containerPort: 8080, name: http }]
          envFrom:
            - configMapRef: { name: order-api-config }
            - secretRef:    { name: order-api-secret }
          env:
            - name: GOMAXPROCS
              valueFrom: { resourceFieldRef: { resource: limits.cpu } }
            - name: GOMEMLIMIT
              value: "400MiB"
          resources:
            requests: { cpu: 200m, memory: 256Mi }
            limits:   { cpu: "1",  memory: 512Mi }
          startupProbe:
            httpGet: { path: /healthz, port: http }
            failureThreshold: 30
            periodSeconds: 2
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            periodSeconds: 5
            failureThreshold: 3
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 10
            failureThreshold: 3
            timeoutSeconds: 2
          lifecycle:
            preStop:
              exec: { command: ["sleep", "5"] }   # 等 Endpoint 摘除传播完
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: ["ALL"] }
---
apiVersion: v1
kind: Service
metadata: { name: order-api, namespace: prod }
spec:
  selector: { app: order-api }
  ports: [{ name: http, port: 80, targetPort: http }]
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: { name: order-api, namespace: prod }
spec:
  minAvailable: 2
  selector:
    matchLabels: { app: order-api }
```

上线验证顺序：

```bash
kubectl apply -f deployment.yaml
kubectl -n prod rollout status deploy/order-api --timeout=180s
kubectl -n prod get pods -l app=order-api -o wide
kubectl -n prod logs deploy/order-api --tail=50
```

---

## 本章自检

**Q1** 团队 6 个人、8 个服务、没有专职运维，现在该不该自建 K8s？为什么？
▸ 答案：不该。自建 K8s 的控制面维护、证书轮换、节点升级需要专职人力；8 个服务用托管 K8s 或云主机 + Compose 足够，等出现弹性或多团队配额需求再迁移。

**Q2** 数据库抖动 30 秒，为什么不能把这个检查放进 livenessProbe？
▸ 答案：liveness 失败会重启容器，所有 Pod 同时被重启，冷启动又放大数据库压力，形成雪崩。liveness 只查进程自身；依赖检查放 readiness，摘流量即可。

**Q3** HPA 把 Pod 从 3 扩到 20，为什么接口还是超时？
▸ 答案：三种可能：节点没有资源，Pod 一直 Pending；服务冷启动慢，扩出来还没预热；瓶颈不在 CPU（连接池/数据库），扩容指标选错了。
