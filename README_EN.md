# MDFlick

A read-only Markdown viewer built with MyGo, supporting macOS 12 and later and Windows. The Quick Look extension is available only on macOS.

- Double-click `.md` or `.markdown` files to open them, with each file displayed in its own window.
- When launched from the app icon, the home screen shows an “Open File” button in the center. The first selected file opens directly in the current window.
- Closing the last window quits the app. Opening a file by double-clicking reuses an existing home window instead of keeping an extra default window open.
- Use `⌘O` to open a file and `⌘R` to reload it. The system's “Open Recent” menu is supported.
- Open Markdown files automatically reload and rerender when modified by other software, including editors that use atomic saves. The current preview and scroll position are preserved during refresh, and unchanged content skips redundant rendering. Rapid successive writes are coalesced into a single refresh.
- Native Markdown rendering supports headings, lists, tables, task lists, code blocks, text selection, and copying, and follows the system's light or dark appearance.
- HTML layout supports common containers, heading alignment, ordered and nested lists, basic tables, and blockquotes. Images inside links and image titles are displayed natively, image dimensions are preserved, and `<picture>` sources are selected according to the system's light or dark appearance. See [Native HTML Support](docs/native-html.md) for capabilities and limitations. Quick Look maintains its own filtering and image policies.
- Mermaid diagrams work in both the main window and Quick Look for fenced code blocks labeled `mermaid`. The bundled offline runtime supports Chinese text and the system's light or dark appearance; syntax errors fall back to the source code.
- Heading anchors within a document support navigation, and safe web links open in the default browser. Local and HTTP/HTTPS images load asynchronously when they enter the viewport or the prefetch area extending 200 points above and below it, with visible images taking priority. Unfinished requests are canceled when scrolled out of range and can be retried on return. Completed images are cached within budget; offscreen items can be reclaimed and reloaded when needed. Each image is limited to 16 MiB and each bitmap to 16 million pixels, with at most two images loading concurrently and a 15-second network timeout. Reloading refreshes the image cache.
- A built-in Quick Look Preview Extension provides formatted previews when you select a file in Finder and press Space.
- Body text, tables, and HTML-aligned paragraphs in the same document share image loading and decoding results. Each document revision caches at most 128 image URLs, with estimated image cache budgets of 128 MiB per document and 256 MiB across the app. At most two images load concurrently per document and four across the app. When a limit is reached, the app first tries to reclaim offscreen images in the current window. If none can be reclaimed, it keeps alt text or a placeholder at the original dimensions; loading can be retried after the retry interval when the viewport is reevaluated. Reloading resets the cache. The budget includes bitmap RGBA pixels and retained compressed data; SVGs use the same RGBA pixel and compressed PNG accounting after browser rasterization. These budgets are not hard limits on total process or GPU memory.
- No editing, saving, or automatic write-back. Files must use UTF-8 and are limited to 32 MB each.

See [Project Architecture](docs/project-architecture.md) for architecture and resource lifetimes, and [Performance Validation](docs/performance.md) for benchmarks, stage timings, and cancellation boundaries. Quick Look's Go renderer uses a 15-second cooperative deadline, processes at most 20,000 parsed blocks, and generates at most 64 MiB of HTML. It renders only the first 128 Mermaid diagrams, leaving the rest as source code.

## Development

Use the Go toolchain specified in go.mod and the macOS Command Line Tools:

```sh
go tool mygo dev
CGO_ENABLED=0 go test ./...
go tool mygo vet .
go test -race ./...
bash scripts/test-macos-bridge.sh  # macOS: tests the actual Swift/C/Go boundary
```

The main window reads and preprocesses documents in the background, with at most two concurrent tasks across the app. Its 15-second cooperative deadline includes queue time. Both the original and converted parse results are limited to 20,000 blocks, and converted Markdown is limited to 64 MiB. HTML conversion reuses the initial parse results. Once the background task finishes the final document and inline parsing, it transfers exclusive ownership of the parsed document, alignment ranges, image attributes, and Mermaid classification to the UI thread. Body text, tables, headings, and aligned paragraphs use the same native Markdown view, without rebuilding Markdown fragments or parsing again on the first frame. MyGo still handles layout and drawing; long-document virtualization has not yet been implemented. See [Local MyGo Patches](docs/mygo-patches.md) for local framework API patches and dependency updates.

The main window uses `ui/markdown.ViewDocument` and routes diagrams through `Options.Code["mermaid"]` to a hidden WKWebView, which asynchronously generates high-resolution images. Results are cached by source and theme, and a button to copy the source is retained. Quick Look uses the same MyGo Markdown parser. Ordinary Markdown is converted directly from the AST to safe HTML; documents containing raw HTML retain full DOM filtering and are displayed through Swift / WKWebView. The CSP authorizes only the bundled Mermaid script: document scripts are not executed, and no network resources are requested. Quick Look preserves tables, code, and task lists; code uses a monospaced font, and images show alt text. The full window supports syntax highlighting and local and network images. Code blocks larger than 32 KiB use complete monospaced plain text while retaining selection and copying. The currently pinned MyGo version does not automatically handle relative document links.

Mermaid uses the bundled 11.4.1 runtime, with no Node.js installation or network connection required. Open the [example document](docs/examples/mermaid.md) to see flowcharts, sequence diagrams, and error fallback. Each diagram is limited to 50,000 characters and 500 edges; oversized or unsupported diagrams fall back to source code. In the main window, diagrams are submitted to an app-wide rendering pool only when they approach the viewport. The pool keeps at most two WebViews, created on demand and destroyed after 30 seconds of inactivity; window reloads can reuse healthy runtimes. Each document retains at most 128 source/theme keys, and the app's pending queue holds up to 16 tasks. The 15-second deadline includes all queue time. Cancellation or timeout destroys the runtime, which is recreated for subsequent tasks. Decoded images are estimated by RGBA pixel size, with cache budgets of 64 MiB per document and 128 MiB across the app. When the cache reaches its limit, offscreen items in the current window are reclaimed first, leaving placeholders that preserve diagram dimensions. If nothing can be reclaimed, the source code is shown and rendering can be retried after the retry interval. Reloading clears the cache. After frame submission, images and diagrams retain offscreen warm caches of up to 16 MiB and 8 MiB respectively, while visible items remain resident. The diagram queue is reordered with the viewport; queued offscreen tasks are canceled, while tasks that have already started using a WebView are allowed to finish. Quick Look also renders diagrams according to the viewport and defers offscreen diagrams during theme changes. See [internal/mermaid](internal/mermaid/README.md) for the third-party runtime version and license.

The main window and Quick Look share the 32 MiB / UTF-8 rules (BOM allowed), validation error codes, and Mermaid code block detection. The word `mermaid` in ordinary text does not start the renderer. Only fenced code blocks with the exact, case-sensitive label `mermaid` are rendered; unclosed fences retain their source code. See [Testing](docs/testing.md) for more validation details.

| Capability | Main window | Quick Look |
| --- | --- | --- |
| Headings, lists, tables, task lists | Native Markdown | Filtered HTML |
| Mermaid | Offline PNG generation; source code on failure | Offline SVG; source code on failure |
| System light/dark appearance | Supported | Supported |
| Code syntax highlighting | Supported | Monospaced text |
| Local images | Asynchronous loading with size and pixel limits | Alt text |
| External web links | Open in the default browser | External navigation blocked |
| Relative document links | Not handled by the current version | External navigation blocked |
| HTTP/HTTPS images | Asynchronous loading with size and pixel limits | Alt text; no network requests |
| Document scripts | Not executed | Not executed |

## Building macOS Installers

**Use the scripts below to build release artifacts.** Running `go tool mygo build` on its own builds only the main app and does not include the Quick Look extension.

```sh
bash scripts/build-macos.sh arm64  # Apple Silicon
bash scripts/build-macos.sh amd64  # Intel
```

Requires `swiftc`, `codesign`, and `hdiutil` from the Apple SDK. Output goes to `build/darwin-arm64/` or `build/darwin-amd64/` and includes `MDFlick.app` and a DMG. The script first builds the MyGo app, then compiles and embeds `Contents/PlugIns/MDFlickQuickLook.appex`, linking the Go renderer into the extension as a static library. It signs the extension and then the main app before creating the DMG. The extension reads and renders documents directly without launching a helper process. WebKit uses the network client entitlement to launch its subprocesses; the document's CSP and navigation policies still block external network content.

By default, the build uses local ad hoc signing, suitable for development. For distribution, set your own production bundle identifier (updating both `mygo.json` and the extension's plist) and use Developer ID:

```sh
SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)' bash scripts/build-macos.sh arm64
```

The signing identity must already exist in your keychain. The script does not notarize automatically. Before public distribution, submit with `xcrun notarytool submit <dmg> --keychain-profile <profile> --wait`, then staple the ticket with `xcrun stapler staple <dmg>` after approval.

## Building Windows Installers

In Windows PowerShell:

```powershell
.\scripts\build-windows.ps1           # x64
.\scripts\build-windows.ps1 -Arch arm64
```

Requires the Go toolchain specified in go.mod. On Windows, MyGo automatically downloads and verifies the NSIS installer compiler if it is missing. You can also cross-compile on macOS / Linux: install NSIS first (`brew install makensis` / `apt install nsis`), then run:

```sh
bash scripts/build-windows.sh amd64
bash scripts/build-windows.sh arm64
```

The release installer is `build/windows-amd64/mdflick-setup-0.1.1-amd64.exe` (ARM64 uses `windows-arm64` and `arm64.exe`; the version follows `mygo.json`). The scripts verify that the current build actually produced an installer. If NSIS is missing, they will not report success after producing only the main executable. When manually distributing a portable version, include the resource files from the same directory. Running the portable version directly does not perform system registration.

### Windows Installation and Uninstallation

1. Run `mdflick-setup-<version>-<arch>.exe` and choose a local installation directory in the wizard. The default is `%LOCALAPPDATA%\Programs\MDFlick`. Installation is per user and requires no administrator privileges. Custom paths must be writable by the current user; choose a dedicated application directory. Reinstalling defaults to the existing path.
2. Installation registers both `.md` and `.markdown` file types and adds MDFlick to the Start menu. The completion page lets you launch the app and choose whether to create a desktop shortcut.
3. If another default app is already set, Windows 10/11 requires confirmation through the system UI. Open **MDFlick - Default Apps** from the Start menu, find MDFlick under “Default apps,” and set `.md` and `.markdown` individually. Alternatively, right-click a file → “Open with” → MDFlick → choose to always use it. The installer does not override existing system default choices; if no handler exists, it sets the default file association.
4. Find MDFlick under Windows “Settings → Apps → Installed apps” and uninstall it, or use **MDFlick - Uninstall** in the Start menu / `Uninstall.exe` in the installation directory. Uninstallation removes the app, shortcuts, and its file type registrations while preserving user documents.

Silent installation is supported with `mdflick-setup-<version>-<arch>.exe /S`, and silent uninstallation with `Uninstall.exe /S`. To specify a custom directory for silent installation, put `/D=C:\Apps\MDFlick` at the end of the command (NSIS's `/D=` argument must not be quoted, even when the path contains spaces). Silent installation does not automatically launch the app or create a desktop shortcut.

Windows release acceptance checks: on Windows 10/11, verify installation paths containing spaces and Chinese characters, launching from the Start menu, double-clicking both file extensions (including when another default app exists), installing over a running app, uninstalling through system settings, and cleanup of shortcuts and file associations. Cross-compiling on macOS does not replace these checks on an actual Windows system.

## macOS Installation and Usage

1. Open the DMG, drag MDFlick into Applications, and launch it once.
2. Double-click a Markdown file. If it is already associated with another app, choose MDFlick in Finder's “Get Info → Open with,” then click “Change All” to make it the default. The app registers as a Viewer and respects existing default app choices.
3. Select a Markdown file in Finder and press Space to use Quick Look. If it is not enabled, enable MDFlick Markdown Preview under “System Settings → Login Items & Extensions → Quick Look” (the exact location and labels may vary by macOS version).

During development, check registration with:

```sh
pluginkit -m -i com.mdflick.app.QuickLook
mdls -name kMDItemContentType /path/to/example.md
```

If another Markdown extension still provides the preview, disable the conflicting extension in System Settings. The file's content type should be `net.daringfireball.markdown`. No global keyboard hook is required.

Quick Look uses Apple's [QLPreviewingController](https://developer.apple.com/documentation/QuickLookUI/QLPreviewingController) and [Quick Look extension](https://developer.apple.com/documentation/quicklook) mechanisms. Verify that the extension is enabled in Finder after an actual installation. Tests do not change local default file associations or system extension settings.


All document SVG images, including text, foreignObject HTML/CSS, embedded images and filters, use the shared browser runtime pool and render to transparent bitmaps at up to 2× resolution while retaining their intrinsic display size. SVG image mode disables scripts and external subresources. On macOS, run `bash scripts/test-svg-browser.sh` for offline WebKit badge pixel regression tests.
