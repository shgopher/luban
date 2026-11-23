# 72.3-money：多币种与汇率

[72.3 多币种与汇率](../../../v2/L4-组件与技术/72-国际化与多地域/72.3-多币种与汇率.md) 的配套代码（Go）。

```bash
cd tools/examples/72.3-money/code
go run ./cmd/money
go run ./cmd/fx ../raw/ecb-hist-90d.xml ../raw/erapi-usd.json
```

`raw/` 里有 ECB 汇率 XML、ISO 4217 列表、CLDR 数据与换算结果。`c-postgres.txt` 是 `numeric` 精度实验的输出，对应正文那张表。
