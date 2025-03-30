# 聚合度量

聚合度量是指将多个数据点按照特定规则进行组合、合并或计算，从而得到有意义的统计指标的过程。

这一概念广泛应用于数据分析、系统监控。

聚合度量是对**原始数据的提炼**，通过数学运算 (如求和、平均值等) 将大量数据压缩为关键指标。例如计算服务器 CPU 使用率的平均值。输出的数据为监控和预警作决策支持。

通常我们在聚合度量领域要使用 Prometheus 这个工具，它是一个开源的系统监控和警报工具包。

通常来说，我们认为**聚合度量是一种对业务的一种监控**我们之前说的 APM 也就是链路追踪其实是一种对技术层面的监控。**业务监控 vs 技术层面的监控**

举个例子，我这网页怎么跳出去这么高，都到百分之 40 了，这就叫聚合度量，也就是业务层面的监控。我这个服务为什么内存占用这么高？这就是链路追踪要搞定的事情，**技术层面的监控 vs 业务层面的监控**。


prometheus 架构分为客户端指标收集、服务端存储查询、终端监控预警。
## 客户端指标收集
prometheus 通过客户端指标收集的方式，收集到我们的指标数据，然后存储到 prometheus 的存储中。

prometheus 主要收集以下指标数据：
- **计数类**的数据：比如库存数量，职工数量等计算数量的数据
- **瞬态数据**：比如网站在线人数，网站访问人数，也属于计数类的数据，但是强调顺时。
- **吞吐率数据**：比如 QPS TPS，是一种在单位时间内的吞吐量
- **直方图数据**：比如横坐标是时间，纵坐标是访问人数，这是一种二维统计图
- 采样点分布数据，比如正态分布的数据，等强调数据分布的数据

### 数据传输方法
这里主要是讲从客户端传递到服务端时传输的方法，推和拉，推就是客户端主动上报数据到服务端，拉就是服务端主动上报数据到客户端。

prometheus 一般是采用先推后拉的方式，先收集数据，数据主动上报到一个中转站，叫做 push gateway，然后 prometheus 再从 push gateway 拉取数据。这主要是为了某些场景而做的特殊设计，比如一些短生命周期的任务，比如一些离线任务，这些任务的生命周期很短，不适合一直保持连接，所以 prometheus 就设计了这种先推后拉的方式。或者 prometheus 的客户端处于内网状态，无法去拉数据，那么就需要 prometheus 的客户端主动推送到 gateway，gateway 再上报给 prometheus server。

prometheus 提出一种 exporter 的概念，这个 exporter 是一个中间件，可以将各种各样的数据转换成 prometheus 可以识别的数据格式，然后再上报给 prometheus server。只要你的软件集成了 prometheus client lib，那么就是一个 exporter。你也可以把 exporter 理解为 prometheus 客户端。

常见的 Exporter：

| 范围         | 常用Exporter                                                                 |
|--------------|-----------------------------------------------------------------------------|
| 数据库       | MySQLExporter、RedisExporter、MongoDBExporter、MSSQLExporter等               |
| 硬件         | ApcupsdExporter、IoTEdisonExporter、IPMIExporter、NodeExporter等             |
| 消息队列     | BeanstalkdExporter、Kafka Export、NSQ Explorer、RabbitMQExporter等           |
| 存储         | Ceph Exporter、Gluster Exporter、HDFS Exporter、ScaleIOExporter等            |
| HTTP服务     | ApacheExporter、HAProxyExporter等                                            |
| API服务      | AWS ECS Exporter、Docker Club Explorer Hub Exporter、GitHub Exporter等       |
| 日志         | FluentdExporter等                                                            |
| 监控系统     | CollectdExporter、GraphiteExporter、InfluxDB Exporter等                       |
| 其它         | Blackbox Exporter、JIRA Exporter、Jenkins Exporter、Confluence Exporter等     |


## 服务端存储查询

对于采集到的数据，他们拥有非常独特的特征，都是以时间为单位排列的，数据相对来说非常的简单，并不存在复杂的结构，比如：
```js
{
  // 时间戳
  "timestamp": 1599125492,
  // 指标名称
  "metric": "total_website_visitors",
  // 标签
  "tags": {
    "host": "steamaple",
    "job": "greenbird"
  },
  // 指标值
  "value": 10110
}
```
但同时数据量又是海量的，所以对于这种数据，首先我们的数据库需要支持海量数据的存储以及查询，其次，我们允许丢失非常早期的数据，毕竟我们要分析的数据都是最新的数据，所有的数据都是多写少读，都是顺序增加，不会乱序，支持再采样，也就是支持对数据进行分析加工来减少数据存储占用的空间。

那么这样的数据库就是时序数据库，prometheus 本身就可以当做一种时序数据库来使用。


## 终端监控预警
当我们收集数据并且存储数据之后，最重要的就是数据的分析以及监控预警，因为这是分析的结果，也是我们要的结果

大多是 Prometheus 配合 Grafana 来进行展示数据的，Grafana 是一个可视化工具，可以展示数据，并且可以进行数据分析，当然你可以使用 ELK 中的 Kibana，也可以使用其他的可视化工具，比如 Tableau，PowerBI 等。

另一个重要的功能就是预警，Prometheus 提供了专门用于预警的 Alert Manager，可以设置某个指标在多长时间内、达到何种条件就会触发预警状态，在触发预警后，Alert Manager 就会根据路由中配置的接收器，比如邮件接收器、Slack 接收器、微信接收器，或者 WebHook 接收器来自动通知



