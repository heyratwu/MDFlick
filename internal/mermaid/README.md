# Embedded Mermaid

`mermaid.min.js` is the unmodified, self-contained Mermaid **11.4.1** bundle:
https://cdn.jsdelivr.net/npm/mermaid@11.4.1/dist/mermaid.min.js

Upstream: https://github.com/mermaid-js/mermaid/tree/v11.4.1 (MIT; see LICENSE).
Pinned to the 11.x runtime for the app's macOS 12 / Safari 15 target.
No CDN or Node.js is needed at runtime. `render.js` configures strict mode,
text labels, diagram limits, theme handling, and bitmap generation.
CSP hashes authorize only these bundled scripts. Document text is never
interpolated into script tags. Updating either script automatically updates
its CSP hash.
