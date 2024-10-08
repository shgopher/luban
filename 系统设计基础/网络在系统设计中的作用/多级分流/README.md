<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-16 09:31:26
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-16 10:04:02
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/多级分流/README.md
 * @Descri分流
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# 系统的多级分流
我们使用的现在互联网系统，都存在多个分流的装置，从最开始的 DNS 开始就可以部署分流，继而去到网关，负载均衡器，缓存，服务集群，数据库集群，等等。

分流系统根据所在的部位不同也可以分为不同的种类

- 客户端等位于用户最近的部分，例如 http 本地缓存，可以减少后端的 io 数量，dns 缓存，减少 dns 的请求数量，cdn，反向代理，这些都是位于客户端的分流系统
- 可线性扩展的构建，比如分布式缓存，数据库集群
- 运维构建，例如，注册中心，配置中心，这种辅助的系统，对于整个系统至关重要，对于这些影响全局的运维构建，必须部署集群，来时刻提高容灾能力
- 容易形成单点的部件，必须提高竖向能力，也就是提示该机器的性能，比如负载均衡器，网关，入口路由

## 设计要点
- **减少单点服务的数量，提高单点服务的性能**
- **最大程度减少达到单点部件的流量，尽可能让需求分流**，比如一个数据，http 缓存可以提供，cdn 可以提供，web 服务器可以提供，后端服务可以提供，数据库可以提供，那么就应该让各个组件分流这些请求，能用 http 缓存的流量就别后面请求，能让 cdn 提供的，就分流一部分给 cdn。
- **简单的系统才是好系统**，在满足需求的情况下，系统越简单越好，组件越多，出错的几率越高

