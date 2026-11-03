import Cocoa
import WebKit
import CryptoKit
let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let runtime = try String(contentsOfFile: CommandLine.arguments[1], encoding: .utf8)
let scriptHash = Data(SHA256.hash(data: Data(runtime.utf8))).base64EncodedString()
let fixtures = CommandLine.arguments[2]
let files = ["gitcode-star-badge.svg", "discord-badge.svg", "trendshift-badge.svg", "simple-shapes.svg"].map { fixtures + "/" + $0 }
func require(_ condition: Bool, _ message: String) {
    if !condition { print("FAIL", message); exit(1) }
}

class Delegate: NSObject, WKNavigationDelegate {
    var web: WKWebView!
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        check(0)
    }
    func check(_ index: Int) {
        if index == files.count {
            let hostile = "<svg xmlns='http://www.w3.org/2000/svg' width='40' height='20'><rect width='40' height='20' fill='red'/><script>document.querySelector('rect').setAttribute('fill','lime');</script><image href='https://example.invalid/never-fetch.svg' width='40' height='20'/></svg>"
            web.callAsyncJavaScript("const r = await window.mdflickRasterizeSVG(source); const i = new Image(); i.src = 'data:image/png;base64,'+r.png; await i.decode(); const c = document.createElement('canvas'); c.width=80;c.height=40; const x=c.getContext('2d'); x.drawImage(i,0,0); return Array.from(x.getImageData(40,20,1,1).data);", arguments: ["source": Data(hostile.utf8).base64EncodedString()], in: nil, in: .page) { result in
                guard case .success(let value) = result, let pixel = value as? [Int] else { print("FAIL image isolation", result); exit(1) }
                require(pixel == [255,0,0,255], "SVG scripts must not run")
                print("PASS SVG image mode isolation")
                exit(0)
            }
            return
        }
        let file = files[index]
        let encoded = try! Data(contentsOf: URL(fileURLWithPath: file)).base64EncodedString()
        web.callAsyncJavaScript("return await window.mdflickRasterizeSVG(source)", arguments: ["source": encoded], in: nil, in: .page) { result in
            do {
                let output = try result.get() as! [String:Any]
                let png = Data(base64Encoded: output["png"] as! String)!
                let width = output["width"] as! Int
                let height = output["height"] as! Int
                let bitmap = NSBitmapImageRep(data: png)!
                let expected = [(124,20), (69,20), (250,55), (80,24)][index]
                require(width == expected.0 && height == expected.1, "SVG intrinsic dimensions")
                require(bitmap.pixelsWide == width*2 && bitmap.pixelsHigh == height*2, "Retina raster dimensions")
                func count(_ start: Int, _ end: Int, _ matches: (NSColor) -> Bool) -> Int {
                    var n = 0
                    for y in 0..<bitmap.pixelsHigh { for x in start..<end {
                        if let c = bitmap.colorAt(x:x,y:y)?.usingColorSpace(.sRGB), matches(c) { n += 1 }
                    } }
                    return n
                }
                if index == 0 {
                    require(count(0,128) { $0.redComponent > 0.75 && $0.greenComponent > 0.4 && $0.blueComponent < 0.4 } > 500, "foreignObject CSS background and embedded SVG missing")
                    require(count(130,248) { $0.redComponent < 0.25 && $0.greenComponent < 0.25 && $0.blueComponent < 0.25 && $0.alphaComponent > 0.8 } > 250, "foreignObject flex text missing")
                } else if index == 1 {
                    let white: (NSColor) -> Bool = { $0.redComponent > 0.85 && $0.greenComponent > 0.85 && $0.blueComponent > 0.85 && $0.alphaComponent > 0.8 }
                    require(count(10,38,white) > 80, "embedded Discord logo missing")
                    require(count(44,128,white) > 180, "Discord text missing")
                }
                if index == 2 {
                    let purple: (NSColor) -> Bool = { $0.redComponent < 0.65 && $0.greenComponent < 0.65 && $0.blueComponent > 0.25 && $0.alphaComponent > 0.8 }
                    require(count(128,492,purple) > 250, "Trendshift repository title missing")
                    require(count(120,310,purple) > 80, "Trendshift trending label missing")
                }
                if index == 3 {
                    // Compare encoded components directly: NSColor conversion
                    // can apply the display's profile to an untagged canvas PNG.
                    var center = [UInt](repeating: 0, count: 4)
                    bitmap.getPixel(&center, atX:80, y:24)
                    require(center[0] == 67 && center[1] == 39 && center[2] == 135, "CSS variables or basic shapes missing")
                    require(bitmap.colorAt(x:0,y:0)!.alphaComponent == 0, "SVG transparent background lost")
                }
                print("PASS", URL(fileURLWithPath:file).lastPathComponent, width, height, "Retina", bitmap.pixelsWide, bitmap.pixelsHigh)
                self.check(index+1)
            } catch { print("FAIL", error); exit(1) }
        }
    }
}
let delegate = Delegate()
let web = WKWebView(frame: NSRect(x:0,y:0,width:1000,height:800))
delegate.web = web
web.navigationDelegate = delegate
web.loadHTMLString("<!doctype html><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src data: blob:; style-src 'unsafe-inline'; script-src 'sha256-\(scriptHash)';\"><script>\(runtime)</script>", baseURL: nil)
DispatchQueue.main.asyncAfter(deadline: .now()+40) { print("TIMEOUT"); exit(1) }
app.run()
