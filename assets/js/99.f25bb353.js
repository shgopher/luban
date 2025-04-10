(window.webpackJsonp=window.webpackJsonp||[]).push([[99],{552:function(t,s,n){"use strict";n.r(s);var a=n(36),p=Object(a.a)({},(function(){var t=this,s=t.$createElement,n=t._self._c||s;return n("ContentSlotsDistributor",{attrs:{"slot-key":t.$parent.slotKey}},[n("h1",{attrs:{id:"传输链路优化"}},[t._v("传输链路优化")]),t._v(" "),n("p",[t._v("虽然我们不能左右网络提供商例如联通移动的网络质量，但是我们可以通过一些手段来优化传输链路。")]),t._v(" "),n("ol",[n("li",[t._v("用 HTTP2 替代 HTTP 1.1")])]),t._v(" "),n("p",[t._v("尽管我们在 1.1 的版本中使用了例如 keep-alive 的方法去保持长连接，然后让所有的请求按照 FIFO 的方法去请求，进而减少三次握手所带来的时间延迟，但随之而来的队列阻塞问题也是一个很大的缺陷。")]),t._v(" "),n("p",[t._v("HTTP1.1 中流是最小的单位，在 1.1 中我们没办法将请求拆开，一起发送，因为客户端无法识别这些碎片，然而 HTTP2 帧 (frame) 是最小的单位，用来描述各种资源，每个帧只要带上唯一标识就能判断是同属一个流，客户端可以通过标识来组装成一个完整的流")]),t._v(" "),n("p",[t._v("通过 io 多路复用，每一个域名仅需一个 TCP 的连接，我们可以任意传输资源，因为不需要考虑 tcp 的延迟问题，所以我们也不用刻意缩小连接的次数，所以在 1.1 上把小数据改成大数据的优化方法在 2 上反而变成了反模式，不过 2 中传输大数据如果不分片的话，反而比 1.1 要慢，毕竟你单点人 1.1 是多路分布式传输肯定比你更快了。")]),t._v(" "),n("p",[t._v("应用层 HTTP 的传输层协议在 1.1 和2时代都是 TCP 协议，然而在 HTTP3 的时候我们将 TCP 协议替换为 UDP 协议，UDP 没有丢包重传的机制，UDP 传了就不管了，所以 http3 的可靠性不依靠传输层，而是在应用层上进行安全设置的，HTTP3 可以对每一个流都能单独控制，在 2 的时候 io 多路复用，TCP 协议接收到了大量的数据然后遇到了损坏，这个时候又要开始重传，这就是 HTTP2 传输大文件慢的原因，HTTP3 不使用 TCP 所以它没有这个缺陷，tcp 在连接时不可避免的会三次握手，这在游戏等领域存在了超时，中断，重连，这个过程带来的问题非常的难受，HTTP3 拥有连接标识符，这个唯一标识符就是标识了客户端和浏览器的连接 token，切换网络的时候，只需要向服务器发送一个包含该 token 的数据包就可以重新连接了，即便 IP 地址发生更改也可以连接")]),t._v(" "),n("p",[t._v("总结一下，搞什么优化，用 HTTP3 不香吗？")]),t._v(" "),n("p",[t._v("下面演示一下 HTTP3 tls 证书，ipv6 的静态服务器")]),t._v(" "),n("div",{staticClass:"language-go extra-class"},[n("pre",{pre:!0,attrs:{class:"language-go"}},[n("code",[n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("package")]),t._v(" main\n\n"),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("import")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"github.com/gin-gonic/gin"')]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"github.com/quic-go/quic-go/http3"')]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"log"')]),t._v("\n"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n\n"),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("func")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("main")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("\n    router "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v(":=")]),t._v(" gin"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("Default")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n    router"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("Static")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"/"')]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"./static"')]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n\n    "),n("span",{pre:!0,attrs:{class:"token comment"}},[t._v("// 指定现成的 TLS 证书和私钥文件路径")]),t._v("\n    tlsCertPath "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v(":=")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"path/to/your/cert.pem"')]),t._v("\n    tlsKeyPath "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v(":=")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"path/to/your/key.pem"')]),t._v("\n\n    server "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v(":=")]),t._v(" http3"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),t._v("Server"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token comment"}},[t._v("// 可以自动开启ipv6和ipv4")]),t._v("\n        Addr"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(":")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('":8080"')]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v("\n        Handler"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(":")]),t._v(" router"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v("\n        TLSConfig"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(":")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("generateTLSConfig")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("tlsCertPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" tlsKeyPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n\n    err "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v(":=")]),t._v(" server"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("ListenAndServeTLS")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('""')]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('""')]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("if")]),t._v(" err"),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v("!=")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token boolean"}},[t._v("nil")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("\n        "),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("panic")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("err"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n\n"),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("func")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("generateTLSConfig")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("certPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" keyPath "),n("span",{pre:!0,attrs:{class:"token builtin"}},[t._v("string")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v("*")]),t._v("http3"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),t._v("TLSConfig "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("return")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v("&")]),t._v("http3"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),t._v("TLSConfig"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("\n        Certificates"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(":")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("loadCertificates")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("certPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" keyPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n\n"),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("func")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("loadCertificates")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("certPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" keyPath "),n("span",{pre:!0,attrs:{class:"token builtin"}},[t._v("string")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("[")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("]")]),t._v("http3"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),t._v("TLSConfigCertificate "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("\n    cert"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" err "),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v(":=")]),t._v(" tls"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("LoadX509KeyPair")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),t._v("certPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" keyPath"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("if")]),t._v(" err"),n("span",{pre:!0,attrs:{class:"token operator"}},[t._v("!=")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token boolean"}},[t._v("nil")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("\n        log"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),n("span",{pre:!0,attrs:{class:"token function"}},[t._v("Fatalf")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("(")]),n("span",{pre:!0,attrs:{class:"token string"}},[t._v('"加载证书和私钥失败: %v"')]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(",")]),t._v(" err"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(")")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n    "),n("span",{pre:!0,attrs:{class:"token keyword"}},[t._v("return")]),t._v(" "),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("[")]),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("]")]),t._v("http3"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v(".")]),t._v("TLSConfigCertificate"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("{")]),t._v("cert"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n"),n("span",{pre:!0,attrs:{class:"token punctuation"}},[t._v("}")]),t._v("\n")])])])])}),[],!1,null,null,null);s.default=p.exports}}]);