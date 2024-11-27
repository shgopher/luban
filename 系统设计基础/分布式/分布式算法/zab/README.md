<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-11-02 22:37:29
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-11-24 00:59:19
 * @FilePath: /luban/系统设计基础/分布式/分布式算法/zab/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# zab

zab 协议是一种对于 multi paxos 的一种改进，multi paxos 无法保证各个值操作的顺序性，也不关心达成共识的值是什么，zab 协议就是对于这两点的一种改进。

