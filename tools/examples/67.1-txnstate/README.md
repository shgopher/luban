# 67.1-txnstate：状态机非法迁移

[67.1-交易流程建模](../../../v2/L4-组件与技术/67-交易与账务/67.1-交易流程建模.md) 的配套代码。

```bash
cd tools/examples/67.1-txnstate
go run .
```

输出：

```text
✓ INIT -> PAYING
✓ PAYING -> PAID
✓ PAID -> REFUNDING
✗ 非法迁移 PAID -> PAYING
✗ 非法迁移 CLOSED -> PAID
✗ 非法迁移 REFUNDED -> PAID
```

合法迁移白名单 + 非法迁移拒绝，防止 PAID 回落、CLOSED 复活。
