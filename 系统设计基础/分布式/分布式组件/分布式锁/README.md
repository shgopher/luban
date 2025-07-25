<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2025-05-20 23:25:27
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2025-07-21 23:12:37
 * @FilePath: /luban/系统设计基础/分布式/分布式组件/分布式锁/README.md
 * @Description: 
 * 
 * Copyright (c) 2025 by shgopher, All Rights Reserved. 
-->
# 分布式锁

分布式锁：核心目标是互斥访问。确保在分布式环境下，同一时间只有一个进程/线程/节点能够访问某个共享资源 (例如：一个文件、数据库中的一行记录、一个共享配置、一个外部 API 调用配额等)。它解决的是**资源竞争问题。**
 
通常作用于一个具体的资源或一个临界区代码段。**锁的范围相对较小和具体。**

分布式锁通常必须满足

- 互斥性：同一时刻只能有一个客户持有锁，其他客户等待。
- 可靠性：锁不会无缘无故丢失，即使节点宕机，甚至是网络故障也要保证锁的可靠性
- 性能：高并发场景下依然能快速争抢和释放锁

锁的实现方式有下面几种方案

- 基于数据库实现的分布式锁
- 基于缓存 redis 实现的分布式锁
- 基于分布式协调软件 zookeeper 实现的分布式锁
- 基于分布式 kv 存储 etcd 实现的分布式锁


基于数据库做的分布式锁原理也很简单，数据库本身会有锁的机制，创建一张锁表，为申请者在锁表里建立一条记录，记录建立成功则获得锁，消除记录则释放锁。

缺点吗？就是数据库的效率并不高，并且容易产生单点故障以及死锁的问题，如果在一个小项目或者 io 特别低的项目，这种方案完全没有问题。

使用缓存作为分布式锁，因为缓存放置在内存中所以无需磁盘的 io 问题，**效率会非常的高**，Redis 通常可以使用 setnx(key，value) 函数来实现分布式锁，其中 key 表示锁 id，value = currentTime + timeOut，表示当前时间 + 超时时间，超过超时时间则锁自动释放，setnx 返回 01 0 表示获取锁失败，1 表示获取锁成功。当0时可以设置一定的重试时间间隔，再次尝试获取锁，当1时则直接返回成功。再通过 Lua 脚本确保只有锁拥有者可以解锁，防范误删。

值得一提的是，redis 去实现的分布式锁实现了等待队列的方案，如果未获得锁，则将当前线程加入到等待队列中，当锁释放时，等待队列中的线程会重新尝试获取锁，顺序执行。

redis 等缓存去充当分布式锁的时候，优点在于，性能高，可以跨集群部署，核心就是不同的集群部署相同的数据，然后数据自动同步。

通过 redis 等缓存去实现的分布式锁最不靠谱的地方在于，通过超时时间去释放锁，万一没有执行完成就释放了锁呢？

zookeeper 使用树形结构去实现分布式锁，zk 的树形结构主要有四种，分别是持久节点，这也是 zk 的模式结构，持久顺序节点，根据创建的时间去节点自动进行排序，临时节点，当客户端与 ZooKeeper 断开连接后，该进程创建的临时节点就会被删除。临时顺序节点，而使用 zk 去实现分布式锁是使用的最后一种结构---临时顺序节点

实现锁的方法：

1. 创建一个持久节点，并设置其值为锁的标识符。在与该方法对应的持久节点的目录下，为每个进程创建一个临时顺序节点。
2. 每个进程获取所有临时节点列表，对比自己的编号是否最小，若最小，则获得锁。
3. 若本进程对应的临时节点编号不是最小的，则继续判断：

若本进程为读请求，则向比自己序号小的最后一个写请求节点注册 watch 监听，当
监听到该节点释放锁后，则获取锁；

若本进程为写请求，则向比自己序号小的最后一个读请求节点注册 watch 监听，当
监听到该节点释放锁后，获取锁。

zk 解决分布式锁单点故障、不可重入、死锁等问题，不过因为频繁的创建临时顺序节点，性能比缓存实现的分布式锁差。

不过 zk 由于解决了分布式锁的大多数问题，并且也没有数据库和缓存的众多问题，实现起来也比较方便，各大框架都已经封装好了对 zk 的使用，所以**分布式锁的首选就是 zookeeper。**

## 分布式锁 go 语言实战

通常在具体实践中，自己去实现一个分布式锁麻烦又不可靠，所以通常，我们会使用中间件去实现分布式锁，比如 zookeeper、redis、etcd 等等。

下面讲解一下使用 go 语言去调用该中间件的 go driver 去实现具体分布式锁的方法。

### redis
使用 SETNX 命令及过期时间 (TTL) 实现锁以及避免死锁这两个功能。

*再通过 Lua 脚本确保只有锁拥有者可以解锁，防范误删。*

**应用场景：电商秒杀等高并发但一致性要求不高的场景。**

示例代码：

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/go-redis/redis/v8"
)

var ctx = context.Background()

func acquireLock(client *redis.Client, key, value string, ttl time.Duration) (bool, error) {
    ok, err := client.SetNX(ctx, key, value, ttl).Result()
    return ok, err
}

func releaseLock(client *redis.Client, key, value string) error {
  // lua 脚本，表示确认是否是锁的设置者
    script := `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end`
    _, err := client.Eval(ctx, script, []string{key}, value).Result()
    return err
}

func main() {
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    defer client.Close()

    key := "lock-now"
    value := "client1"// Unique ID
    ttl := 5 * time.Second // 超时设置时间5秒

    if ok, err := acquireLock(client, key, value, ttl); ok && err == nil { // 尝试获取锁
        fmt.Println("任务开始")
        time.Sleep(2 * time.Second) // 模拟任务耗时
        releaseLock(client, key, value) // 释放锁
        fmt.Println("释放锁")
    } else {
        fmt.Println("错误：", err)
    }
}
```

### zookeeper
通过临时有序节点机制进行**排队式锁竞争**，保障严格一致性。每个客户端创建节点后，**检查自己编号是否最小，从而决定是否获得锁**

**应用场景：强一致性诉求，如金融、关键位置的调度等。需要严格一致性要求。**

示例代码：
```go
package main

import (
    "fmt"
    "sort"
    "time"

    "github.com/samuel/go-zookeeper/zk"
)

func acquireLock(conn *zk.Conn, path string) (string, error) {
    node, err := conn.Create(path+"/lock-", nil, zk.FlagEphemeral|zk.FlagSequence)
    
    if err != nil {
        return"", err
    }

    for {
        kids, _, err := conn.Children(path)
        if err != nil {
            return"", err
        }
        sort.Strings(kids)
        if path+"/"+kids[0] == node {
            return node, nil// You’re up!
        }
        prev := kids[0] // Watch the guy in front
        for i, k := range kids {
            if path+"/"+k == node {
                prev = kids[i-1]
                break
            }
        }
        _, _, ch, _ := conn.Get(path + "/" + prev)
        <-ch // Wait for them to leave
    }
}

func main() {
    conn, _, err := zk.Connect([]string{"localhost:2181"}, 5*time.Second)
    if err != nil {
        panic(err)
    }
    defer conn.Close()

    path := "/locks"
    if node, err := acquireLock(conn, path); err == nil {
        fmt.Println("Locked:", node)
        time.Sleep(2 * time.Second)
        conn.Delete(node, -1)
        fmt.Println("Unlocked!")
    } else {
        fmt.Println("Oops:", err)
    }
}
```
### etcd
etcd 采用**租约 (lease) 与键竞争机制**，客户端只要持有租约且键未被他人占用，即可获取锁。

**应用场景：云原生、Kubernetes 周边应用，兼顾性能与一致性。**

示例代码：
```go
package main

import (
    "context"
    "fmt"
    "time"

    "go.etcd.io/etcd/client/v3"
)

func acquireLock(cli *clientv3.Client, key string, ttl int64) (*clientv3.LeaseGrantResponse, error) {
    
    lease, err := cli.Grant(context.Background(), ttl)
    
    if err != nil {
        returnnil, err
    }

    txn := cli.Txn(context.Background()).
        If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
        Then(clientv3.OpPut(key, "locked", clientv3.WithLease(lease.ID)))
    
    resp, err := txn.Commit()
    
    if err != nil || !resp.Succeeded {
        returnnil, fmt.Errorf("lock failed")
    }

    return lease, nil
}

func main() {
    cli, _ := clientv3.New(clientv3.Config{
        Endpoints:   []string{"localhost:2379"},
        DialTimeout: 5 * time.Second,
    })
    defer cli.Close()

    key := "/desk_lock"
    
    if lease, err := acquireLock(cli, key, 10); err == nil {
        fmt.Println("Desk’s mine!")
        time.Sleep(2 * time.Second)
        cli.Revoke(context.Background(), lease.ID)
        fmt.Println("Desk’s free!")
    } else {
        fmt.Println("No desk:", err)
    }
}
```
### 三者对比

|工具|特点|优劣权衡|典型场景|
|-|-|-|-|
|Redis|超高并发、简单部署|一致性略弱|秒杀系统|
|ZooKeeper|强一致性、公平排队|部署和维护复杂|关键资源调度|
|etcd|Go 原生、云原生契合|高压下有延迟风险|K8s 周边|
## 工程实践
### 常见优化方法
**细颗粒度分布式锁**

细分锁，不能全局一把锁，一把锁的性能很差

**控制超时以及重试**

```go
// 一定要设置锁的存活时间 ttl ，防止死锁
func tryLock(client *redis.Client, key string, ttl time.Duration, retries int) (bool, error) {
    ctx, cancel := context.WithTimeout(context.Background(), ttl)
    defer cancel()
    backoff := 100 * time.Millisecond
    for i := 0; i < retries; i++ {
        if ok, err := acquireLock(client, key, "client-123", ttl); ok && err == nil {
            return true, nil
        }
        time.Sleep(backoff)
        backoff *= 2
    }
    return false, fmt.Errorf("gave up after %d tries", retries)
}
```

**监控锁的使用情况**

用 Prometheus 等埋点的方法去追踪锁请求/释放延时，发现瓶颈。
### 常见错误
- 使用 redis 锁时，锁被误删的情况，解决方案就是使用 lua 脚本去限制只能设置者才能删除锁
- ZooKeeper 网络波动时锁丢失，解决方案是，增加断线重连和**状态二次确认机制**
    ```go
    func lockWithRetry(conn *zk.Conn, path string) (string, error) {
        for {
          node, err := acquireLock(conn, path)
          // 状态二次确认
          if err == nil && conn.State() == zk.StateConnected {
              return node, nil
          }
          time.Sleep(time.Second)
          // 重连
          conn, _, _ = zk.Connect([]string{"localhost:2181"}, 5*time.Second)
        }
      }
    ```
- etcd 高并发下租约阻塞：提前分配租约，缓存复用。
  ```go
      type LeasePool struct {
      leases []clientv3.LeaseID
        sync.Mutex
      }

    func (p *LeasePool) Get(cli *clientv3.Client, ttl int64) (clientv3.LeaseID, error) {
        p.Lock()
        defer p.Unlock()
        if len(p.leases) > 0 {
            id := p.leases[0]
            p.leases = p.leases[1:]
            return id, nil
        }
        lease, err := cli.Grant(context.Background(), ttl)
        return lease.ID, err
    }
  ```
### 电商秒杀防超卖
```go
package main

import (
    "fmt"
    "time"

    "github.com/go-redis/redis/v8"
)

type Shop struct {
    client *redis.Client
}

func (s *Shop) Buy(itemID, userID string) (bool, error) {
    lockKey := fmt.Sprintf("lock:%s", itemID)
    uuid := userID + "-" + fmt.Sprint(time.Now().UnixNano())
    ttl := 5 * time.Second

    // 获取锁
    if ok, err := acquireLock(s.client, lockKey, uuid, ttl); !ok || err != nil {
        returnfalse, err
    }
    // 最后释放锁
    defer releaseLock(s.client, lockKey, uuid)

    stockKey := fmt.Sprintf("stock:%s", itemID)
    stock, _ := s.client.Get(context.Background(), stockKey).Int()
    if stock <= 0 {
        returnfalse, nil
    }
    s.client.Decr(context.Background(), stockKey)
    returntrue, nil
}

func main() {
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    shop := &Shop{client}
    client.Set(context.Background(), "stock:item1", 5, 0) // 5 units
    for i := 0; i < 10; i++ {
        gofunc(id int) {
            if ok, _ := shop.Buy("item1", fmt.Sprintf("user%d", id)); ok {
                fmt.Printf("User %d scored!\n", id)
            } else {
                fmt.Printf("User %d out of luck\n", id)
            }
        }(i)
    }
    time.Sleep(2 * time.Second)
}
```
### 分布式任务调度唯一执行
基于 etcd，为定时任务 (如日志清理) 提供 “全局唯一运行” 保障，防止重复执行
```go
package main

import (
    "fmt"
    "time"

    "go.etcd.io/etcd/client/v3"
)

type Scheduler struct {
    client *clientv3.Client
}

func (s *Scheduler) Run(taskID string) error {
    key := fmt.Sprintf("/lock/%s", taskID)
    lease, err := acquireLock(s.client, key, 10)
    if err != nil {
        return err
    }
    defer s.client.Revoke(context.Background(), lease.ID)

    fmt.Printf("Running %s\n", taskID)
    time.Sleep(2 * time.Second) // Fake work
    fmt.Printf("%s done\n", taskID)
    returnnil
}

func main() {
    cli, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
    defer cli.Close()
    s := &Scheduler{cli}
    for i := 0; i < 3; i++ {
        gofunc() {
            s.Run("cleanup")
        }()
    }
    time.Sleep(5 * time.Second)
}
```
## 参考资料

- https://mp.weixin.qq.com/s/FsOkz265kFMh_fuQZYDlvA







