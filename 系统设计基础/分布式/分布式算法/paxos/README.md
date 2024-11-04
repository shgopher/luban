# Paxos

Paxos 算法是一种非拜占庭将军问题共识算法 (故障容错共识算法)，它是现代多种故障容错算法的鼻祖，比如 Fast Paxos，Cheap Paxos，Raft，ZAB 等。

paxos 分为 basic paxos 算法，它描述的是多节点之间就某个值达成共识；以及 Multi-Paxos 算法，描述的是执行多个 basic paxos 实例，一系列值达成共识。


