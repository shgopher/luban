<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:27
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-21 14:25:53
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/传输链路/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# 传输链路优化

虽然我们不能左右网络提供商例如联通移动的网络质量，但是我们可以通过一些手段来优化传输链路。

1. 用 HTTP2 替代 HTTP 1.1

尽管我们在 1.1 的版本中使用了例如 keep-alive 的方法去保持长连接，然后让所有的请求按照 FIFO 的方法去请求，进而减少三次握手所带来的时间延迟，但随之而来的队列阻塞问题也是一个很大的缺陷。

HTTP1.1 中流是最小的单位，在 1.1 中我们没办法将请求拆开，一起发送，因为客户端无法识别这些碎片，然而 HTTP2 帧 (frame) 是最小的单位，用来描述各种资源，每个帧只要带上唯一标识就能判断是同属一个流，客户端可以通过标识来组装成一个完整的流

通过 io 多路复用，每一个域名仅需一个 TCP 的连接，我们可以任意传输资源，因为不需要考虑 tcp 的延迟问题，所以我们也不用刻意缩小连接的次数，所以在 1.1 上把小数据改成大数据的优化方法在 2 上反而变成了反模式，不过 2 中传输大数据如果不分片的话，反而比 1.1 要慢，毕竟你单点人 1.1 是多路分布式传输肯定比你更快了。

应用层 HTTP 的传输层协议在 1.1 和2时代都是 TCP 协议，然而在 HTTP3 的时候我们将 TCP 协议替换为 UDP 协议，UDP 没有丢包重传的机制，UDP 传了就不管了，所以 http3 的可靠性不依靠传输层，而是在应用层上进行安全设置的，HTTP3 可以对每一个流都能单独控制，在 2 的时候 io 多路复用，TCP 协议接收到了大量的数据然后遇到了损坏，这个时候又要开始重传，这就是 HTTP2 传输大文件慢的原因，HTTP3 不使用 TCP 所以它没有这个缺陷，tcp 在连接时不可避免的会三次握手，这在游戏等领域存在了超时，中断，重连，这个过程带来的问题非常的难受，HTTP3 拥有连接标识符，这个唯一标识符就是标识了客户端和浏览器的连接 token，切换网络的时候，只需要向服务器发送一个包含该 token 的数据包就可以重新连接了，即便 IP 地址发生更改也可以连接

总结一下，搞什么优化，用 HTTP3 不香吗？

下面演示一下 HTTP3 tls 证书，ipv6 的静态服务器

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/quic-go/quic-go/http3"
    "log"
)

func main() {
    router := gin.Default()
    router.Static("/", "./static")

    // 指定现成的 TLS 证书和私钥文件路径
    tlsCertPath := "path/to/your/cert.pem"
    tlsKeyPath := "path/to/your/key.pem"

    server := http3.Server{ // 可以自动开启ipv6和ipv4
        Addr: ":8080",
        Handler: router,
        TLSConfig: generateTLSConfig(tlsCertPath, tlsKeyPath),
    }

    err := server.ListenAndServeTLS("", "")
    if err!= nil {
        panic(err)
    }
}

func generateTLSConfig(certPath, keyPath string) *http3.TLSConfig {
    return &http3.TLSConfig{
        Certificates: loadCertificates(certPath, keyPath),
    }
}

func loadCertificates(certPath, keyPath string) []http3.TLSConfigCertificate {
    cert, err := tls.LoadX509KeyPair(certPath, keyPath)
    if err!= nil {
        log.Fatalf("加载证书和私钥失败: %v", err)
    }
    return []http3.TLSConfigCertificate{cert}
}
```