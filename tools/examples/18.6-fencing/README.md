# 18.6-fencing：脑裂与 fencing

[18.6-脑裂与防护](../../../v2/L1-分布式原理/18-数据复制/18.6-脑裂与防护.md) 的配套代码（Python 3，仅标准库）。

```bash
cd tools/examples/18.6-fencing
python3 sim.py
```

输出节选：

```text
实现                            旧主写请求  被接受    被拒    重复写次数  重复的 key          重复入账
无 fencing（到达即接受）        13          13        0       3           k_11,k_12,k_13      300 元  
有 fencing（先 +token 再提升）  13          10        3       0           —                   0 元    
有 fencing（顺序写反：先提升）  13          11        2       1           k_11                100 元  

校验：重叠窗口 = A_STOP(13) - PROMOTE(11) + 1 = 3 秒
      无 fencing 的重复写应等于重叠窗口：3
断言全部通过（3 行结果可复算）
```

三种实现对比：无 fencing 重复入账 300 元；先 +token 再提升，重复 0 元；顺序写反仍漏 1 次。
