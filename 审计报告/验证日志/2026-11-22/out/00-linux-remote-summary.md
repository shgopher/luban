# 远端实测汇总（Ubuntu 24.04.1 LTS / KVM / kernel 6.8.0-48-generic / 1 vCPU / 1968 MB）

> 本文件是**人工汇总**，原始输出见同目录各编号 log。所有数字均来自本次远端真实执行，无编造。

## 任务 1：15.9 性能剖析工具链（perf → 火焰图）—— 通过

| 项目 | 真实值 |
|---|---|
| perf 版本 | `perf version 6.8.12`（/usr/bin/perf，实际二进制 /usr/lib/linux-tools-6.8.0-48/perf） |
| perf_event_paranoid 初值 | 4（本机 /etc/sysctl.d、/usr/lib/sysctl.d 均无对应配置，属运行时状态，重启即失效） |
| 调整后 | 1（`sysctl -w kernel.perf_event_paranoid=1`，rc=0） |
| kptr_restrict | 1（root 仍可解析内核符号） |
| lockdown | none |
| 采样命令 | `perf record -F 99 -g -p <pid> -- sleep 10` |
| 样本数 | **656**（Lost 0；10s / 1 vCPU，约 65.6 样本/秒） |
| perf.data | 824,725 B |
| `perf script` 行数 | 6,939 |
| 唯一折叠栈 | 18 |
| 最大栈深度 | 24 帧 |
| SVG | **23,018 B**，51 个 `<rect>` 帧 |
| perf stat -p | 5.005s 窗口：task-clock 4238.16ms（0.847 CPUs）、cycles 7,736,034,636、instructions 21,144,301,949、IPC 2.73、context-switches 3,395（801/秒）、branch-misses 0.27% |
| 自耗时 top | work_math 67.59%、work_sort 22.13%、hash_bytes 5.96%、rnd 2.49%、work_hash 1.08% |
| 内核态噪声 | asm_sysvec_apic_timer_interrupt 链 0.75%（定时器中断打断用户态采样，真实存在） |

远端脚本：`/root/perf-verify/run-perf-flamegraph.sh`（md5 80118b8553e806deb064a52267da15c2）
负载源码：`/root/perf-verify/cpu-burn.c`（md5 89c018028fc2ad9226de770b14806cfb）
FlameGraph：commit 41fee1f99f9276008b7cd112fca19dc3ea84ac32（2024-10-21）

## 任务 2：11.14 内核启动链 —— 部分通过（工具可用性分裂）

| 项目 | 真实值 |
|---|---|
| systemd-analyze | `Startup finished in 3.547s (kernel) + 2min 35.639s (userspace) = 2min 39.187s` |
| 分段缺失原因 | boot_mode=**BIOS**（无 EFI loader 变量）、initrd 内无 systemd、无 systemd-boot |
| blame top1 | `2min 18.841s cloud-final.service` |
| grub-mkconfig | rc=0，**285 行 / 9,348 B**，md5 97140da1d31716bd6fc7b291d4ac8809 |
| grub 版本 | `grub-mkconfig (GRUB) 2.12-1ubuntu7` |
| grub2-mkconfig | 不存在（RHEL 专名） |
| dracut | 可装：`dracut 060+5-1ubuntu3.4` |
| dracut 干跑 | `--no-hostonly --force /tmp/initramfs-test2.img` rc=0，**54.88 s**，**69,529,502 B**，MAXRSS 55,648 KB |
| grubby | Ubuntu 源中无此包（apt-cache policy 无输出、apt-cache search 无结果、命令不存在） |
| BLS | `/boot/loader/entries` 不存在（rc=2），Ubuntu 不使用 BLS |
| /proc/1/comm | systemd |
| systemctl is-system-running | running |

### ⚠ 副作用与恢复（如实记录）
- `apt-get install -y dracut` 触发 dpkg trigger，**用 dracut 重写了 /boot/initrd.img-{48,149,151}**（各约 69.5 MB）。
- 排查时发现 `initramfs-tools 0.142ubuntu25.4` 的安装文件清单**缺少 /usr/sbin/update-initramfs**（25.8 的 deb 里确实有此文件）。
- 已执行 `apt-get install --reinstall -y initramfs-tools initramfs-tools-core`（升到 25.8）+ `/usr/sbin/update-initramfs -u -k all`，镜像恢复为 initramfs-tools 生成（76 MB 级），`update-initramfs` 已存在。
- k3s 全程未动：最终 `systemctl is-active k3s` = active，节点 Ready。

## 任务 3：38.11 安全应急响应证据保全命令 —— 通过（30 条，26 条 rc=0）

| 项目 | 真实值 |
|---|---|
| 执行命令数 | 30 |
| rc=0 | 26 |
| journalctl 总行数 | 63,725 |
| journal 占用 | 78.4 MB |
| journalctl -u k3s 近 1h | 1,004 行 |
| ss -antp | 125 行（含表头） |
| ps auxf | 146 行 |
| 失败项 | avml/winpmem 未安装（rc=1）；crontab -l 无 crontab（rc=1）；atq 不存在（rc=127） |
| dd 只读验证 | 1 MiB，368 MB/s，rc=0（仅读 /dev/vda 前 1 MiB，未覆盖） |

远端脚本：`/root/sec-verify/sec-forensics.sh`（md5 e9efa5aff74544083f7ce4babf497d69）
内核链脚本：`/root/sec-verify/run-kernel-chain-check.sh`（md5 69da14b64077e3a49cfc39b8055e527c）

## 备注（写书时避免踩坑）
1. flamegraph.svg 里 `<title>main (20,316,100,092 samples, 100.00%)</title>` 的绝对数字是 **cycles 周期权重之和**（等于 perf 的 Event count），不是样本条数；样本条数以 `perf report` 的 **656** 为准，百分比是正确的。
2. `perf script` 输出行数 6,939 含样本间空行，不能直接当样本数。
3. 采样率：请求 -F 99，10 s 在 1 vCPU 上实际得 656 个样本（≈65.6/s），因为单个 CPU 同时要跑被测进程与采样开销。
4. Ubuntu 上安装 dracut 会触发 dpkg trigger 重写 /boot/initrd.img-*；本书若写“Ubuntu 装 dracut 只影响 /tmp”必须删掉，这是真实副作用。
