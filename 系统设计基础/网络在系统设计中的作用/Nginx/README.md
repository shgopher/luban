<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 16:49:34
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-09-21 22:19:35
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/Nginx/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# web 服务器 Nginx

web 服务器 (web server) 又叫做 HTTP 服务器，它有两大功能，其一是承载了 HTML JS CSS 等静态资源，第二是接收来自浏览器的请求，并返回响应。

常见的 web 服务器有 Apache Nginx，当然现在更多的使用 GO Nodejs 去构建服务，充当了 web 服务器

关于 GO 去充当 web 服务器的，可以去查阅 [GOFamily](https://github.com/shgopher/GOFamily)，本章主要介绍 Nginx 充当 web 服务器这个功能的使用，至于 Nginx 的七层负载均衡的使用可以查看本章的[负载均衡](../负载均衡/README.md)

Nginx 性能非常的不错，这跟它使用了基于事件驱动的并发模型有关

Nginx 使用了主进程去管理工作进程，每一个工作进程都是一个单线程，所以 Nginx 属于多进程和单线程结合的模型，每一个线程都是使用基于事件驱动的并发模型去处理高 io，这跟 js 的并发模型是一致的。
> 基于事件驱动的并发模型之所以会设计成单线程模型，主要有四个原因：1。避免线程切换带来的开销，2。简化并发模型 3。单线程更易水平扩展 4。单线程 cpu 缓存命中率高

由于 nginx 并没有并行的需求，所以对于一个只考虑高 IO 的设备来说，基于事件驱动的单线程回调模型性能更好，实现更简单，出错率更低。
## epoll --- 事件驱动的模型产物

**io 多路复用：**

IO 多路复用允许一个进程同时监视多个文件描述符 (或者其它的内容)，当其中任何一个文件描述符变为可读或可写状态时，进程就会被通知，从而可以进行相应的读写操作。这样，进程就**不需要为每个文件描述符都创建一个单独的线程或进程来进行阻塞式的等待，而是可以在一个线程中高效地处理多个文件描述符**

**事件驱动的并发模型：**基于事件，当事件触发的时候即可执行之前注册的 callback 函数。当没有触发事件的时候，callback 函数处于等待状态。具体到 epoll 的实现上，它使用一个红黑树去管理文件描述符。当文件描述符上的事件发生时，epoll 会将事件通知放入一个就绪列表中，随后用户程序可以从就绪列表中获取事件，并根据事件类型执行相应的 callback 函数进行处理。

epoll 是 linux 内核提供的一个事件驱动的基于 io 多路复用并发模型，它基于事件驱动的回调模型，在 linux 中，每一个文件描述符都可以注册一个事件，当文件描述符发生变化的时候，内核会通知应用层，应用层通过回调函数处理，并且建议一个通道，复用多个 io，这样就完成了高并发的处理

Nginx 利用了 Linux 内核的 epoll API 来实现了海量流量的处理，Nginx 的工作进程通过 epoll 来监听客户端的请求，当有客户端请求的时候，nginx 会通过回调函数处理请求，处理完请求后，nginx 会通过 epoll 将请求响应给客户端。







