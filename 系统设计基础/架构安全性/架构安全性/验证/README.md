<!--
 * @Author: shgopher shgopher@gmail.com
 * @Date: 2024-12-07 14:31:10
 * @LastEditors: shgopher shgopher@gmail.com
 * @LastEditTime: 2025-02-11 16:06:38
 * @FilePath: /luban/系统设计基础/架构安全性/架构安全性/验证/README.md
 * @Description: 
 * 
 * Copyright (c) 2024 by shgopher, All Rights Reserved. 
-->
# 验证
对于每一个提交到服务中的数据，都进行验证，验证通过后，才允许提交到服务中。从数量上来说，验证做的不完善是最大的安全隐患。

> 拖库攻击：利用系统漏洞绕过验证等保护机制，连接到数据库，将数据库的数据全部下载的攻击手段。

缺失校验会影响数据质量，但过度校验并不会使系统更加健硕。

比如最近出的数据问题，例如邮箱的格式，姓名长度，密码的复杂程度，都应该在前端进行验证，当然后端也是需要校验的，这是为了避免黑客的攻击，那么问题来了，后端的校验应该放在哪一层呢？

一、分层验证的必要性
数据验证不应局限于单一层级，而应根据验证类型分层实施：

- 接入层 (Controller/API Gateway)

  - 职责：基础格式校验 (非空、正则、类型)

  - 技术实现：利用框架注解 (如 Spring Validation) 校验 DTO

  - 价值：拦截明显无效请求，降低后续处理开销

- 业务逻辑层 (Service/Domain)

  - 职责：复杂业务规则校验 (状态流转、业务约束)

  - 示例：订单支付前校验库存、用户权限、优惠券有效期

  - 实现：领域服务或领域对象内部方法

- 持久层 (Repository/DAO)

  - 职责：最终一致性校验 (唯一性、外键约束)

  - 实现：数据库约束 (UNIQUE、FOREIGN KEY) 配合乐观锁

## 独立验证层的适用场景

是否需要独立验证模块需评估以下维度：
- 建议独立的情况

  - 多入口共用规则 (HTTP API + 消息队列 + RPC)

  - 复杂校验逻辑存在多场景复用需求

  - 需要统一管理校验规则元数据

  - 企业级合规要求 (如金融行业风控规则集中管理)

- 无需独立的情况

  - 简单 CRUD 应用

  - 校验逻辑高度业务定制化

  - 团队规模较小且迭代快速

## 工程化实践

1. 校验框架选型

  - 基础校验：Hibernate Validator + JSR380

  - 复杂规则：Drools (规则引擎) 或自定义 DSL

  - 异步校验：Validated (响应式编程支持)
  - 校验规则管理

2. 校验元数据

3. 防御性编程
  - 输入预处理：参数标准化，去除空格，转换大小写等
  - 校验短路机制：按照校验成本排序，先校验成本低的规则
  - 错误信息分级，格式错误与业务错误分开

## 性能与安全考虑
- 校验性能优化

  - 正则表达式预编译

  - 高频校验结果缓存

  - 并行校验 (对独立校验项)

- 安全增强措施

  - 深度校验：防御嵌套对象攻击

  - 大小限制：防范 DoS 攻击 (如超大 JSON 解析)

  - 敏感字段过滤：在 DTO 层进行输出过滤

## 演进是架构策略

- 初期：集中校验，controller 层/API Gateway 层校验
- 业务复杂化：领域模型內聚校验 + 独立校验模块
- 平台化：抽象校验引擎服务，支持动态配置

## 反模式
1. 过度校验：在 DAO 层重复校验已经存在的规则
2. 校验遗漏：仅仅依赖前端校验或者第三方的调用校验
3. 错误抽象：将领域相关的相关校验放入通用模块
4. 异常滥用：校验异常不抛出，而是返回错误码


## 总结

数据验证应采用分层防御架构，在保持领域内聚性的前提下，对于高频通用校验逻辑建议抽象为独立模块。关键是要建立清晰的《校验契约文档》，明确定义各层的校验职责边界，同时通过自动化测试 (契约测试、属性测试) 确保校验逻辑的一致性。对于日均百万级以上的系统，建议将核心校验逻辑下

# 解释

## controller -  service - repository 和 API Gateway - domain - dao 之间的区别

controller - service - repository
这是一种经典的三层架构模式，在很多基于 MVC (Model - View - Controller) 思想的 Web 应用中广泛使用。

- Controller (控制器层)
职责：负责接收客户端的请求，对请求进行参数解析、验证，调用相应的 Service 层方法处理业务逻辑，最后将处理结果返回给客户端。通常与 HTTP 请求和响应直接交互，处理请求的路由、请求方法 (如 GET、POST 等)。

- Service (服务层)
职责：处理业务逻辑，协调多个 Repository 层的数据访问操作，实现业务规则和事务管理。它是业务逻辑的核心，不直接与客户端或数据库交互，而是专注于业务流程的处理

- Repository (数据访问层)
职责：负责与数据库进行交互，执行数据的增删改查操作。通常使用 ORM (对象关系映射) 框架 (如 Hibernate、MyBatis 等) 来实现，将对象映射到数据库表。



API Gateway - domain - dao
这是一种更偏向于微服务架构的分层模式，强调服务的边界和领域驱动设计 (DDD) 的思想。

- API Gateway (API 网关)
职责：作为系统的统一入口，接收客户端的所有请求，进行请求的路由、负载均衡、身份验证、限流、熔断等操作。它将客户端的请求转发到相应的后端服务，并将后端服务的响应返回给客户端。API 网关可以隐藏后端服务的复杂性，提供统一的 API 接口给客户端。

- Domain (领域层)
职责：包含业务领域的核心概念、业务逻辑和规则，是业务的核心抽象。领域层定义了领域模型 (如实体、值对象、聚合根等) 和领域服务，专注于业务问题的解决，不依赖于具体的技术实现。

- DAO (数据访问对象)
职责：与 Repository 类似，负责与数据库进行交互，执行数据的持久化操作。但在 DDD 中，DAO 更强调对领域对象的持久化，而不是简单的数据库操作。


总结：

controller - service - repository：侧重于传统的 Web 应用分层，通过 Controller 处理 HTTP 请求，Service 处理业务逻辑，Repository 进行数据访问。

API Gateway - domain - dao：更适合微服务架构，API 网关作为统一入口，Domain 层专注于业务领域的核心逻辑，DAO 层负责数据持久化。

### 对比两种架构的差异性

***相似之处***

1. 分层架构思想

两种架构都采用了分层设计理念，将不同功能模块分离，以提高代码的可维护性、可扩展性和可测试性。分层可以让每个层次专注于特定的职责，降低模块间的耦合度。

2. 持久层功能

在这两种架构中，repository 和 dao 都属于持久层，它们的主要职责都是负责与数据库或其他数据存储系统进行交互，实现数据的持久化操作，如数据的增删改查等。

3. 业务逻辑处理层

service 层和 domain 层都承担着业务逻辑处理的核心任务。它们会调用持久层的接口来获取或保存数据，并根据业务规则对数据进行处理，实现特定的业务流程。


4. 与外部交互层

controller 和 API Gateway 都处于架构的最上层，负责与外部进行交互。controller 主要接收客户端的 HTTP 请求，处理请求参数并调用相应的业务逻辑，最后返回响应结果给客户端；API Gateway 作为系统的统一入口，接收客户端的各种请求，进行请求路由、负载均衡、身份验证等操作，将请求转发到后端的服务，并将后端服务的响应返回给客户端。

***差异***

1. 架构侧重点
    - controller - service - repository：这种架构更侧重于传统的 Web 应用开发，是一种经典的三层架构模式，适用于中小型的单体应用。它强调将 Web 应用的不同功能模块进行分离，以实现代码的清晰组织和易于维护。
    
    - API Gateway - domain - dao：该架构更偏向于微服务架构和领域驱动设计 (DDD)。它将业务拆分成多个独立的领域服务，强调服务的边界和业务领域的核心概念。API Gateway 的引入使得系统能够更好地管理和保护后端服务，适用于大型的分布式系统和复杂的业务场景。

2. 与外部交互方式

   - controller：主要处理来自客户端的 HTTP 请求，通常与具体的业务接口紧密相关，一个 controller 可能对应一个或多个业务接口，直接返回业务数据给客户端。
   
   - API Gateway：作为系统的统一入口，不仅处理请求的路由和转发，还承担了一些全局的功能，如身份验证、限流、熔断等。它可以隐藏后端服务的复杂性，为客户端提供统一的 API 接口，并且可以对多个后端服务进行聚合和编排。

3. 业务逻辑抽象程度
    
    - service 层：业务逻辑的实现相对较为具体，更关注于业务流程的实现和数据的处理，通常会调用多个 repository 层的方法来完成一个业务操作。
    - domain 层：业务逻辑的抽象程度更高，它基于领域模型来设计，包含了业务领域的核心概念和规则。domain 层更注重业务的本质和内在逻辑，与具体的技术实现解耦，使得业务逻辑更加独立和可复用。**domain 层有具体业务逻辑且与技术实现解耦，DAO 层、API Gateway 层以及基础设施层共同承载了系统中各种具体的技术实现。(domin 都是抽象逻辑，它不管具体的技术实现)**

4. 适用场景
   - controller - service - repository：适合简单的 Web 应用、内部管理系统等对业务复杂度和扩展性要求不高的场景。
  
  - API Gateway - domain - dao：适用于需要处理高并发、高可用和复杂业务逻辑的大型分布式系统，如电商平台、社交网络等。
## mvc 架构也不非要出现 HTML 这种前端的东西出现在 controller 中

MVC 架构将应用程序划分为三个主要部分：

- Model (模型)：负责处理数据和业务逻辑，比如数据库操作、数据计算等。
- View (视图)：负责展示数据，是用户直接看到的界面，在传统 Web 应用中可能是 HTML 页面，在移动应用中可能是原生的 UI 界面。
- Controller (控制器)：接收用户的请求，调用 Model 进行相应的业务处理，然后根据处理结果选择合适的 View 进行展示。


在早期的 Web 开发中，MVC 架构经常与 HTML 紧密结合。Controller 接收 HTTP 请求，调用 Model 获取数据，然后将数据传递给 HTML 模板 (View) 进行渲染，最终返回给客户端

随着 Web 技术的发展，前后端分离的开发模式越来越流行。在这种模式下，Controller 不再负责渲染 HTML 页面，而是作为 API 接口的提供者，返回 JSON 或 XML 等数据格式给前端
















