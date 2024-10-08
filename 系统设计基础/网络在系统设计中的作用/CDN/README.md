<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:30
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-21 16:21:48
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/CDN/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# CDN 内容分发网络
我们是用 CDN 就是要解决三个问题
1。网站服务器接入网络运营商带宽有限的问题
2。网络各个中间节点连接太长的问题
3。网站到用户延迟太长的问题

当我们的网站将源地址发送给 CDN 的时候，CDN 服务商就会给你一个 CNAME 网址，例如 shgopher.cdn.github.io
> CNAME：域名别名

**你得到的 CNAME 会在你的 DNS 服务商注册一条 CNAME 记录**

当第一次未命中 CDN 地址的 DNS 查询，域名服务商解析 CNAME 后会返回给本地 DNS 缓存

当你为你的网站资源部署 CDN 的时候，CDN 域名会在 DNS 或者 HTTP DNS 系统中返回 N 个 IP 地址，这些是位于全国或者全球各个部分，距离用户最近的 IP 地址，所以使用 HTTP DNS  + CDN 才能获取最佳的体验

这个时候你访问的 CDN 就会去访问源站，CDN 会将请求转发到源站，源站返回数据，CDN 将数据返回给用户，然后 CDN 就会缓存数据，下次访问的时候直接从缓存中读取，从而减少请求的次数，从而提高网站访问速度

关于 CDN 请求源站的方法其实有两种，一种是被动的，等 cdn 访问就行了，另一种是主动的，就是在部署 cdn 的时候就同时主动去访问源站来形成缓存，一般都采用主动触发加上被动设置缓存失效时间的方式。

## CDN 寻找最近的 ip
CDN (内容分发网络) 在众多 IP 地址中选中一个距离用户最近的 IP 主要通过以下几种方式：

**一、地理定位**

CDN 通常会利用 IP 地址定位技术来确定用户的大致地理位置。每个 IP 地址都可以被映射到一个特定的地理区域，CDN 系统会维护一个庞大的 IP 地址数据库，通过查询这个数据库，能够快速判断用户的位置。例如，根据 IP 地址的分配规则，可以确定某个特定 IP 段来自亚洲地区，更具体地可能来自中国的某个省份或城市。

**二、网络测量和延迟评估**

***主动测量***

CDN 节点会定期向不同的网络位置发送探测数据包，以测量网络延迟和带宽等参数。这些探测可以针对特定的 IP 地址范围或者广泛的网络区域进行。例如，CDN 节点会向各个主要的网络运营商的骨干节点发送探测包，以了解到不同地区的网络状况。
通过这些主动测量，CDN 可以建立一个网络延迟矩阵，记录从每个 CDN 节点到不同地理位置的网络延迟情况。当用户请求内容时，CDN 系统可以根据用户的地理位置，查询这个延迟矩阵，找到延迟最小的 CDN 节点对应的 IP 地址返回给用户。

***被动测量***

当用户首次访问 CDN 服务时，CDN 系统可以在后台记录**用户的请求路径和延迟信息**。

例如，记录用户请求经过的网络路由器、交换机等设备的 IP 地址和延迟时间。通过对大量用户请求的被动测量，CDN 系统可以不断优化对不同地区用户的 IP 地址选择策略。如果发现某个地区的用户普遍对某个特定的 CDN 节点响应速度较快，那么在后续的请求中，对于来自该地区的用户，系统就更有可能选择这个节点的 IP 地址。

**三、DNS 智能解析**

***负载均衡考虑***

除了距离因素外，CDN 的 DNS 系统还会考虑各个 CDN 节点的负载情况。如果某个距离用户较近的 CDN 节点负载过高，系统可能会选择一个稍微远一些但负载较低的节点，以确保用户能够获得快速稳定的服务。例如，当多个用户同时请求同一内容时，DNS 系统会动态地分配请求到不同的 CDN 节点，以平衡负载。


## CDN 应用内容

1. 静态资源：如 CSS、JS、图片等，这些资源可以缓存一段时间，减少请求次数，提高网站性能。
2. 安全防御：CDN 可以作为堡垒机来防范攻击对于源站的影响
3. 协议隔离：很多源站是 http 协议，但是又想使用 https，CDN 可以做协议转换，将 http 协议转换为 https 协议
4. 缓存控制：CDN 可以根据缓存策略来控制缓存时间，例如，对于静态资源，可以设置缓存时间为一年，对于动态资源，可以设置缓存时间为10分钟。
5. 修改资源：CDN 可以修改源站的资源，例如，将源站的图片压缩，将源站的 CSS、JS 合并，将源站的 HTML 压缩等，从而提高网站性能。
6. 访问控制：CDN 可以实现白名单，黑名单的控制，可以根据 ip 的访问流量去限流控制
7. 注入功能：CDN 可以在不改变源站的配置的情况下，向源站注入自定义的脚本，例如，注入广告代码，统计代码等。
8. 充当 VPN 的功能
