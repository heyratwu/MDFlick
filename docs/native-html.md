# 主窗口的原生 HTML 支持

主窗口继续使用 MyGo 原生 UI。Markdown 和 HTML 共用一份文档解析、正文选择、链接策略以及图片缓存；不增加网页渲染模式。Finder Quick Look 使用原来的独立过滤策略，以下能力仅描述主窗口。

## 支持范围

| 内容 | 支持范围 |
| --- | --- |
| 容器 | `div`、`p`、`center`、`section`、`article`、`header`、`footer`、`figure`、`figcaption`；未知被动容器保留内部可读内容 |
| 对齐 | 容器和 `h1`～`h6` 的 `align="left|center|right"`；嵌套内容继承，内部声明覆盖外部声明 |
| 图片 | 独立图片、图片链接、只包含图片的标题、连续图片和徽章；HTML `<img>` 与 Markdown 图片使用同一个原生构建入口 |
| 图片属性 | `src`、`alt`、`title`、数值 `width`/`height`、百分比 `width`；只指定一个尺寸时按原比例计算另一个，窄窗口按可用宽度缩小 |
| 主题图片 | `<picture>` 内匹配 `prefers-color-scheme: dark/light` 的 `<source srcset>`；主题变化重新选择来源；没有对应来源时使用 `<img src>` |
| `srcset` | 使用第一个候选地址；不根据屏幕密度、视口断点或 `sizes` 选择候选；`img` 没有 `src` 时可使用其 `srcset` |
| SVG 图片 | 全部通过共享浏览器池按 SVG image 规则渲染，支持 CSS 变量、文字、`foreignObject` 布局、内嵌 data 图片和滤镜；生成最多双倍分辨率的透明位图，保持原始显示尺寸；不执行脚本或请求外部子资源 |
| 列表 | `ul`、`ol`、`li`；保留有序列表的 `start`，支持嵌套列表和列表项内段落 |
| 表格 | `table`、`thead`、`tbody`、`tr`、`th`、`td` 映射为原生 Markdown 表格；第一行作为表头 |
| 引用 | `blockquote`，包括其中的段落和行内格式 |
| 行内格式 | `b`、`strong`、`i`、`em`、`u`、`ins`、`s`、`del`、`strike`、`code`、`kbd`、`samp`、`mark`、`sub`、`sup`、`small`，采用 MyGo 对应的文字样式 |
| 代码与分隔 | `pre`、`code`、`br`、`hr`；HTML 排版缩进不会成为代码缩进，`pre` 保留换行、空格和代码围栏字面量 |
| 折叠区块 | `details`、`summary` 保留标题和正文，始终展开，不提供折叠交互 |

HTML 块内的空白按 HTML 规则折叠；原生 Markdown、行内代码与围栏代码不经过 HTML 空白处理。HTML 图片的尺寸、主题来源和标题存为转换结果的结构化元数据，以转换后源码位置与图片节点关联，不写入图片 URL。普通 Markdown 的引用链接仍由完整文档解析。

图片链接支持点击和键盘激活，目标经过现有协议策略检查。图片仍由视口调度器按需加载，继续受文件大小、像素数、并发数、超时与缓存预算约束。

`<img>` 同时声明有效的宽度（数值或百分比）和高度时，加载前按声明尺寸显示适配深浅主题的灰色渐变占位，加载后在相同区域显示图片；窄窗口仍按比例缩小。尺寸未声明完整时保留原有替代文字逻辑。标准 Markdown 图片语法没有宽高属性，仍按原逻辑处理。

## 当前边界

- 正文文字中夹杂的图片、表格单元格内的图片仍采用 MyGo 的替代文字显示；实际图片渲染适用于图片段落及只包含图片的标题。
- 不实现任意 CSS、`class`、`style`、定位布局、表单、音视频、内联 SVG 或浏览器脚本。
- SVG 文字采用静态子集：不实现 `textPath`、逐字符坐标列表、复杂 HTML/CSS 布局及彩色字形；使用本机字体回退，不加载网页字体或外部 HTML 资源。文字内容最多 64 KiB，展开后的 SVG 最多 16 MiB，转换结果继续计入图片缓存预算；转换在图片加载 worker 中完成，取消时不发布部分结果。
- 表格不实现 `rowspan`、`colspan`、嵌套表格和单元格 CSS；列表不实现 `reversed` 和单项 `value`。
- `script`、`style`、`iframe`、`object`、`template` 的内容丢弃。HTML 注释不显示。
- 位图是图像元素，加载后不作为可拖选文字参与纯文本复制；跨正文选区复制 Markdown 时仍保留中间图片源码。
- 继续使用 UTF-8、文件大小、转换输出大小和处理期限限制；转换失败不会发布部分文档。

## 验证

`internal/document/html_test.go` 覆盖转换后的节点、图片元数据、主题来源、空白、嵌套列表、基础表格、引用、文字格式、代码字面量、主动内容过滤与取消/资源限制。

`svg_text_darwin_test.go` 使用真实 Trendshift SVG 固定样例，检查文字区域实际像素、普通 SVG 文字、原始图形保留、静态 HTML 过滤、并发排版与取消。可用 `MDFLICK_SVG_TEXT_SCREENSHOT=/tmp/mdflick-trendshift-fixed.png` 导出渲染结果。

`svg_variables_test.go` 检查 SVG 根级变量、别名、回退、循环引用，以及深浅主题下的实际绘制像素。

`reader_html_images_test.go` 使用本地 HTTP 服务和 `ui.NewTester` 验证居中、作者尺寸、主题切换、图片链接、窄窗口、跨正文复制及文档替换。设置 `MDFLICK_HTML_IMAGES_SCREENSHOT=/tmp/mdflick-html-images.png` 可导出测试画面。
