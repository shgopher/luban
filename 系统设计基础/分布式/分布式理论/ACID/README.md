<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-10-27 23:45:14
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-10-27 23:50:48
 * @FilePath: /luban/系统设计基础/分布式/分布式理论/ACID/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# ACID
ACID 的含义是：Atomicity (原子性)、Isolation (隔离性)、Durability (持久性)，Consistency (一致性)，你可以理解为这个理论就是 CAP 中的 CP 模式

在分布式中如果追求 CP 模式，通常是二阶段提交协议，以及 TCC try-confirm-cancel，以及满足幂等性