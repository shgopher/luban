<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-11-02 22:37:04
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-11-08 15:45:43
 * @FilePath: /luban/系统设计基础/分布式/分布式算法/raft/README.md
 * @Descripti
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# raft

raft 是一种 multi paxos 算法。

paxos 麻烦的一批，所以我们在使用强一致性方案的时候通常首选是 raft 算法或者是 raft 算法的变形而不是 paxos。

在 raft 中角色有三个
- 领导者
- 跟随者
- 候选人

有领导的时候就是领导和跟随者的关系，领导死了，就是全部都是候选人了，谁选上了就又变成了领导，其他的候选人又变成了跟随者，所以同时存在的关系最多只有两个，情景一【领导者-跟随者】，情境二【候选人】。



