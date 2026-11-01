# MDFlick 项目架构

## 所有权与线程

| 模块 | 职责 | 生命周期 |
| --- | --- | --- |
| `application` | 文件路径到窗口的注册、首页复用、系统打开文件 | 应用 |
| `readerWindow` | 原生窗口、布局、链接交互、展示配置与资源服务的装配 | 窗口 |
| `documentSession` | 加载版本、取消、原文与预处理快照的统一发布 | 窗口内文档 |
| `preparedReaderDocument` | 转换后的 Markdown、对齐区间、引用已解析的表格/段落片段、Mermaid 判定 | 文档版本，只读 |
| `diagramService` | 图表准入、去重、结果状态、缓存预算、关闭后拒绝结果发布 | 文档版本 |
| `diagramRenderer` | 隐藏 WebView、串行 worker、排队期限、PNG 解码 | 图表服务 |
| `imageStore` | 跨 Markdown 区域共享图片、取消、文档预算 | 文档版本 |
| `imageResources` | 全应用图片加载并发与预算统计 | 应用 |

窗口、会话和图表服务的可变状态均由 UI 线程拥有。后台任务只处理捕获的输入和 `context.Context`；文档结果通过 `Window.Update` 发布，图表结果通过服务注入的 `publish` 调度器发布。重新载入先取消旧会话、关闭旧服务，再装配新版本的资源。窗口关闭统一经过 `readerWindow.close`。

图表结果回调绑定所属服务，不查找“当前窗口的缓存”。旧服务一旦关闭，即使已有发布回调在 UI 队列中，也不能恢复旧缓存。缓存满时保留源码，不自动淘汰，避免立即模式 UI 重建时循环提交图表。

## 两个展示适配层

`internal/document` 提供输入校验、稳定错误码、Mermaid 语法判定、标题锚点命名以及 `DisplayPolicy`。它不依赖 MyGo UI 或 WebKit。策略函数返回值，避免一个适配层修改另一个的默认设置。

| 能力 | NativePolicy | QuickLookPolicy |
| --- | --- | --- |
| 本地图片、HTTP/HTTPS 图片 | 受预算约束地加载 | 仅替代文字 |
| data 图片 | 不加载 | 不加载 |
| HTTP/HTTPS、mailto 链接 | 默认应用打开 | 不允许 |
| 相对文档链接 | 不处理 | 不允许 |
| 标题锚点 | 窗口统一路由，包括表格/对齐段落 | 生成标题 ID，同文档导航 |
| Mermaid | 隐藏 WebView 生成 PNG | 过滤后的 HTML 中生成 SVG |
| 文档脚本 | 不执行 | 不执行；CSP 仅授权内置运行时 |

原生正文、表格、对齐段落共享链接处理器。Quick Look 的 HTML 过滤与 Swift 导航委托分别执行策略；Swift 通过 C 接口查询 Go 的导航判定，避免两种语言各维护一套地址规则。重复标题的裸锚点选择第一个同名标题。

## Quick Look 渲染边界

Swift 的 `PreviewRequest` 只持有 Go 请求注册表的整数句柄，不保存 Go 指针。新请求取消旧请求；工作闭包保持句柄存活，析构时释放。Go 请求在创建时启动 15 秒期限，覆盖 Swift 排队及 Go 渲染；同一个扩展进程最多同时执行一个 Go 渲染任务。

`preview.Render` 在解析、Markdown 转 HTML、DOM 解析、过滤、输出之间检查取消，在 DOM 读取、过滤遍历和输出写入过程中继续检查。解析块最多 20,000 个，最终 HTML 最多 64 MiB；超限返回稳定错误码，不发布半份结果。最多标记前 128 个 Mermaid 图表，其余保留源码。输入仍受共享的 32 MiB / UTF-8 限制。

底层 Markdown 的单次解析/HTML 生成，以及系统文件读取不能被协作式取消硬中断；取消在调用返回后生效。期限不等于进程强杀或 WebKit JavaScript 的执行上限。WebKit 的载入、图表完成时间单独记录。上述上限也不代表进程峰值内存限制。

C 渲染接口借用输入，直接校验借用字节并生成自有字符串，避免原先 `C.GoBytes`、`io.ReadAll` 等中间副本。返回的 C 字符串仍由 Swift 通过 `MDPreviewFree` 释放。旧 `MDPreviewRender` 接口保留，内部创建临时请求以维持相同所有权约定。
