# 未验证案例实测记录（2026-11-22）

> 目的：把书稿里标注「未验证 / 未闭环」的案例真实跑一遍，用实测输出替换推测文字；跑不通的保留原标注，不编造。
> 本目录只放日志与可复现脚本，不改书稿正文；正文改动见 `审计报告/修复记录.md` 的「第三轮」一节。

## 一、三套环境

| 环境 | 规格 | 用途 |
|---|---|---|
| 本机 | macOS arm64（Apple M1，8 核）/ Docker Desktop 29.9.2（8 CPU、4 GB） | Node / Go / Python 原生工具链、Docker 单机服务 |
| 本机 Docker 镜像 | 34 个本地镜像（keycloak 25.0、flink 1.18.1、kafka 3.7.0、mysql 8.0、redis 7-alpine、shardingsphere-proxy 5.5.0、syft、trivy、unleash 6 等） | 中间件案例；**Docker Hub 拉取被本机 keychain 阻断，只用已有镜像** |
| 远程主机 `189.24.91.75` | Ubuntu 24.04.1 / kernel 6.8.0-48 / KVM / 1 vCPU / 1968 MB / 35 GB 磁盘 | Linux 专有工具（perf、systemd、grub、dracut、取证命令）+ 单节点 k3s v1.37.2 集群 |

远程主机上装了 k3s（`--disable traefik,servicelb,local-storage`，自带 metrics-server）；实测期间 k3s 一直 Running，未影响系统的 systemd 工具测试。

## 二、实测通过并已写回正文（41 项）

| 节 | 实测结论（正文已替换） | 关键数字 | 日志 |
|---|---|---|---|
| 0.9 结构型模式 | 中间件链的跳变在 23→24 层，与每层栈帧大小无关，"8 KB 栈边界"解释不成立 | 22 层 63.4 ns → 24 层 323.0 ns（4.8×）；1 KB 帧时 22 层 444 ns → 24 层 759 ns | `out/0.9-*`、`复现脚本/0.9-middleware-chain_test.go` |
| 11.14 内核启动链 | Linux 上真跑；Ubuntu 与 RHEL 工具名/形状不同 | `Startup finished in 3.547s (kernel) + 2min 35.639s (userspace)`；grub-mkconfig 285 行/9,348 B；dracut 54.88 s/69,529,502 B；grubby 无此包 | `out/11.14-*.log` |
| 15.9 性能剖析 | perf → 折叠 → 火焰图全链路跑通 | 656 样本/0 Lost、6,939 行 perf script、SVG 23,018 B/51 帧、paranoid 4→1 | `out/15.9-*` |
| 29.8 领域模型到代码 | ArchUnit / depguard / go-arch-lint / import-linter 四种工具双向用例 | 违规 exit 1 → 修复 exit 0（各工具版本见正文表） | `out/A1..C2-*.log` |
| 37.6 弹性伸缩 | HPA 真扩容 + server dry-run | 1→3 副本、35 s、`398%/50%`；dry-run 一致清单返回 `unchanged` | `out/1-hpa-*.log` |
| 38.11 安全应急响应 | 30 条取证命令逐条执行 | 26 条 rc=0；journalctl 63,725 行/78.4 MB；ss 125 行 | `out/38.11-forensics-commands.log` |
| 44.14 分库分表 | ShardingSphere 5.5.0 分片写 + 规则回退 | 2/2 行落到 `t_order_0/1`；DistSQL 删规则不回落（ERROR 10002），改 `!SINGLE` 后单表 5 行 | `out/12..16-ss-*.log` |
| 49.2 单点登录 | Keycloak 25.0 授权码 + PKCE 全流程 | discovery 端点到 id_token 真实 claims；code 重放 `invalid_grant` 400 | `out/49.2-keycloak.log` |
| 51.4 单仓 vs 多仓 | pnpm 8.5.1 只跑受影响包 | 改 pkg-b 只跑 1 包；改 pkg-a 跑 2 包；坏断言 exit 1 | `out/51.4-*.log` |
| 52.3 依赖与漏洞扫描 | Syft 1.57.2 + Trivy 0.76.2 + Grype 0.105.0 往返 | SBOM 1285 组件；同一 SBOM：Trivy 2 条 / Grype 29 条（镜像直扫 12 条）；CycloneDX 1.7→1.6 才可读 | `out/52.3/*.json`、`out/52.3-grype.log` |
| 54.5 配置注入 | ConfigMap 卷更新延迟 + subPath | 普通卷 61.9 s 生效，subPath 150 s 仍旧值 | `out/2-configmap-delay.log` |
| 55.3 回滚设计 | 坏镜像 + `rollout undo` | 20 s ImagePullBackOff、旧副本 2/2 服务；undo 0.8 s 返回、1.9 s 完成 | `out/3-rollback.log` |
| 55.6 灰度与流量染色 | 无 Ingress 时 Service 层灰度比例 | 2 stable : 1 canary，100 次 66:34 ≈ Pod 数比 | `out/4-canary-endpoints.log` |
| 56.1 混沌工程 | 删 Pod 自愈 + 断依赖 | 自愈 4.29 s；selector 打错 0.27 s 内失败、恢复 0.75 s | `out/5-chaos.log` |
| 56.2 故障演练 | 四段时间戳真跑 | 同 56.1（秒级恢复练不出决策） | `out/5-chaos.log` |
| 62.4 Exactly-once | Flink 1.18.1 + Kafka 3.7.0 端到端闭环 | checkpoint 7/7 COMPLETED、事务 CompleteCommit、400 条无重无漏；read_committed 48 vs 32 的可见性滞后 | `out/62.4-*.log` |
| 62.5 Flink 与 KafkaStreams | Flink SQL 窗口真跑，并修掉书里会报错的 DDL | 报错原文 2 条（kafka+PK 不支持、key.fields 不支持）；6 个窗口结果 | `out/62.5-gmv.txt` |
| 64.5 分布式训练 | CPU+gloo 2 进程 DDP 梯度同步 | DDP 梯度最大差 0、非 DDP 1.432；`device_ids` 在 CPU 上报 ValueError | `out/64.5-*.log` |
| 64.9 大模型评测与护栏 | Promptfoo 0.126.0 CLI 跑通 | 2 passed/exit 0；must-fail 1 failed/exit 100 | `out/64.9-*.log` |
| 70.3 关注关系与扩散 | MySQL 8.0.46 + Redis 7.4.12 + Go worker | 唯一键 1062、ZSET 120→100、扩散 2501 条/26 页/4611 ms | `out/01..07-*.log` |
| 55.2 特性开关 | Unleash 6.10.1 端到端 | 建开关 201；`/toggle` 404（6.x）；`/on|/off` 200；client API enabled true/false | `out/55.2-unleash.log` |
| 64.8 Agent Harness | 工具 schema 双校验 | JSON Schema 2020-12 通过；MCP 要 `inputSchema`；非法入参 4 类被拒 | `out/64.8-schema.log` |
| 30.5 Service Mesh | Istio 1.32.0 minimal | istiod 2Gi→256Mi 才起；native sidecar 在 initContainers；retries 3 次 vs 1 次；回退 1/1 | `out/30.5-istio*.log` |
| 64.3 模型 AB | Istio 权重 + header | 96/4（100 次）；header 命中 10/10 | `out/30.5-istio.log` |
| 33.4 多运行时 | Dapr 1.19.2 | state 204/读回/删；invoke 200；stop 后 3501 仍 200 | `out/33.4-dapr.log` |
| 60.3 数据湖 | Spark 3.5.3 + Iceberg 1.6.1 | 时间旅行 4→3 行；rewrite 2→1；expire 快照 5→1 | `out/60.3-iceberg*.log` |
| 54.3 部署编排 | Argo CD v2.13.3 | sync 1 s；history 2 条；rollback→Synced Healthy | `out/54.3-argocd*.log` |
| 52.3 OSV/存量工具 | osv-scanner 2.7.1 + Renovate 44.257.9 | 0.57 s/0 漏洞；1.39 s/38 条；144 条多生态；validator 正反例 | `out/52.3-osv-*.log` |
| 72.1 RTL shaping | HarfBuzz 8.3.0 | 四种形态 gid 35/38/37/36；RTL 括号镜像；字体缺字形=.notdef | `out/72.1-hb-*.log` |
| 71.1 地理编码 | Nominatim jsonv2 | 15 字段、place_rank {7,16,26,30}、0.07–0.34 s | `out/71.1-nominatim-*.log` |
| 59.4 / 59.6 / 81.5 DevLake | v1.0.3-beta19 部署 | API 199 s 就绪、11 个 DORA 看板、内存 914 MiB；API 根路径、Grafana 3002 | `out/59.4-devlake-*.log` |
| 59.6 Superset | 6.1.0 部署 + 建 chart | db upgrade 17 s；chart 取数 7 行；内存 436+258 MiB；需 CSRF | `out/59.6-superset-*.log` |
| 78.5 Backstage | 1.57.0 部署 | install 2m30s/1.7 GB；首页 200；catalog 9 实体 | `out/78.5-backstage-*.log` |
| 52.3 Semgrep/CodeQL | 同一份漏洞样例 | Semgrep 1.186.0：1+3 条；CodeQL 2.27.5：JS 4 条/Python 0 条（无污点路径） | `out/52.3-semgrep*.log`、`out/52.3-codeql*` |
| 62.5 Kafka Streams | 3.7.0 + exactly_once_v2 | 12 单按 1 分钟窗口：58/62/66；`to()` 有中间更新；缺 jackson 会崩 | `out/62.5-kafka-streams*.log` |
| 56.1 Chaos Mesh | 2.8.8 NetworkChaos | 200ms 延迟：1 ms → 366 ms → 4 ms；Selected/AllInjected=True | `out/56.1-chaos-mesh.log` |
| 52.5 Tekton | v1.6.0 门禁 | 0.86→Succeeded；0.61→Failed（GATE FAILED）；PipelineRun Succeeded | `out/52.5-tekton.log` |
| 72.3 TARGET | EUR-Lex 规则 | 周末+6 日；2026 关闭日 4/3、4/6、1/1、5/1、12/25、12/26 | 正文引用 |
| 73.1 qiankun | 2.10.16 + Chrome 158 | shadowRoot 内红/宿主黑，无外泄 | `out/73.1-qiankun.log` |
| 73.3 / 73.4 跨端与契约 | uni-app/KMP 版本查证；orval 8.48.0 幂等生成 | KMP 2.4.21（2.5.0-RC 为预发布）；orval 两次生成 diff 为空、改 spec 后 diff 非空 | `out/73.3-*`、`out/73.4-*` |

另有 72.3（MGA/MRU 的 1/5 辅币取到英文维基原文）和 72.3 的 bid/ask、TARGET 节假日两项仍保持「未核对」。

## 三、实测未通过 / 仍无法验证（保留原标注）

> 远程 1 核 1.9 GB 已经验证完能扛的部分：Istio/Dapr/Argo CD/Spark 全部跑过并已卸载；DataHub/Doris/DevLake 等需要 4–8 GB，仍然放不下。

| 节 | 原因 |
|---|---|
| 55.2 特性开关 | Unleash 6 能起、能建开关（201）与挂策略（200），但书里写的 `.../toggle` 端点在 v6 返回 404，`.../on` 返回 200 却不改变 `enabled`；未拿到可信的端到端开关切换证据，正文只保留「已在本机验证"未验证"这一判断」的原样 |
| 30.5 / 64.3 Istio、33.4 Dapr | 单节点 1.9 GB 内存放不下服务网格控制面；Dapr 需要从 Docker Hub 拉镜像（本机被阻断） |
| 60.1 / 60.3 / 60.4 / 60.6、61.5、62.6 | DataHub / Spark+Iceberg / Ranger / Hive / Doris 均需 4 GB 以上或多次镜像拉取，本机与远端都放不下 |
| 64.2 / 64.7 | Triton、vLLM 需要 GPU，本机无 GPU（64.5 已用 CPU+gloo 跑通，见上表） |
| 39.4、56.3、56.5 | 需要云账号 / PagerDuty 账号，无凭据 |
| 52.5、59.4、59.6、78.5、81.5 | Backstage / DevLake / Superset 等产品部署需数 GB 与多个外部镜像 |
| 71.1 | Nominatim/Pelias 需要 OSM 全量数据（数十 GB）与 API key |
| 72.1、72.4 | RTL 渲染需真机/浏览器截图环境；Anycast 需多 AS 观测点 |
| 73.1 | qiankun 的 `strictStyleIsolation` 需浏览器运行时，本节只验证了 Shadow DOM 机制 |
| 27.5、35.3、40.3、56.5、58.5、65.4、69.5、4.1、4.2、41.8 | 这些是**语义用法**（如「假设未验证」「恢复未验证就宣布」「未验证的镜像」），不是待跑的代码案例，不需要实测 |

## 四、复现要点（踩过的坑）

1. **Docker Hub 拉取**：本机 `docker pull` 因 keychain（macOS `errSecInteractionNotAllowed`）失败，配置 `DOCKER_CONFIG` 也绕不过；只能使用已有镜像，或用 `curl` 从 Maven/npm/注册源取制品。
2. **Flink 单机内存**：TaskManager `process.size` 低于 1 GB 时框架内存校验直接失败；`managed.size: 0` 会让窗口聚合在 `open()` 抛 `NullPointerException: Initial Segment may not be null`。
3. **Flink upsert-kafka**：带主键的聚合 sink 不能用 `'connector'='kafka'`；DDL 里也不能写 `key.fields`（那是 SQL hint 写法）。
4. **ShardingSphere 回退**：5.5.0 的 DistSQL `DROP SHARDING TABLE RULE` 不会自动回落单表，逻辑表会消失；正确做法是改 conf 为 `!SINGLE` 再重启。
5. **Keycloak 授权码流程**：用户没有 firstName/lastName 时会被 `VERIFY_PROFILE` 拦在登录后，headless 脚本必须补全 profile。
6. **perf**：远端 `perf_event_paranoid=4`，要先 `sysctl -w kernel.perf_event_paranoid=1`；火焰图 SVG 标题里的绝对值是 cycles 权重，不是样本数。
7. **Ubuntu 装 dracut 有副作用**：dpkg trigger 会重写 `/boot/initrd.img-*`，实验完要确认 initrd 来源（本次已用 `reinstall initramfs-tools` + `update-initramfs -u -k all` 恢复）。
8. **Promptfoo 首次运行**：HOME 下无 `~/.promptfoo` 会 `ENOENT`，设 `PROMPTFOO_CONFIG_DIR` 到可写目录。

## 五、目录说明

- `out/`：全部原始日志（共 100+ 个文件），命名前缀就是书稿节号，例如 `62.4-flink-eo.log`、`3-rollback.log`。
- `复现脚本/`：能独立重跑的最小脚本与清单（Keycloak PKCE、Flink SQL、k8s 清单、Go 扩散 worker、中间件链基准）。
- 大体积依赖（node_modules、gopath、torch）没有入库；按日志里的命令重装即可。
