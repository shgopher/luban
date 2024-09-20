<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:22
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-21 00:42:31
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/HTTPDNS/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# HTTP DNS 技术
传统本地 DNS 服务器使用 udp 的方式进行多级查询，每一层都有被钓鱼，被更换属于很多的风险，那么我们的本地 DNS 服务器是否可以改为使用 HTTP 协议呢？是否可以不需要逐层查询直接访问到目标服务器？HTTP DNS 就是基于 HTTP 协议的域名解析服务，它将域名解析请求发送到 HTTP 服务器，HTTP 服务器根据域名解析请求直接返回对应的 ip 地址，从而实现域名解析的功能，从而解决了传统 DNS 服务器多级缓存和 DNS 劫持的问题，你可以理解为 HTTP DNS 仿佛一个代理，它就拥有了所有域名解析的功能，并且可以做到无中间商，阿里的 [HTTP DNS](https://www.alibabacloud.com/help/zh/dns/what-is-alibaba-cloud-public-dns) 介绍在这里
## 使用 dns 的弊端
1. 传统的 local DNS 服务尽管拥有缓存机制，但不可避免的还是会有递归查询的问题，这严重的影响了响应时间
2. 权威 dns 服务是针对本地域名服务器的 ip 去返回 ip 的，这跟真实用户的 ip 地址不同，严重影响了负载均衡的本意
3. DNS 劫持，给你一个假网站，运营商通过本地域名服务器直接给你造假 DNS 劫持，黑客选择篡改计算器的 Hosts 去 dns 劫持
## HTTP DNS 的原理
客户端使用 http DNS 去解析域名，HTTP DNS 如果有缓存直接返回，如果没有它会去权威 DNS 服务器发起域名解析请求，并返回最优 ip，这其中减少了去根 dns，顶层 dns 的访问，客户端可能发送一个类似于 “http://11.1.1.1/?dn=[domain_name]” 的请求，其中 “[domain_name]” 是要解析的域名，如果你需要更加安全，也可以将此处的 http 改成 https

客户端通过访问一个固定的 HTTP DNS IP 地址去解析域名，不是通过一个域名 (如果通过域名还不是还得解析这个域名的 ip 嘛。。。对吧)

为了保证高性能和高可用，HTTP DNS 通过 BGP 边界网关协议让这个 ip 地址让全国的运营商客户都能就近访问，同时多个数据中心部署多个 HTTP DNS 服务节点，主备方案，任意节点故障均可切换备份节点
## 配置高可用的 DNS 服务 --- 包括 HTTP DNS 和 local DNS

1. 首先我们优先在客户端布置 HTTP DNS 服务
2. 为了防止 HTTP DNS 服务器故障，我们可以在 HTTP DNS 后面部署传统的 local DNS 服务
3. HTTP DNS 服务可以返回多个 ip 地址，我们可以逐个测试连通性，不通就换下一个
4. 如果本地 local DNS 服务也失败了，我们为了高可用，可以在 app 中硬写入一个映射表，写入一些兜底的 ip 地址
5. 预请求 IP 并且缓存起来
## 传统使用 UDP 协议的 DNS 服务请求代码

```py
import struct
import socket

def send_dns_query(domain_name, query_type):
    # 创建 UDP 套接字
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM) # 从这看出来 使用的是 UDP 的传输协议

    # 设置 DNS 服务器地址和端口
    dns_server = ("8.8.8.8", 53)

    # 构造 DNS 请求报文
    transaction_id = 0x1234  # 可以设置一个随机的事务 ID
    flags = 0x0100  # 标准查询，递归查询标志位设为 1
    num_questions = 1
    num_answers = 0
    num_authority_rrs = 0
    num_additional_rrs = 0

    # 处理域名，采用域名压缩格式
    parts = domain_name.split('.')
    encoded_domain = b""
    for part in parts:
        encoded_domain += bytes([len(part)]) + part.encode()
    encoded_domain += b"\x00"

    query_type_code = {
        "A": 1,
        "AAAA": 28,
        # 添加其他查询类型的代码映射
    }
    qtype = query_type_code[query_type]
    qclass = 1  # IN（Internet）类

    request = struct.pack(">HHHHHH", transaction_id, flags, num_questions, num_answers, num_authority_rrs, num_additional_rrs)
    request += encoded_domain
    request += struct.pack(">HH", qtype, qclass)

    # 发送请求
    sock.sendto(request, dns_server)

    # 接收响应
    response, _ = sock.recvfrom(4096)

    # 处理响应
    #...（这里省略处理响应报文的具体代码）

    sock.close()
```
## 在浏览器中使用 HTTP DNS
使用支持 HTTP DNS 的浏览器插件：

一些专门的插件可以在浏览器环境下实现 HTTP DNS 功能。这些插件通常会拦截浏览器的 DNS 请求，然后通过 HTTP 协议将请求发送到特定的 HTTP DNS 服务器进行解析。安装并启用这类插件后，插件会在后台自动处理域名解析，用户无需进行复杂的设置。例如，某些网络优化插件就具备这样的功能，它们可以改善网络访问速度和稳定性，同时防止 DNS 劫持。

浏览器厂商集成 HTTP DNS 功能：

部分先进的浏览器可能会在未来的版本中集成 HTTP DNS 功能。如果浏览器厂商决定支持 HTTP DNS，他们可以在浏览器的设置中提供相应的选项，让用户可以选择启用 HTTP DNS 解析。当用户开启此功能后，浏览器会自动使用 HTTP DNS 服务器进行域名解析，而不再依赖于传统的 DNS 系统。这样可以提高浏览器的安全性和性能，确保用户能够快速、准确地访问网站。

通过网络代理或 VPN 服务：

一些网络代理或 VPN 服务可能会内置 HTTP DNS 功能。当用户连接到这些代理或 VPN 时，它们可以拦截浏览器的网络流量，并使用自己的 HTTP DNS 服务器进行域名解析。这种方式虽然不是直接在浏览器中实现 HTTP DNS，但可以通过第三方服务来实现类似的效果。不过，使用网络代理或 VPN 也可能会带来一些其他问题，如网络速度下降、隐私问题等，因此需要谨慎选择可靠的服务提供商。
总的来说，目前在浏览器中使用 HTTP DNS 可能需要借助一些额外的工具或服务，但随着技术的发展，未来可能会有更多直接在浏览器中实现 HTTP DNS 的方法出现。
## 在 app 中使用 HTTP DNS
许多流行的网络请求库 (如 OkHttp、Volley 等) 支持自定义 DNS 解析器。可以通过编写自定义的 DNS 解析器来实现 HTTP DNS 功能。

```java
OkHttpClient client = new OkHttpClient.Builder()
   .dns(new MyHttpDnsResolver())
   .build();
```
全局设置 http dns
```java
HttpDnsManager.getInstance().init("your_http_dns_server_url");
```