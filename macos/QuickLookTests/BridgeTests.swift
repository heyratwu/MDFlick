import Foundation

@main
struct BridgeTests {
    static func main() throws {
        precondition(PreviewBridge.maxSize > 0)
        func call(_ input: UnsafeMutablePointer<CChar>?, _ size: Int32, _ expected: String) {
            // A failure must clear even a previously non-null output slot.
            var output: UnsafeMutablePointer<CChar>? = UnsafeMutablePointer(bitPattern: 1)
            let status = MDPreviewRender(input, size, &output)
            precondition(status != 0 && output == nil)
            precondition(PreviewBridge.error(status).localizedDescription == expected)
        }
        call(nil, -1, "无效的预览请求")
        call(nil, 1, "无效的预览请求")
        call(nil, Int32(PreviewBridge.maxSize + 1), "文件超过 32 MB，无法预览")
        precondition(MDPreviewRender(nil, 0, nil) != 0)
        var invalid: CChar = -1
        withUnsafeMutablePointer(to: &invalid) { call($0, 1, "请使用 UTF-8 编码的 Markdown 文件") }
        // NULL input with zero bytes is a valid empty document.
        var empty: UnsafeMutablePointer<CChar>?
        precondition(MDPreviewRender(nil, 0, &empty) == 0 && empty != nil)
        MDPreviewFree(empty)
        MDPreviewFree(nil)
        for _ in 0..<100 {
            let html = try PreviewBridge.render(Data("\u{FEFF}# 中文\n".utf8))
            precondition(html.contains("<h1>中文</h1>"))
        }
        do {
            _ = try PreviewBridge.render(Data([0xff]))
            preconditionFailure("invalid UTF-8 accepted")
        } catch {
            precondition((error as NSError).domain == "MDFlick")
            precondition(error.localizedDescription == "请使用 UTF-8 编码的 Markdown 文件")
        }
        print("Swift/Go bridge tests passed")
    }
}
