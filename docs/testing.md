# 项目验证

```sh
CGO_ENABLED=0 go test ./...
go tool mygo vet .
go test -race ./...
bash scripts/test-macos-bridge.sh
```

最后一项需要 macOS、Go CGO 与 Xcode Command Line Tools。脚本在临时目录构建当前 Go 静态库和 C 头文件，再链接 Swift 测试程序，执行后删除临时产物；同时对扩展控制器与桥接适配器进行类型检查。它不安装应用、不注册 Finder 扩展。

## 共享文档行为

`internal/document/policy.go` 定义稳定的校验错误码、中文文案和 Mermaid 判定。`internal/testfixtures` 提供两端共用的 Markdown 样例；原生测试经过实际 MyGo 代码块分发，HTML 测试检查代码块标记、脚本授权以及主动内容过滤。覆盖中文、表格、任务列表、反引号和波浪线围栏、嵌套引用、普通单词、其他语言、未闭合围栏和大小写差异。

原生视图与 Quick Look 的展示能力差异见 README 中的能力表。共享规则不要求二者生成相同布局。

## 异步生命周期

会话测试验证旧成功/错误不能覆盖新文档、关闭后拒绝发布；图表测试验证旧版本或旧渲染器不能占用缓存、缓存预算、队列容量、排队到期、worker 串行执行及取消后退出。图片测试验证重新加载会更换 Options 与图片内容，并检查取消与大小限制；使用本地 HTTP 测试服务器验证网络 PNG、无扩展名 SVG、重定向、错误响应、流式超限及请求中途取消。

## Swift/C/Go 所有权约定

- `MDPreviewRender` 在调用期间借用输入；零字节允许 NULL，正字节不允许 NULL。
- 必须传入输出槽；每次调用先清空该槽。返回 0 时得到 C 分配的 UTF-8 字符串，其余返回稳定的 `document.Code`。
- 输出字符串与 `MDPreviewErrorMessage` 的字符串均通过 `MDPreviewFree` 释放；NULL 可安全释放。
- `MDPreviewMaxSize` 是 Swift 文件读取上限的来源。Swift 读取最多上限加一字节，将超限和编码错误交给 Go 统一识别。

桥接测试覆盖空输入、负长度、超限、无效指针组合、失败输出清空、非法 UTF-8、BOM/中文、重复分配与释放以及 Swift NSError 映射。重复释放不属于 API 合约。

## 仍需手动验证

自动化测试不覆盖实际 WKWebView 的布局与 Mermaid 执行、Finder 扩展注册、窗口关闭动画和系统主题切换。发布前应在安装后的应用及 Finder 中打开 `docs/examples/mermaid.md`，检查图表、错误回退、主题切换，并快速切换多个预览文件。
