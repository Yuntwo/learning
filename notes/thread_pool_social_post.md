# 线程池 - 社交发布草稿

> 自动发布失败（Twitter 402 付费、小红书 MCP 弹窗遮挡），保留文案供手动发布。

---

## Twitter/X

```
线程池不只是"new ThreadPoolExecutor"：

1. 场景：异步任务用 ThreadPoolExecutor，心跳用 ScheduledThreadPoolExecutor 递归再调度
2. 三步执行：核心线程 → 队列 → max（队列是第二关）
3. LinkedBlockingQueue 默认无界，小心 OOM
4. beforeExecute/afterExecute 埋耗时，ThreadLocal 记得 remove
5. 拒绝策略别默默丢，落库+告警才是生产姿势

#Java #并发编程 #线程池
```

---

## 小红书

**标题**：线程池的 5 条生产级心得

**正文**：
```
做 Java 后端绕不开线程池，但真正用好它，靠的不是背参数。

1️⃣ 两大经典场景
· 异步任务（邮件/通知）用 ThreadPoolExecutor，主流程立即返回
· 周期任务（心跳/轮询）用 ScheduledThreadPoolExecutor，通过"递归再调度"持续触发

2️⃣ 三步执行逻辑要记牢
任务进来：核心线程 → 队列 → max 线程 → 拒绝
很多人以为"量大直接扩到 max"，其实队列是第二关。

3️⃣ 队列选型决定稳定性
· ArrayBlockingQueue：有界，有背压
· LinkedBlockingQueue：默认无界，生产环境 OOM 隐患
· SynchronousQueue：不缓冲，直接交给线程

4️⃣ 监控 Hook 别漏
继承 ThreadPoolExecutor，重写 beforeExecute / afterExecute，用 ThreadLocal 记录耗时。
⚠️ ThreadLocal 一定要 remove()，线程复用会串味 + 内存泄漏。

5️⃣ 拒绝策略要"落地"
默认 4 种策略里 Discard 最危险——任务静默消失。
生产环境应该自定义：落库、发 MQ、上报告警，至少让被拒任务"看得见"。

进阶：单池扛不住时，做"主队列 → 子队列"二级分发，按 key 分片并行消费。
```

**标签**：#Java #后端开发 #程序员 #并发编程 #线程池

**封面图**：`~/.claude/learning/thread_pool_cover.png`
