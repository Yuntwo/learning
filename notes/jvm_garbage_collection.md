# JVM 垃圾收集器全解析

> 来源：[腾讯云开发者社区 - JVM垃圾收集器13连问](https://cloud.tencent.com/developer/article/1628831)
> 记录时间：2026-04-09

---

## 核心观点

JVM 垃圾回收的本质是自动内存管理：通过判断对象可达性，选择合适的回收算法和收集器，在不同内存分代中高效回收无用对象。

---

## 要点整理

### 一、GC 基础

- **GC（Garbage Collection）** 自动监测对象是否超过作用域，回收不再被引用的对象
- GC 线程是低优先级的，在 JVM 空闲或堆内存不足时触发
- `System.gc()` 可以建议 GC 运行，但不保证一定执行

### 二、四种引用类型

| 引用类型 | 回收时机 | 用途 |
|---------|---------|------|
| **强引用** | GC 时不回收 | 普通变量赋值 |
| **软引用** | 内存溢出前回收 | 缓存 |
| **弱引用** | 下一次 GC 回收 | WeakHashMap |
| **虚引用** | 任何时候 | GC 回收通知 |

### 三、对象可达性判断

1. **引用计数法**：引用 +1，释放 -1，为 0 则回收。缺点：无法解决循环引用
2. **可达性分析算法**：从 GC Roots 沿引用链搜索，不可达的对象即可回收（JVM 实际采用）

### 四、四种垃圾回收算法

#### 1. 标记-清除（Mark-Sweep）
- 过程：标记可回收对象 → 清除
- 优点：实现简单，不需要移动对象
- 缺点：效率低，产生大量内存碎片

#### 2. 复制算法（Copying）
- 过程：内存分两块，存活对象复制到另一块 → 清空原区域
- 优点：顺序分配，无碎片，运行高效
- 缺点：可用内存缩小为一半

#### 3. 标记-整理（Mark-Compact）
- 过程：标记可回收对象 → 存活对象压缩到一端 → 清除端边界外内存
- 优点：无碎片
- 缺点：需要移动对象，效率略低
- 适用于：老年代（对象存活率高，复制算法不适合）

#### 4. 分代收集算法
- 新生代：复制算法（对象存活率低，复制少）
- 老年代：标记-整理算法（对象存活率高）
- 商业 JVM 普遍采用

### 五、七种垃圾收集器

#### 新生代收集器（复制算法）
| 收集器 | 线程 | 特点 |
|-------|------|------|
| **Serial** | 单线程 | 简单高效，Client 模式默认 |
| **ParNew** | 多线程 | Serial 的多线程版本 |
| **Parallel Scavenge** | 多线程 | 追求高吞吐量，适合后台运算 |

#### 老年代收集器
| 收集器 | 算法 | 特点 |
|-------|------|------|
| **Serial Old** | 标记-整理 | Serial 的老年代版本 |
| **Parallel Old** | 标记-整理 | Parallel Scavenge 的老年代版本 |
| **CMS** | 标记-清除 | 低停顿，追求最短 GC 暂停时间 |

#### 全堆收集器
| 收集器 | 算法 | 特点 |
|-------|------|------|
| **G1** | 标记-整理 | JDK7+，回收整个 Java 堆，无碎片 |

### 六、CMS 收集器详解

- 全称：Concurrent Mark-Sweep
- 目标：最短回收停顿时间，适合对响应速度要求高的服务
- 启用参数：`-XX:+UseConcMarkSweepGC`
- 缺点：使用标记-清除算法会产生碎片，可能触发 Concurrent Mode Failure，退化为 Serial Old

### 七、分代回收工作流程

**内存布局：**
- 新生代占 1/3，老年代占 2/3
- 新生代内部：Eden : From Survivor : To Survivor = 8 : 1 : 1

**新生代回收流程：**
1. Eden + From Survivor 的存活对象 → 复制到 To Survivor
2. 清空 Eden 和 From Survivor
3. From/To Survivor 角色交换
4. 每次存活 age +1，达到 15（默认）时晋升老年代
5. 大对象直接进入老年代

**老年代回收：** 空间占用达到阈值后触发 Full GC，使用标记-整理算法

---

---

## 补充：HotSpot 垃圾回收器深入解析

> 来源：[CSDN - HotSpot垃圾回收器](https://blog.csdn.net/xiaoanzi123/article/details/107033275/)
> 记录时间：2026-04-16

### 八、收集器组合关系

各收集器并非独立使用，而是按新生代+老年代的组合方式搭配：

```
新生代             老年代
─────────         ─────────
Serial       ──→  Serial Old
Serial       ──→  CMS
ParNew       ──→  Serial Old
ParNew       ──→  CMS
Parallel Scavenge ──→ Serial Old
Parallel Scavenge ──→ Parallel Old

G1 同时管理新生代和老年代（不需要搭配）
```

- **JDK 8 默认**：Parallel Scavenge + Parallel Old
- **JDK 9+ 默认**：G1

### 九、各收集器详细工作机制

#### 1. Serial 收集器
- **参数**：`-XX:+UseSerialGC`（同时启用 Serial + Serial Old）
- **特点**：单线程，GC 时必须暂停所有用户线程（Stop The World）
- **适用场景**：Client 模式、小型应用（堆 < 100MB）
- **优势**：没有线程交互开销，单线程效率最高

#### 2. ParNew 收集器
- **参数**：`-XX:+UseParNewGC`
- **本质**：Serial 的多线程版本，GC 时仍需 STW
- **线程数**：默认等于 CPU 核数，可通过 `-XX:ParallelGCThreads` 调整
- **重要性**：**唯一能与 CMS 配合的新生代收集器**

#### 3. Parallel Scavenge 收集器
- **参数**：`-XX:+UseParallelGC`
- **目标**：控制**吞吐量**（= 用户代码运行时间 / 总时间）
- **核心参数**：
  - `-XX:MaxGCPauseMillis=<N>`：最大 GC 停顿时间（毫秒），值越小停顿越短但 GC 更频繁
  - `-XX:GCTimeRatio=<N>`：吞吐量大小，默认 99，即 GC 时间占比 ≤ 1/(1+99) = 1%
  - `-XX:+UseAdaptiveSizePolicy`：自适应调节策略（自动调整 Eden/Survivor 比例、晋升阈值等）
- **适用场景**：后台计算、批处理任务等对停顿不敏感的场景

#### 4. Serial Old 收集器
- **算法**：标记-整理
- **用途**：
  - Client 模式下的老年代默认收集器
  - Server 模式下作为 CMS 的后备方案（Concurrent Mode Failure 时触发）

#### 5. Parallel Old 收集器
- **参数**：`-XX:+UseParallelOldGC`
- **算法**：标记-整理，多线程
- **意义**：JDK 6 引入，让 Parallel Scavenge 有了真正的老年代搭档，实现"吞吐量优先"的完整方案

#### 6. CMS 收集器（四阶段详解）

**完整工作流程：**

| 阶段 | STW? | 说明 |
|------|------|------|
| ① 初始标记 | **是** | 仅标记 GC Roots 直接关联的对象，速度很快 |
| ② 并发标记 | 否 | 从 GC Roots 开始遍历整个对象图，与用户线程并发执行，耗时最长 |
| ③ 重新标记 | **是** | 修正并发标记期间因用户线程继续运行导致的标记变动，比初始标记稍长 |
| ④ 并发清除 | 否 | 清除已标记的垃圾对象，与用户线程并发执行 |

**三大缺点：**
1. **CPU 敏感**：并发阶段占用线程资源，默认启动线程数 = (CPU核数 + 3) / 4，CPU 少时影响大
2. **浮动垃圾**：并发清除阶段用户线程产生的新垃圾只能留到下次 GC
   - 因此老年代不能等满了再回收，需预留空间（`-XX:CMSInitiatingOccupancyFraction` 控制触发阈值）
   - 预留空间不够 → **Concurrent Mode Failure** → 退化为 Serial Old（长时间 STW）
3. **内存碎片**：标记-清除算法的固有问题
   - `-XX:+UseCMSCompactAtFullCollection`：Full GC 后开启碎片整理（默认开启，但整理过程 STW）
   - `-XX:CMSFullGCsBeforeCompaction=<N>`：执行 N 次不压缩的 Full GC 后，进行一次带压缩的 GC（默认 0，即每次都压缩）

#### 7. G1 收集器（Region 化详解）

**核心设计思想：化整为零**

G1 将整个 Java 堆划分为多个大小相等的独立区域（Region），每个 Region 大小为 1~32MB（2 的幂次），总数约 2048 个。

```
┌──────┬──────┬──────┬──────┬──────┐
│ Eden │  Old │Surv. │ Eden │Humong│
├──────┼──────┼──────┼──────┼──────┤
│  Old │ Eden │  Old │Surv. │  Old │
├──────┼──────┼──────┼──────┼──────┤
│  Old │  Old │ Eden │  Old │ Eden │
└──────┴──────┴──────┴──────┴──────┘
Region 不要求连续，角色可动态变化
```

- **Humongous Region**：大对象（超过 Region 50%）专用区域，避免大对象在新生代频繁复制
- 每个 Region 可以动态充当 Eden / Survivor / Old，不需要固定的连续内存

**G1 回收流程：**

| 阶段 | STW? | 说明 |
|------|------|------|
| ① 初始标记 | **是** | 标记 GC Roots 直接关联对象，借助 Minor GC 同步完成 |
| ② 并发标记 | 否 | 从 GC Roots 遍历对象图，找出可回收区域 |
| ③ 最终标记 | **是** | 处理并发标记遗留的 SATB 记录 |
| ④ 筛选回收 | **是** | 对各 Region 回收价值排序，优先回收价值最大的 Region（Garbage First 名称由来） |

**核心参数：**
- `-XX:+UseG1GC`：启用 G1
- `-XX:G1HeapRegionSize=<N>`：设置 Region 大小
- `-XX:MaxGCPauseMillis=200`：目标最大停顿时间（默认 200ms）
- `-XX:InitiatingHeapOccupancyPercent=45`：堆使用率达到 45% 时触发并发标记

**G1 的核心优势：**
1. **可预测的停顿模型**：通过 Region 价值排序，在指定时间内优先回收收益最大的区域
2. **无碎片**：Region 间使用复制算法，Region 内使用标记-整理算法
3. **大堆友好**：适用于 6GB 以上的堆，能有效管理超大内存

### 十、收集器选择建议

| 场景 | 推荐收集器 | 理由 |
|------|-----------|------|
| 小型应用 / Client 模式 | Serial + Serial Old | 简单高效，堆小时 STW 极短 |
| 多核 CPU + 高吞吐量 | Parallel Scavenge + Parallel Old | 吞吐量优先，适合批处理 |
| Web 服务 / 低延迟 | ParNew + CMS | 最短停顿时间，适合 B/S 架构 |
| 大堆 (≥6GB) / 可控停顿 | G1 | Region 化管理，可预测停顿 |
| JDK 9+ 通用场景 | G1（默认） | 平衡吞吐量和延迟 |

---

## 参考链接

- [原文：JVM垃圾收集器13连问](https://cloud.tencent.com/developer/article/1628831)
- [CSDN：HotSpot垃圾回收器详解](https://blog.csdn.net/xiaoanzi123/article/details/107033275/)
- Java8 已移除永久代，替换为元数据区（Metaspace，使用 native 内存）
