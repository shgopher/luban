# cilium

## eBPF 
eBPF [go库](https://github.com/cilium/ebpf)提供了一个通用的 eBPF 库，它将 eBPF 字节码的获取过程与 eBPF 程序的加载和管理操作分离。 

eBPF 程序通常是通过编写高级语言创建的，然后使用 clang/LLVM 编译器编译为 eBPF 字节码。

这是它的基本运行原理图
![](./ebpf1.png)
## 参考资料
- https://ebpf.io/what-is-ebpf/
- https://isovalent.com/data/liz-rice-what-is-ebpf.pdf?continueFlag=6021d2a27ddea20f1b8750770c4bd9fb
- https://mp.weixin.qq.com/s/KIsIA2tPzXZwGJSrfxm54g
