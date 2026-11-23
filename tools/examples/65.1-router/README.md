# 65.1-router：租户路由与连接预算

[65.1 租户隔离模型](../../../v2/L4-组件与技术/65-多租户SaaS/65.1-租户隔离模型.md) 的配套代码。

| 路径 | 内容 | 跑法 |
|---|---|---|
| `router/main.go` | 租户路由核心：miss 时 fail-closed、本地缓存、版本号失效 | `go run ./router`（纯内存） |
| `conns/main.go` | 每租户连接预算探测 | 需 PostgreSQL：`go run ./conns` |
| `*.sql` / `*.sh` | 建表与压测脚本（`pg-init.sh` 起库，`conn-probe.sh` 探连接） | 见脚本首行 |

Postgres 起库（Docker）：

```bash
docker run --rm -d --name pg65 -e POSTGRES_PASSWORD=pg -p 5432:5432 postgres:16
docker exec -i pg65 psql -U postgres -f /path/to/baseline.sql
```

> `catalog2.sql`（882 KB 批量插入数据）没有入库，按 README 里的 schema 重新生成即可。
