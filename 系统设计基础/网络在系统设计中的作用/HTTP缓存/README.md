# HTTP 缓存
HTTP 缓存指的是不经过后端，客户端直接根据缓存的内容对目标网站状态进行判断。

常见的 HTTP 缓存的典型应用就是 301 重定向，你会发现如果你不手动删除浏览器缓存，服务器设置的新的 301 重定向，浏览器还是读取旧的缓存，而不是重新请求。

## HTTP 强制缓存
强制缓存，强制你去缓存，设置一个时间，这个时间内，你后端不管咋变，反正我都给你缓存了
- Expires
- Cache-Control

用后面那个，前面那个设置的不完善，设计的 bug 很多
## 协商缓存
协商缓存跟强制缓存不同，它没有规定时间段内必须缓存的机制，当你设置参数时它才会去缓存，所以属于协商机制

- Last-Modified if-modified-since：Last-Modified 服务器响应时返回的 header，告知客户端这个资源的最后修改时间，当客户端再次请求的时候会通过 if-modified-since 把之前收到的资源最后修改时间传给服务器，服务器根据这个时间判断是否需要返回最新资源，如果不需要返回最新资源，则返回 304

比如 last-modified：Wed，08 Jun 2016 08:05:08 GMT，最后的修改时间是 8 点5分8秒，那么客户端的下次请求就将这个时间发送给服务器，服务器去查看资源有没有更新的版本，如果没有就返回 304，如果有就返回新的资源和新的 last-modified

- ETag 和 if-none-match 跟上面的那一对差不多其实，只是这个 Etag 是一个资源唯一标识，如果客户端发送的 if-none-match 和这个 ETag 相同，则返回 304，服务器根据这个 ETag 判断是否需要返回最新资源

那么 Etag 机制和 Last-Modified 机制有什么区别呢？**Etag 精度高但是性能差，因为它需要计算哈希值嘛，而 Last-Modified 精度低但是性能高，它只需要比较时间戳**

### 内容协商机制
在 http 协议中，一个 url 地址是可能根据比如语言不同，压缩方法不同，一个 URL 返回不同的资源的，所以我们需要设置 Accept-*和 content-* 去协商内容的，使用 Vay 标识符去设置协商内容

```js
//客户端请求：
GET /resource HTTP/1.1
Host: example.com
Accept-Language: zh-CN,zh;q=0.9
User-Agent: [Your User-Agent String]


// 服务器响应：
HTTP/1.1 200 OK
Vary: Accept-Language, User-Agent // Vay是服务器告知客户端，服务器将根据vay后面的字段来返回不同的资源内容
Content-Language: zh-CN
Content-Type: [Appropriate Content-Type]
[Resource Content in Chinese Simplified]

```

原理：当客户端发送请求时，Accept-Language 传达了语言偏好，同时 User-Agent 可以提供关于客户端类型等信息。服务器在响应时，将 Accept-Language 和 User-Agent 放在 Vary 响应头后面，表示缓存机制需要根据这两个请求头的值来决定是否返回缓存的资源。如果不同的客户端发送的 Accept-Language 或 User-Agent 不同，服务器可能会返回不同版本的资源。这样可以确保在不同语言偏好和不同客户端类型的情况下，都能为用户提供最合适的资源内容。

### 协商机制的失效

协商机制在地址输入，跳转，甚至刷新的时候都会生效，如果想让失效，只能强制刷新 control+ F5 或者禁止缓存才会失效，失效的话，客户端就会向服务器发送 “Cache-Control：no-cache” 的标志，服务器如果获取这个标识就会发生新的资源

### 协商机制的局限性
http 协议并非强制性协议，服务器也可以完全不 care 这个 no-cache 标志，不过这属于服务器设计与实际预期不一致的情况，属于设计不规范的行为。