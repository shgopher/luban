<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:18
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-20 17:58:34
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/DNS/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# DNS 域名协议服务
dns 是域名解析协议，它负责将域名解析为具体的 ip 地址，例如 www.example.com 解析为 11.32.33.433
## DNS 的多级寻找机制
dns 系统拥有本地缓存，所以它会优先查询本地缓存，这也是我们所谓的 “更改就能访问 xx” 的实现原理，因为 `hosts` 中保存的就是域名对应的 ip 地址，当本地缓存中存在，所以不会去请求 dns 服务器，直接返回具体的 ip，当本地缓存失效之后，系统就会按照域名的层级来查询
> hosts 不是本地缓存，系统会查询 hosts 然后再去查询本地缓存，不过 hosts 的优先级大于本地缓存，hosts 存在于 Linux 的 /etc/hosts Windows 的 C:\Windows\System32\drivers\etc\hosts 中 macOS 的 /etc/hosts 中

DNS 缓存存在于浏览器和操作系统中，hosts 也会存在一定的记录，其中优先级是 hosts 最高，浏览器缓存次之，操作系统缓存最低

在上述的描述中，当本地无缓存目标 URL 对应的 ip 时，通常会去使用本地域名服务商去查询，比如谷歌的 8.8.8.8，如果本地域名服务商有 URL 的缓存，返回数据即可

那么，如果本地域名服务商也没有数据，它会替代用户去不断的去请求更高一级的域名服务器，从 com 这个根域名服务器为开始，本地域名服务器就开始缓存代表了不同层级域名的 ip 查询地址

**这是一个域名解析的步骤图**
![dns](./dns.svg)

需要说明的是，权威服务器返回的不一定就是 ip 地址，也可以有很多[类型](https://zh.wikipedia.org/wiki/DNS%E8%AE%B0%E5%BD%95%E7%B1%BB%E5%9E%8B%E5%88%97%E8%A1%A8)

**这是一个 dns 记录的例子**

|Domin（域名）|TTL（生存周期）|Class（协议类型）|Type（记录类型）|Rdata（记录数据）|
|----|----|----|----|----|
|www.example.com|86400|IN|A|2.2.2.2|


## DNS 预留技术
```html
<link rel="dns-prefetch" href="//www.example.com">
```
dns 预留技术，通过在前端设置标签的方法，让浏览器提前对该域名进行预解析，从而减少请求时间
## DNS 的作用
dns 其实就是用户和真实 ip 之间的桥梁，用户使用域名通过 dns 来访问网站。

我们在部署 dns 的时候，可以部署多个入口 ip，这样当某个 ip 挂了，其他 ip 可以继续提供服务，从而保证了服务的可用性，并且实现了负载均衡的功能，如果我们突然要更换 ip，那么就可以通过 dns 服务内部更换 ip 对客户完全无感
## DNS 的安全性
最后，我们要知道，dns 服务通过多级缓存是，多级访问是有效的减少了不同层级的服务器的压力，但是同时也增加了被中间攻击的比例，分了多少层就会被攻击多少次，记不记得你浏览某个网站的时候你使用宽带上网和手机无线上网获取到的网页不一致的情况，就是因为被 dns 劫持了，某个正确 ip 被换成了广告 ip，广告 ip 将正常的网页 download 下来，然后加入广告发给你 (本来要去喝雪碧呢，tmd 饭店老板把雪碧买回来兑进去了好多白开水)

最近出现的 HTTP DNS 服务将原来的多级缓存多级查询的 DNS 服务给改成了基于一个 https 协议的查询服务器，一次性查询到 ip 地址，虽然增加了服务器开销和容积，但是确实减少了中间商，[下一节](../HTTPDNS/README.md)我们重点谈谈 HTTP DNS 服务

