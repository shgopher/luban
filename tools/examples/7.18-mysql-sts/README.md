# 7.18 有状态应用实战：MySQL 8 主从（StatefulSet + Init + Sidecar）

对应书稿：[7.18 有状态应用实战：StatefulSet 跑 MySQL 主从](../../../v2/L0-运行时底座/7-容器编排/7.18-有状态应用实战-MySQL主从.md)。

课程原版用 `mysql:5.7` + `xtrabackup:2.4`。本示例是**可运行的现代替换**：

| 课程原版 | 本示例 | 原因 |
|---|---|---|
| `mysql:5.7` | `mysql:8.0`（实测 8.0.46） | 5.7 已 EOL |
| XtraBackup 物理备份 | `mysqldump --single-transaction --source-data=2` | 不依赖额外镜像；MySQL 8 自带 |
| `CHANGE MASTER TO` | 仍兼容（8.0.46 的 dump 也输出 `CHANGE MASTER TO`） | 8.0 未移除；8.4 才移除 |
| `hostname` 命令 | `$HOSTNAME` | `mysql:8.0` 镜像不含 `hostname` 二进制 |

## 跑法

前置：kind 集群、`mysql:8.0` 已导入节点。

```bash
kind create cluster --name mysql-sts
kind load docker-image mysql:8.0 --name mysql-sts

kubectl apply -f mysql-sts.yaml
kubectl -n mysql-verify rollout status statefulset/mysql --timeout=300s
```

> 如果 `kind load` 因多平台镜像报 `content digest ... not found`，用：
> `docker save mysql:8.0 -o /tmp/mysql.tar`
> `docker exec -i <cluster>-control-plane ctr -n k8s.io images import --platform linux/arm64 --snapshotter=overlayfs - < /tmp/mysql.tar`

## 期望输出（真实粘贴）

```text
$ kubectl -n mysql-verify get pods
NAME      READY   STATUS    RESTARTS   AGE
mysql-0   2/2     Running   0          2m
mysql-1   2/2     Running   0          1m
mysql-2   2/2     Running   0          1m

$ kubectl -n mysql-verify get pvc
NAME           STATUS   VOLUME     CAPACITY   ACCESS MODES   STORAGECLASS
data-mysql-0   Bound    pvc-...    1Gi        RWO            standard
data-mysql-1   Bound    pvc-...    1Gi        RWO            standard
data-mysql-2   Bound    pvc-...    1Gi        RWO            standard

$ kubectl -n mysql-verify exec mysql-1 -c mysql -- \
    mysql -uroot -e "SHOW REPLICA STATUS\G" | grep -E 'Source_Host|Replica_(IO|SQL)_Running'
                  Source_Host: mysql-0.mysql
           Replica_IO_Running: Yes
          Replica_SQL_Running: Yes
```

验证复制：

```bash
kubectl -n mysql-verify exec mysql-0 -c mysql -- \
  mysql -uroot -e "INSERT INTO test.messages (message) VALUES ('verified');"

kubectl -n mysql-verify exec mysql-1 -c mysql -- \
  mysql -uroot -e "SELECT * FROM test.messages;"
# 1  hello
# 2  verified
```

扩容同样有效（实测到 4 副本）：

```bash
kubectl -n mysql-verify scale statefulset mysql --replicas=4
kubectl -n mysql-verify exec mysql-3 -c mysql -- \
  mysql -uroot -e "SELECT @@server_id, @@super_read_only; SELECT * FROM test.messages;"
# 103  1
# 1  hello
# 2  verified
```

## 安全边界

这是**验证用最小示例**：`MYSQL_ALLOW_EMPTY_PASSWORD=1` + `MYSQL_ROOT_HOST=%`，root 空密码可从集群内任意 Pod 连接。生产必须改成 Secret 注入的强密码、禁用 root 远程、给复制专用账号最小权限，并开启 TLS 或 `caching_sha2_password` 的安全通道。

## 三个必须知道的坑（实测踩到）

1. **`mysqldump` 的 binlog 位点属于“被 dump 的节点”**。
   课程用 XtraBackup，`xtrabackup_slave_info` 记录的是**源主库**的位点，所以可以从上一个 Slave 备份、再指向 Master。
   换成 `mysqldump` 后，如果从 `mysql-1` dump，位点是 `mysql-1` 自己的 binlog；再让 `mysql-2` 指向 `mysql-0`，I/O 线程会失败。
   本示例让所有 Slave 都从 **Master** dump，最简单且正确；要“从上一个 Slave 备份”，需用 `--dump-slave` 或开启 `log_replica_updates` 并链式指向该 Slave。

2. **`mysql:8.0` 镜像没有 `hostname` 命令**。
   课程脚本里的 `` `hostname` `` 在 8.0 镜像里直接报 `command not found`；改用 `$HOSTNAME`，或读 `/etc/hostname`。

3. **默认 1 秒的探针超时会把 MySQL 打死**。
   在 3.8GB 的 Docker Desktop 里，负载稍高 `mysqladmin ping` 就超过 1s，liveness 连续失败 → 容器重启 → 复制中断。
   本示例把 `timeoutSeconds` 调到 5、`failureThreshold` 调到 6、`initialDelaySeconds` 调到 60。**探针不是越灵敏越好**。

## 清理

```bash
kubectl delete ns mysql-verify
kind delete cluster --name mysql-sts
```
