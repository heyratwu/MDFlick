import Cocoa
import QuickLookUI
import WebKit
import OSLog

// Thread-safe cancellation checkpoints around synchronous file/Go operations.
private final class PreviewRequest {
    private let lock = NSLock()
    private var cancelled = false
    func cancel() { lock.lock(); cancelled = true; lock.unlock() }
    func check() throws {
        lock.lock(); let stopped = cancelled; lock.unlock()
        if stopped { throw CocoaError(.userCancelled) }
    }
}

@objc(PreviewViewController)
final class PreviewViewController: NSViewController, QLPreviewingController, WKNavigationDelegate {
    private var webView: WKWebView!
    private var generation = 0
    private var activeRequest: PreviewRequest?
    // Only one Go renderer runs at a time; obsolete queued requests exit early.
    private let renderQueue = DispatchQueue(label: "com.mdflick.preview", qos: .userInitiated)

    deinit { activeRequest?.cancel() }
    private let logger = Logger(subsystem: "com.mdflick.app.QuickLook", category: "Preview")

    override func loadView() {
        let config = WKWebViewConfiguration()
        // Generated HTML authorizes only the bundled Mermaid scripts by CSP
        // hash. Markdown content remains escaped and network loads blocked.
        config.defaultWebpagePreferences.allowsContentJavaScript = true
        webView = WKWebView(frame: NSRect(x: 0, y: 0, width: 800, height: 640), configuration: config)
        webView.navigationDelegate = self
        view = webView
        preferredContentSize = NSSize(width: 800, height: 640)
    }

    func preparePreviewOfFile(at url: URL, completionHandler handler: @escaping (Error?) -> Void) {
        _ = view
        generation += 1
        let request = generation
        activeRequest?.cancel()
        let token = PreviewRequest()
        activeRequest = token
        renderQueue.async { [weak self] in
            var stage = "open document"
            do {
                try token.check()
                let input = try FileHandle(forReadingFrom: url)
                defer { try? input.close() }
                stage = "render document"
                let data = try input.read(upToCount: (32 << 20) + 1) ?? Data()
                guard data.count <= (32 << 20) else {
                    throw NSError(domain: "MDFlick", code: 1, userInfo: [NSLocalizedDescriptionKey: "文件超过 32 MB，无法预览。"])
                }
                try token.check()
                let rendered = data.withUnsafeBytes { bytes in
                    MDPreviewHTML(UnsafeMutablePointer(mutating: bytes.baseAddress?.assumingMemoryBound(to: CChar.self)), Int32(data.count))
                }
                guard let rendered else {
                    throw NSError(domain: "MDFlick", code: 1, userInfo: [NSLocalizedDescriptionKey: "请使用 UTF-8 编码的 Markdown 文件。"])
                }
                defer { free(rendered) }
                try token.check()
                let html = String(cString: rendered)
                DispatchQueue.main.async {
                    guard let self, self.generation == request else { handler(CocoaError(.userCancelled)); return }
                    self.webView.loadHTMLString(html, baseURL: nil)
                    handler(nil)
                }
            } catch {
                self?.logger.error("Preview failed at \(stage, privacy: .public): \(String(describing: error), privacy: .public)")
                DispatchQueue.main.async {
                    guard let self, self.generation == request else { handler(CocoaError(.userCancelled)); return }
                    handler(error)
                }
            }
        }
    }

    // Only generated content and same-document anchors may navigate.
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        let url = action.request.url
        decisionHandler(url?.scheme == "about" ? .allow : .cancel)
    }
}
