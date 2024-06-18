# 微内核架构
微内核架构的核心思想是，拥有一个简单的核心系统，拥有一个插件系统，这两者是这个系统的核心，用户按照某种规则去定制插件，注册插件到插件系统，进而在核心系统运行的时候自动运行第三方插件。

例如我们常见的 IDE，或者相关的工业软件，在架构设计中，都是，或者都会包含微内核架构。

## 微内核架构 demo1
这个 Demo 将包含一个核心组件 (core) 和一些插件 (plugins)。核心组件负责加载和管理插件，而插件则提供特定的功能

```bash
microkernel-demo/
├── core/
│   └── core.go
├── plugins/
│   ├── plugin1/
│   │   └── plugin1.go
│   └── plugin2/
│       └── plugin2.go
├── main.go
└── go.mod
```

```go
// main.go

package main

import (
    "fmt"
    "microkernel-demo/core"
    "microkernel-demo/plugins/plugin1"
    "microkernel-demo/plugins/plugin2"
)

func main() {
    // 创建核心组件
    core := core.NewCore()

    // 注册插件
    core.RegisterPlugin(plugin1.NewPlugin())
    core.RegisterPlugin(plugin2.NewPlugin())

    // 启动核心组件
    core.Start()
}
```

```go
//core/core.go

package core

import "fmt"

// Plugin 接口定义了插件必须实现的方法
type Plugin interface {
    Name() string
    Start()
    Stop()
}

// Core 结构体表示核心组件
type Core struct {
    plugins []Plugin
}

// NewCore 创建一个新的核心组件
func NewCore() *Core {
    return &Core{
        plugins: make([]Plugin, 0),
    }
}

// RegisterPlugin 注册一个插件
func (c *Core) RegisterPlugin(p Plugin) {
    c.plugins = append(c.plugins, p)
}

// Start 启动核心组件及所有插件
func (c *Core) Start() {
    fmt.Println("Starting core...")
    for _, p := range c.plugins {
        fmt.Printf("Starting plugin: %s\n", p.Name())
        p.Start()
    }
}
```

```go
//plugins/plugin1/plugin1.go

package plugin1

import "fmt"

// Plugin1 结构体实现了 Plugin 接口
type Plugin1 struct{}

// NewPlugin 创建一个新的 Plugin1 实例
func NewPlugin() *Plugin1 {
    return &Plugin1{}
}

// Name 返回插件名称
func (p *Plugin1) Name() string {
    return "Plugin1"
}

// Start 启动插件
func (p *Plugin1) Start() {
    fmt.Println("Plugin1 started")
}

// Stop 停止插件
func (p *Plugin1) Stop() {
    fmt.Println("Plugin1 stopped")
}
```
```go
//plugins/plugin2/plugin2.go

package plugin2

import "fmt"

// Plugin2 结构体实现了 Plugin 接口
type Plugin2 struct{}

// NewPlugin 创建一个新的 Plugin2 实例
func NewPlugin() *Plugin2 {
    return &Plugin2{}
}

// Name 返回插件名称
func (p *Plugin2) Name() string {
    return "Plugin2"
}

// Start 启动插件
func (p *Plugin2) Start() {
    fmt.Println("Plugin2 started")
}

// Stop 停止插件
func (p *Plugin2) Stop() {
    fmt.Println("Plugin2 stopped")
}
```