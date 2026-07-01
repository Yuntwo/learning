# learning

统一管理 Go 学习实验代码与学习沉淀内容的仓库。

## 目录结构

- `basic/`：Go 语言基础示例与练习
- `concurrency/`：并发相关示例与说明
- `framework/`：常见框架与组件实验
- `app/`：小型示例应用
- `hotrow/`：热点行/库存等高并发专题实验代码
- `notes/`：学习笔记、社交发布文案、封面图、配套 HTML/SQL 等沉淀内容
- `assets/`：跨笔记复用的静态资源

## notes 约定

`notes/` 用于承接原来的学习沉淀目录内容，当前主要包含：

- `*.md`：学习笔记正文
- `*.post.md` / `*_social_post.md` / `*_xhs_post.txt`：社交平台发布文案沉淀
- `*_cover.png` / `*_cover.html`：封面图与生成模板
- `*.sql`：与文章配套的示例 SQL

Claude 的 `learning` skill 已调整为默认写入：

- `/Users/bytedance/go/src/yuntwo/learning/notes/`

## 代码与笔记的关系

仓库中的代码实验目录可以作为 `notes/` 中文章的实践印证来源。例如：

- `hotrow/` 对应热点更新、库存扣减、分桶等相关学习笔记
- `basic/`、`concurrency/` 中的示例可以为 Go 基础/并发类笔记提供代码支撑

## 迁移说明

当前仓库由原 `go_learning` 重命名为 `learning`，并将原 `~/.claude/learning` 的长期知识资产迁入 `notes/`。

明显的临时运行态文件（例如小红书登录二维码）不会作为正式内容迁入仓库。