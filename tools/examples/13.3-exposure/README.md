# 13.3-exposure：暴露面清单与分级

[13.3-网络隔离与最小暴露面](../../../v2/L0-运行时底座/13-基础设施安全/13.3-网络隔离与最小暴露面.md) 的配套代码。

```bash
cd tools/examples/13.3-exposure
go run .
```

输出节选：

```text
== 暴露面清单（14 条）==
服务               端口     协议    可达范围       认证     加密     类型           等级    理由
edge-lb          443    tcp   internet   有      有      public       P2    受控公网业务入口
admin-console    8443   tcp   internet   有      有      management   P1    管理面上公网（有认证）
ssh-prod         22     tcp   internet   有      无      management   P1    管理面上公网（有认证）
mysql-primary    3306   tcp   private    有      有      data         P2    数据面可达，需最小权限
redis-cache      6379   tcp   private    无      无      data         P1    数据面未认证，内网可达即拖库
kube-apiserver   6443   tcp   internet   有      有      management   P1    管理面上公网（有认证）
node-exporter    9100   tcp   private    无      无      management   P1    管理面未认证
prometheus       9090   tcp   private    有      无      management   P2    内网管理面
```

14 条入口按可达范围、认证、加密定级，管理面暴露在公网的一律 P1。
