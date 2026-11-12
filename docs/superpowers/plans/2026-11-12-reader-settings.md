# RSS 阅读管理实施计划

**Goal:** 在现有本机 Go 插件上增加分组、范围已读、阅读设置和系统浏览器打开。

**Architecture:** 保留 state.json 并以兼容字段扩展分组与设置。Go 提供本地浏览器调用及图片代理，前端仅通过插件 RPC 调用；不使用 host.openUrl。

**Tech Stack:** Go 标准库、现有 DBX SDK、原生 HTML/CSS/JavaScript。

**Constraints:** 数据仅本机；Apache-2.0；Windows x64；现有数据兼容；弹窗操作使用按钮而非原生表单。

- [ ] 在 backend/rss/features_test.go 先验证分组持久化、删除分组保留订阅、跨分页范围已读、旧数据默认设置、设置校验、浏览器地址校验。
- [ ] 扩展 service.go 的状态与分组筛选，在 features.go 实现分组和设置事务、范围已读、抓取超时及保留上限。
- [ ] 在 browser_windows.go 使用 ShellExecuteW 打开默认浏览器或已检测的固定浏览器程序；在 media.go 获取限量栅格图片供 data URL 显示。
- [ ] 在 main.go 暴露分组、设置、浏览器和图片 RPC；在 ui 中增加分组管理、移动订阅、当前范围全部已读、设置中心、图文阅读。
- [ ] 执行 Go 测试/vet、JS 测试/语法检查，验证真实开发页面，更新 README 与版本至 0.2.0 并打包。
