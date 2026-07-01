# 线程池的经典应用场景

> 来源：https://juejin.cn/post/7070873571899211812
> 作者：DannyIdea（腾讯基础架构工程师）
> 原文发布：2022-03-03
> 记录时间：2026-04-17

---

## 核心观点

线程池的本质是"生产者-消费者模型"的工程化：用一组常驻线程消费任务队列，避免频繁创建/销毁线程，同时通过队列缓冲和拒绝策略控制系统资源。真正用好线程池，关键不是参数调优，而是根据场景选对模型（异步任务 / 定时任务 / 多消费者分发）并做好可观测。

---

## 要点整理

### 1. 两个经典场景

**场景一：异步邮件通知**
- 邮件发送属于耗时且非关键路径操作，放到线程池中异步执行，主流程立即返回。
- 模型：普通 `ThreadPoolExecutor` + `submit(Runnable)`。

**场景二：心跳任务**
- 定期上报心跳、健康检查、定时轮询等周期性任务。
- 模型：`ScheduledThreadPoolExecutor` + `schedule(...)`，通常通过"递归再调度"实现持续心跳（每次任务执行完再次调 `schedule`）。

### 2. 从单消费者到线程池

最朴素的消费者模型是一个线程 + `BlockingQueue`：
- 生产者 `put`、消费者 `take`，队列满/空时自动阻塞。
- **局限**：消费者能力有限时，队列容易堆积，单线程消费无法横向扩展。

线程池是对这个模型的多消费者扩展：底层仍然是一个 `BlockingQueue`，但维护了一个 `HashSet<Worker>` 来管理多个工作线程，并用 CAS 保证 worker 数量的线程安全。

### 3. ThreadPoolExecutor 三步执行逻辑

任务提交到线程池时：
1. **核心线程未满** → 创建新核心线程执行任务；
2. **核心线程已满** → 放入 `workQueue` 等待；
3. **队列也满了** → 创建临时线程，直到达到 `maximumPoolSize`；
4. **已达 max** → 触发 `RejectedExecutionHandler`。

理解这个顺序是线程池参数调优的基础，很多人误以为"任务量大就直接创建到 max"，其实队列是第二关。

### 4. 核心参数速记

| 参数 | 作用 |
|------|------|
| `corePoolSize` | 常驻线程数，即使空闲也不回收 |
| `maximumPoolSize` | 峰值线程数上限（队列满了才会扩到这个值） |
| `keepAliveTime` | 非核心线程空闲存活时间 |
| `workQueue` | 缓冲队列（Array/Linked/Synchronous） |
| `threadFactory` | 自定义线程名、优先级、daemon 属性 |
| `RejectedExecutionHandler` | 任务被拒绝时的处理策略 |

**队列选型**：
- `ArrayBlockingQueue`：有界，背压明显，适合资源敏感场景；
- `LinkedBlockingQueue`：默认无界，要注意 OOM 风险；
- `SynchronousQueue`：不存储任务，强制立即交给线程（`newCachedThreadPool` 用的就是它）。

### 5. 实战代码

**异步邮件服务：**
```java
@Service
public class SendEmailServiceImpl implements SendEmailService {
    @Resource
    private ExecutorService emailTaskPool;

    @Override
    public void sendEmail(EmailDTO emailDTO) {
        emailTaskPool.submit(() -> {
            System.out.printf("sending email .... emailDto is %s%n", emailDTO);
            Thread.sleep(1000);
            System.out.println("sended success");
        });
    }
}
```

**心跳任务（递归再调度）：**
```java
@Service
public class HeartBeatTaskServiceImpl implements HeartBeatTaskService {
    @Resource
    private ScheduledThreadPoolExecutor scheduledThreadPoolExecutor;

    public void sendBeatInfo() {
        HeartBeatInfo info = new HeartBeatInfo();
        info.setInfo("test-info");
        info.setNextSendTimeDelay(1000);
        scheduledThreadPoolExecutor.schedule(
            new HeartBeatTask(info),
            info.getNextSendTimeDelay(),
            TimeUnit.MILLISECONDS);
    }
}
```

**线程池配置（配合自定义 ThreadFactory 做线程命名）：**
```java
@Configuration
public class ThreadPoolConfig {
    @Bean
    public ExecutorService emailTaskPool() {
        return new ThreadPoolExecutor(
            2, 4,
            0L, TimeUnit.MILLISECONDS,
            new LinkedBlockingQueue<>(),
            new SysThreadFactory("email-task"));
    }
}
```

### 6. 监控 Hook：线程池的可观测性

通过继承 `ThreadPoolExecutor` 并重写 `beforeExecute` / `afterExecute`，可以记录每个任务的执行耗时：

```java
public class SysThreadPool extends ThreadPoolExecutor {
    private final ThreadLocal<Long> startTime = new ThreadLocal<>();

    @Override
    protected void beforeExecute(Thread t, Runnable r) {
        startTime.set(System.currentTimeMillis());
    }

    @Override
    protected void afterExecute(Runnable r, Throwable t) {
        long cost = System.currentTimeMillis() - startTime.get();
        logger.info("Thread {}: ExecuteTime {}", r, cost);
        startTime.remove(); // 注意释放 ThreadLocal，避免内存泄漏
    }
}
```

关键指标建议采集：`activeCount`、`queueSize`、`completedTaskCount`、任务耗时分布、拒绝计数。

### 7. 拒绝策略不只是"抛异常"

JDK 默认 4 种：`AbortPolicy`（抛异常）、`CallerRunsPolicy`（调用线程自己执行）、`DiscardPolicy`（静默丢弃）、`DiscardOldestPolicy`（丢最老任务）。

生产环境通常需要**自定义策略**：落库 / 上报监控 / 告警，而不是让任务默默消失：

```java
new RejectedExecutionHandler() {
    @Override
    public void rejectedExecution(Runnable r, ThreadPoolExecutor executor) {
        log.warn("Task rejected: {}", r);
        // 持久化到 DB / 发到 MQ / 上报告警
    }
}
```

### 8. 进阶模式：多消费者队列分发

当单一线程池吞吐不够、且任务之间可分片时，可以构建**主队列 → 子队列**的二级分发：主线程池从主队列取任务、按 key hash 分发到多个子线程池的子队列，子队列各自并行消费。适合订单处理、日志聚合等需要按某个维度保序又要并行的场景。

---

## 实践印证

- 字节场景里很多消费类服务（邮件、Push、IM 异步转存）都是类似模型，线程池参数通常配置为 `corePoolSize = CPU * N`（N 视任务 I/O 比例而定）。
- 心跳场景如果追求更强的稳定性，可以叠加"任务漂移检测"——每次执行时记录实际起始时间和期望时间的差值，超过阈值说明线程池被堵塞。
- `ThreadLocal` 在监控 Hook 里务必 `remove()`，否则线程复用时会导致数据串味 + 内存泄漏。

---

## 参考链接

- 原文：https://juejin.cn/post/7070873571899211812
- 作者：DannyIdea（腾讯基础架构）
- 相关：《Java 并发编程实战》第 8 章——线程池的使用
