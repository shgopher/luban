# 72.4-region：多地域部署的测量数据

[72.4 多地域部署与就近接入](../../../v2/L4-组件与技术/72-国际化与多地域/72.4-多地域部署与就近接入.md) 的配套 fixture。

| 文件 | 来源 |
|---|---|
| `raw/a-dns-probe.txt` | `dig` 对各地区 DNS 的解析结果 |
| `raw/b-ecs-matrix.txt` | EDNS Client Subnet 矩阵 |
| `raw/c-rtt-probe.txt` / `d-public-rtt.txt` | 各地到源的 RTT |
| `raw/e-ttl-jitter.txt` | TTL 抖动采样 |
| `raw/f-nginx-geo.txt` | `geoip` 分流结果 |
| `raw/g-ipgeo.txt` | IP 库比对 |
| `nginx.conf` | 就近接入的分流配置 |

这一节没有可运行程序：数据由 `dig`、`curl`、nginx 日志采集，配置文件按正文说明改 `upstream` 即可复测。
