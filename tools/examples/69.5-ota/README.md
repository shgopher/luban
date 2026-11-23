# 69.5-ota：固件升级的三件保命事

[69.5 固件升级-OTA](../../../v2/L4-组件与技术/69-物联网/69.5-固件升级-OTA.md) 的配套代码：清单签名（ed25519）、断点续传、A/B 回滚；不实现真正的镜像刷写。

```bash
cd tools/examples/69.5-ota
go test -race -v ./...
```

```text
--- PASS: TestManifestSignature (0.01s)   # 改 version / 改 URL / 改 SHA / 换公钥 全部拒绝
--- PASS: TestTamperRejected (0.01s)      # 固件改 1 字节 → SHA256 不匹配
```

签名覆盖的是**清单**（版本、大小、URL、SHA256），不是固件本体；固件本体靠 SHA256 钉住。测试里的私钥是演示用固定值，生产环境私钥必须在离线签名机/HSM，设备只存公钥。
