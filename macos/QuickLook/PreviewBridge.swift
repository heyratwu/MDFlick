import Foundation

// Go owns the thread-safe request registry and the deadline. This object owns
// one opaque handle, retained by the queued work until rendering finishes.
final class PreviewRequest {
    let handle = MDPreviewNewRequest()
    deinit { MDPreviewReleaseRequest(handle) }
    func cancel() { MDPreviewCancelRequest(handle) }
    func check() throws {
        let status = MDPreviewRequestStatus(handle)
        if status != 0 { throw PreviewBridge.error(status) }
    }
}

// Only this adapter owns C allocations. Document limits and user-facing error
// messages come from Go; Swift owns platform file I/O errors.
enum PreviewBridge {
    static var maxSize: Int { Int(MDPreviewMaxSize()) }

    static func error(_ status: Int32) -> NSError {
        let message = MDPreviewErrorMessage(status)
        defer { MDPreviewFree(message) }
        return NSError(domain: "MDFlick", code: Int(status), userInfo: [
            NSLocalizedDescriptionKey: message.map { String(cString: $0) } ?? "无法生成 Markdown 预览"
        ])
    }

    static func allowsNavigation(_ url: URL?) -> Bool {
        guard let url else { return false }
        let data = Data(url.absoluteString.utf8)
        return data.withUnsafeBytes { bytes in
            MDPreviewAllowsNavigation(UnsafeMutablePointer(mutating: bytes.baseAddress?.assumingMemoryBound(to: CChar.self)),
                                      Int32(clamping: data.count)) == 1
        }
    }

    static func render(_ data: Data, request supplied: PreviewRequest? = nil) throws -> String {
        let request = supplied ?? PreviewRequest()
        try request.check()
        var output: UnsafeMutablePointer<CChar>?
        let status = data.withUnsafeBytes { bytes in
            MDPreviewRenderRequest(request.handle, UnsafeMutablePointer(mutating: bytes.baseAddress?.assumingMemoryBound(to: CChar.self)),
                            Int32(clamping: data.count), &output)
        }
        defer { MDPreviewFree(output) }
        guard status == 0 else { throw error(status) }
        guard let output else { throw error(6) }
        let result = String(cString: output)
        try request.check()
        return result
    }
}
