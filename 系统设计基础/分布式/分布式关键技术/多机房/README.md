<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2025-05-20 18:25:09
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2025-05-20 23:24:10
 * @FilePath: /luban/系统设计基础/分布式/分布式关键技术/多机房/README.md
 * @Description
 * 
 * Copyright (c) 2025 by shgopher, All Rights Reserved. 
-->
# 多机房

使用单机房搭建后台集群的方案，虽然接入层，业务层，存储层都具备高可用的架构，但是由于机房是单点，一旦发生了故障，整个集群还是无法提供服务。
## 主备方案
解决机房单点问题的最简单的方案就是主备方案，在住机房所在的城市再搞一个备份的机房，整个备份机房中拥有主机房的所有内容，在正常情况下，仅仅主机房在工作。

在存储层，备份机房中的存储内容可以作为主机房存储的从库，也就是说，从库并不部署到同一个机房，直接部署于备份机房即可，不过由于存储内容的重要性，我们应该将主机房的存储内容和从机房的存储内容做专线网络，来尽可能的保证存储数据的一致性。

为了保证专线的高可用行，专线可以不是一根，可以多部署几根。

当主机房出现问题时，可以使用 DNS 服务，直接切换为从机房来提供业务。
## 同城双活
同城双活就是双主方案，不再存在一个备份的基本上不使用的机房了，我们的两个机房都提供服务。不过你可能注意到了，这两个机房的存储可是一个是主一个是备，那么 b 机房如果也要活跃使用的话，它是一个只读存储器，该如何写入数据呢？答案就是当 b 机房有写的需求时直接访问主机房的存储器，将写的内容写入到主存储器中，然后从存储器中读取数据同步给从存储器。

如果采用同一个城市，并且 ab 机房之间还是专线网络的情况下，基本上跨机房访问的网络延迟只会有稍微的增加，基本上可以忽略不计，大可当成一个机房来处理。并且在现实生活中，写的需求的确是原小于读的需求。

我们也可以部署多个机房，形成同城多活的局面，之所以强调同城，其实还是因为网络延迟问题，跨机房的网络延迟是不可避免的，尤其是跨国的机房之间，网络延迟会非常大。如果是物理位置非常近的情况下，就可以极大的避免网络延迟的问题了，所以我们才会强调同城。

我们在部署多活方案时，只要在存储层部署一个主存储器就可以了，其他的机房都可以作为从存储器来使用。这样就可以避免在存储层部署多个主存储器的问题了。一主多备可以避免数据的不一致问题。

部署同城双活的时候，我们要在客户端部署分流政策，可以使用设备 id 的哈希值去分流，并且在 ab 机房都部署一个分流服务，分流服务数据保证强一致性，当客户端访问该服务时就可以知道去哪个机房，并且当某个机房出现故障时就可以自动切换另一个机房的分流服务来保证数据的切换。

当 b 机房，部署从存储器的机房出现问题时，我们可以直接切换为全部使用 a 机房数据，但是当 a 机房出现问题时，我们在切换流量之后还需要将 b 机房的存储器切换为主存储器，这样才能保证 b 机房可以继续提供服务。

实际上分流系统也可以设计在 HTTP DNS 中，或者其他的接入层。不过设计在客户端中还是比较常见的做法，服务器端也需要做相应的分流设计即可 (该分流设计就是配置一个分流服务即可)
## 两地三中心
其实就是在考虑整个城市出问题的情况，在同城多活的基础之上再在一个遥远的城市部署一个平时不提供服务的机房。感觉用途不大，还是那句话那个备份的平时不用的机房属实是浪费，并且平时不用的机房很容易出现各种问题。
## 异地多活
这种方案主要是全球业务，因为同城多活会带来延迟问题，比如你部署在美国的服务器在日本访问延迟就会比较高。也就是说在全球各地部署多个机房，各个机房都是主存储模式，我们在各个主存储的情况下互相传递数据。