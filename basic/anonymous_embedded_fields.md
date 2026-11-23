# 概念
- 匿名嵌入字段(anonymous embedded field)是指在结构体里只写`类型`，不写字段名。
```go
type preliminaryLLMInput struct {
    types.AlertAnalysisInput
    IsMeasurement bool
}
```
- 上面这行`types.AlertAnalysisInput`虽然没显式写字段名，但它仍然是一个字段。
- 这个字段的隐式字段名是类型名的`未限定部分`，也就是`AlertAnalysisInput`。

# 初始化
- 在结构体字面量里，给匿名嵌入字段赋值时，要用它的隐式字段名。
```go
modelInput := preliminaryLLMInput{
    AlertAnalysisInput: input,
    IsMeasurement:      isMeasurement,
}
```
- 这里要写`AlertAnalysisInput`，不能写`types.AlertAnalysisInput`，因为字面量里写的是`字段名`，不是完整类型路径。
- 也不能直接写被提升后的子字段。
```go
preliminaryLLMInput{
    EventID: "xxx", // 错误：promoted field 不能直接作为 literal key
}
```
- 如果要在字面量里逐个填匿名嵌入结构体的字段，需要先把这个匿名字段整体写出来。
```go
preliminaryLLMInput{
    AlertAnalysisInput: types.AlertAnalysisInput{
        EventID: "xxx",
        RuleID:  "yyy",
    },
    IsMeasurement: true,
}
```

# 访问
- 匿名嵌入后，内部字段会被提升(promoted)，所以可以直接访问。
```go
modelInput.EventID
modelInput.RuleID
```
- 也可以显式通过嵌入字段访问。
```go
modelInput.AlertAnalysisInput.EventID
```

# JSON 序列化
- 匿名嵌入的结构体在 JSON 序列化时，通常会把其中可导出字段打平到外层。
```go
type A struct {
    X string `json:"x"`
}

type B struct {
    A
    Y string `json:"y"`
}
```
- 序列化结果通常是：
```json
{"x":"...","y":"..."}
```
- 如果想保留嵌套层级，就不要用匿名嵌入，而是改成普通命名字段。
```go
type B struct {
    A A `json:"a"`
    Y string `json:"y"`
}
```
- 这样序列化结果会变成：
```json
{"a":{"x":"..."},"y":"..."}
```

# 适用场景
- 想复用另一份结构体字段，并且希望调用方像访问当前结构体字段一样直接访问时，可以用匿名嵌入。
- 想把多份上下文拼成一个扁平 JSON 喂给大模型时，匿名嵌入也很方便。
- 如果更强调结构清晰、边界清晰、避免字段冲突，普通命名字段通常更稳妥。

# 注意点
- 只有可导出字段(首字母大写)才会参与 JSON 序列化。
- 如果外层结构体和嵌入结构体里有同名 JSON 字段，可能会发生覆盖或冲突，需要单独确认。
- 匿名嵌入是 Go 的语法糖；底层仍然是“有一个字段”，只是在访问和序列化时有字段提升行为。
