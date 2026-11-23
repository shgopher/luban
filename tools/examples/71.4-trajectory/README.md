# 71.4-trajectory：轨迹存储与查询

[71.4 轨迹存储与查询](../../../v2/L4-组件与技术/71-地理位置服务/71.4-轨迹存储与查询.md) 的配套代码。

```bash
cd tools/examples/71.4-trajectory/code
go mod download
go run .
```

依赖 MySQL 驱动；`raw/` 是编码、压缩与查询的原始输出。数据库文件（`mysql/` 目录）没有入库，按 README 里的建表语句重建即可。
