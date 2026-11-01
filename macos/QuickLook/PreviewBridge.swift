import Foundation

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

    static func render(_ data: Data) throws -> String {
        var output: UnsafeMutablePointer<CChar>?
        let status = data.withUnsafeBytes { bytes in
            MDPreviewRender(UnsafeMutablePointer(mutating: bytes.baseAddress?.assumingMemoryBound(to: CChar.self)),
                            Int32(clamping: data.count), &output)
        }
        defer { MDPreviewFree(output) }
        guard status == 0 else { throw error(status) }
        guard let output else { throw error(6) }
        return String(cString: output)
    }
}
