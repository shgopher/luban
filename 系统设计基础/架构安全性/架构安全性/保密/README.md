<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-12-07 14:30:59
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-12-18 00:12:52
 * @FilePath: /luban/系统设计基础/架构安全性/架构安全性/保密/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# 保密

- 保密的强度
- 客户端加密
- 密码的存储和验证

根据加密的位置来划分，加密和解密分为客户端保密，传输保密，服务端保密
## 保密强度
保密是有成本的，保密的等级越高花费的工作量和资源就越多，所以在设置加解密的时候还是要充分考虑信息的重要程度将保密分级。

- 摘要替代明文，也就是说我们保存的不是经过哈希加密后的密码，而是保留该哈希值的摘要，即使密码泄露了，也不会影响其它系统的使用，不过它无法防止弱密码被彩虹表攻击
- 给密码先加上盐 (一个随机字符串) 然后再获取该字符串的哈希值，这种方法可以防止彩虹表的攻击，但是不能阻止加密结果被窃取之后，攻击者直接发送该加密后的结果给服务器进行冒领的行为。
- 将盐值作为动态值，这样就可以有效的防止冒领的行为，每次向服务器传递的加密后的值都不一样。我个人不推荐这种方式，因为使用动态盐值基本上就等同于你服务器要存储明文密码了。
- 给服务加上**动态**的令牌，在网关或者其他的流量公共位置建立校验逻辑，可以防止重放攻击，每次请求的时候都携带一个令牌，服务端根据令牌来判断是否是同一个用户，如果用户没有登录，就返回错误，不过这样还是不能防止在传输过程中的数据被嗅探的泄露信息的问题。
- 启用 https 来防止恶意的嗅探，也能在通信协议的层面解决重放攻击的行为，不过，也会出现有些客户端被攻击产生伪造的根证书的问题，也有服务器被攻破产生的证书泄露问题，tls 协议加密不足产生的风险问题
- 使用独立于客户端的存储证书的加密物理设置 (比如银行的 u 盾)，大型网站设计到金钱的时候会使用双重验证的方式 (比如二次验证密码，接收手机验证码等方法)，最强的保密措施，建立一个内网，隔绝一切的对外开放的接口 (比如军队的内网)

通常这些手段中
- 使用 https 是最常见也最管用的首选方法，
- 其次就是使用动态令牌，这个令牌是服务器分发给客户端，客户端请求的时候，服务器就可以去甄别是否是自己信任的客户端，
- 最基础的密码 + 盐值加密，取哈希摘要应该是**最低的加密手段**，不能比这个更低了。
## 客户端加密
为了保证不被黑客窃取而做客户端加密是没有意义的。因为网络传输过程中充满了各种中间人，各个节点都有可能被劫持，比如 http 代理，路由器攻击 (一个本身就有后门的坏蛋路由器)，dns 劫持，

对于绝大多数的系统来说，启用 https 是唯一可行的客户端安全方案。

不过为了防止服务器端的做的垃圾，比如被拖库，密码明文被写进日志中等低级错误，做系统设计的时候在客户端加密也不是不行。

目前有很多的设备，比如手机，它的密码就是存储在客户端上，为了鉴别用户就是由手机系统密钥加密的，然后通过加密后的密码去请求服务器，服务器端通过解密密码来验证用户。我们在设计系统的时候没必要优先考虑 web 端，毕竟这个年代使用浏览器取访问服务器的少之又少。
## 密码的存储和验证
我们使用一个具体的案例来讲解存储和验证，在这里我们的密码保密等级是中等或者普通。指的是安全的加密但是并不涉及到大量的资源浪费

首先我们对保密要进行一个**基础规定**，不管是什么等级的保密措施，下面两条必须要满足：

首先我们要对用户的密码做一些规定：

- 首先我们要规定客户使用的密码难度，比如必须是超过某个长度
- 必须拥有特殊字符
- 必须包含大写和小写字母等

其次，客户端和服务器之间的网络协议必须开启 https。

这两条是现代标准架构设计中的铁律。

下面开始介绍中等保密措施的实现过程：

1. 用户在客户端注册，输入明文密码

    ```ts
    let password = 'uyuiII3@'
    ```
2. 开始客户端加密 (客户端加密的原因是不想让明文密码被服务端获取，造成明文密码的泄露)，我们可以选例如 MD5，sha1，sha256，bcrypt，pbkdf1/2 等加密算法
    ```ts
    let passwordHash = md5(password)
    ```
3. 为了防止被彩虹表攻击，我们在客户端的时候只需要使用固定的字符串即可，比如日期，或者是用户名称
    ```ts
    let salt = 'my_user_name'
    let client_hash = MD5(passwordHash + salt)
    ```
4. 这是对 3 的补充和替代，如果攻击者截获了客户端发送的信息，得到了摘要和 salt，然后使用彩虹表对密码进行暴力破译，我们使用慢函数来防止彩虹表攻击 (**因为慢函数通常要消耗大量的资源，所以一般放在客户端去处理**)，比如 bcrypt，pbkdf2，scrypt 等。慢哈希函数是指这个函数执行时间是可以调节的哈希函数，通常是以控制调用次数来实现的，它做哈希计算时接受盐值 Salt 和执行成本 Cost 两个参数，通常，cost 一般混入 salt 中，就是花费的成本体现在这个盐上，使用层面，就无需设置 cost 了。
    ```ts
    let client_hash = bcrypt(passwordHash, salt)
    ```

**现在开始进入服务器端的设置和存储阶段**

5. 服务器接收客户端传递来的一串代码之后，为每一个密码 (也就是这一串哈希值) 生成一个随机的 salt，比如 CSPRNG (密码学安全伪源随机数生成器) 来生成一个长度和这个哈希值长度相同的随机字符串，比如：
     ```go
    func generateRandomString(clientHash string) (string, error) {
	// Decode the hex string to find out its byte length
	decoded, err := hex.DecodeString(clientHash)
	if err != nil {
		return "", fmt.Errorf("decoding client hash failed: %v", err)
	}

	// Create a byte slice of the same length as the decoded hash
	randomBytes := make([]byte, len(decoded))

	// Generate cryptographically secure random bytes
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("generating random bytes failed: %v", err)
	}

	// Convert the random bytes back to a hex string to match the input format
	randomHex := hex.EncodeToString(randomBytes)

	return randomHex, nil
     ```
6. 将动态盐值和客户端传递来的哈希值再做一次哈希值，产生最终的密文，并且和上一步随机生成的盐值一起写入到数据库的同一条数据中 (比如存储的形式为 salt.hash)，由于慢哈希函数比较消耗资源，因此在服务器中我们的哈希函数并不采用慢哈希函数，而是采用快哈希函数，比如 sha256，sha512 等。
    ```go
        server_hash = SHA256(client_hash + server_salt)
        DB.save(server_hash, server_salt)
    ```
下面我们演示验证密码的过程：

7. 客户端在登录界面输入明文密码，经过和注册相同的客户端生成 hash 的过程发送给服务器
    ```ts
    authentication_hash = 'NNDE294443DFDFDF'
    ```
8. 服务端接收到哈希值之后，从数据库中取出登录用户的哈希值和盐，采用相同的哈希算法 (sha256) 对客户端的哈希值+服务端存储的 salt 进行哈希计算
    ```go
    result := SHA256(authentication_hash + server_salt)
    ```
9. 比较计算结果是否一致，一致则验证通过，否则验证失败。
    ```go
    authentication_success := compare(result, server_hash)
    ```

