import Cocoa
import QuickLookUI
import WebKit
import OSLog

@objc(PreviewViewController)
final class PreviewViewController: NSViewController, QLPreviewingController, WKNavigationDelegate, WKScriptMessageHandler {
    private var webView: WKWebView!
    private var generation = 0
    private var activeRequest: PreviewRequest?
    private var activeNavigation: WKNavigation?
    private var loadStarted: TimeInterval = 0
    private var requestStarted: TimeInterval = 0
    private var frameReported = false
    // Only one Go renderer runs at a time; obsolete queued requests exit early.
    private let renderQueue = DispatchQueue(label: "com.mdflick.preview", qos: .userInitiated)

    deinit { activeRequest?.cancel() }
    private let logger = Logger(subsystem: "com.mdflick.app.QuickLook", category: "Preview")

    override func loadView() {
        let config = WKWebViewConfiguration()
        // Generated HTML authorizes only the bundled Mermaid scripts by CSP
        // hash. Markdown content remains escaped and network loads blocked.
        config.defaultWebpagePreferences.allowsContentJavaScript = true
        config.userContentController.add(WeakPreviewMessageHandler(self), name: "mdflickFrame")
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
        webView.stopLoading()
        activeNavigation = nil
        let queuedAt = ProcessInfo.processInfo.systemUptime
        requestStarted = queuedAt
        frameReported = false
        webView.configuration.userContentController.removeAllUserScripts()
        // Two animation frames mark a rendering opportunity after the body is
        // available, not physical presentation. Each message belongs to a request.
        let frameScript = "requestAnimationFrame(() => requestAnimationFrame(() => window.webkit.messageHandlers.mdflickFrame.postMessage(\(request))))"
        webView.configuration.userContentController.addUserScript(WKUserScript(source: frameScript, injectionTime: .atDocumentEnd, forMainFrameOnly: true))
        let token = PreviewRequest()
        activeRequest = token
        renderQueue.async { [weak self] in
            var stage = "open document"
            do {
                try token.check()
                self?.logger.debug("Preview queue wait: \(ProcessInfo.processInfo.systemUptime - queuedAt, privacy: .public)s")
                let readStarted = ProcessInfo.processInfo.systemUptime
                guard try url.resourceValues(forKeys: [.isRegularFileKey]).isRegularFile == true else { throw PreviewBridge.error(4) }
                let input = try FileHandle(forReadingFrom: url)
                defer { try? input.close() }
                stage = "read document"
                let data = try input.read(upToCount: PreviewBridge.maxSize + 1) ?? Data()
                try token.check()
                self?.logger.debug("Preview read: \(ProcessInfo.processInfo.systemUptime - readStarted, privacy: .public)s, bytes: \(data.count, privacy: .public)")
                stage = "render document"
                let renderStarted = ProcessInfo.processInfo.systemUptime
                let html = try PreviewBridge.render(data, request: token)
                self?.logger.debug("Preview bridge: \(ProcessInfo.processInfo.systemUptime - renderStarted, privacy: .public)s")
                try token.check()
                DispatchQueue.main.async {
                    guard let self, self.generation == request else { handler(CocoaError(.userCancelled)); return }
                    self.loadStarted = ProcessInfo.processInfo.systemUptime
                    self.activeNavigation = self.webView.loadHTMLString(html, baseURL: nil)
                    handler(nil)
                }
            } catch {
                if MDPreviewRequestStatus(token.handle) != 0 {
                    self?.logger.debug("Preview cancelled or expired at \(stage, privacy: .public)")
                } else {
                    self?.logger.error("Preview failed at \(stage, privacy: .public): \(String(describing: error), privacy: .private)")
                }
                DispatchQueue.main.async {
                    guard let self, self.generation == request else { handler(CocoaError(.userCancelled)); return }
                    handler(error)
                }
            }
        }
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        guard message.frameInfo.isMainFrame, let request = message.body as? Int,
              request == generation, activeNavigation != nil, !frameReported else { return }
        frameReported = true
        logger.debug("Preview request=\(request, privacy: .public) open-to-frame-opportunity: \(ProcessInfo.processInfo.systemUptime - self.requestStarted, privacy: .public)s")
    }

    // Only generated content and same-document anchors may navigate.
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        let url = action.request.url
        decisionHandler(PreviewBridge.allowsNavigation(url) ? .allow : .cancel)
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        guard let navigation, navigation === activeNavigation else { return }
        let request = generation
        let started = loadStarted
        logger.debug("Preview WebKit load: \(ProcessInfo.processInfo.systemUptime - started, privacy: .public)s")
        // Completion above means HTML was submitted. Track the initial viewport
        // diagram batch separately; offscreen diagrams wait until visible.
        // Neither event claims to measure first-paint time.
        webView.callAsyncJavaScript("await window.mdflickReady; return true", arguments: [:], in: nil, in: .page) { [weak self] result in
            guard let self, self.generation == request else { return }
            switch result {
            case .success:
                self.logger.debug("Preview initial viewport diagrams ready: \(ProcessInfo.processInfo.systemUptime - started, privacy: .public)s")
            case .failure(let error):
                self.logger.error("Preview content failed: \(String(describing: error), privacy: .private)")
            }
        }
    }

}

// WKUserContentController retains handlers; avoid retaining the controller/view.
private final class WeakPreviewMessageHandler: NSObject, WKScriptMessageHandler {
    weak var target: PreviewViewController?
    init(_ target: PreviewViewController) { self.target = target }
    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        target?.userContentController(controller, didReceive: message)
    }
}
