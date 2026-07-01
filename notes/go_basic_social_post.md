# Go 基础社交发布草稿

## X

把 Go 学习拆成四条线会更有效：1) 方法、接口、错误模型；2) goroutine、channel、select、context；3) Proto + Buf 的 schema/生成/兼容性检查；4) GORM 关联背后的数据建模。别把资源收藏当掌握，先按工程路径学。#golang #GoLang #Backend

已发布：
- https://x.com/i/web/status/2043369174030762084

## 小红书

标题：

Go基础学习路线

正文：

最近把一份 Go 入门资料重新整理了一遍，发现真正有效的学习方式不是按零散教程刷，而是按工程能力拆成 4 条主线：

1. 语言抽象
先把方法、接口、错误模型学明白。Go 的接口是隐式实现的，值接收者和指针接收者会直接影响行为设计，这部分决定你后面写出来的代码是不是 idiomatic。

2. 并发协作
goroutine 不是重点，重点是怎么协作。channel、select、WaitGroup、context 要放在一起理解，才能真正写出可控的并发逻辑。

3. Proto + Buf
如果要做服务开发，这块基本绕不过去。`.proto` 是接口契约，Buf 则把 lint、generate、breaking check 这些工程动作补齐了，适合直接进 CI。

4. 数据建模
GORM 的 has many 不只是 ORM 写法，本质是一对多关系、外键和约束设计。先想清楚数据关系，再决定 ORM 怎么写。

我自己的结论是：
不要把 `awesome-go`、roadmap、GoBooks 这种资源仓库当成任务清单，它们更适合在缺知识点、缺库、缺路线时拿来查。

推荐顺序：
接口/方法 -> 并发 -> Proto/Buf -> GORM 建模 -> 生态资源

如果你也在补 Go，可以先照这条线过一遍，效率会高很多。

标签：

- Golang
- Go语言
- 后端开发
- ProtoBuf
- 并发编程

素材：

- 封面图：`/Users/bytedance/.claude/learning/go_basic_cover.png`
- 登录二维码：`/Users/bytedance/.claude/learning/xiaohongshu_login_qr.png`
