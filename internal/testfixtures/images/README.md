# SVG 图片回归样例

`trendshift-badge.svg` 来自用户报告的 [Claude-Mem 仓库徽章](https://raw.githubusercontent.com/thedotmack/claude-mem/main/docs/public/trendshift-badge.svg)，于 2026-11-02 获取。保留原始 SVG，用于离线验证浏览器能绘制 `<foreignObject>` 中的三处文字，测试不访问网络。


`gitcode-star-badge.svg` 来自 [Kornia GitCode star 徽章](https://gitcode.com/kornia/kornia/star/badge.svg)，`discord-badge.svg` 来自 [Shields Discord 徽章](https://img.shields.io/badge/Discord-5865F2?logo=discord&logoColor=white)，均于 2026-11-03 获取。保留原始 SVG，离线验证 `foreignObject` 中的 CSS/flex 布局、嵌套 SVG、data URL 图标、`textLength` 与滤镜。执行 `bash scripts/test-svg-browser.sh` 可用 macOS WebKit 检查真实像素与双倍分辨率尺寸。

`simple-shapes.svg` 是本项目的最小 CSS 变量与透明形状样例。所有 SVG 走同一浏览器适配器；Go 测试验证分派、缓存和尺寸，WebKit 测试验证这四个样例的实际像素。
