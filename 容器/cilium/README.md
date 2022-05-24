# cilium

## eBPF
用一句话来形容ebpf，它是可以运行在Linux内核中的沙箱，你可以想象成docker运行在了内核里。它可以做到不改变内核源代码以及不加载内核的情况下，做到安全的拓展内核功能。

eBPF [go库](https://github.com/cilium/ebpf)提供了一个通用的 eBPF 库，它将 eBPF 字节码的获取过程与 eBPF 程序的加载和管理操作分离。 

eBPF 程序通常是通过编写高级语言创建的，然后使用 clang/LLVM 编译器编译为 eBPF 字节码。

所以ebpf有点像wasm，它不是一种软件，更像是一种规则，只要符合它的规则任何高级语言都可以去生成复合它要求的代码。

这是它的基本运行原理图
![](./ebpf1.png)
## 参考资料
- https://ebpf.io/what-is-ebpf/
- https://isovalent.com/data/liz-rice-what-is-ebpf.pdf?continueFlag=6021d2a27ddea20f1b8750770c4bd9fb
- https://mp.weixin.qq.com/s/KIsIA2tPzXZwGJSrfxm54g
