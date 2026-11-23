# 70.4-counter：互动计数的四种实现

[70.4 互动计数](../../../v2/L4-组件与技术/70-内容与社交/70.4-互动计数.md) 的配套代码（Go + Redis + MySQL）。

```bash
cd tools/examples/70.4-counter/code
go mod download
go run ./cmd/e1_bitmap     # 或 e2_pipeline / e3_mysql / e4_batch
```

四条路径对比：Bitmap、Pipeline、MySQL 行更新、批量合并。`comment_seed.sql`（1.5 MB 种子数据）没有入库，按 `e3_mysql` 里的建表语句重新生成即可。
