# 72.2-zones：多时区处理

[72.2 多时区处理](../../../v2/L4-组件与技术/72-国际化与多地域/72.2-多时区处理.md) 的配套代码（Go，纯标准库）。

```bash
cd tools/examples/72.2-zones/code
go run ./cmd/b-dst        # 或 b2-resolve / e-history / e-loadtz / e-zonediff / f-abbr
```

`raw/` 里是各区时、DST 切换、缩写歧义与 Go 版本差异的原始输出（含 2026b/2026c/2026f 的 tzdata 对比）。
