# Go 基础学习笔记

> 来源：原始材料为 `~/.claude/learning/go_basic.md` 中收集的一组 Go 学习链接
> 记录时间：2026-04-13
> 整理方式：基于原始链接内容重组，作为 Go 入门到工程化的学习路线

---

## 核心观点

Go 的学习不该只停在语法层面。这组资料实际覆盖了四条主线：语言基础与接口模型、并发编程、数据协议与代码生成、数据库建模与生态资源。把这四条线串起来，才能从“会写 demo”走到“能写工程代码”。

---

## 要点整理

### 1. 语言基础先抓住 Go 的设计重心

`Tour of Go` 这条链接对应的是 methods / interfaces 章节中的后半段内容。结合章节索引，可以把这一部分理解为 Go 面向抽象建模的核心入口：

- 方法是“类型的行为”，不是类体系的成员函数迁移。
- 指针接收者和值接收者会直接影响修改语义和接口实现方式。
- 接口是隐式实现的，这让 Go 更适合做小接口、组合式设计。
- 错误本质上也是接口值，`error` 的使用是 Go 控制流的一部分，而不是异常机制的替代品。

实践理解：

- 优先思考“这个类型暴露什么行为”，再决定是否定义接口。
- 接口尽量小，比如 `io.Reader`、`io.Writer` 这种一职责抽象。
- 需要修改接收者内部状态时，优先使用指针接收者。
- 错误返回要成为函数签名的一部分，而不是最后再补。

### 2. 并发是 Go 的一等能力，但重点是通信和生命周期管理

并发教程资料把几个核心点讲得比较清楚：

- `goroutine` 是轻量级并发执行单元，由 Go runtime 调度。
- `channel` 用于 goroutine 之间通信，核心价值是通过通信协调状态，而不是直接共享内存。
- 无缓冲通道强调同步，带缓冲通道强调削峰和解耦，但缓冲不是越大越好。
- `select` 适合处理多路通信、超时和退出信号。
- `sync.WaitGroup` 解决“等待一组任务结束”的问题。
- 更进一步时，应该引入 `context` 管理取消、超时和请求级生命周期。

实践建议：

- 不要为了“用了 Go”而滥开 goroutine，先确认任务是否真的可以并发。
- channel 主要用来表达协作关系，不要把它当通用队列随处滥用。
- worker pool、超时控制、退出通知，通常都要配合 `context` 一起设计。
- 遇到共享状态时，先判断是“消息传递”还是“互斥保护”更自然。

### 3. Proto + Buf 是现代 Go 服务开发里的基础设施能力

Proto 教程和 Buf 文档组合起来，形成了一条非常实用的工程链路：

- Protocol Buffers 用 `.proto` 定义结构化数据。
- 代码生成负责把 schema 转成 Go 类型与序列化逻辑。
- Proto 的价值不只是“省去手写编解码”，更关键的是跨语言、可演进、结构稳定。
- Buf 把 schema 管理进一步工程化，提供了 `buf lint`、`buf generate`、`buf breaking` 等能力。

可以把学习重点放在这几个问题上：

- 如何定义 message、enum、service。
- 字段编号为什么必须谨慎维护。
- 如何在 schema 演进时避免 breaking change。
- 如何用 `buf.gen.yaml` 管理 Go 代码生成。

工程理解：

- `.proto` 是接口契约，不只是“生成代码的中间文件”。
- `buf lint` 用来统一规范，`buf breaking` 用来保护兼容性，这两个动作非常适合放进 CI。
- 如果项目涉及 gRPC、Connect RPC、跨服务通信，Proto/Buf 基本是必修项。

### 4. GORM 的 `has many` 关系体现的是数据建模，不只是 ORM 语法

GORM `has many` 文档核心在于说明“一对多”关联如何落地：

- 父模型通过切片字段持有关联对象。
- 子模型通过外键字段关联父模型，例如 `UserID`。
- 查询时通常用预加载减少手写 join 的复杂度。
- 关联的创建、约束和删除策略需要显式设计，不要完全依赖默认行为。

学习这部分时不要只记语法，重点要理解：

- 业务对象之间到底是什么关系。
- 外键命名、约束和级联行为是否符合数据一致性要求。
- 什么时候应该用 ORM 关联，什么时候应该回到显式 SQL。

### 5. 学习资源要分层，不要把“资源收藏”误当成“已掌握”

后面三条 GitHub 链接分别代表三种不同用途：

- `awesome-go`：查生态库和工具选型。
- `golang-developer-roadmap`：看学习路径和知识地图。
- `GoBooks`：按主题补系统性材料。

这类资源的正确用法：

- 缺库时查 `awesome-go`，不是一开始就从头刷完。
- 不知道下一步学什么时看 roadmap，校正学习顺序。
- 某一块需要体系化深入时，再按 `GoBooks` 找对应书或教程。

---

## 推荐学习顺序

1. 先用 `Tour of Go` 把方法、接口、错误模型过一遍。
2. 再补 goroutine、channel、`select`、`WaitGroup`、`context`。
3. 接着学习 Proto 基础，再上手 Buf 的生成、lint、breaking check。
4. 然后进入数据库建模，理解 GORM 关联关系背后的表设计。
5. 最后把 `awesome-go`、roadmap、`GoBooks` 当作长期参考索引，而不是短期任务清单。

---

## 实践印证

如果后续要把这份笔记转成项目能力，可以按下面顺序落地：

- 写一个包含接口、错误返回和值/指针接收者差异的小 demo。
- 写一个带 `context` 和 `WaitGroup` 的并发任务执行器。
- 用 `.proto` 定义一个简单的用户服务，并用 Buf 生成 Go 代码。
- 用 GORM 建一个 `User -> Orders` 的一对多模型，练习预加载和约束配置。

---

## 参考链接

- Go Tour Methods / Interfaces 入口: https://golang.google.cn/tour/methods/20
- Go 并发教程: https://www.runoob.com/go/go-concurrent.html
- Buf Quickstart: https://buf.build/docs/cli/quickstart/
- Buf Installation: https://buf.build/docs/cli/installation/
- GORM Has Many: https://gorm.io/docs/has_many.html
- Protocol Buffer Basics: Go: https://protobuf.dev/getting-started/gotutorial/
- Awesome Go: https://github.com/avelino/awesome-go
- Golang Developer Roadmap: https://github.com/darius-khll/golang-developer-roadmap
- GoBooks: https://github.com/dariubs/GoBooks
