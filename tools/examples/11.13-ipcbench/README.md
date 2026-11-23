# 11.13-ipcbench：四种 IPC 的真实两进程基准

[11.13 进程间通信选型](../../../v2/L0-运行时底座/11-操作系统与内核基础/11.13-进程间通信选型.md) 的配套代码：pipe / unix socket / TCP loopback / 共享内存，全部是**两个真实进程**，父进程计时，子进程做对端。

```bash
cd tools/examples/11.13-ipcbench
go run . -kind pipe -mode lat -n 2000
go run . -kind unix -mode tput -size 65536 -n 2000
```

参数：`-kind pipe|unix|tcp|shm`、`-mode lat|tput`、`-size 字节`、`-n 次数`。子进程由父进程自己拉起（`-child` 是内部参数，不要手工传）。

**数字随平台变**。正文表格是 Linux 上测的（pipe 2.42µs、共享内存 125ns）；同一份代码在 macOS 上（Apple M1）：

```text
kind=pipe size=1 n=2000 mean=8.523µs p50=6.5µs p90=8.75µs p99=27.667µs
kind=shm  size=1 n=2000 mean=101ns  p50=84ns  p90=125ns  p99=125ns
```

量级结论不变：共享内存比管道快两个数量级；绝对值只用于同机对比，不跨平台比较。

平台细节：

- 共享内存优先落 `/dev/shm`（Linux tmpfs），没有就退到 `os.TempDir()`（macOS），mmap 语义不变；
- `pipesize/` 只测 Linux 的 `F_GETPIPE_SZ`，macOS 没有这个 fcntl，跑出来无意义（`go run ./pipesize` 仅 Linux）。

> 正文说的「125ns 是通知的价，不是通信的价」在这份代码里能直接看到：`seq`/`ack` 两个原子量 + 自旋，数据一致性全靠使用者。
