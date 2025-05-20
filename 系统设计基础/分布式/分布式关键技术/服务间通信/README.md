<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-12-06 00:31:03
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2025-05-20 15:59:50
 * @FilePath: /luban/系统设计基础/分布式/分布式关键技术/服务间通信/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# 服务间通信
## REST
REST (Representational State Transfer) 是一种架构风格，它本身只是一种设计理念，并不是具体的协议，其本身最重要的理念就是资源。

REST 本身并不受限底层协议，但是大多数情况下它是基于 HTTP 协议的，因为 HTTP 的很多特性刚好跟 REST 的设计理念吻合，比如 HTTP 的动词 (GET、POST、PUT、DELETE) 可以直接映射到 REST 的操作上。

很多项目都会根据自己的 API 去提供一个客户端，比如 k8s go 客户端，Java 客户端，客户端都是对自身 REST API 的一种封装。

我们可以使用 swagger 提供的 [OpenAPI](./OpenAPI/README.md) 来描述 REST API，OpenAPI 是一个开放的 API 描述规范，它允许开发者使用一种标准化的方式来描述 RESTful API 的结构和行为，从而自动的生成文档，并且可以生成客户端代码。

OpenAPI 规范的主要特点包括：
- ​​ 统一文档 ​​：通过 YAML 或 JSON 格式定义接口，生成可读性强的交互式文档 (如 Swagger UI)。
​- ​ 自动化开发 ​​：支持代码生成工具自动创建客户端 SDK 或服务端框架
​- ​ 协作优化 ​​：前后端团队可基于同一规范并行开发，减少沟通成本
## RPC 通信协议
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
## GraphQL

## 消息代理/消息队列
## 发布/订阅模式
## 数据序列化技术


