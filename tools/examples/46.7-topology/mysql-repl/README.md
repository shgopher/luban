# mysql-repl：一主一从的四个真实结论

[46.7 数据库的架构定位与部署拓扑选型](../../../../v2/L4-组件与技术/46-数据架构/46.7-数据库架构定位与部署拓扑选型.md) 的实测脚本：在 Docker 里起 MySQL 8.0 一主一从，验证四件在架构评审里常被说错的事。

## 前置

```bash
export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=unix://$HOME/.docker/run/docker.sock
docker images | grep mysql          # 需要本地有 mysql:8.0（本机实测 8.0.46）
```

脚本自己建网络、起容器、跑完删干净（`trap cleanup EXIT`），不需要预先准备。

## 跑起来

```bash
cd tools/examples/46.7-topology/mysql-repl
./verify.sh
```

## 验证的四件事与真实输出

```text
== A. 本机回环：写后立刻读从库 20 次（正常复制）==
  #01 可见=1 读到的那一刻该行已存在 34.5 ms
  ...
  #20 可见=1 读到的那一刻该行已存在 24.1 ms
  → 20 次里写完立刻读不到的次数：0

== B. 人为停掉回放线程：制造滞后，看「传输到了但看不见」==
  主库最新 5 条：1001.00,1002.00,1003.00,1004.00,1005.00
  从库读到：      (空)
  从库 IO 线程：  Retrieved=1-6
  回放 5 条积压事务耗时：55 ms，之后从库读到 5 条

== C. 大事务：一次提交 20 万行，测后续提交的可见延迟 ==
  大事务之后那条插入：commit → 从库可见 = 782 ms
  Seconds_Behind_Source=1
  Seconds_Behind_Source=0
  ...

== D. read_only / super_read_only 挡不挡得住手写 ==
  read_only=ON：root 写入成功（照写不误）
  从库多出的本地 GTID 段：\ncf1a8d16-...:1-7
  super_read_only=ON：ERROR 1290 (HY000) at line 1: The MySQL server is running with the --super-read-only option so it cannot execute this statement

== E. 从库不是备份：主库 DROP TABLE，从库跟着没 ==
  删表前从库行数=2
  删表后从库：ERROR 1146 (42S02) at line 1: Table 'app.t_drop' doesn't exist

== F. 切换演练：追平 + 提升 + 可写（数据侧 RTO）==
  停止复制+清配置+解除只读+写入成功 = 43 ms
  新主已可写，查到行数=1
```

四条结论：

1. **同机房读己之写复现不出来**：延迟小于一次客户端建连开销（20 次全命中），所以这类问题会被联调掩盖到上线。
2. **“传输到了” ≠ “读得到”**：停掉回放线程后 IO 线程仍在收日志，但应用读到空。
3. **`Seconds_Behind_Source` 不能判断读一致性**：782 ms 的陈旧窗口，指标只显示 1 秒然后归 0。
4. **`read_only` 挡不住 SUPER 账号**：root 照写，从库 `gtid_executed` 出现主库没有的段；`super_read_only=ON` 才拒绝。

## 数字会漂

- 24–35 ms 是**行龄**（含一次客户端建连），不是复制延迟；本机回环的真实回放延迟远小于它。换机器、换负载，这个区间会变。
- 782 ms 与 55 ms 随机器负载、InnoDB 配置（`innodb_flush_log_at_trx_commit`、`sync_binlog`）浮动，看量级不看小数。
- 43 ms 是**数据侧**提升耗时，不含故障检测与应用重连；真实 RTO 通常由那两段决定。
- 容器如果拉不到镜像，脚本会在 `docker run` 处失败；先把 `mysql:8.0` 拉到本地。

## 踩过的坑

- **等就绪必须用 TCP**：初始化阶段入口脚本会起一个 `--skip-networking` 的临时实例，只用 socket 探测会连到它，此时下发的 `CHANGE REPLICATION SOURCE` 会随临时实例一起消失，之后 `Replica_IO_Running` 一直是 `Connecting`。
- 从库要先 `CREATE DATABASE IF NOT EXISTS app`，否则在复制启动前查 `app.*` 会报 `ERROR 1049`。
- 脚本里 `docker exec` 一定要带 `-i`，否则 heredoc 不会送进容器，整段静默不执行。
