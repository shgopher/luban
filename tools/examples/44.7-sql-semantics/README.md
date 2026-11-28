# 44.7 SQL 语义最小验证

对应正文：[44.7 SQL 与查询优化](../../../v2/L4-组件与技术/44-数据库/44.7-SQL与查询优化.md)。

验证六件事：`COUNT(*)` 与 `COUNT(col)` 对 NULL 的差异、`GROUP BY` 的 NULL 组、`HAVING` 能否引用 SELECT 别名、`LIKE '%'` 不匹配 NULL、`CAST` 与字符长度的真实语义、`COUNT(*)` 为什么走二级索引。

- `verify.sql` → `expected.txt`：前五件事。
- `count.sql` → `count-expected.txt`：3000 行表 + 二级索引，`EXPLAIN SELECT COUNT(*)` 选中 `idx_k`，`Extra=Using index`。

## 跑法

前置：Docker、`mysql` 客户端。用一次性容器，跑完即删。

```bash
export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=unix://$HOME/.docker/run/docker.sock

docker run -d --rm --name sql-semantics \
  -e MYSQL_ROOT_PASSWORD=root -p 127.0.0.1:33308:3306 mysql:8.0

for i in $(seq 1 60); do
  mysql -h127.0.0.1 -P33308 -uroot -proot -e 'select 1' >/dev/null 2>&1 && break
  sleep 1
done

mysql -h127.0.0.1 -P33308 -uroot -proot \
  --default-character-set=utf8mb4 --table < verify.sql

mysql -h127.0.0.1 -P33308 -uroot -proot \
  --default-character-set=utf8mb4 --table < count.sql

docker rm -f sql-semantics
```

本机结果（MySQL 8.0.46，Apple M1，2026-11-28）已存到 `expected.txt`，关键行：

| 查询 | 结果 | 说明 |
|---|---|---|
| `COUNT(*) / COUNT(v) / COUNT(DISTINCT v)` | `6 / 4 / 3` | `v` 有两个 NULL，`COUNT(v)` 只数 4 行 |
| `GROUP BY grp` | `NULL 2 / a 2 / b 2` | NULL 自成一组，不是被丢掉 |
| `HAVING num > 1` | 3 行 | MySQL 允许 HAVING 引用 SELECT 别名 |
| `'abc' LIKE '%'` / `NULL LIKE '%'` | `1` / `NULL` | `LIKE '%'` 不等于“匹配所有” |
| `CAST(123.9 AS SIGNED)` | `124` | 四舍五入，不是截断 |
| `LENGTH('你好') / CHAR_LENGTH('你好')` | `6 / 2` | 字节数 vs 字符数；连接字符集必须是 utf8mb4 |

## 假设与边界

- 字符长度依赖连接字符集：`mysql` 客户端不加 `--default-character-set=utf8mb4` 时，`CHAR_LENGTH('你好')` 可能也返回 6（按 latin1 解释）。
- `CAST(x AS INT)` 不在脚本里，因为 MySQL 会直接报语法错误：CAST 的目标类型是 `SIGNED/UNSIGNED/DECIMAL/CHAR/DATE...`，没有 `INT`。
- 输出是确定值，不随机器漂；只有 `mysql` 客户端的表格线宽度会变。
