# 性能验证与预览资源边界

## 当前版本：首屏、滚动与内存采样

主窗口设置 `MDFLICK_TRACE=1` 后，为每次打开/重载分配 request ID。`native.open-to-publish` 包含路径处理、窗口创建（新窗口）、排队、读取和预处理；`native.open-to-first-present` 再包含主线程发布、首帧构建、布局、绘制和 surface 提交。计时从应用收到打开请求开始，不包括用户在文件选择器内停留的时间。首页、错误页、加载占位符不计作正文首屏；同一版本只记录一次，旧版本回调不会记入新文档。

`native.main-to-ready` 测 main 入口到框架 ready。冷启动的进程创建至正文提交时间由下方 runner 的外部单调时钟补充，包含 main 之前的启动成本；该时间仍含日志传输调度误差。热打开应保持进程存活，通过 `⌘O` 打开不同文档，以该 request 的 `native.open-to-first-present` 为准。`⌘R` 是重载，不能当成新窗口热打开。

`MYGO_FRAME_STATS=all` 使用框架现有的逐帧 build/layout/paint/present、分配和 GC 记录。只在诊断时开启，日志与内存采样也会产生开销。`native.image-load-decode` 记录每个唯一图片任务的排队/加载/解码结束时间（成功或失败都记录）；正文首屏不等待图片和图表全部完成。

### 可重现的原生运行

```sh
python3 scripts/make-perf-fixtures.py
# 用独立的诊断二进制，包含框架识别文件参数所需的关联元数据。
go build -ldflags='-X github.com/egoist/mygo.packageFileExtensions=md,markdown' -o build/perf/MDFlick .
python3 scripts/profile-macos.py build/perf/MDFlick build/perf/fixtures/normal.md
# 已用 MyGo 构建的 .app 可直接传入其 Contents/MacOS 下的实际可执行文件。
```

runner 每 0.5 秒从 `ps` 采样主进程 RSS 与可识别子进程 RSS，并给应用日志附加从进程创建前开始的单调时间。关闭最后一个窗口即结束采样；`Ctrl+C` 或 `--seconds 30` 只结束本次启动的进程，不是正常关窗回落测试。传入文档但没有测到正文提交时，runner 报错，避免把无文件元数据的普通 `go build` 产物首页误当成首屏。

依次用 `normal.md`、`long.md`、`tables.md`、`aligned.md`、`images.md`、`mermaid.md` 各做至少 5 次独立冷启动，记录正文首屏中位数/p95。合成语料离线可重现；普通文档需再配合真实 README 样本。重复运行受到字体、磁盘和 GPU 缓存影响，不能解释为清空系统缓存后的启动保证。

- 热打开：保持一个窗口，`⌘O` 打开其他语料，逐 request 记录首屏时间。
- 滚动：单独运行长文、表格文档，先等首次内容稳定再滚动；从日志中取滚动时段的 frame p50/p95。runner 末尾汇总的是全运行帧，包含启动帧，不自动等同于滚动成绩；无帧期间不计入帧耗时。
- 多图：等六张本地图片完成后记录 RSS，再重载及关闭文档；首屏内存并非图片全部完成后的内存。
- 多窗口：一次传入 `window-1.md` 到 `window-4.md`，再打开 `images.md`；逐个关闭，保留一个小文档窗口，观察立即和 5 秒后的 RSS/Go heap/预算。关闭最后窗口会退出，不能再从该进程采样回落。

`memory` 的 `heap_objects` 与 `go_retained` 是 Go runtime 指标，不是进程 RSS；`resources` 是应用逻辑图片/图表缓存的预算占用。关闭后预算归还不保证 RSS 立刻下降，测量不主动调用 GC。WebKit XPC 子进程可能被 launchd 接管；runner 的 descendant RSS 只包含可发现后代，**不是整个应用和扩展的内存总量**。需要完整 WebKit/Quick Look 进程归因时，使用 Activity Monitor/Instruments 的实际进程记录，Quick Look 与主应用分别测量。

### Quick Look

Console 中使用 `com.mdflick.app.QuickLook` / `Preview` debug 日志。新增 `request=N open-to-frame-opportunity` 从预览请求进入计时，包含队列、文件读取、Go/C/Swift 和 WebKit；在 document-end 后两次 `requestAnimationFrame` 时报告，带请求版本校验，避免旧页面混入新请求。它是浏览器提供绘制机会的代理指标，**不是显示器首像素时间**。原有 WebKit load、图表完成事件仍分别记录。Finder 实际切换、取消和内存需要安装扩展后验证，Swift bridge 测试不代表 Finder 端到端测试。

## 本次解析复用优化的对照

2026-11-01，Apple M4、darwin/arm64，修改前 HEAD 导出到独立临时目录与当前工作区，使用同一 `BenchmarkNativeOpen`、`-benchtime=10x -count=3` 顺序运行，取三次中位数。包含后台预处理和实际 reader view 的第一帧 headless 构建/布局/软件绘制，不含磁盘 I/O、原生窗口创建、GPU 和 WebKit。

| 语料 | 修改前 ms/op | 当前 ms/op | 修改前累计分配 MiB/op | 当前累计分配 MiB/op |
| --- | ---: | ---: | ---: | ---: |
| 1,000 段正文 | 8.809 | 9.081 | 31.95 | 31.23 |
| 100 个表格 | 2.447 | 1.784 | 9.65 | 6.84 |
| 100 个 HTML 对齐容器 | 1.893 | 1.067 | 9.05 | 4.21 |

表格总耗时约减少 27%，对齐容器约减少 44%；普通段落样本未显示总耗时改善（本次约增加 3%）。inline 解析移到后台会增加预处理阶段时间与分配，不能把 UI 阶段下降直接解释为整体提速。当前收益主要来自删除片段重建/嵌套视图及重复解析，普通长文仍需全量构建和布局。

```sh
CGO_ENABLED=0 go test -run '^$' -bench '^BenchmarkNativeOpen$' -benchmem -benchtime=10x -count=3 .
CGO_ENABLED=0 go test -run '^$' -bench 'BenchmarkNative(FirstFrame|Scroll)$' -benchmem -benchtime=10x -count=3 .
```

`BenchmarkNativeFirstFrame` 在相同的当前块样式下比较字符串 API 与预解析 API，仅隔离 UI 阶段，不是完整旧实现对照。`BenchmarkNativeScroll` 每次 op 含一次下滚和一次上滚，使用 headless Tester，不是实际 GPU 帧时间。虚拟化尚未实施；是否继续投入由真实长文首屏与滚动采样决定。

下列章节保留此前优化的历史基准。原生实现现已移除片段重建，当前阶段用 `native.inlines` 取代 `native.fragments`，不再使用派生片段预算。输入、转换输出与解析块限制继续生效。

## 重现基准

```sh
CGO_ENABLED=0 go test -run '^$' -bench 'Benchmark(NativePrepare|PreviewRender|PreviewCancelAfterParse)$' -benchmem -benchtime=5x . ./internal/preview
```

用于诊断的固定语料在 `internal/testfixtures/adapters.go`：约 1 MiB 多段落文本、500 个表格、1,000 组图片引用、256 个 Mermaid 围栏、500 个 HTML 对齐容器。可提高 `-benchtime` 并增加 `-count=5` 进行稳定性比较。比较时应使用相同工具链、机器和语料，避免其他构建/测试争抢资源。

以下是 2026-11-01 的本机示例采样，Apple M4、macOS 26.6.2、Go 1.27.3、darwin/arm64，每项 5 次迭代。它是当前代码的诊断基线，不是优化前后的对照实验或 CI 性能阈值。

| 场景 | 原生预处理 ms/op | 累计分配 MiB/op | Quick Look Go 渲染 ms/op | 累计分配 MiB/op |
| --- | ---: | ---: | ---: | ---: |
| 约 1 MiB 多段落文本 | 2.03 | 8.30 | 52.60 | 150.88 |
| 500 个表格 | 1.31 | 8.52 | 3.95 | 11.09 |
| 1,000 组图片引用 | 0.15 | 0.70 | 3.56 | 11.90 |
| 256 个 Mermaid 围栏 | 0.11 | 0.44 | 1.35 | 9.30 |
| 500 个 HTML 对齐容器 | 3.35 | 26.95 | 2.17 | 8.07 |

`B/op` 是累计分配字节数，不是峰值存活内存或进程 RSS。原生基准只测后台预处理，不含 MyGo 原生解析/布局/绘制；Quick Look 基准只测 Go 解析、过滤与 HTML 输出，不含 Swift/C 拷贝、WebKit 首屏或真实 Mermaid 执行。图片语料测的是引用处理，不发网络请求、不解码位图。256 个图表语料中最多 128 个会被标记为待渲染，其余源码保留。

解析完成后立即取消的约 1 MiB 文本基准为约 1.12 ms/op、8.30 MiB/op。这是当前语料的解析阶段成本，不能解释为所有文档的取消延迟上限。

## 原生预处理优化对照

2026-11-01，在同一 Apple M4 上用 `-benchtime=20x -count=3` 对照优化前 HEAD 的临时导出与当前工作区，以下耗时为三次采样的中位数。两次测试顺序执行，结果是本地样本，不是稳定的性能保证；分配为累计分配量，不是 RSS。测量仍不包含 MyGo UI 端解析/布局。

| 场景 | 原耗时 ms/op | 当前耗时 ms/op | 原累计分配 MiB/op | 当前累计分配 MiB/op |
| --- | ---: | ---: | ---: | ---: |
| 500 个表格 | 0.912 | 0.687 | 8.52 | 3.19 |
| 500 个 HTML 对齐容器 | 2.781 | 1.562 | 26.95 | 8.83 |

改善来自复用 HTML 转换所需解析、删除块间空隙解析、按片段使用情况恢复引用定义，并跳过无方括号块的 inline 引用扫描。原生 MyGo 视图及嵌套片段的 UI 端解析仍然保留。

## Quick Look DOM 与输出优化对照

2026-11-01，同一 Apple M4、相同语料上，在修改前后分别执行：

```sh
CGO_ENABLED=0 go test -run '^$' -bench BenchmarkPreviewRender -benchmem -benchtime=20x -count=3 ./internal/preview
```

下表采用三次采样的中位数；两组顺序执行，未同时运行其他检查。累计分配不代表峰值 RSS，耗时只包含 Go 渲染，不包含 Swift/C 拷贝、WebKit 或真实 Mermaid 执行。

| 场景 | 原耗时 ms/op | 当前耗时 ms/op | 原累计分配 MiB/op | 当前累计分配 MiB/op |
| --- | ---: | ---: | ---: | ---: |
| 约 1 MiB 多段落文本 | 52.320 | 25.846 | 150.87 | 43.38 |
| 500 个表格 | 3.901 | 2.617 | 11.09 | 5.42 |
| 1,000 组图片引用 | 3.511 | 2.037 | 11.90 | 3.27 |
| 256 个 Mermaid 围栏 | 0.599 | 0.398 | 7.82 | 3.19 |
| 500 个 HTML 对齐容器 | 2.164 | 1.530 | 8.07 | 3.22 |

过滤直接复用解析树和属性切片，删除不允许的整棵子树，只展开文档、html、body 包装节点。一次序列化整个过滤后的文档，复用一个序列化缓冲区，避免为每个顶层块创建缓冲区；正文直接写入最终字符串，内置脚本独立追加，省去正文缓冲与大脚本尾部拼接。输出上限先预留完整页头、内置脚本与结尾，再约束正文写入。

已用共享功能/性能语料及畸形 HTML 样例与旧实现进行逐字节输出比对；固定回归测试覆盖属性过滤、危险子树移除、相邻节点遍历、外来命名空间、标题去重与 Mermaid 标记，以及正文/脚本共用输出预算和输出中途取消。仍保留 Markdown → HTML → DOM 的解析链路，未改为流式 HTML 过滤。

## 从测量确定的限制

多段落文本的 DOM 和过滤链路存在明显的分配放大，因此在 Markdown 解析完成后、生成 HTML/DOM 前检查 20,000 个解析块的上限（包括解析器根块）。生成的中间 HTML 与最终 HTML 均受 64 MiB 限制，最终输出通过有界写入器组装。超限以稳定错误码 8 返回，不输出半份文档。单文件输入仍限于 32 MiB。

Quick Look 最多渲染前 128 个 Mermaid 围栏。图表源码和复杂度仍由内置 Mermaid 的 50,000 字符、500 条边规则限制。静态内置脚本与 CSP 哈希只计算一次，避免每次预览重复拼接和哈希完整运行时。

每个请求从 Swift 创建句柄开始计算 15 秒期限，覆盖 Swift 排队、文件读取检查点、Go 进程内排队及 Go 渲染。新请求取消旧请求；同一扩展进程只允许一个 Go 渲染进入执行。底层单次 Markdown 解析、Markdown 转 HTML，以及系统读取无法硬中断；它们返回后才响应取消。期限也不限制 WebKit 中已经开始的 JavaScript。块数、输出字节数和图片预算都不是进程/GPU 峰值内存的硬保证。

## 分阶段记录

在终端开发启动时设置 `MDFLICK_TRACE=1`，Go 输出 `pipeline stage=... elapsed=... bytes=...`，包含原生读取/预处理、Quick Look 解析、Markdown 转 HTML、DOM、过滤、输出以及 C 桥接的输入解码/输出复制。记录不包含文档路径或正文。

Swift 使用 OSLog 的 `com.mdflick.app.QuickLook` / `Preview` 分类记录排队、读取、桥接、WebKit 载入、图表完成耗时。调试时在 Console 中启用对应 debug 日志；错误详情使用 private 隐私级别。HTML 提交成功与图表执行完成分别记录，都不能直接当成首屏像素时间。实际 Finder 快速切换、WebKit 布局、主题切换和图表耗时仍需安装后验证。

## 原生处理与应用总量限制

原生加载的 15 秒期限包含应用槽位排队、读取和预处理；应用最多执行两个加载任务。`ProcessingLimits` 由共享文档包提供，原文与转换后解析结果各最多 20,000 块，转换后 Markdown 最多 64 MiB；取消和超限不发布部分文档。图片资源生命周期使用会话 context，与加载操作的期限分离。

图表仍每窗口一个 WebView/串行 worker，应用最多两个 worker 进入渲染调用，缓存估算预算每文档 64 MiB、全应用 128 MiB。槽位在 Go 调用返回时释放，超时的 JavaScript 仍可能继续执行；这不是 WebKit 执行或进程峰值内存的硬限制。

`MDFLICK_TRACE=1` 还可记录 `native.parse`、`native.html`、`native.inlines`；其中 inlines 的 bytes 是处理的 Markdown 字节数。
