# 43.7-shard-retry：分片倾斜与重试幂等

[43.7-延迟消息与定时消息](../../../v2/L4-组件与技术/43-消息队列/43.7-延迟消息与定时消息.md) 的配套代码（Python 3，仅标准库）。

```bash
cd tools/examples/43.7-shard-retry
python3 shard_retry.py
```

输出节选：

```text
分片倾斜（最大/平均，理想=1.00）
range(task_id):   13.01×
hash(key):        1.11×

失败率 5%、最多重试 3 次：无幂等重复执行 2501 次；有幂等重复 0 次
```

range(task_id) 倾斜 13.01×，hash(key) 1.11×；失败重试下无幂等重复执行 2501 次。
