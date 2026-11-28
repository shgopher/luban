# 44.25-mysql-ops-lab：自增空洞、ID 到顶、权限表一致性

[44.25 MySQL 运维与故障处置](../../../v2/L4-组件与技术/44-数据库/44.25-MySQL运维与故障处置.md) 的配套实验。

实验内容：

1. 唯一键冲突、事务回滚、`INSERT IGNORE` 之后自增值为什么不回退；
2. `INSERT ... SELECT` 的 2 的幂预分配（插 4 行消耗 7 个 id）；
3. `INT UNSIGNED` 到顶后的真实报错；
4. `GRANT` 与 `FLUSH PRIVILEGES`：直接 DML `mysql.user` 才会造成内存/磁盘不一致。

## 跑法

依赖 Docker 与 `mysql` 客户端。默认端口 `33199`，容器用完即删：

```bash
cd tools/examples/44.25-mysql-ops-lab
bash code/run.sh
# 端口被占用时
PORT=34199 bash code/run.sh
```

首次要初始化数据目录，本机约 1–3 分钟；脚本等到 `select 1` 成功才继续。
`autoinc.sql` 里故意保留两条报错语句（唯一键冲突、id 到顶），脚本用 `mysql --force` 跑完，最后两条 `ERROR` 是预期结果。

## 实测输出（MySQL 8.0.46）

环境：

```text
+---------+---------+-----------------+------------+--------------+
| version | bp_mb   | iso             | binlog_fmt | autoinc_mode |
+---------+---------+-----------------+------------+--------------+
| 8.0.46  | 128.000 | REPEATABLE-READ | ROW        |            2 |
+---------+---------+-----------------+------------+--------------+
```

自增空洞（节选，`SHOW CREATE TABLE` 只保留 `AUTO_INCREMENT` 一行）：

```text
| 1. 初始：AUTO_INCREMENT=2 |
) ENGINE=InnoDB AUTO_INCREMENT=2 ...
ERROR 1062 (23000) at line 17: Duplicate entry '1' for key 't.c'
) ENGINE=InnoDB AUTO_INCREMENT=3 ...
| 4. INSERT IGNORE 也消耗一个 id：AUTO_INCREMENT=5 |
) ENGINE=InnoDB AUTO_INCREMENT=5 ...
| 5. 事务回滚后号不回退：AUTO_INCREMENT=7 |
) ENGINE=InnoDB AUTO_INCREMENT=7 ...
| 6. INSERT ... SELECT 插 4 行，按 1+2+4 预分配 7 个号 |
) ENGINE=InnoDB AUTO_INCREMENT=8 ...
| 7. int unsigned 到顶：第一条成功，AUTO_INCREMENT 仍是 4294967295 |
) ENGINE=InnoDB AUTO_INCREMENT=4294967295 ...
ERROR 1062 (23000) at line 58: Duplicate entry '4294967295' for key 'tmax.PRIMARY'
```

第 5 步的完整数据：回滚掉的是 id=6，下一行拿到 id=7，id=6 成为空洞。

权限表一致性：

```text
| 1. grant 之后无需 flush，磁盘和内存都已更新 |
| GRANT USAGE ON *.* TO `ub`@`%`               |
| GRANT SELECT ON `lab4425_perm`.* TO `ub`@`%` |
| 2. 磁盘行已删：此时 GRANT 失败（1410） |
ERROR 1410 (42000) at line 16: You are not allowed to create a user with GRANT
| 3. 此时 CREATE USER 也失败（1396，内存认为用户还在） |
ERROR 1396 (HY000) at line 19: Operation CREATE USER failed for 'ub'@'%'
| 4. flush 之后内存与磁盘一致，可以重新建用户 |
| GRANT USAGE ON *.* TO `ub`@`%`               |
| GRANT SELECT ON `lab4425_perm`.* TO `ub`@`%` |
```

## 模式 1 的补充实验

`innodb_autoinc_lock_mode` 是只读参数，不能 `SET GLOBAL` 动态改。要验证模式 1，用启动参数另起一个容器（脚本默认是 8.0 的默认值 2）：

```bash
docker run -d --rm --name mysqllab-4425-m1 \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p 127.0.0.1:34199:3306 mysql:8.0 --innodb-autoinc-lock-mode=1
```

本机在同一份 `autoinc.sql` 上验证：模式 1 与模式 2 的 `INSERT ... SELECT` 都给 `AUTO_INCREMENT=8`（1+2+4 预分配），多行 `VALUES` 都精确分配、不跳号。模式 2 的并发交错（别的会话插进这批 id 中间）本轮未做并发压测，机制见正文。

## 环境说明

- MySQL 8.0.46，`innodb_buffer_pool_size=128M`，RR 隔离，`binlog_format=ROW`；
- 镜像 tag 为 `mysql:8.0`，会随上游更新浮动，换版本后数字可能变化；
- 容器是一次性的，`run.sh` 退出时 `docker rm -f`，不会留下数据。
