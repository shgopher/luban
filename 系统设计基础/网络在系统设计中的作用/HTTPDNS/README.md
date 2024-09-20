<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:22
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-20 23:41:36
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/HTTPDNS/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# HTTP DNS 技术
传统本地 DNS 服务器使用 udp 的方式进行多级查询，每一层都有被钓鱼，被更换属于很多的风险，那么我们的本地 DNS 服务器是否可以改为使用 HTTP 协议呢？是否可以不需要逐层查询直接访问到目标服务器？HTTP DNS 就是基于 HTTP 协议的域名解析服务，它将域名解析请求发送到 HTTP 服务器，HTTP 服务器根据域名解析请求直接返回对应的 ip 地址，从而实现域名解析的功能，从而解决了传统 DNS 服务器多级缓存和 DNS 劫持的问题，你可以理解为 HTTP DNS 仿佛一个代理，它就拥有了所有域名解析的功能，并且可以做到无中间商，阿里的 [HTTP DNS](https://www.alibabacloud.com/help/zh/dns/what-is-alibaba-cloud-public-dns) 介绍在这里
## 在浏览器中使用 HTTP DNS
使用支持 HTTP DNS 的浏览器插件：

一些专门的插件可以在浏览器环境下实现 HTTP DNS 功能。这些插件通常会拦截浏览器的 DNS 请求，然后通过 HTTP 协议将请求发送到特定的 HTTP DNS 服务器进行解析。安装并启用这类插件后，插件会在后台自动处理域名解析，用户无需进行复杂的设置。例如，某些网络优化插件就具备这样的功能，它们可以改善网络访问速度和稳定性，同时防止 DNS 劫持。

浏览器厂商集成 HTTP DNS 功能：

部分先进的浏览器可能会在未来的版本中集成 HTTP DNS 功能。如果浏览器厂商决定支持 HTTP DNS，他们可以在浏览器的设置中提供相应的选项，让用户可以选择启用 HTTP DNS 解析。当用户开启此功能后，浏览器会自动使用 HTTP DNS 服务器进行域名解析，而不再依赖于传统的 DNS 系统。这样可以提高浏览器的安全性和性能，确保用户能够快速、准确地访问网站。

通过网络代理或 VPN 服务：

一些网络代理或 VPN 服务可能会内置 HTTP DNS 功能。当用户连接到这些代理或 VPN 时，它们可以拦截浏览器的网络流量，并使用自己的 HTTP DNS 服务器进行域名解析。这种方式虽然不是直接在浏览器中实现 HTTP DNS，但可以通过第三方服务来实现类似的效果。不过，使用网络代理或 VPN 也可能会带来一些其他问题，如网络速度下降、隐私问题等，因此需要谨慎选择可靠的服务提供商。
总的来说，目前在浏览器中使用 HTTP DNS 可能需要借助一些额外的工具或服务，但随着技术的发展，未来可能会有更多直接在浏览器中实现 HTTP DNS 的方法出现。