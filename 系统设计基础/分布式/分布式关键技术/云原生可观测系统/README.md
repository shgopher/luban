<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2025-03-30 15:25:55
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2025-05-18 16:02:16
 * @FilePath: /luban/系统设计基础/分布式/分布式关键技术/云原生可观测系统/README.md
 * @Description: 
 * 
 * Copyright (c) 2025 by shgopher, All Rights Reserved. 
-->
ELK (日志收集)、Jaeger (链路追踪)、Prometheus (聚合度量) 是云原生领域三大核心可观测性工具，虽然功能存在交叉，但各自定位和实现机制有本质差异。以下是它们的核心区别与联系分析：

---

### 一、功能定位差异
| **维度**         | **ELK**                  | **Jaeger**              | **Prometheus**          |
|------------------|--------------------------|-------------------------|-------------------------|
| **核心功能**     | 日志采集、存储、检索     | 分布式链路追踪          | 指标监控与告警          |
| **数据模型**     | 非结构化日志（文本）     | 结构化追踪数据（Span）  | 时间序列指标（数值）    |
| **应用场景**     | 日志分析、故障回溯        | 请求链路可视化、性能瓶颈定位 | 系统资源监控、实时告警  |
| **采集方式**     | 被动接收（Filebeat推送） | Agent主动上报           | 主动拉取（Pull模型）    |
| **存储系统**     | Elasticsearch            | Cassandra/Elasticsearch | 本地TSDB + 远程存储集成 |

---

### 二、功能重叠与互补性
1. **日志与链路追踪的关联**  
   • ELK 通过日志中的 `Trace ID` (需手动注入) 与 Jaeger 的追踪数据关联，实现日志与链路上下文的结合 (ELK 缺少 Trace ID 的问题可通过集成解决)。
   • **示例场景**：当某个微服务报错时，通过 Kibana 检索日志中的 `Trace ID`，跳转到 Jaeger UI 查看完整调用链路。

2. **指标与链路追踪的协同**  
   • Prometheus 监控系统级指标 (如 CPU、请求延迟)，而 Jaeger 提供请求级细粒度分析 (如某个 API 调用耗时分布)。两者结合可定位从宏观到微观的性能问题。
   • **集成方案**：通过 OpenTelemetry 将 Jaeger 的追踪数据转化为 Prometheus 可识别的指标 (如请求成功率)。

3. **数据存储的交叉性**  
   • Jaeger 和 ELK 均可选择 Elasticsearch 作为存储后端，实现数据统一管理 (Jaeger+ES 部署方案)。
   • Prometheus 的长期存储可接入 Elasticsearch 或 Thanos，与 ELK 形成互补。

---

### 三、典型技术栈组合
1. **基础监控方案**  
   Prometheus (资源指标) + Grafana (可视化) + Alertmanager (告警)，覆盖系统健康度监控。

2. **全链路可观测性方案**  
   ELK (日志) + Jaeger (追踪) + Prometheus (指标)，通过 `Trace ID` 和 `Service Name` 实现三类数据关联 (如网页 5、6 提到的 SkyWalking 与 ELK 集成思路类似)。

3. **云原生扩展方案**  
   引入 OpenTelemetry 标准化数据采集，将日志、指标、追踪统一接入后端存储 (如 Elasticsearch)，通过同一平台 (如 Kibana 或 Grafana) 展示。

---

### 四、选择建议
• **日志分析优先**：ELK 更适合处理海量日志的检索与分析 (如安全审计、业务日志分析)。
• **微服务诊断优先**：Jaeger 在分布式系统调用链可视化上更专业 (如跨服务延迟分析)。
• **实时告警优先**：Prometheus 的拉取模型和告警规则引擎更适合快速响应系统异常。

---

### 五、总结
三者看似功能重叠，实则在数据粒度 (日志 → 追踪 → 指标)、分析维度 (文本 → 调用链 → 数值) 和应用目标 (事后分析 → 实时诊断 → 预警) 上形成互补。**最佳实践是组合使用**：通过标准化字段 (如 Trace ID) 实现数据关联，构建覆盖日志、链路、指标的全方位可观测性体系。