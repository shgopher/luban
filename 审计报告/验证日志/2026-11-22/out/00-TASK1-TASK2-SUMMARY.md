# 本次实测汇总（2026-11-22，macOS arm64 / Docker Desktop）

两个任务均在本机真实执行，日志原文见本目录编号文件。未修改 /Users/shgopher/Desktop/family/luban 下任何文件。

## 任务 1：关注关系 + Redis 收件箱 + Go 扩散 worker — 通过

- MySQL：8.0.46（Community Server），容器 mysql:8.0，宿主端口 13306
- Redis：7.4.12（镜像 redis:7-alpine；书稿写的 Redis 8 本机没有，照实记录）
- Go：go1.27.1 darwin/arm64；go-sql-driver/mysql v1.8.1；go-redis/v9 v9.7.0
- follow 表：唯一键 `uk_follower_followee(follower_id, followee_id)`，`deleted_at` 软删
- 首次插入 ROW_COUNT=1；重复插入真实报错 `ERROR 1062 (23000): Duplicate entry '1001-2002' for key 'follow.uk_follower_followee'`
- `INSERT IGNORE` 重复行 ROW_COUNT=0，表内仍 1 行；软删后 `ON DUPLICATE KEY UPDATE` ROW_COUNT=2，`deleted_at` 复活为 NULL
- 造数：2501 条有效关系（9001 有 2000 粉，9002 有 500 粉，另有 1001→2002 一条），seq_snapshot=200500
- ZSET：批量 ZADD 120 条全部返回 1；`ZCARD` 120 → `ZREMRANGEBYRANK inbox 0 -101` 删除 20 条 → `ZCARD` 100；剩余 u:21..u:120
- Go worker：page_size=100、限速 500 行/秒、收件箱上限 100、按页重试上限 3
  - followers_scanned=2501，diffused_rows=2501，pages=26，retries=0
  - rate_limit_wait_ms=4494，elapsed_ms=4611，avg_rows_per_sec=542.4
  - inbox:2002 1→1；inbox:9001 2000→100；inbox:9002 500→100；裁剪后 ZCARD 合计=201
- 日志：01-mysql-start.log、02-mysql-verify.log、05-redis.log、06-redis-zset.log、07-go-diffusion-worker.log
- 源码：/tmp/luban-verify/work-data/diffusion-worker/main.go（go.mod / go.sum 同目录）

## 任务 2：ShardingSphere-Proxy 5.5.0 分片 → 单表回退 — 通过

- Proxy：apache/shardingsphere-proxy:5.5.0（镜像 Created 2024-04-30，backend-core-5.5.0.jar），宿主端口 13307 → 容器 3307
- 后端 MySQL：8.0.46，物理表 t_order_0 / t_order_1 / t_order
- 分片规则：`t_order` → `ds_0.t_order_${0..1}`，`order_id % 2`
- 经 Proxy 插入 order_id 1..4：逻辑表 4 行 / 100.00；直连物理表 t_order_0=2 行/60.00（2,4），t_order_1=2 行/40.00（1,3），t_order=0
- DistSQL `DROP SHARDING TABLE RULE t_order` 本身 exit=0；再 `REFRESH TABLE METADATA` 后查询报
  `ERROR 10002 (42S02): Table or view 't_order' does not exist.`（说明只删规则不会自动回落单表）
- 可行回退：conf 改 `!SINGLE`（`tables: ds_0.t_order`，`defaultDataSource: ds_0`）→ 重启 Proxy
  - `SHOW SINGLE TABLES`：t_order / ds_0；`SHOW SHARDING TABLE RULES` 为空
  - 回退后 Proxy 查询 4 行 / 100.00；经 Proxy 写 order_id=6 后直连：t_order=5 行，t_order_0/t_order_1 仍各 2 行
- 环境坑：镜像不自带 MySQL JDBC 驱动，本次从 maven.aliyun.com 下载 mysql-connector-j-8.4.0.jar 挂载（非 docker pull）；
  本机 mysql 26.7.0 客户端连 Proxy 报 ERROR 2059（缺 mysql_native_password 插件），改用 mysql:8.0 容器内 8.0.46 客户端
- 日志：08~18 开头文件（12 启动、13 分片写入、14 DROP/REFRESH 失败、16 回退成功）
- 配置：/tmp/luban-verify/work-data/ss-proxy/conf/{global.yaml,database-sharding.yaml}

## 清理

docker rm -f luban-ss-proxy luban-mysql1（Redis 在任务 1 后已清理）

## 配置模板（回放用）

- 分片配置：/tmp/luban-verify/work-data/ss-proxy/templates/database-sharding.sharded.yaml
- 单表配置：/tmp/luban-verify/work-data/ss-proxy/templates/database-sharding.single.yaml
- 当前运行目录 conf/database-sharding.yaml 为单表版；回放分片时先 cp sharded 模板过去。
