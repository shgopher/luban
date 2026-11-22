# 远程轻量验证总结（verify-remote-light）

主机：root@189.24.91.75 / Ubuntu 24.04.1 LTS / kernel 6.8.0-48-generic / 1 vCPU / 1968 MB RAM / 39 GB 磁盘（可用 33 GB）
基线：`free -m` 显示 used 1035 MB（k3s 常驻），本次未停 k3s、未装 docker、未跑 >500 MB 进程。
时间：2026-11-22（本机时区 UTC+8；远端 date 显示 15:44 前后，容器时区可能为 UTC）。

## 任务 1：OSV-Scanner（书稿 52.3 表行 "OSV-Scanner | 多生态锁文件 | 包级 | 多语言单仓 | 未验证"）

- 版本：osv-scanner **2.7.1**（osv-scalibr 0.5.3，commit 02f96a4，built 2026-11-09T03:39:40Z），GitHub Releases `osv-scanner_linux_amd64`，58,187,936 B，下载耗时 3 s。
- 干净 npm 锁文件（本机 orval-demo package-lock.json，lockfileVersion 3，116 个 node_modules 条目）：解析 **111 个包**，**0 漏洞**，`No issues found`，退出码 **0**，耗时 **0.566 s**（JSON 复跑 0.588 s）。
- 构造的旧版本 npm 锁文件（lodash 4.17.15 / axios 0.21.1 / minimist 1.2.0 / node-fetch 2.6.0 / ejs 3.1.6 / jsonwebtoken 8.5.1）：解析 **6 个包**，**38 条漏洞**（2 Critical、17 High、17 Medium、2 Low），退出码 **1**，耗时 **1.388 s**。
  例：GHSA-p6mc-m468-83gw lodash 4.17.15 → 4.17.19（CVSS 7.4）；GHSA-phwq-j96m-2c2q ejs 3.1.6 → 3.1.7（CVSS 9.8）；GHSA-xvch-5gv4-984h minimist 1.2.0 → 1.2.6（CVSS 9.8）。
- 多生态单目录（package-lock.json + requirements.txt）：一次 `scan source -r` 同时识别 **npm 6 包 + PyPI 4 个直接依赖（另 3 个 deps.dev 解析出的传递依赖，被记为 unknown extractor）**，合计 **144 条漏洞条目（npm 38 / PyPI 106，去重后仍 144 个 ID）**，退出码 **1**，耗时 **2.764 s**。
- 离线能力：`--offline` 与 `--offline-vulnerabilities` 两个 flag 都存在，但本机未预下载 OSV 离线库；实测两者都输出 `could not load db for npm ecosystem: ... no offline version of the OSV database is available`，同时表格仍打印 `Total 0 packages affected by 0 known vulnerabilities`（**假绿**），退出码 **127**，耗时 0.144 s / 0.155 s。`--download-offline-databases` 未执行（体积超出本次预算）。
- 书稿提示：v2.7.1 官方推荐子命令为 `osv-scanner scan source --lockfile ...`；直接把 `--lockfile` 挂在 `scan` 后仍可用但会打印 `Starting filesystem walk for root: /`。另：锁文件名必须叫 `package-lock.json`，改名后报 `could not determine extractor suitable to this file` 且退出码 127（初始放置为 `/tmp/orval-demo-package-lock.json` 时实测）。

## 任务 2：HarfBuzz / FriBidi（书稿 72.1 第 326、337 行）

环境：libharfbuzz-bin **8.3.0-2build2**（`hb-shape (HarfBuzz) 8.3.0`，shapers: graphite2,ot,fallback）、libfribidi-bin **1.0.13-3build1**（`fribidi (GNU FriBidi) 1.0.13`，Unicode 14.0.0）、fonts-noto-core **20201225-2**、fonts-dejavu-core。`fribidi` 包名在 Ubuntu 24.04 不存在（`E: Unable to locate package fribidi`，rc=100），实际包名 **libfribidi-bin**。

字体：NotoNaskhArabic-Regular.ttf / NotoSansArabic-Regular.ttf / NotoSansHebrew-Regular.ttf / DejaVuSans.ttf。

### 阿拉伯文位置形态（NotoNaskhArabic，`--direction=rtl`）

| 输入 | 输出（glyph 名，视觉序） | glyph id |
|---|---|---|
| `ب`（独立） | `[uni0628=0+772]` | 35 |
| `بيت` | `[uniFE96|uniFEF4|uniFE91]`（ت 词尾、ي 词中、ب 词首） | 54 / 576 / 38 |
| `بببب` | `[uniFE90|uniFE92|uniFE92|uniFE91]`（词尾/词中/词中/词首） | 36 / 37 / 37 / 38 |

结论：同一字母 ب 的独立/词首/词中/词尾分别为 **uni0628(35) / uniFE91(38) / uniFE92(37) / uniFE90(36)**，四个 glyph id 全不同。
**与书稿预期不符**：书中示例名 `uni0628.init/.medi/.fina/.isol` 未出现；HarfBuzz+Noto 实际输出 Unicode 表现形 `uniFE90/FE91/FE92`，独立形直接落到基本码位 `uni0628`。

### 括号镜像（hb-shape，真实 glyph 名）

- LTR：`(a)[b]{c}<d>` → `parenleft parenright bracketleft bracketright braceleft braceright less greater`。
- RTL：`(a)[b]{c}<d>` → 字符 `(` 输出 **parenright**、`)` 输出 **parenleft**；`[`→**bracketright**、`]`→**bracketleft**；`{`→**braceright**、`}`→**braceleft**；`<`→**greater**、`>`→**less**。镜像成立。
- 阿拉伯文 run：DejaVuSans `(مرحبا)` RTL → `[parenleft=6|uniFE8E|uniFE92|uniFEA3|uniFEAE|uniFEE3|parenright=0]`，即 `(`(cluster0) 显示为 parenright、`)`(cluster6) 显示为 parenleft，同时阿拉伯字母连写。NotoSansArabic 不含 ASCII 括号（输出 `.notdef`），需换含括号的字体验证。
- fribidi 1.0.13：`printf "(A" | fribidi --rtl --nopad` → 字节 `41 29`（"A)"，已镜像）；`(אבג)` → `28 … 29`；但 `--nomirror` 在 4 组对照中输出与默认**逐字节相同**（`(A`→`41 29`），未能用该开关关掉镜像；`--levels/--ltov/--vtol` 可正常输出 bidi 重排结果。
- 工具边界：`hb-shape` 不做 bidi 段落重排，方向需按 run 显式传 `--direction`；`fribidi` 只做重排/镜像，不做 OpenType 连写（输出仍是 U+06xx 码位或表现形，不产出 `.init` 类 glyph 名）。
- 未闭环：**无 iOS/Android/浏览器真机截图**，无法验证系统级 RTL 布局、行内数字 run 摆放与最终像素；本节“字体 shaping 与括号镜像未观测”可在字体/工具层改为“已观测”，但“真机渲染”仍为未验证。

## 任务 3：Nominatim 公共 API（书稿 71.1 第 295-298 行）

UA：`luban-book-verify/1.0 (technical book fact-check)`；请求间隔 2 s（政策上限 1 req/s）。

| # | 请求 | HTTP | 耗时 | 关键结果 |
|---|---|---|---|---|
| A | search `q=北京市海淀区中关村` jsonv2+addressdetails+limit=1 | 200 | 0.252 s | place_id 222044519，node 7861046851，39.9826574/116.3103787，category=railway，type=station，addresstype=railway，place_rank=30 |
| B | search `q=Zhongguancun, Beijing` + `Accept-Language: en` | 200 | 0.144 s | 同一 osm_id，name=Zhongguancun，display_name 全英文 |
| C | reverse 39.9834/116.3167 zoom=18 + zh | 200 | 0.265 s | way 72590878，category=highway，type=residential，addresstype=residential，place_rank=26，display_name 含 100190 |
| D | 同上 + en | 200 | 0.124 s | 同 place_id，`suburb` 变 Zhongguancun，`city` 变 Haidian District，`neighbourhood` 仍为中文“科育社区” |
| E | `/status` | 200 | 0.070 s | 正文仅 2 字节 `OK`（非 JSON） |
| F | search 同 A 但 `format=json`（v1） | 200 | 0.338 s | 字段名是 **class**（不是 category），其余同 |
| G | search `q=北京市` limit=1 | 200 | 0.325 s | relation 912940，category=boundary，type=administrative，addresstype=**city**，place_rank=7 |
| H | reverse 同坐标 zoom=10 | 200 | 0.246 s | relation 18250355，category=boundary，type=administrative，addresstype=**suburb**，place_rank=16 |

真实响应字段（jsonv2，15 个）：`place_id, licence, osm_type, osm_id, lat, lon, category, type, place_rank, importance, addresstype, name, display_name, address, boundingbox`。
v1（`format=json`）把 `category` 换成 `class`（14 个字段）。
`address.*` 实际出现子键：`railway, road, commercial, quarter, neighbourhood, suburb, city, ISO3166-2-lvl4, postcode, country, country_code`。
观测到的枚举：category ∈ {railway, highway, boundary}；type ∈ {station, residential, administrative}；addresstype ∈ {railway, residential, city, suburb}；place_rank ∈ {7, 16, 26, 30}。
多语言：同一对象 zh/en 的 `display_name` 不同；en 下 `address.suburb` 被译成 Zhongguancun、`city` 译成 Haidian District，但 `neighbourhood`（科育社区）保持中文——**地址字段是部分本地化**。同一 node 的 postcode 在 zh 响应为 100086、en 响应为 100080。

政策原文（https://operations.osmfoundation.org/policies/nominatim/ ，2026-11-22 抓取）：
- “No heavy uses (an absolute **maximum of 1 request per second**).”
- “Provide a valid **HTTP Referer** or **User-Agent** identifying the application (stock User-Agents as set by http libraries will not do).”
- “Clearly display attribution … Data is provided under the ODbL license…”
- Bulk：single thread、1 machine、no distributed scripts；脚本长于一天/定期运行限 **4 requests per minute**；“Results must be cached on your side.”
- 禁止：“Systematic queries … reverse queries in a grid, searching for complete lists of postcodes, towns etc.”；禁止 autocomplete。

未验证（不得写成已验证）：自建 Nominatim/Pelias + OSM 全量数据的召回率与延迟（本次未部署 OSM 数据，也未做并发压测）；三家商业逆地理（高德/腾讯/百度）真实字段与精度枚举（仍缺 API key）。公共 API 的 0.12–0.34 s 只是单次公网往返，不能当自建延迟。
