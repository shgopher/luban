# 第 6 章 数据层：架构师最常背锅的地方

> **本章一句话**：表、索引、事务、连接池、备份，五个决定都在方案评审时定，别等出事再补。

**读完你会：**
- 给出一张扛得住三年增长的表结构和索引
- 用 EXPLAIN 三列判断慢查询该加索引、改 SQL，还是归档数据
- 算出连接池、分库分表、备份恢复的具体数字，而不是"看情况"

---

## 6.1 表设计十条规则

**结论先行**：表结构定错了，后面所有优化都在给错误付利息。十条规则直接照抄，评审时逐条打勾。

| # | 规则 | 怎么写 | 不这么做会怎样 |
|---|---|---|---|
| 1 | 命名 | 小写 + 下划线 + 复数：`orders`、`order_items` | `order` 是保留字，每次都要反引号 |
| 2 | 主键 | 必须有，且与业务无关 | 拿手机号当主键，换号就得改全表 |
| 3 | 金额 | `DECIMAL(12,2)`，单位元；汇率用 `(18,6)` | 用 DOUBLE 求和，实测得 `0.30000000000000004` |
| 4 | 状态 | `TINYINT UNSIGNED` + 注释 + 代码常量 | VARCHAR 存状态，`paid` / `PAID` 各存一份 |
| 5 | 时间 | `created_at`、`updated_at`，`DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` | 缺 `updated_at`，增量同步没抓手 |
| 6 | 软删除 | 默认硬删；要审计才加 `deleted_at` | 唯一索引失效、查询到处漏过滤条件 |
| 7 | 空值 | `NOT NULL` + 默认值：`''`、`0`、固定时间 | NULL 让 `COUNT`、`!=`、唯一索引统计失真 |
| 8 | 字符集 | `utf8mb4` + `utf8mb4_0900_ai_ci`（8.0 默认） | 三字节 utf8 存不了 emoji，昵称直接报错 |
| 9 | 语义 | 不用 NULL 表达业务状态，用明确值（`0`=待支付） | 报表里 NULL 和 0 混为一谈，对账对不上 |
| 10 | 宽度 | 单表 ≤ 50 列；大文本、JSON 拆扩展表 | 单行太大，全表扫描更慢，缓存也装不下 |

主键三种做法，选一个就定死三年：

| 方案 | 适合 | 代价 |
|---|---|---|
| 自增 BIGINT | 单库单表、写入分散 | 分库后不唯一；ID 暴露业务量 |
| 雪花 ID | 分库分表、要全局唯一且趋势递增 | 依赖机器号分配，时钟回拨要处理 |
| UUID | 对外暴露、不可猜的标识 | 36 字节，随机写入分散，索引膨胀 2–4 倍 |

90% 的单库业务用自增 BIGINT 就够。只有确定要分库、或 ID 要对外时才上雪花。

软删除的隐性代价：唯一索引 `(user_id, phone)` 在删除后仍占位，新用户注册同一手机号会冲突。解法是把唯一键改成 `(phone, deleted_at)`，未删除统一写 `'1970-01-01 00:00:00'`。

## 6.2 索引：三个必会

**结论先行**：联合索引按"等值在前、范围在后、排序跟上"排。三个必会：最左前缀、覆盖索引、回表。

200 万行级别的表上建一条联合索引（实测数据量 20 万行）：

```sql
ALTER TABLE orders ADD INDEX idx_user_status_created (user_id, status, created_at),
  ALGORITHM=INPLACE, LOCK=NONE;
```

最左前缀：索引 `(user_id, status, created_at)` 到底能用几列，看 `key_len` 最快。bigint 8 字节 + tinyint 1 字节 + datetime 5 字节。

| 查询条件 | 用到的列 | type | rows | key_len |
|---|---|---|---|---|
| `user_id = 1002` | user_id | ref | 40 | 8 |
| `user_id = 1002 AND status = 1` | 前两列 | ref | 13 | 9 |
| `user_id = 1002 AND created_at > '2026-01-01'` | 只有 user_id | ref | 40 | 8 |
| `status = 1` | 跳扫（8.0.13+） | range | 66542 | 9 |
| `created_at > '2026-06-01'` | 全索引扫 | index | 199626 | 14 |

第三行是关键：`status` 夹在中间，`created_at` 只能当过滤条件，不能定位。第四行是 8.0 的"跳扫"：看着用了索引，`rows` 六万多，等于没加。

覆盖索引：查询的列全在索引里，不需要回表。

```sql
EXPLAIN FORMAT=TRADITIONAL
SELECT user_id, status, created_at FROM orders
WHERE user_id = 1002 AND status = 1 ORDER BY created_at;
-- Extra: Using index
```

回表：`SELECT *` 拿不到 `Using index`，40 行就要按主键再读 40 次；扫 10 万行就是 10 万次随机 IO。

```sql
EXPLAIN FORMAT=TRADITIONAL SELECT * FROM orders WHERE user_id = 1002;
-- Extra: NULL（没有 Using index）
```

💡 列表页只取需要的列，不是因为"省带宽"，是因为每多回表一次就多一次随机 IO。

## 6.3 慢查询：先看执行计划

**结论先行**：EXPLAIN 只看 `type`、`rows`、`Extra` 三列。三列都安全才算过关；不安全就先加索引，再改 SQL，最后才归档数据。

运营后台"查某状态最近订单"，20 万行表的真实输出：

```sql
EXPLAIN FORMAT=TRADITIONAL
SELECT id, user_id, amount, created_at FROM orders
WHERE status = 1 AND created_at >= '2026-01-01'
ORDER BY created_at DESC LIMIT 20;
```

```text
type | key  | rows   | filtered | Extra
ALL  | NULL | 199626 |    3.33  | Using where; Using filesort
```

加一条 `(status, created_at)`：

```sql
ALTER TABLE orders ADD INDEX idx_status_created (status, created_at),
  ALGORITHM=INPLACE, LOCK=NONE;
```

```text
type  | key                | rows  | filtered | Extra
range | idx_status_created | 14886 |   100.00 | Using index condition; Backward index scan
```

`Using filesort` 消失，因为排序键在索引里连续，取前 20 行就能停。

| 列 | 安全值 | 危险信号 | 处置 |
|---|---|---|---|
| `type` | `const`、`eq_ref`、`ref`、`range` | `ALL`（全表）、`index`（全索引扫） | 补索引或改写条件 |
| `rows` | 几十到几千 | 超过 10 万，或超返回行数 100 倍 | 加更精确的索引；分页改游标 |
| `Extra` | `Using index`、`Using index condition` | `Using filesort`、`Using temporary` | 把排序、分组字段放进索引 |

`Using temporary` 的典型来源：边分组边按聚合值排序。

```sql
EXPLAIN FORMAT=TRADITIONAL
SELECT user_id, COUNT(*) c FROM orders
WHERE created_at >= '2026-01-01' GROUP BY user_id ORDER BY c DESC LIMIT 10;
-- Extra: Using index; Using temporary; Using filesort
```

这种查询不要放线上接口，走离线或预聚合。深分页同理：`LIMIT 100000, 20` 要扫 10 万行再丢掉，改成 `WHERE id > 上次最大 id ORDER BY id LIMIT 20`。

## 6.4 事务与隔离级别：只会用到这三档

**结论先行**：线上只有三档——单语句自动提交、RC、RR。MySQL 默认 RR；互联网 OLTP 多数改 RC；只有依赖"同事务内多次读一致"或防并发插入时才留 RR。

| 维度 | RC（读已提交） | RR（可重复读，默认） |
|---|---|---|
| 同事务两次读 | 可能读到别人新提交的数据 | 结果一致 |
| 范围查询加锁 | 基本不加间隙锁 | 加间隙锁 |
| 并发插入 | 不被挡 | 被挡（实测报 1205） |
| 死锁概率 | 低 | 高，尤其范围更新和批量插入 |
| 适合 | 高并发写不同行、批量导入 | 对账、库存"读-判断-写" |

间隙锁实测，会话 A 在 RR 下锁一个不存在的 ID：

```sql
BEGIN;
SELECT id FROM orders WHERE id = 999999 FOR UPDATE;  -- 行不存在，锁住间隙
```

会话 B 两秒后插入该区间：

```text
ERROR 1205 (HY000): Lock wait timeout exceeded; try restarting transaction
```

同样的两条语句，在 RC 下会话 B 插入直接成功（实测）。所以"先查再插"的防重，RR 靠间隙锁能挡住并发，RC 挡不住，必须靠唯一索引。

什么时候用 RC：高并发、更新分散、批量插入多，能明显减少锁等待。前提是 `binlog_format=ROW`（8.0 默认），statement 格式下 RC 会导致主从不一致。切换方式：

```sql
SET PERSIST transaction_isolation = 'READ-COMMITTED';  -- 重启后仍生效
```

什么时候必须 RR：同一事务里"读余额 → 判断 → 扣减"，需要两次读一致；或者没有唯一索引、暂时靠间隙锁兜"查不到就插入"。不改默认值也行，代价是把所有事务压到 100ms 以内。

### 坑

| 坑 | 后果 | 正确做法 |
|---|---|---|
| 事务里调 HTTP / RPC | 连接和锁被占到超时 | 事务只包 DB 操作，RPC 放外面 |
| RR 下大范围 UPDATE | 间隙锁互相等待，死锁率上升 | 按主键排序更新，或该业务降 RC |
| 改 RC 但 binlog 是 STATEMENT | 主从数据不一致，且很难查 | 先确认 `binlog_format=ROW` |
| 长事务跑报表 | 历史版本堆积、磁盘涨、复制延迟 | 报表走从库或离线，拆小批 |

## 6.5 连接池：参数怎么算

**结论先行**：池大小按数据库的并发处理能力算，不按 QPS 算。HikariCP 给的公式是 `connections = (core_count * 2) + effective_spindle_count`；8 核 + 1 块 SSD 算出来 17，取 20。

| 机器 | 公式 | 设多少 |
|---|---|---|
| 8 核 + 1 SSD | 8×2+1 = 17 | 20 |
| 16 核 + RAID | 16×2+2 = 34 | 40 |
| 32 核 + 本地 NVMe | 32×2+1 = 65 | 64–80 |

别设 200。连接本身占内存（经验值每连接几百 KB 到 1MB），真正干活的是 CPU，多出来的连接只是在 MySQL 里排队。

| 参数 | 推荐值 | 理由 |
|---|---|---|
| `SetMaxOpenConns` | 20–50，按公式 | 上限；超过 DB 处理能力只会增加等锁 |
| `SetMaxIdleConns` | 等于 MaxOpen，或至少一半 | 空闲小于上限会反复建连、关连 |
| `SetConnMaxLifetime` | 30 分钟 | 必须小于 `wait_timeout`（默认 28800 秒 = 8 小时） |
| `SetConnMaxIdleTime` | 5 分钟 | 主从切换、DB 重启后尽快淘汰死连接 |

先算总账再动手：总连接数 = Pod 数 × MaxOpen。20 个 Pod × 50 = 1000，而 `max_connections` 默认 151（实测），直接爆 "too many connections"。要么把 MaxOpen 降到 7，要么把 `max_connections` 调到 1500 并确认内存扛得住。

### 可直接抄

```go
// go-sql-driver/mysql v1.8.1，Go 1.22+；已通过 go vet 与 go build
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// openDB 按「核数*2+磁盘数」定上界，并保证连接寿命短于 MySQL wait_timeout。
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)                  // 8C16G 单实例的经验上界
	db.SetMaxIdleConns(25)                  // 空闲保一半，避免反复建连
	db.SetConnMaxLifetime(30 * time.Minute) // 必须 < wait_timeout(28800s)
	db.SetConnMaxIdleTime(5 * time.Minute)  // 空闲 5 分钟回收

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil { // 启动即探活，别等第一个请求
		db.Close()
		return nil, err
	}
	return db, nil
}

func main() {
	db, err := openDB(os.Getenv("MYSQL_DSN"))
	if err != nil {
		slog.Error("db init failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()
}
```

Java 团队用 HikariCP 的等价配置：

```yaml
spring:
  datasource:
    hikari:
      maximum-pool-size: 20
      minimum-idle: 20        # 与上限一致，池子稳定
      connection-timeout: 3000
      max-lifetime: 1800000   # 30 分钟，小于 wait_timeout
      keepalive-time: 60000
```

## 6.6 读写分离与主从延迟

**结论先行**：读写分离扩的是读，不是写；延迟一定存在，靠"写后读走主 + 半同步 + 按 GTID 等待"三招兜。

延迟多少算大，按数量级判断：

| 延迟 | 现象 | 动作 |
|---|---|---|
| < 100ms | 绝大多数业务无感 | 不用处理 |
| 100ms–1s | 下单后立刻查详情可能查不到 | 写后读走主 1 秒 |
| 1–10s | 列表数据旧、对账错 | 告警；关键读切主 |
| > 10s | 视为故障 | 摘除从库，读全切主并限流 |

三种应对，按成本从低到高：

1. **写后读走主**：请求上下文打标（cookie / header），N 秒内同一用户读走主库。成本最低，先做这个。
2. **半同步复制**：主库等至少一个从库 ACK 才返回。

```sql
INSTALL PLUGIN rpl_semi_sync_source SONAME 'semisync_source.so';  -- 8.0.26+ 名称
SET GLOBAL rpl_semi_sync_source_enabled = ON;
SET GLOBAL rpl_semi_sync_source_timeout = 1000;  -- 1 秒等不到，退回异步
```

代价是写 RT 多一次网络往返（同机房约 0.5ms）。超时后会静默退回异步，必须监控 `Rpl_semi_sync_source_status`。

3. **按 GTID 等待**：写事务拿到 GTID，读之前等从库追上。

```sql
SELECT WAIT_FOR_EXECUTED_GTID_SET('0903c33e-d673-11f1-af69-7cd6be32c131:1-23', 1);
-- 返回 0 = 已追上；返回 1 = 超时（实测返回 0）
```

判断延迟别只看一个数：

```sql
SHOW REPLICA STATUS;  -- 8.0.22+；更早版本用 SHOW SLAVE STATUS
```

`Replica_IO_Running`、`Replica_SQL_Running` 必须都是 `Yes`；`Seconds_Behind_Source` 只能当参考，大事务时它会跳到 0 或剧烈波动。可靠做法是心跳表：每秒更新一行时间戳，从库读出来算差值，这才是端到端真实延迟。

## 6.7 分库分表：什么时候才做

**结论先行**：单表 2000 万行、单表 500GB、写 5000 QPS 是"开始准备"的信号，不是"立刻拆"的信号。先归档、加索引、读写分离，都不够再拆。

| 信号 | 阈值（经验值） | 先做什么（成本从低到高） |
|---|---|---|
| 单表行数 | 2000 万–5000 万 | 归档冷数据 |
| 单表磁盘 | 500GB–1TB | 归档 + 分区表 |
| 写入 QPS | 单实例 3000–5000 | 批量合并写、读写分离 |
| 备份时长 | 超过 2 小时 | 换物理备份，再考虑拆 |
| DDL 时长 | 超过 30 分钟 | 上 gh-ost，再考虑拆 |
| 连接数 | 超 `max_connections` 的 70% | 收敛连接池、加从库 |

分片键三原则：**基数要高**（`user_id` 行，`status` 不行）、**分布要匀**（别按大客户切段，会热点）、**高频查询都带它**（否则每次都是广播到所有分片）。

订单表举例：按 `user_id` 分片，`order_id` 用"用户 ID 后 4 位 + 雪花"，这样按订单号也能直接算出分片，不用广播。

拆完这些功能会坏，方案评审时必须一起答：

| 能力 | 为什么坏 | 替代方案 |
|---|---|---|
| 跨分片 JOIN | 不能跨库 join | 冗余字段、宽表、应用层拼装 |
| 全局排序 + 深分页 | 每片各取 N 条再归并，越翻越慢 | 搜索走 ES，列表按时间或 ID 范围 |
| 分布式事务 | 单机事务跨不了库 | 本地消息表 + 最终一致（第 8 章） |
| 自增 ID | 各片各自自增，必然冲突 | 雪花、号段（Leaf）、Redis INCR |
| 全局唯一约束 | 建不了跨库唯一索引 | 独立去重表、Redis SETNX |
| COUNT / 聚合报表 | 要扫所有分片再拼 | 预聚合、离线数仓 |

🔑 能不分就不分。10 张表各 300 万行就上 16 个库，运维成本远超收益。

## 6.8 大表 DDL 与数据变更上线

**结论先行**：8.0 的 INSTANT 只覆盖加列、改默认值这类元数据操作；加索引走 INPLACE + LOCK=NONE；改列类型只能 COPY 锁写。大表一律上 gh-ost 或 pt-osc。

| 算法 | 支持的操作 | 锁 | 8.0 行为 |
|---|---|---|---|
| INSTANT | 加列（8.0.12+ 末尾，8.0.29+ 任意位置）、DROP COLUMN | 无 | 秒级完成 |
| INPLACE | 加删二级索引、部分加列 | `LOCK=NONE` 可读写 | `ADD INDEX` 实测成功 |
| COPY | 改列类型、改字符集、重建表 | 锁写 | 报错 1846，提示改用 COPY |

判断方法：显式写算法，让 MySQL 替你决定。不支持就立刻报错，而不是偷偷锁表 40 分钟。

```sql
ALTER TABLE orders ADD COLUMN remark VARCHAR(64) NOT NULL DEFAULT '', ALGORITHM=INSTANT;
ALTER TABLE orders ADD INDEX idx_status_created (status, created_at), ALGORITHM=INPLACE, LOCK=NONE;
```

改列类型时的真实报错：

```text
ERROR 1846 (0A000): ALGORITHM=INPLACE is not supported.
Reason: Cannot change column type INPLACE. Try ALGORITHM=COPY.
```

看到这条就说明必须锁写，大表不能直接上，改用工具：

| 工具 | 版本 | 适合 | 前提 |
|---|---|---|---|
| gh-ost | 1.1.x | 写入压力大、要限流和暂停 | `binlog_format=ROW`、`binlog_row_image=FULL`、有主键 |
| pt-online-schema-change | Percona Toolkit 3.x | 团队已有运维脚本 | 有主键或唯一键，且表上没别的触发器 |

gh-ost 命令，先 `--test-on-replica` 演练再上主库：

```bash
gh-ost \
  --host=10.0.0.12 --port=3306 --user=ghost --password="$GHOST_PASS" \
  --database=shop --table=orders \
  --alter="ADD COLUMN remark VARCHAR(64) NOT NULL DEFAULT ''" \
  --assume-rbr --allow-on-master \
  --max-load='Threads_running=30' \
  --critical-load='Threads_running=80' \
  --chunk-size=1000 \
  --panic-flag-file=/tmp/gh-ost.panic \
  --execute
```

`--max-load` 到了就暂停（`Threads_running` 超 30），`--critical-load` 到了就退出，`touch` 一下 panic 文件也能停。没有限流参数就别上生产。

字段变更的上线顺序，四步走完再删旧列：

1. **加列**：可空或带默认值，走 INSTANT。
2. **双写**：新代码同时写新旧两列，旧代码继续读旧列。
3. **回填**：按主键分批，别一条 UPDATE 改全表。

```sql
UPDATE orders SET remark = '' WHERE id > 0 AND id <= 5000 AND remark IS NULL;
-- 每批 1000–5000 行，批间 sleep 100ms，观察主从延迟
```

4. **切读再删**：灰度读新列 → 全量 → 停双写 → 观察一周 → 删旧列。

### 坑

| 坑 | 后果 | 正确做法 |
|---|---|---|
| 高峰跑 DDL | 从库延迟、锁等待、接口超时 | 02:00–05:00 执行，先从库演练 |
| 回填用一条大 UPDATE | 主从延迟飙到分钟级 | 按主键分批，批间 sleep |
| 直接 DROP 旧列 | 还在跑旧代码的实例报 Unknown column | 先停读、再删列，至少隔一个版本 |
| 不设 `--max-load` | gh-ost 打满主库 | 必设 max-load 和 critical-load |

## 6.9 备份与恢复：能恢复才算备份

**结论先行**：没演练过的备份约等于没有备份。验收标准只有一个：给你一台空机器，能不能在 RTO 内恢复到 RPO 那个时间点。

| 类型 | 工具 | 频率 | 保留 | RPO |
|---|---|---|---|---|
| 全量物理 | XtraBackup 8.0 | 每天 | 14 天 | 24h |
| 增量物理 | `xtrabackup --incremental` | 每 6 小时 | 7 天 | 6h |
| binlog | `mysqlbinlog` 归档到对象存储 | 每 5 分钟 | ≥ 14 天 | 分钟级 |
| 逻辑 | `mysqldump --single-transaction` | 小库、单表 | 7 天 | 24h |

### 可直接抄

```bash
# 1. 逻辑全量备份（InnoDB 热备；8.0.26+ 用 --source-data，更早用 --master-data=2）
mysqldump -h127.0.0.1 -uroot -p"$MYSQL_PWD" \
  --single-transaction --source-data=2 --set-gtid-purged=OFF \
  --databases shop > /backup/shop_$(date +%F).sql

# 2. 从备份头拿位点（实测输出）
grep -m1 "CHANGE REPLICATION SOURCE" /backup/shop_2026-11-02.sql
# -- CHANGE REPLICATION SOURCE TO SOURCE_LOG_FILE='binlog.000001', SOURCE_LOG_POS=13664512;

# 3. 恢复全量
mysql -h127.0.0.1 -uroot -p"$MYSQL_PWD" < /backup/shop_2026-11-02.sql

# 4. 回放 binlog 到误操作之前
mysqlbinlog --skip-gtids --start-position=13664512 --stop-position=13664828 \
  /var/lib/mysql/binlog.000001 | mysql -h127.0.0.1 -uroot -p"$MYSQL_PWD"

# 5. 校验行数和关键汇总
mysql -h127.0.0.1 -uroot -p"$MYSQL_PWD" -e "SELECT COUNT(*) FROM shop.orders"
```

实测结果：恢复后 4 行，备份之后写入的那 1 行被 binlog 补回。不加 `--skip-gtids` 时这行丢了——GTID 模式下目标实例已有该事务记录，事件会被当成重复跳过，这是最容易踩的一脚。

XtraBackup 适合大库（超过 100GB）、要求分钟级 RTO 的场景：

```bash
xtrabackup --backup --target-dir=/backup/full --user=backup --password="$XB_PASS"
xtrabackup --prepare --target-dir=/backup/full      # 恢复前必须执行
xtrabackup --copy-back --target-dir=/backup/full --datadir=/var/lib/mysql
```

五条不能省的规矩：

- **备份不在同一机房**。本机备份等于没备份；至少一份在对象存储的另一个 region，并开版本控制防误删。
- **每季度做一次恢复演练**：随机抽一份备份恢复到新实例，记录耗时（那才是 RTO），抽查 3 张表的行数和金额汇总。
- **备份要监控**：失败告警，文件大小突变也告警（突然变小 = 备份被截断）。
- **binlog 保留 ≥ 2 个全量周期**：`binlog_expire_logs_seconds` 默认 30 天（实测 2592000），别调到 7 天。
- **出事第一步是抢 binlog**：`FLUSH BINARY LOGS;` 后立刻把当前文件拷走，慢一步可能被 PURGE，PITR 直接没有窗口。

⚠️ 恢复顺序不能反：先全量、再回放 binlog、最后才切流量；中间用只读账号校验数据，确认后再放开写。

## 本章自检

**Q1** 索引是 `(user_id, status, created_at)`，查询 `WHERE user_id=? AND created_at>? ORDER BY created_at` 为什么还是慢？
▸ 答案：`key_len` 只有 8，`status` 夹在中间，`created_at` 只能过滤不能定位，排序也用不上索引。改成 `(user_id, created_at)`，范围定位和排序一次解决。

**Q2** 把隔离级别从 RR 改成 RC，哪类代码会立刻出问题？
▸ 答案：依赖间隙锁防并发插入的"先查再插"。RC 下间隙锁基本不加，并发插入不再被挡；改用唯一索引 + `INSERT ... ON DUPLICATE KEY UPDATE` 兜底。

**Q3** 备份每天都成功，为什么还说"没有备份"？
▸ 答案：没演练过恢复。常见死法：binlog 已被 PURGE、备份和库在同一机房、GTID 回放漏了 `--skip-gtids`、恢复要 8 小时超过 RTO。验收只看空机器恢复演练的耗时和数据校验。
