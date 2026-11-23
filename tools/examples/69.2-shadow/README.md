# 69.2-shadow：设备影子与物模型校验

[69.2 设备影子与物模型](../../../v2/L4-组件与技术/69-物联网/69.2-设备影子与物模型.md) 的配套代码。

| 目录 | 是什么 | 跑法 |
|---|---|---|
| `demo/` | 单文件版：物模型校验 + delta 场景复现（对应正文「实测 1」） | `go run ./demo` |
| `shadow/` | 包 + 测试：delta、版本 CAS、离线重放、并发（对应正文「实测 2」） | `go test -race ./shadow` |

```bash
cd tools/examples/69.2-shadow
go run ./demo          # 实测 1
go test -race ./shadow # 实测 2
```

`demo` 的输出：

```text
① 下发期望后 version=1 delta=[{"path":"power","desired":true},{"path":"target_temp","desired":26}] err=<nil>
② 设备上报后 delta=[{"path":"target_temp","desired":26}]
③ 旧版本写入 err=版本冲突：期望 1，实际 2
④ 拒绝 map[current_temp:20] → 只读属性不接受下行: current_temp
④ 拒绝 map[no_such:1] → 物模型里没有这个属性: no_such
④ 拒绝 map[mode:dry] → 属性 mode: 不在枚举 [cool heat auto] 里：dry
④ 拒绝 map[power:true] → 属性 power: 期望 bool，得到 string
④ 拒绝 map[target_temp:80] → 属性 target_temp: 高于上限 32: 80
```

`shadow` 的 5 个用例：`TestDeltaThreeStates`、`TestVersionConflict`、`TestOfflineReplay`、`TestThingModelValidation`、`TestConcurrent`（100 写者 + 100 上报，`-race` 下无竞争，最终 version=200）。

> `demo/` 与 `shadow/` 是同一套逻辑的两份形态：前者单文件、便于 `go run` 阅读，后者带并发与 CAS 的完整测试。正文说的「同一实现」指行为一致，不是同一份源码。
