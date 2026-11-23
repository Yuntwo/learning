# Java 转 Go 算法题语法速记

> 来源：基于 2026-08-16 与 Claude 的对话整理
> 记录时间：2026-08-16

---

## 核心观点

Java 转 Go 做算法题时，最大的难点不是算法思路本身，而是脑子里还残留着 Java 的语法惯性：函数签名位置、数组和切片、map 判断存在、字符串和字符处理、树链表结构定义这些地方最容易写错。把这些高频差异压缩成一套固定模板后，Go 写题的速度就会明显稳定下来。

---

## 为什么值得看

这类内容特别适合已经会用 Java 刷题、但中途切到 Go 的人。通常不是不会做题，而是：

- 明明知道该用哈希表，但一时想不起 Go 的 map 判断写法。
- 想写 while，突然反应过来 Go 只有 for。
- 想当然把 `int[]`、`String.charAt()`、`containsKey()` 那套习惯带过来。
- 在链表、二叉树、DFS 闭包、排序这些题里频繁被语法卡住。

所以这份笔记的重点不是完整 Go 教程，而是“面试手撕算法最常用、最容易混的语法差异”。

---

## 关键概念拆解

### 1. Go 不是 Python 化的 Java，而是更简洁的静态类型语言

Go 看起来比 Java 短很多，`if` / `for` 不加括号，`range` 遍历也有点像 Python，但本质仍然是强类型语言。写算法题时，最应该保留的心智模型是：

- 思路上仍然按静态类型语言组织。
- 语法层面比 Java 更轻。
- 不是 Python 那种“容器和语法糖很自由”的风格。

### 2. Go 的高频坑主要集中在“语法默认值”和“容器行为”

比如：

- map 取不到 key 不报错，而是返回零值。
- 字符串下标得到的是 `byte`，不是 Java 里的 `char` 对象语义。
- `append` 可能返回新切片，必须接住。
- 数组 `[n]int` 和切片 `[]int` 是不同概念，做题时几乎总是用切片。

### 3. 算法题里要形成固定模板，而不是临场回忆细节

最好的方式不是记零散语法点，而是把数组、map、set、DFS、BFS、链表、二叉树、排序这些常见套路固化成可复制模板。这样进到面试时，更多精力可以放在思路，而不是语法回忆。

---

## 重点对比表

| 维度 | Java | Go | 我的理解 |
|------|------|----|----------|
| 函数签名 | `int f(int a, int b)` | `func f(a, b int) int` | Go 把类型放在变量名后面，返回值写在最后 |
| 局部变量初始化 | `int x = 1;` | `x := 1` | 刷题时优先用 `:=` |
| 数组参数 | `int[] nums` | `[]int` | Go 刷题几乎都传切片，不传数组 |
| while 循环 | `while (l < r)` | `for l < r {}` | Go 没有 while，全部统一成 for |
| 哈希表判断存在 | `map.containsKey(x)` | `v, ok := mp[x]` | 这是 Java 转 Go 最常写错的地方之一 |
| 字符串长度 | `s.length()` | `len(s)` | Go 基本统一 `len(...)` |
| 动态数组追加 | `list.add(x)` | `arr = append(arr, x)` | `append` 返回值必须接住 |
| 集合 | `HashSet<Integer>` | `map[int]struct{}` | Go 没有内置 set，通常拿 map 模拟 |
| 空指针 | `null` | `nil` | 树和链表判断空节点时高频出现 |
| 多返回值 | 不支持原生多返回值 | 支持 `func f() (int, int)` | Java 需要封装对象/数组，Go 可以直接多返回 |
| 三元表达式 | `a > b ? a : b` | 不支持 | 只能写 if |

---

## 要点整理

### 1. 函数签名和变量声明是第一层切换成本

Java：

```java
public int twoSum(int[] nums, int target) {
    return 0;
}
```

Go：

```go
func twoSum(nums []int, target int) int {
    return 0
}
```

关键区别：

- Go 的参数写法是“变量名 + 类型”。
- 返回值放在参数列表后面。
- 多个同类型参数可以合并：`a, b int`。

变量声明在算法题里推荐按这个优先级记：

```go
x := 1
s := "abc"
mp := make(map[int]int)
arr := []int{}
```

其中 `:=` 是最常用写法，但只能在函数内部使用。

### 2. Go 只有 for，没有 while

Java：

```java
while (l < r) {
    l++;
}
```

Go：

```go
for l < r {
    l++
}
```

做双指针、二分、滑动窗口时，这个转换会频繁出现。可以直接把脑内规则改成：

- 普通计数循环：`for i := 0; i < n; i++ {}`
- while 风格循环：`for condition {}`
- 遍历容器：`for i, v := range nums {}`

### 3. 数组和切片不是一回事，刷题几乎都用切片

Go 里：

```go
var a [5]int
b := []int{1, 2, 3}
nums := make([]int, 5)
```

理解方式：

- `[5]int` 是定长数组。
- `[]int` 是切片，才是算法题里最常用的“动态数组视角”。
- 函数参数基本都写成 `[]int`。

常见追加：

```go
arr := []int{}
arr = append(arr, 1)
arr = append(arr, 2, 3)
```

注意：`append` 可能返回底层扩容后的新切片，所以必须写成 `arr = append(...)`。

### 4. map / set 是刷题最关键的容器差异

Java 写 two sum 时，常见思路是：

```java
Map<Integer, Integer> map = new HashMap<>();
if (map.containsKey(x)) {
    return new int[]{map.get(x), i};
}
```

Go 对应写法：

```go
mp := make(map[int]int)
if j, ok := mp[x]; ok {
    return []int{j, i}
}
```

需要重点记住：

- `mp[x]` 即使不存在也不会报错，只会返回类型零值。
- 所以“是否存在”必须通过 `v, ok := mp[x]` 来判断。
- 删除 key 用 `delete(mp, x)`。

Go 没有内置 set，常见替代方案：

```go
set := make(map[int]struct{})
set[x] = struct{}{}
_, ok := set[x]
```

如果只想图省事，写成 `map[int]bool` 也可以。

### 5. 字符串和字符处理比 Java 更需要留心

Java：

```java
char c = s.charAt(0);
int n = s.length();
```

Go：

```go
c := s[0]
n := len(s)
```

这里最容易忽略的是：

- `s[i]` 得到的是 `byte`。
- 对英文题通常够用，比如判断 `'a'`、`'b'`。
- 涉及 Unicode 时要转成 `[]rune`。

示例：

```go
rs := []rune(s)
fmt.Println(rs[0])
```

另外，Go 字符串默认不可直接原地修改，想改字符通常要先转 `[]byte` 或 `[]rune`。

### 6. 树和链表的定义要适应 struct + 指针

Java：

```java
class ListNode {
    int val;
    ListNode next;
}
```

Go：

```go
type ListNode struct {
    Val  int
    Next *ListNode
}
```

创建节点：

```go
node := &ListNode{Val: 1}
```

判空：

```go
if node == nil {
    return
}
```

这里和 Java 最大的切换点是：

- Go 用 `struct`，不是 `class`。
- 链表和树几乎都会配合指针使用。
- 判空从 `null` 变成 `nil`。

### 7. DFS 闭包递归写法是一个隐藏高频坑

普通递归函数不难：

```go
func dfs(root *TreeNode) {
    if root == nil {
        return
    }
    dfs(root.Left)
    dfs(root.Right)
}
```

但如果想在函数内部写闭包递归，Go 要先声明再赋值：

```go
var dfs func(node *TreeNode)
dfs = func(node *TreeNode) {
    if node == nil {
        return
    }
    dfs(node.Left)
    dfs(node.Right)
}
```

这是很多 Java 使用者在回溯、树形 DP、图搜索里容易卡住的点。

### 8. Java 没有原生多返回值，Go 可以直接返回多个结果

这也是 Java 转 Go 时一个非常直观的语法差异。

Java 方法一次只能 `return` 一个值。

比如下面这种写法在 Java 里是合法的：

```java
public int add(int a, int b) {
    return a + b;
}
```

但如果想同时返回两个结果，Java 不能像 Go 那样直接写多个返回值。也就是说，这种思路在 Java 里行不通：

```java
// Java 不支持这种语法
return a + b, a - b;
```

Java 里常见替代方式有三种：

- 返回自定义对象
- 返回数组
- 新版本 Java 用 `record` 封装结果

例如：

```java
record Result(int sum, int diff) {}

public Result calc(int a, int b) {
    return new Result(a + b, a - b);
}
```

而 Go 可以非常自然地直接返回多个值：

```go
func calc(a, b int) (int, int) {
    return a + b, a - b
}
```

调用时也可以直接接收：

```go
sum, diff := calc(8, 3)
```

这个差异在算法题里特别常见，因为 Go 很适合返回“值 + 状态”这样的组合，比如：

- `v, ok := mp[x]`
- `node, exists := ...`
- 某些递归函数返回“答案 + 是否成立”

所以可以把它当成一条非常重要的迁移结论：

- Java：多结果通常要封装
- Go：多结果通常直接返回

### 9. 排序、自增和条件表达式这些细节也容易失分

排序：

```go
sort.Ints(nums)
sort.Slice(intervals, func(i, j int) bool {
    return intervals[i][0] < intervals[j][0]
})
```

自增：

```go
i++
i--
```

但 Go 的 `++` / `--` 只能单独成句，不能写进表达式。

Java 常见三元写法：

```java
int x = a > b ? a : b;
```

Go 不支持三元运算符，只能老老实实写 if：

```go
x := b
if a > b {
    x = a
}
```

---

## 高频模板

### two sum

```go
func twoSum(nums []int, target int) []int {
    mp := make(map[int]int)
    for i, v := range nums {
        x := target - v
        if j, ok := mp[x]; ok {
            return []int{j, i}
        }
        mp[v] = i
    }
    return []int{}
}
```

### 队列模拟 BFS

```go
q := []*TreeNode{root}
for len(q) > 0 {
    node := q[0]
    q = q[1:]
    if node.Left != nil {
        q = append(q, node.Left)
    }
    if node.Right != nil {
        q = append(q, node.Right)
    }
}
```

### 集合判重

```go
set := map[int]struct{}{}
for _, v := range nums {
    if _, ok := set[v]; ok {
        return true
    }
    set[v] = struct{}{}
}
```

---

## 我的总结

如果只从“手撕算法”这个目标看，Java 转 Go 不需要系统重学整门语言，先把下面这些点打牢就够了：函数签名、`:=`、`for` 代替 while、切片、`append`、map 的 `ok` 写法、`nil`、struct 指针、字符串下标和 DFS 闭包递归。把这些写成固定反应后，Go 刷题的阻力会明显下降。

换句话说，真正该练的不是 Go 的全部语法，而是“Java 习惯切换到 Go 刷题模板”的那一步。

---

## 原文链接

> 本次内容来自对话整理，无外部原文链接。
