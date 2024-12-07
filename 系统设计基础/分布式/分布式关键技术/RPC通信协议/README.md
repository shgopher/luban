<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-12-06 00:31:03
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-12-07 11:21:10
 * @FilePath: /luban/系统设计基础/分布式/分布式关键技术/RPC通信协议/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# RPC 通信协议

所谓的微服务架构，主要就是指的是 RPC 服务，RPC 服务基本上仅存在于后台系统的内部。

rpc 远程服务调用，目的是屏蔽网络编程的细节，能够像调用本地方法本地函数一样调用远程方法。

rpc 框架通过代理模式将网络通信屏蔽，服务调用者仅需像本地调用者一样调用一个 RPC 方法即可调用执行远程方法。

**rpc 通信的本质就是调用方将调用的方法和参数发送到被调用方，被调用方处理后将结果返回给调用方的过程**

- 首先，方法的输入参数，输出参数，这些对象要进行二进制的转换传输，比如 google 的 gRPC 使用的就是 protobuf 二进制序列化
- 被调用方接收到数据包后，将二进制数据进行解码，然后获取其方法名称，参数，然后调用本地方法，最后将结果返回给调用方。

 常见的 RPC 框架有：
- grpc
- thrift

rpc 框架只是一种用于屏蔽远程调用的设计手段，其与 http 协议或者 tcp 协议不在一个层面，我们可以将 rpc 的底层设计为 tcp 亦或者 http，比如 grpc 底层就是基于 http2 协议的。