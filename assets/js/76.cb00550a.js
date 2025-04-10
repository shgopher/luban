(window.webpackJsonp=window.webpackJsonp||[]).push([[76],{521:function(v,_,l){"use strict";l.r(_);var i=l(36),t=Object(i.a)({},(function(){var v=this,_=v.$createElement,l=v._self._c||_;return l("ContentSlotsDistributor",{attrs:{"slot-key":v.$parent.slotKey}},[l("h1",{attrs:{id:"拜占庭将军问题"}},[v._v("拜占庭将军问题")]),v._v(" "),l("p",[v._v("拜占庭将军将军问题讨论的核心内容是"),l("strong",[v._v("分布式共识问题")])]),v._v(" "),l("h2",{attrs:{id:"拜占庭将军问题算法和非拜占庭将军问题算法"}},[v._v("拜占庭将军问题算法和非拜占庭将军问题算法")]),v._v(" "),l("p",[v._v("***非***拜占庭拜占庭将军问题算法：")]),v._v(" "),l("ul",[l("li",[v._v("Paxos (强一致性的主备方案)")]),v._v(" "),l("li",[v._v("Raft (强一致性的主备方案)")]),v._v(" "),l("li",[v._v("ZAB (强一致性的主备方案)")]),v._v(" "),l("li",[v._v("Gossip (最终一致性的无主方案)")])]),v._v(" "),l("p",[l("strong",[v._v("拜占庭将军问题算法：")])]),v._v(" "),l("ul",[l("li",[v._v("PBFT (强一致性的主备方案)")]),v._v(" "),l("li",[v._v("Pow (最终一致性方案)")])]),v._v(" "),l("h2",{attrs:{id:"常见的共识算法架构"}},[v._v("常见的共识算法架构")]),v._v(" "),l("p",[v._v("在分布式系统中，常见的共识方案可以分为以下三大类：")]),v._v(" "),l("p",[l("strong",[v._v("主备方案 (Leader-Follower 模式)")])]),v._v(" "),l("p",[v._v("这是最常见的一种形式，如 Raft、ZAB 等算法采用此模式")]),v._v(" "),l("ul",[l("li",[v._v("Leader 负责处理所有的写请求")]),v._v(" "),l("li",[v._v("Follower 节点可以提供读服务，并作为备份节点")])]),v._v(" "),l("p",[v._v("集群通过共识算法来保证主节点和备份节点之间的数据一致性")]),v._v(" "),l("p",[l("strong",[v._v("多主方案")])]),v._v(" "),l("p",[v._v("Multi-Paxos 变体：支持多个主节点，每个主负责不同的值域范围，就是使用一致性哈希等分片算法组合起来的"),l("strong",[v._v("多个主备方案")]),v._v("。")]),v._v(" "),l("p",[v._v("典型例子：")]),v._v(" "),l("ul",[l("li",[v._v("MySQL 分片集群")]),v._v(" "),l("li",[v._v("MongoDB 分片集群")]),v._v(" "),l("li",[v._v("Redis 集群")])]),v._v(" "),l("p",[l("strong",[v._v("无主方案")])]),v._v(" "),l("p",[v._v("CRDT (无主)：所有节点都可以处理写入；通过数学特性保证最终一致性；适用于协同编辑等场景。")]),v._v(" "),l("ul",[l("li",[l("p",[v._v("Gossip 协议：")]),v._v(" "),l("ul",[l("li",[v._v("去中心化设计")]),v._v(" "),l("li",[v._v("通过节点间随机通信传播更新")]),v._v(" "),l("li",[v._v("用于服务发现等场景")])])]),v._v(" "),l("li",[l("p",[v._v("Pow 协议：")]),v._v(" "),l("ul",[l("li",[v._v("支持对等节点设计")]),v._v(" "),l("li",[v._v("可处理恶意节点")]),v._v(" "),l("li",[v._v("常用于区块链系统")])])])]),v._v(" "),l("p",[v._v("方案选择考虑因素：")]),v._v(" "),l("p",[l("strong",[v._v("主备方案优势")]),v._v("：")]),v._v(" "),l("ul",[l("li",[v._v("架构相对"),l("strong",[v._v("简单清晰")]),v._v("，易于理解和实现。主节点负责主要的业务处理，备节点处于待命状态，在主节点出现故障时能快速接替其工作，保障系统的连续性。")]),v._v(" "),l("li",[l("strong",[v._v("数据一致性维护相对容易")]),v._v("。通常主节点处理完业务后会将数据同步到备节点，在切换过程中能较好地保证数据状态的一致性")]),v._v(" "),l("li",[v._v("系统稳定性有一定保障。备节点的存在使得即使主节点突发故障，如宕机、网络中断等，也能在较短时间内恢复服务，减少因主节点故障导致的业务中断时间")])]),v._v(" "),l("p",[l("strong",[v._v("主备方案劣势")]),v._v("：")]),v._v(" "),l("ul",[l("li",[l("strong",[v._v("资源利用率可能不高")]),v._v("。备节点在大部分时间处于闲置状态，只在主节点故障时才发挥作用，造成了一定的硬件资源浪费。")]),v._v(" "),l("li",[v._v("存在单点故障风险。虽然有备节点，但如果主节点和备节点同时出现故障 (比如遭受相同的网络攻击、硬件故障等)，系统仍可能陷入瘫痪。")]),v._v(" "),l("li",[v._v("主备切换过程可能存在短暂的数据不一致或服务中断情况。")])]),v._v(" "),l("p",[l("strong",[v._v("多主方案优势")]),v._v("：")]),v._v(" "),l("ul",[l("li",[l("strong",[v._v("更好的写入性能")]),v._v("，多个主节点可以同时处理不同的业务任务，分担了系统的工作量")]),v._v(" "),l("li",[v._v("具备一定的容错能力更高的可用性。即使其中一个主节点出现故障，其他主节点仍能继续处理业务，不至于使整个系统陷入瘫痪，保障了系统的基本运行。")]),v._v(" "),l("li",[v._v("可以"),l("strong",[v._v("灵活配置资源")]),v._v("。根据业务需求和负载情况，可以灵活调整各个主节点的功能和任务分配，使系统资源得到更有效的利用。")])]),v._v(" "),l("p",[l("strong",[v._v("多主方案劣势")]),v._v("：")]),v._v(" "),l("ul",[l("li",[v._v("数据一致性维护复杂。多个主节点同时处理业务，要确保它们处理的数据在整个系统中保持一致难度较大，需要复杂的同步机制和冲突处理机制。")]),v._v(" "),l("li",[v._v("实现复杂度高，协调管理困难。多个主节点之间需要进行有效的协调和沟通，以确保业务的正常开展，否则可能出现任务分配不均、业务冲突等问题，增加了系统管理的复杂性。")]),v._v(" "),l("li",[v._v("增加了安全风险。多个主节点意味着更多的攻击入口，更容易遭受恶意攻击，且一旦某个主节点被攻破，可能会影响到整个系统的安全和稳定。")])]),v._v(" "),l("p",[l("strong",[v._v("无主方案优势")]),v._v("：")]),v._v(" "),l("ul",[l("li",[v._v("资源利用更加"),l("strong",[v._v("均衡")]),v._v("。所有节点地位平等，都参与到业务处理中，不存在某个节点闲置等待的情况，使得系统资源得到更充分的利用。")]),v._v(" "),l("li",[v._v("具有较好的"),l("strong",[v._v("可扩展性")]),v._v("。新节点可以很容易地加入到系统中，不需要考虑与主节点的适配等问题，只需遵循系统的基本规则即可参与业务处理，便于系统的扩展和升级")]),v._v(" "),l("li",[v._v("避免了单点故障问题。由于不存在主节点，也就不存在因主节点故障而导致系统瘫痪的风险，")])]),v._v(" "),l("p",[l("strong",[v._v("无主方案劣势")]),v._v("：")]),v._v(" "),l("ul",[l("li",[v._v("达成共识难度大。由于没有主节点来主导决策过程，所有节点需要通过复杂的共识算法 (如 PBFT、RAFT 等) 来达成一致意见，这需要消耗大量的时间和计算资源，可能导致业务处理速度较慢。")]),v._v(" "),l("li",[v._v("数据一致性维护挑战大。所有节点平等参与业务处理，要确保数据在各个节点间保持一致需要更为复杂的机制，比如分布式事务处理等，否则容易出现数据不一致的情况。")]),v._v(" "),l("li",[v._v("系统管理和监控相对复杂。没有主节点作为核心监控和管理对象，需要对所有节点进行全面的监控和管理，增加了系统管理和监控的难度和工作量。")])]),v._v(" "),l("p",[v._v("实际选择时需要根据业务场景在 CAP 三者间做出权衡，选择最适合的方案。")]),v._v(" "),l("h2",{attrs:{id:"多主和无主的区别"}},[v._v("多主和无主的区别")]),v._v(" "),l("p",[v._v("如果所有节点都是对等的，都可以处理写请求，这种是无主 (Leaderless) 架构，而不是多主。典型的就是 Gossip 协议，和")]),v._v(" "),l("p",[v._v("也就是说，多主，并不是所有节点完全平等，有多个主节点，每个主节点下都可能有自己的备份节点，主节点之间是对等的，都可以处理写请求，"),l("strong",[v._v("每个主节点通常负责不同的数据范围")])]),v._v(" "),l("p",[l("strong",[v._v("多主就等于多个主从，然后使用一致性哈希等算法，划片儿治理的意思")])])])}),[],!1,null,null,null);_.default=t.exports}}]);