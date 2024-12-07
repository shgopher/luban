# 凭证
- Coookie-Session
- JWT

所谓凭证其实就是在前面章节授权中提到的令牌。

凭证存储在哪里一直都有争议，客户端还是服务端？以 http 协议的 cookie session 机制来说，服务端存储令牌实际的内容，客户端仅仅将存储代表令牌的名字存放在 cookie 中。

不过在分布式架构中，我们采用的 JWT 方案则是存储在了客户端上。

## Cookie-Session
一般来说，系统会把状态信息保存在服务端，在 Cookie 里只传输的是一个无字面意义的、不重复的字符串，习惯上以 sessionid 或者 jsessionid 为名

服务器拿这个字符串为 Key，在内存中开辟一块空间，以 Key/Entity 的结构存储每一个在线用户的上下文状态，再辅以一些超时自动清理之类的管理措施。

在单点方案的架构中，cookie session 非常的好用，因为它将最危险的数据都保存在了服务器中，并且依靠客户端的同源策略 (同源策略：同源是指协议、域名、端口号相同) 来保证数据的安全性。）已经 https 加密来保证整个过程的安全，并且，服务端可以主动的控制整个状态的管理，比如可以强制某个用户下线。

但是在分布式架构中，cookie session 就存在问题了。因为它无法进行水平的扩展，如果部署的是一个集群，由于 session 基本上存在于内存中，如果要部署集群它必须经过改造，比如

- 牺牲集群的一致性，将均衡器采用亲和式 (亲和式：将来自同一个客户端或会话的请求，在一定时间内持续地发送到同一个后端服务器) 的负载均衡算法，每一个节点都保存不同的用户状态，但是一旦某个节点失效，数据状态完全丢失
- 牺牲集群的可用性，所有节点都保存全部的信息，比如可以使用传播的方案让所有数据都最终一致 (比如 gossip 算法)，但是存储数据很大，同步成本也非常的高
- 牺牲集群的分区容错性，所有集群共同使用一个单点存储 session，但是一旦该节点失效，数据状态完全丢失，不过倒是可以将这个所谓的单点改成一个集群，这样就可以解决问题了。
## JWT

cookie -session 无法适配一般的分布式架构，还有，如果是多方系统呢？就更不可能使用 cookie session 了，即便是服务端之间共享了数据，客户端的 cookie 也无法共享跨域，所以当服务器存在多个，但是客户端只有一个的时候，把状态保存在客户端才是唯一的选择，不过信息泄露的问题必须解决，那么就是使用 JWT

JWT (JSON Web Token) 是一种用于在分布式系统中传递和验证身份信息的开放标准，经常跟第二章中的 oauth2 协议一起使用。

一个 jwt 案例
```bash
GET /restful/products/1 HTTP/1.1
Host: n.cn
Connection: keep-alive
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX25hbWUiOiJpY3lmZW5peCIsInNjb3BlIjpbIkFMTCJdLCJleHAiOjE1ODQ5NDg5NDcsImF1dGhvcml0aWVzIjpbIlJPTEVfVVNFUiIsIlJPTEVfQURNSU4iXSwianRpIjoiOWQ3NzU4NmEtM2Y0Zi00Y2JiLTk5MjQtZmUyZjc3ZGZhMzNkIiwiY2xpZW50X2lkIjoiYm9va3N0b3JlX2Zyb250ZW5kIiwidXNlcm5hbWUiOiJpY3lmZW5peCJ9.539WMzbjv63wBtx4ytYYw_Fo1ECG_9vsgAn8bheflL8

```

请注意这里的一大串数据并不是加密，而是转码，base64 转码而已，所以 jwt 是不加密的，它只解决了篡改的问题，至于为什么不加密，主要是加密也没有必要。

那么 jwt 的数据分为三个部分

令牌头：

```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```
主要是描述了令牌的类型 (type：JWT)，以及令牌的算法 (alg：HS256)

负载 payload：

```json

```
签名 signature：
```json

```
