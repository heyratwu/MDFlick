# MDFlick

基于 MyGo 的 macOS Markdown 只读查看器，支持 macOS 12 及以上。

- 双击 `.md`、`.markdown` 打开，多个文件分别显示在独立窗口。
- 从应用图标启动时，首页中央显示「打开文件」按钮，选中的首个文件直接在当前窗口显示。
- 关闭最后一个窗口即退出程序；双击文件打开时复用已有首页，不额外保留默认窗口。
- `⌘O` 打开文件，`⌘R` 重新读取，支持系统「最近打开」。
- 原生 Markdown 渲染：标题、列表、表格、任务列表、代码块、文本选择与复制，跟随系统深浅色。
- Mermaid 图表：主窗口和 Quick Look 均支持围栏标记为 `mermaid` 的图表，内置离线运行时，支持中文和系统深浅色；语法错误时保留源码。
- 文档内的标题锚点可跳转，安全的网页链接交给默认浏览器，文档同目录图片由 MyGo 加载。
- 内置 Quick Look Preview Extension：在 Finder 选中文件后按空格查看排版预览。
- 不提供编辑、保存或自动写回。文件使用 UTF-8，单文件最大 32 MB。

## 开发

使用 go.mod 指定的 Go 工具链和 macOS Command Line Tools：

```sh
go tool mygo dev
CGO_ENABLED=0 go test ./...
go tool mygo vet .
```

主窗口使用 `ui/markdown.View`，通过 `Options.Code["mermaid"]` 将图表交给隐藏的 WKWebView 异步生成高清图片，按源码和主题缓存，并保留复制源码按钮。Quick Look 使用同一 MyGo Markdown parser 生成经过过滤的 HTML，通过 Swift / WKWebView 展示；CSP 仅授权内置 Mermaid 脚本，不执行文档脚本、不请求网络资源。快速预览保留表格、代码和任务列表；代码按等宽字体显示，图片显示替代文字。完整窗口支持代码语法高亮和本地图片。当前锁定的 MyGo 版本不自动处理相对文档链接。

Mermaid 使用内置的 11.4.1 运行时，无需安装 Node.js 或联网。可打开 [示例文档](docs/examples/mermaid.md) 查看流程图、时序图和错误回退。单个图表最多 50,000 字符、500 条边；过大或不支持的图表会回退显示源码。第三方运行时版本与许可证见 [internal/mermaid](internal/mermaid/README.md)。

## 构建 macOS 安装包

**使用下面的脚本构建发布产物**；单独运行 `go tool mygo build` 只会生成主程序，不会包含 Quick Look 扩展。

```sh
bash scripts/build-macos.sh arm64  # Apple Silicon
bash scripts/build-macos.sh amd64  # Intel
```

需要 Apple SDK 中的 `swiftc`、`codesign`、`hdiutil`。输出在 `build/darwin-arm64/` 或 `build/darwin-amd64/`，包含 `MDFlick.app` 和 DMG。脚本先构建 MyGo 主应用，再编译并嵌入 `Contents/PlugIns/MDFlickQuickLook.appex`，将 Go 渲染器以静态库链接到扩展，对扩展和主程序依次签名，最后创建 DMG。扩展直接读取并渲染文档，不启动辅助进程。WebKit 使用 network client entitlement 启动其子进程；文档的 CSP 和导航策略仍阻止外部网络内容。

默认使用本机 ad hoc 签名，适合本地开发。对外分发时，设置自己的正式 bundle identifier（同步更新 `mygo.json` 与扩展 plist），使用 Developer ID：

```sh
SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)' bash scripts/build-macos.sh arm64
```

签名身份须已存在于钥匙串。脚本不自动公证；公开分发前，使用 `xcrun notarytool submit <dmg> --keychain-profile <profile> --wait` 提交，批准后用 `xcrun stapler staple <dmg>` 装订票据。

## 安装与使用

1. 打开 DMG，把 MDFlick 拖到「应用程序」，启动一次。
2. 双击 Markdown 文件。如果此前已关联其他软件，在 Finder「显示简介 → 打开方式」选择 MDFlick，再点「全部更改」设为默认。应用注册为 Viewer，并尊重已有的默认应用选择。
3. 在 Finder 选择 Markdown 文件，按空格使用 Quick Look。如果没有启用，在「系统设置 → 登录项与扩展 → 快速查看」启用 MDFlick Markdown Preview（不同 macOS 版本入口名称可能不同）。

开发调试可检查注册状态：

```sh
pluginkit -m -i com.mdflick.app.QuickLook
mdls -name kMDItemContentType /path/to/example.md
```

如预览仍由其他 Markdown 扩展提供，可在系统设置中关闭冲突的扩展。文件的内容类型应为 `net.daringfireball.markdown`。无需安装全局键盘钩子。

Quick Look 使用 Apple 的 [QLPreviewingController](https://developer.apple.com/documentation/QuickLookUI/QLPreviewingController) 与 [Quick Look 扩展](https://developer.apple.com/documentation/quicklook)机制；需在实际安装后验证 Finder 的扩展启用状态。测试不会更改本机默认文件关联或系统扩展设置。
