# MyGo 本地补丁

当前仍锁定 `github.com/egoist/mygo v0.7.3`。使用 Go 标准 `vendor/`，仅修改以下框架文件：

- `ui/markdown/view.go`：`ViewDocument` 接收预解析文档；`Options.BlockStyle` 配置原始块容器；`Options.ResolveImage` 允许应用直接查询图片缓存，绕过框架默认的逐地址 goroutine 和缓存。
- `ui/bitmap.go`：`Bitmap.SetLogicalSize` 将默认显示尺寸与实际像素尺寸分离，用于浏览器生成的高清 SVG 位图；`Size` 仍返回像素尺寸，`ImageSize` 返回显示尺寸。
- `ui/markdown/image.go`：可选图片解析回调接收当前图片容器，应用只绑定持久 Handle，不保留元素；未设置回调时保留框架默认行为；图片比例通过 `ui.ImageSize` 读取，尊重高清位图的逻辑尺寸。
- `ui/presentation.go`、`ui/runtime.go`、`ui/context.go`：`Context.AfterPresent` 在最终构建 pass 的帧提交到 surface 后执行一次回调。窗口尚未实际提交时不报告；回调不在绘制函数内执行。

- `ui/markdown/code.go`：未改变的代码块按 revision 复用缓存；`Options.HighlightLimit` 可让大代码块使用可选择的完整纯文本，零值保持无限制高亮。

完整补丁在 `patches/mygo-v0.7.3.patch`。vendor 中的其余第三方源码保持 `go mod vendor` 原样，许可证随源码保留。源码纳入仓库不等于向二进制添加框架功能或另一个运行时。

## 更新依赖

```sh
bash scripts/vendor.sh
CGO_ENABLED=0 go test ./...
go tool mygo vet .
go test -race ./...
bash scripts/test-macos-bridge.sh
```

不要单独运行 `go mod vendor`：它会覆盖本地补丁。不要使用 `-mod=mod` 绕过 vendor；应用需要这里新增的 API。`scripts/vendor.sh` 先检查 MyGo 版本、重新生成 vendor，再验证并应用补丁。升级 MyGo 时必须先审查并移植补丁；脚本会拒绝将 v0.7.3 的补丁直接用于其他版本。

## 文档所有权

`parser.Document` 包含可变的 lazy inline 缓存。加载 worker 构建并预热后，通过版本检查将它**独占移交 UI 线程**；发布后 worker 不再访问它。不跨窗口、不跨视图共享，不在展示期间编辑。文档替换时以新指针重置段落、代码和表格缓存。输入与 HTML 转换规则没有变化。

`BlockStyle` 收到的 `ui.Element` 只在当前 build 有效；应用只即时配置，不保存元素。所有表格和对齐段落属于同一个 Markdown 视图，因此引用解析、全文选择与复制使用同一份文档。

## 测量边界

`AfterPresent` 表示 MyGo 已调用实际 surface 渲染路径并返回，不表示显示器 scanout、用户看到了像素或所有图片/图表已就绪。回调用于诊断、状态发布和布局后的图片可见性收集，不应保存 build 元素。测试通过 `ui.NewTester` 验证文档采用、替换、跨块复制及首帧事件；真实 GPU 与操作系统调度需要原生运行采样。


2026-11-03 的逻辑位图尺寸补丁使用同一工具链和发布参数（`-trimpath -tags mygo_noinspector -ldflags "-s -w -X github.com/egoist/mygo.production=1"`）对比：`hello` 大小不变，`counter-native` 增加 16 字节，来自原有图片尺寸读取路径的条件分支；未将浏览器或 SVG 脚本引入未使用该功能的框架示例。
