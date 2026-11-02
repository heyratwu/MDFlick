# MDFlick

基于 MyGo 的 macOS Markdown 只读查看器，支持 macOS 12 及以上。

- 双击 `.md`、`.markdown` 打开，多个文件分别显示在独立窗口。
- 从应用图标启动时，首页中央显示「打开文件」按钮，选中的首个文件直接在当前窗口显示。
- 关闭最后一个窗口即退出程序；双击文件打开时复用已有首页，不额外保留默认窗口。
- `⌘O` 打开文件，`⌘R` 重新读取，支持系统「最近打开」。
- 原生 Markdown 渲染：标题、列表、表格、任务列表、代码块、文本选择与复制，跟随系统深浅色。
- HTML 排版：支持常见容器、标题对齐、有序/嵌套列表、基础表格与引用；原生显示图片链接和图片标题，保留图片尺寸，并根据系统深浅色选择 `<picture>` 来源。具体能力与边界见 [原生 HTML 支持](docs/native-html.md)。Quick Look 保持独立的过滤与图片策略。
- Mermaid 图表：主窗口和 Quick Look 均支持围栏标记为 `mermaid` 的图表，内置离线运行时，支持中文和系统深浅色；语法错误时保留源码。
- 文档内的标题锚点可跳转，安全的网页链接交给默认浏览器，文档本地及 HTTP/HTTPS 网络图片进入视口及上下 200 点预取区域后异步加载，可见图片优先；滚走后取消未完成请求，返回时可重试，已完成图片在预算内复用缓存，离屏项可回收并在返回时重新加载；单张最多 16 MiB、位图最多 1600 万像素，同时最多加载两张，网络请求超时为 15 秒；重新载入会刷新图片缓存。
- 内置 Quick Look Preview Extension：在 Finder 选中文件后按空格查看排版预览。
- 同一文档内正文、表格和 HTML 对齐段落共享图片加载与解码结果；每版文档最多同时缓存 128 个图片地址，图片缓存估算预算为每文档 128 MiB、应用合计 256 MiB，每文档最多同时加载两张、应用最多四张。超限先尝试回收本窗口的离屏图片；无法回收时保留替代文字或原尺寸占位，再次评估视口且经过重试间隔后可重试。重新载入会重置缓存。预算包含位图 RGBA 像素与保留的压缩数据，SVG 按源码大小的 8 倍加 4 KiB 估算，不代表进程或 GPU 总内存的硬上限。
- 不提供编辑、保存或自动写回。文件使用 UTF-8，单文件最大 32 MB。

架构与资源生命周期见 [项目架构](docs/project-architecture.md)，性能基准、阶段耗时和取消边界见 [性能验证](docs/performance.md)。Quick Look 的 Go 渲染采用 15 秒协作式期限，最多处理 20,000 个解析块、生成 64 MiB HTML；最多渲染前 128 个 Mermaid 图表，其余保留源码。

## 开发

使用 go.mod 指定的 Go 工具链和 macOS Command Line Tools：

```sh
go tool mygo dev
CGO_ENABLED=0 go test ./...
go tool mygo vet .
go test -race ./...
bash scripts/test-macos-bridge.sh  # macOS：实际 Swift/C/Go 边界测试
```

主窗口在后台读取并预处理文档，全应用最多两个任务并行，15 秒协作式期限包含排队时间；原文与转换后的解析结果各最多 20,000 块，转换后 Markdown 最多 64 MiB。HTML 转换复用首次解析结果；后台完成最终文档及 inline 解析后，将解析文档、对齐区间、图片属性和 Mermaid 判定独占移交 UI 线程。正文、表格、标题和对齐段落使用同一个原生 Markdown 视图，不再重建 Markdown 片段或在首帧重复解析。MyGo 仍负责布局和绘制，暂未做长文虚拟化。本地框架 API 补丁与依赖更新方式见 [MyGo 本地补丁](docs/mygo-patches.md)。

主窗口使用 `ui/markdown.ViewDocument`，通过 `Options.Code["mermaid"]` 将图表交给隐藏的 WKWebView 异步生成高清图片，按源码和主题缓存，并保留复制源码按钮。Quick Look 使用同一 MyGo Markdown parser；普通 Markdown 由 AST 直接输出安全 HTML，含原始 HTML 的文档保留完整 DOM 过滤，通过 Swift / WKWebView 展示；CSP 仅授权内置 Mermaid 脚本，不执行文档脚本、不请求网络资源。快速预览保留表格、代码和任务列表；代码按等宽字体显示，图片显示替代文字。完整窗口支持代码语法高亮、本地和网络图片；超过 32 KiB 的代码块使用完整等宽纯文本，保留选择和复制。当前锁定的 MyGo 版本不自动处理相对文档链接。

Mermaid 使用内置的 11.4.1 运行时，无需安装 Node.js 或联网。可打开 [示例文档](docs/examples/mermaid.md) 查看流程图、时序图和错误回退。单个图表最多 50,000 字符、500 条边；过大或不支持的图表会回退显示源码。主窗口的图表进入视口附近后才提交到应用共享渲染池，最多同时保留两个按需创建的 WebView，空闲 30 秒后销毁；窗口重载可复用健康运行时。每文档最多同时保留 128 个源码/主题 key，应用待执行队列容量为 16；15 秒期限包含所有排队等待，取消或超时后销毁运行时，后续任务重新创建。解码图片按 RGBA 像素估算，缓存预算为每文档 64 MiB、应用合计 128 MiB。缓存达到上限时先回收本窗口的离屏项，保留图表尺寸占位；无法回收时显示源码并允许经过重试间隔后重试，重新载入清空缓存。图片和图表在帧提交后分别保留至多 16 MiB、8 MiB 的离屏预热缓存，可见项保持驻留。图表等待队列随视口重排，离屏等待任务取消，已开始使用 WebView 的任务允许完成。Quick Look 同样按视口渲染图表，主题切换时暂缓处理离屏图表。第三方运行时版本与许可证见 [internal/mermaid](internal/mermaid/README.md)。

主窗口与 Quick Look 共享 32 MiB / UTF-8（允许 BOM）规则、校验错误码和 Mermaid 代码块判定。普通文本中的 `mermaid` 不会启动渲染器；只有大小写匹配的 `mermaid` 围栏代码块参与渲染，未闭合围栏保留源码。更多验证说明见 [测试说明](docs/testing.md)。

| 能力 | 主窗口 | Quick Look |
| --- | --- | --- |
| 标题、列表、表格、任务列表 | 原生 Markdown | 过滤后的 HTML |
| Mermaid | 离线生成 PNG，失败显示源码 | 离线 SVG，失败显示源码 |
| 系统深浅色 | 支持 | 支持 |
| 代码语法高亮 | 支持 | 等宽文本 |
| 本地图片 | 有大小与像素限制的异步加载 | 显示替代文字 |
| 外部网页链接 | 交给默认浏览器 | 禁止外部导航 |
| 相对文档链接 | 当前版本不处理 | 禁止外部导航 |
| HTTP/HTTPS 网络图片 | 有大小与像素限制的异步加载 | 显示替代文字，不请求网络 |
| 文档脚本 | 不执行 | 不执行 |

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
