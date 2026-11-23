# 72.1-i18n：多语言与本地化

[72.1 多语言与本地化](../../../v2/L4-组件与技术/72-国际化与多地域/72.1-多语言与本地化.md) 的配套代码（Go）。

```bash
cd tools/examples/72.1-i18n/code
go mod download
go run ./cmd/negotiate     # 或 plural / bidi / length / formatnum / grapheme
```

`cmd/` 下每个子命令对应正文一个实验：语言协商、复数规则、双向文本、长度限制、数字格式、字素簇。依赖 `golang.org/x/text` 与 `rivo/uniseg`；`raw/` 是原始输出。
