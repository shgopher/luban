# 73-frontend：前端架构的 fixture

[73.1 前端架构](../../../v2/L4-组件与技术/73-前端与客户端架构/73.1-前端架构.md)、[73.2 移动端架构](../../../v2/L4-组件与技术/73-前端与客户端架构/73.2-移动端架构.md)、[73.5 前端发布与灰度](../../../v2/L4-组件与技术/73-前端与客户端架构/73.5-前端发布与灰度.md) 的配套 fixture。

| 路径 | 是什么 | 支撑哪一节 |
|---|---|---|
| `bench/probe.js` | 用 CDP 抓首屏请求与体积的探针 | 73.1 |
| `s731/site/` | 四种渲染模式的测试站点 | 73.1 |
| `s731/raw/c/` | 2026-11-21 抓取的 nextjs.org / linear.app 首页快照 + 汇总 | 73.1 |
| `s731/raw/c-next/` | 同一批站点不同渲染模式的抓取输出 | 73.1 |
| `s732/raw/` | SQLite WAL 写入、重启恢复的原始输出 | 73.2 |
| `s735/nginx/` | `nginx-good.conf` / `nginx-spa.conf` 两份发布配置 | 73.5 |

跑探针：

```bash
node tools/examples/73-frontend/bench/probe.js tools/examples/73-frontend/s731/site /csr.html /ssr.html
```

抓取快照是**静态数据**（2026-11-21），用于复算正文里的 script 数与字节数，不保证线上仍一致；需要最新数据时用 `probe.js` 重抓。
