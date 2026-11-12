# DBX RSS 阅读器

使用 Go Sidecar 和原生 JavaScript 实现的 DBX RSS/Atom 阅读插件。订阅、文章、收藏、阅读状态和设置仅保存在本机，暂不支持跨设备同步。

当前版本：**v0.1.0** · 插件 ID：`io.github.yuzhi.rss` · [Apache License 2.0](LICENSE)

## 功能

- 支持 RSS 2.0、RSS 1.0/RDF 和 Atom 1.0，添加、刷新和批量删除订阅。
- 可折叠订阅分组，添加或移动订阅时可选择或新建分组。
- 全部/未读筛选、搜索、分页，按当前订阅、分组或全部范围一键已读。
- OPML 导入预览、去重和进度显示；导出全部、分组或单个订阅。
- 导入时选择本地文件；导出时使用 Windows“另存为”选择目录和文件名。
- 富文本正文、缺失正文时尝试提取原文、专注阅读和文章收藏分类。
- 收藏保存正文快照，删除订阅后仍可阅读。
- 原文通过电脑的系统默认浏览器或检测到的 Chrome、Edge、Firefox 打开。
- 设置图片显示及宽度、列表密度、抓取超时、文章保留数量和浏览器。
- 图片代理支持 PNG、JPEG、GIF、WebP，复用连接，并发预加载和滚动懒加载。

## 安装

需要 DBX 0.6.0+，当前发行目标为 **Windows x64、Linux x64 和 macOS x64**。

在本仓库 Releases 页面按系统下载对应包：`io.github.yuzhi.rss-0.1.0-windows-x64.dbxp`、`io.github.yuzhi.rss-0.1.0-linux-x64.dbxp` 或 `io.github.yuzhi.rss-0.1.0-darwin-x64.dbxp`，通过 DBX 插件管理器安装，再从工作台打开 RSS 阅读器。

添加订阅时填写实际 RSS/Atom 地址。RSSHub、FeedBurner 等服务输出上述标准格式时也可订阅。暂不支持 JSON Feed、网页自动发现以及需要登录或携带账号密码的订阅。

## 本机数据

数据保存于 `DBX_PLUGIN_DATA_DIR/rss/state.json`。备份或迁移时先关闭插件，再复制对应插件数据目录。OPML 仅包含订阅地址和分组，不包含文章、收藏及设置。图片在线获取，不打包为离线图片。

抓取限制响应大小，拒绝本机及局域网地址。图片代理校验公网地址、重定向、图片类型和大小（每张最多 4 MiB）。

## 开发与验证

需要 Go 1.27.1+、Node.js 20+，以及 [DBX Plugin CLI](https://dbxio.com/cn/docs/plugin-development)。

```powershell
npm install --global @dbx-app/plugin-cli
go -C backend test ./... -count=1
go -C backend vet ./...
node --check ui/app.js
node tests/ui.test.cjs
```

如果 Windows 默认 Go 缓存目录不可写，可先设置：

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'dbx-rss-gocache'
```

本地打包：

```powershell
dbx-plugin package . --target windows-x64 --output-dir dist
```

输出为 `dist/*.dbxp` 和 `dist/*.artifact.json`。开发宿主需设置 `DBX_PLUGIN_DATA_DIR`，推荐使用项目内 `.dbx-dev/data`，该目录不进入发行包。

## GitHub 自动构建与发布

[工作流](.github/workflows/release.yml) 在推送代码及 Pull Request 时分别在 Windows、Linux 和 macOS 原生 runner 上测试并构建对应平台的 x64 插件，产物保存在 Actions Artifacts 中，也支持手动运行。

推送 `v*` 标签时，工作流验证标签与 manifest、Go 后端、界面版本一致，三平台构建全部通过后自动创建 GitHub Release，上传 Windows、Linux 和 macOS 插件包、构建元数据和 SHA-256 校验文件，并生成发布说明。使用内置 `GITHUB_TOKEN`，无需额外发布密钥。插件包当前未签名。

版本统一固定为 `0.1.0`，发布标签为 `v0.1.0`。先将代码提交并推送到 GitHub，再执行：

```bash
git tag v0.1.0
git push origin v0.1.0
```

本地配置不会自动推送代码或发布远程 Release。

## 技术结构

```text
DBX Workbench iframe (ui/)
    → window.dbxPlugin.invoke
Go Sidecar (backend/main.go)
    → rss.Service / RSS、Atom 解析与抓取 / 图片代理
    → 原子 JSON 文件存储
本机插件数据目录
```

| 目录 | 用途 |
| --- | --- |
| `ui/` | HTML/CSS/JavaScript 界面，无 CDN 依赖 |
| `backend/` | Go Sidecar Protocol v1 JSON-RPC |
| `backend/rss/` | 订阅、文章、分组、收藏、设置及持久化 |
| `tests/` | 前端回归测试 |
| `assets/` | 插件图标 |
| `.github/workflows/` | 自动构建与标签发布 |
