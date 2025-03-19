# 日志处理

现代化分布式系统中的日志处理主要分为以下几个部分
- 应用输出日志
- 采集日志
- 缓冲
- 聚合，加工
- 索引存储
- 分析查询

我们使用常见的分布式日志系统 **Elastic Stack** (Elastic 家族软件) 来说明日志处理的流程。

Elastic Stack 主要包含以下几部分 (按照流程排序)：
1。Beats 日志收集工具
2。Logstash 日志处理、传输工具
3。Elasticsearch 分布式搜索引擎，日志索引，存储，查询，分析引擎
4。Kibana 数据可视化工具，用于展示 Elasticsearch 中的数据，是 Elasticsearch 的 GUI。
## 应用输出日志

