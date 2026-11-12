# 收藏与批注实施计划

**Goal:** 实现已批准的收藏分类、整篇笔记/选区批注和专注护眼阅读，发行 0.3.0。
**Spec:** docs/superpowers/specs/2026-11-12-collections-annotations-design.md
**Architecture:** 扩展原子 JSON 状态；独立文章快照支撑收藏和笔记；前端以文本锚点恢复标记，避免修改源文章 HTML。
**Tech Stack:** Go 标准库、DBX SDK、原生 JS/CSS。

1. [ ] backend/rss/library_test.go：验证分类、收藏、笔记与快照重启/删除/清理后的存活，校验输入与失败回滚。
2. [ ] backend/rss/library.go 和 annotations.go：实现分类 CRUD、收藏快照、分页查询、批注 CRUD，扩展 State/snapshot 与刷新合并。
3. [ ] backend/rss/features.go：兼容旧设置，增加阅读配色、字号、行距和宽度；测试边界。
4. [ ] backend/library_rpc.go：新增 RPC，以后端本地文章作为快照来源，拒绝客户端伪造正文。
5. [ ] ui/annotations.js：先测试文本锚点算法，再实现跨节点标记、选区工具和标记编辑；ui/library.js 实现收藏分类与笔记中心；与 app.js 阅读器集成。
6. [ ] ui/reading.js、styles.css：DOM 弹窗、专注阅读、主题和字号控制、键盘与窄窗口退出。
7. [ ] 验证 Go tests/vet、Node 测试、实际开发宿主交互；更新 README/版本并打包 Windows x64。

用户已经批准实施；在本会话内顺序执行，无需再次询问执行方式。当前目录无 Git 仓库，不创建分支或提交。
