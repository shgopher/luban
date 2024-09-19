<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:18
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-19 17:32:31
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/DNS/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# DNS 域名协议服务
dns 是域名解析协议，它负责将域名解析为具体的 ip 地址，例如 www.example.com 解析为 11.32.33.433
## DNS 的多级寻找机制
dns 系统拥有本地缓存，所以它会优先查询本地缓存，这也是我们所谓的 “更改 hosts 就能访问 xx” 的实现原理，因为 hosts 中保存的就是域名对应的 ip 地址，当本地缓存中存在，所以不会去请求 dns 服务器，直接返回具体的 ip，当本地缓存失效之后，系统就会按照域名的层级来查询

例如：www.example.com，当本地缓存中有 www.example.com 对应的 ip 地址时，直接返回 ip，当本地缓存中没有 www.example.com 对应的 ip 地址时，就会查询是否有 examole.com 对应的 ip 地址，如果有，那么就去这个 ip 去查询 www.example.com 对应的 ip，以此类推

那么假设本地毫无缓存，也就是说，不得不去查询。com 这个服务，这个时候就要引出来 “根域名服务器” 和 “权威域名服务器” 了，只有像 `.cn` `.com` 这种顶级域名才会存在于根域名服务器中，而且根域名服务器的地址无需查询，操作系统会内置在系统中

而像 `example.com` 这种子域名会存在于权威域名服务器中，当本地缓存为空的时候，自然会去根域名服务去查询。com，根域名服务器就会返回代表了。com 的权威域名服务器的 ip 地址，然后我们顺着这个路线自然就会找到对应的 ip 地址
## DNS 的多级缓存机制
在上述的描述中，当本地无缓存的时候，我们会不断的去请求更高一级的域名服务器，从。com 这个根域名服务器为开始，本地就开始缓存代表了不同层级域名的 ip 查询地址

比如 www.example.com，你向根域名请求。com 的 IP，根域名请求返回了 1.1.1.1 然后系统自动记录到了本地缓存中，哦，原来代表了。com 的 ip 地址就是 1.1.1.1 啊，以后就不用问根目标服务器了，然后继续请求。com 的服务器要查询 example.com 的 ip，假设是 2.2.2.2，那么就记录到本地缓存中，这样下次再请求 example.com 就不用再请求。com 了，直接返回 2.2.2.2，最后成功将 www.example.com 对应的 ip 地址记录到本地缓存中

需要说明的是，权威服务器返回的不一定就是 ip 地址，也可以有很多[类型](https://zh.wikipedia.org/wiki/DNS%E8%AE%B0%E5%BD%95%E7%B1%BB%E5%9E%8B%E5%88%97%E8%A1%A8)
## DNS 预留技术
```html
<link rel="dns-prefetch" href="//www.example.com">
```
dns 预留技术，通过在前端设置标签的方法，让浏览器提前对该域名进行预解析，从而减少请求时间

## DNS 的安全性
最后，我们要知道，dns 服务通过多级缓存是，多级访问是有效的减少了不同层级的服务器的压力，但是同时也增加了被中间攻击的比例，分了多少层就会被攻击多少次，记不记得你浏览某个网站的时候你使用宽带上网和手机无线上网获取到的网页不一致的情况，就是因为被 dns 劫持了，某个正确 ip 被换成了广告 ip，广告 ip 将正常的网页 download 下来，然后加入广告发给你 (本来要去喝雪碧呢，tmd 饭店老板把雪碧买回来兑进去了好多白开水)

最近出现的 HTTP DNS 服务将原来的多级缓存多级查询的 DNS 服务给改成了基于一个 https 协议的查询服务器，一次性查询到 ip 地址，虽然增加了服务器开销和容积，但是确实减少了中间商，[下一节](../HTTPDNS/README.md)我们重点谈谈 HTTP DNS 服务

