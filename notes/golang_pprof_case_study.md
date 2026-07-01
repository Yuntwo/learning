# Golang pprof 案例实战与原理解析

> 来源：https://mp.weixin.qq.com/s/Qwmo9FHCF010-0rMUbyuww
> 记录时间：2026-04-13

---

## 核心观点

这篇文章把 `pprof` 分成两层来讲：先用一个带“性能炸弹”的 demo 学会怎么查 CPU、内存、阻塞、锁竞争和 goroutine 问题，再回到 Go runtime / `net/http/pprof` 源码，解释这些指标到底是怎么采样、存储和输出的。

---

## 要点整理

### 1. pprof 的使用前提

- 通过匿名导入 `net/http/pprof`，就能把一组 `/debug/pprof/*` 路由注册到默认 `http` server。
- `block` 和 `mutex` 默认不开，需要显式开启：
  - `runtime.SetBlockProfileRate(1)`
  - `runtime.SetMutexProfileFraction(1)`
- 图形化分析通常依赖 `graphviz`。

### 2. 实战里最重要的 5 类指标

- `profile`：看 CPU 热点，核心先看 `flat` / `flat%`，因为它最能反映函数自身消耗。
- `heap`：看活跃对象和历史分配，能区分“当前还活着的内存”和“历史累计分配”。
- `block`：看 goroutine 因 `chan`、锁、条件变量等进入 waiting 的次数和时长。
- `mutex`：看锁被持有的时间，但只有出现锁竞争时才有价值。
- `goroutine`：看协程数量和堆栈，适合查 goroutine 泄漏或异常增长。

### 3. demo 中各类问题是怎么被定位出来的

- CPU：`Tiger.Eat` 通过空转循环打满 CPU，`pprof` 很快能定位到。
- Heap：`Mouse.Steal` 持续向 buffer 追加内容，导致活跃内存异常增长。
- Block：`Cat.Pee` 通过定时器/chan 触发阻塞，profile 中能看到阻塞次数和总时长。
- Mutex：`Wolf.Howl` 持锁休眠，导致锁占用时间明显偏高。
- Goroutine：通过 goroutine profile 能找到异常创建协程的位置。

### 4. CPU profile 的底层原理

- `profile` 本质是抽样，不是完整埋点。
- Go 会启动定时器，周期性向线程发送 `SIGPROF`。
- 线程收到信号后记录当前函数栈。
- 后台 writer goroutine 持续消费这些栈信息，写入 profile 结果。
- 文中还结合 `runtime.asyncPreempt` 解释了为什么纯空转循环里也会看到抢占相关调用。

### 5. heap / block / mutex 的底层原理

- 这三类指标底层都依赖 runtime 中的 bucket 结构存储。
- `heap`：
  - 内存分配经过 `mallocgc`。
  - 达到采样阈值后记录一笔分配信息。
  - GC 结束前再记录释放信息。
  - 最终按 bucket 聚合输出。
- `block`：
  - goroutine 阻塞并被唤醒后，按采样规则把阻塞次数和时长记入 `blockBucket`。
- `mutex`：
  - 解锁前按采样规则上报持锁时长，记入 `mutexBucket`。

### 6. goroutine profile 的底层原理

- 读取 goroutine profile 时，不是查一个现成快照文件，而是运行时遍历各个 goroutine，抓取它们的栈信息后输出。
- 因此它更像一次“现场盘点”。

---

## 我的收获

- 用 `pprof` 查问题时，不要一上来就钻源码，先把各 profile 的“含义边界”搞清楚。
- `CPU` 看热点，`heap` 看活跃内存，`block` 看等待，`mutex` 看持锁，`goroutine` 看数量和栈，五者关注点完全不同。
- `pprof` 不是黑盒工具，它和 Go runtime 的调度、GC、信号、内存采样机制是直接连着的。理解原理后，看到指标会更有把握，不容易误判。

---

## 参考链接

- 原文：https://mp.weixin.qq.com/s/Qwmo9FHCF010-0rMUbyuww
- 实战项目：https://github.com/wolfogre/go-pprof-practice
