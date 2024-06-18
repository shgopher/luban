<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2023-12-10 14:06:10
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2024-06-18 14:43:14
 * @FilePath: /luban/系统设计基础/常见的架构风格/洋葱架构/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# 洋葱架构
## 清洁架构的架构设计思想
清洁架构 (Clean Architecture) 本质上是一套架构设计的原则和思想，而不是一种严格的具体架构模式。

DDD (领域驱动设计) 也可以视作为是对清洁架构原则的一种实现方式。其核心概念比如：

- 领域层：包含实体和业务规则
- 应用层：协调业务用例
- 基础设施层：技术实现

这与清洁架构的 ideas 是一脉相承的。

所以**清洁架构只是一个抽象理念**，它主张**业务是架构核心**，其它都是可更换的实现细节。理念本身不构成具体的架构样式或模式。

实际应用中，我们可以根据这些原则设计不同的架构，比如**洋葱架构**、**六边形架构**、**DDD** 等等。它们都是在实践中对清洁架构这个设计思想的体现。

清洁架构是一系列架构设计的原则和理念，而真正的架构实现则有多种形式和可能性。

例如本篇就是介绍的洋葱架构
## 洋葱架构的具体内容
洋葱架构是清洁架构设计思想的一种具体实现

洋葱架构 (Onion Architecture) 的核心部分主要包括：

1. 领域层 (Domain Layer)
   - 包含业务实体 (Entity)、仓储接口 (Repository Interface)、领域服务接口 (Domain Service Interface) 等。
   - 这些接口和规范不依赖任何外部技术。
   
2. 应用层 (Application Layer)
   - 包含应用服务 (Application Service) 实际实现，协调和组织业务流程。
   - 实现和依赖领域层的接口规范。
   
3. 基础技术实现层 (Infrastructure Layer)
   - 包含数据层、技术接口和框架等具体实现代码。
   - 实现应用层定义的接口要求。
   
4. 用户界面层 (UI Layer)
   - 包含各种面向用户的界面，依赖应用层。

总结核心特性：

- **业务内聚**，**面向接口编程**
- **依赖关系从外向内**
- **内层只定义抽象**
- **外层实现内层细节**

这使得业务规则独立且通用，系统容易迭代更新。
### 洋葱架构 demo1

```bash
clean-arch-demo/
├── domain/
│   ├── entities/
│   │   └── entity.go
│   └── repositories/
│       └── repository.go
├── usecases/
│   └── usecase.go
├── interfaces/
│   ├── persistence/
│   │   └── persistence.go
│   └── presentation/
│       └── presenter.go
├── infrastructure/
│   ├── persistence/
│   │   └── persistence.go
│   └── presentation/
│       └── presenter.go
├── main.go
└── go.mod
```
- domain 包含业务实体和仓储接口，这是最内层，不依赖于任何外部代码。
- usecases 包含应用程序用例，只依赖于 domain 层。
- interfaces 包含持久化和表现层的接口定义，不包含任何实现细节。
- infrastructure 包含持久化和表现层的具体实现，依赖于 interfaces 层。
- main.go 是入口点，负责组装各个层的实现。

```go
// domain/entities/entity.go

package entities

// Entity 表示业务实体
type Entity struct {
    ID   string
    Name string
}
```

```go
// domain/repositories/repository.go

package repositories

import "clean-arch-demo/domain/entities"

// Repository 定义了仓储接口
type Repository interface {
    GetEntity() *entities.Entity
}
```

```go
// usecases/usecase.go

package usecases

import "clean-arch-demo/domain/repositories"

// Usecase 表示应用程序用例
type Usecase struct {
    repository repositories.Repository
}

// NewUsecase 创建一个新的应用程序用例
func NewUsecase(r repositories.Repository) *Usecase {
    return &Usecase{
        repository: r,
    }
}

// Execute 执行业务逻辑
func (u *Usecase) Execute() {
    // 获取实体数据
    entity := u.repository.GetEntity()

    // 执行业务逻辑
    // ...
}
```
```go
// interfaces/persistence/persistence.go

package persistence

import "clean-arch-demo/entities"

// Persistence 定义了持久化层接口
type Persistence interface {
    GetEntity() *entities.Entity
}
```

```go
// interfaces/presentation/presenter.go

package presentation

import "clean-arch-demo/usecases"

// Presenter 表示表现层
type Presenter struct {
    usecase *usecases.Usecase
}

// NewPresenter 创建一个新的表现层
func NewPresenter(u *usecases.Usecase) *Presenter {
    return &Presenter{
        usecase: u,
    }
}

// Execute 执行业务逻辑
func (p *Presenter) Execute() {
    p.usecase.Execute()
}
```
```go
// infrastructure/persistence/persistence.go

package persistence

import "clean-arch-demo/entities"

// Persistence 实现了持久化层接口
type Persistence struct{}

// NewPersistence 创建一个新的持久化层实现
func NewPersistence() *Persistence {
    return &Persistence{}
}

// GetEntity 获取实体数据
func (p *Persistence) GetEntity() *entities.Entity {
    return &entities.Entity{
        ID:   "1",
        Name: "Entity",
    }
}
```
```go
// infrastructure/presentation/presenter.go

package presentation

import "clean-arch-demo/usecases"

// Presenter 实现了表现层
type Presenter struct {
    usecase *usecases.Usecase
}

// NewPresenter 创建一个新的表现层实现
func NewPresenter(u *usecases.Usecase) *Presenter {
    return &Presenter{
        usecase: u,
    }
}

// Execute 执行业务逻辑
func (p *Presenter) Execute() {
    p.usecase.Execute()
}
```
在这个 Demo 中，main.go 是入口点，它创建了持久化层、用例层和表现层的实现。entities 包定义了业务实体，usecases 包定义了应用程序用例，interfaces 包定义了持久化层和表现层的接口，infrastructure 包提供了持久化层和表现层的具体实现。
当你运行 main.go 时，程序将执行业务逻辑，但由于我们没有实现任何具体的业务逻辑，所以没有可见的输出。
这个 Demo 展示了清洁架构的基本结构，每一层只与相邻内层进行交互，从而实现了代码的高内聚、低耦合。你可以根据实际需求，在各个层中添加具体的业务逻辑。