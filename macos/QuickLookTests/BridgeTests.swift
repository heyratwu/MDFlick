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
            precondition(html.contains("<h1 id=\"中文\">中文</h1>"))
        }
        do {
            _ = try PreviewBridge.render(Data([0xff]))
            preconditionFailure("invalid UTF-8 accepted")
        } catch {
            precondition((error as NSError).domain == "MDFlick")
            precondition(error.localizedDescription == "请使用 UTF-8 编码的 Markdown 文件")
        }
        // Opaque request handles propagate cancellation through the real C ABI.
        let cancelled = PreviewRequest()
        cancelled.cancel()
        precondition(MDPreviewRequestStatus(cancelled.handle) == 9)
        var cancelledOutput: UnsafeMutablePointer<CChar>? = UnsafeMutablePointer(bitPattern: 1)
        precondition(MDPreviewRenderRequest(cancelled.handle, nil, 0, &cancelledOutput) == 9)
        precondition(cancelledOutput == nil)
        do {
            _ = try PreviewBridge.render(Data("# Cancelled".utf8), request: cancelled)
            preconditionFailure("cancelled request rendered")
        } catch {
            precondition((error as NSError).code == 9)
        }
        let handle = MDPreviewNewRequest()
        MDPreviewReleaseRequest(handle)
        MDPreviewReleaseRequest(handle)
        precondition(MDPreviewRequestStatus(handle) == 5)
        precondition(MDPreviewRenderRequest(handle, nil, 0, &cancelledOutput) == 5)
        precondition(cancelledOutput == nil)
        for address in ["about:blank", "about:blank#heading"] {
            precondition(PreviewBridge.allowsNavigation(URL(string: address)))
        }
        for address in ["https://example.com", "file:///tmp/doc.md", "about:config", "about:blank?other"] {
            precondition(!PreviewBridge.allowsNavigation(URL(string: address)))
        }
        // A cancelled request cannot poison the subsequent render.
        let fresh = try PreviewBridge.render(Data("# New".utf8))
        precondition(fresh.contains("<h1 id=\"new\">New</h1>"))
        print("Swift/Go bridge tests passed")
    }
}
