<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-09-15 17:04:57
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-10-09 00:59:10
 * @FilePath: /luban/系统设计基础/网络在系统设计中的作用/SDN/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# sdn
(sdn software defined network) 软件定义网络，它将网络控制平面和数据层面分离，这里面有一个重要的技术 lvs 它就是三层负载均衡，二层负载均衡的运用。

**其核心技术就是 LVS**
## lvs 基本原理
通过修改七层网络中的数据链路层的 mac 协议，网络层的 ip 协议，实现了交换机 (处理 mac 协议) 网关 (处理 ip 协议) 的数据包转发。从而将数据转发到真正的服务器上。

lvs 有四种模式

- NAT 模式 (三层负载均衡)
- FULL-NAT 模式 (三层负载均衡)
- TUN 模式 (二层负载均衡，部分三层负载均衡)
- DR 模式 (二层负载均衡)

只有仅有二层负载均衡的就只能用在内网中，因为没有 ip 识别是无法在公网中使用的。

只有涉及到了三层负载均衡，就能用在公网中，所以显而易见的，DR 模式最底层，性能最高，但是只能用在内网中
### NAT 模式
![nat](./nat.svg)
### FULLNAT 模式
![fullNat](./full-nat.svg)
### TUN 模式
由于第三层的数据包，即 IP 数据包中包含了源 (客户端) 和目标 (均衡器) 的 IP 地址，只有真实服务器保证自己的 IP 地址与数据包中的目标 IP 地址一致，这个数据包才能被正确处理。因此，使用这种负载均衡模式时，需要把真实物理服务器集群所有机器的虚拟 IP 地址 (Virtual IP Address，VIP) 配置成与负载均衡器的虚拟 IP 一样，这样经均衡器转发后的数据包就能在真实服务器中顺利地使用

![tun](./tun.svg)
### DR 模式
![dr](./dr.svg)
由于需要更改目标 mac 地址，这就意味着该负载均衡器必须与真实的服务在 mac 层能建立通信，所以这就意味着它只能运行在内网中
## keepalived
Keepalived 是一款基于 VRRP (Virtual Router Redundancy Protocol，虚拟路由器冗余协议) 实现的高可用性 (HA) 软件。它主要用于服务器的负载均衡和高可用性场景，能够确保在服务器出现故障或网络异常时，服务仍能持续、稳定地提供。

它的基本原理是：
1。两台机器配置同一个 vip (虚拟 ip)，比如真实 ip 是 1.1.1.1 和 1.1.1.2 但是虚拟出来的 ip 是 7.7.7.7
2。两台 lvs 系统通过 keepalived 频繁通信，评估哪个机器分数高，高的那个作为 master，另一台作为备份，得分高的通过 vrrp 组播报文宣称 vip 在这里
3。如果 master 宕机了，备份机器会立刻宣称 vip 在自己这里

> 作为最前面的网关，防火墙作为网关，其公网接口同样具有真实 IP 地址
## 更多优化方法
- dpdk，因为 lvs 基于 Linux 内核的 netilter，需要内核态和用户态切换，因此 dpdk，通过申请大内存，以及轮询替代中断，完成更高性能的表现，[dpvs](https://github.com/iqiyi/dpvs) 是 dpdk 技术的开源项目
- 
