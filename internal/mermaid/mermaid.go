// Package mermaid embeds the offline Mermaid runtime shared by both previews.
package mermaid

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
)

// Mermaid 11.4.1, MIT licensed; see LICENSE and README.md.
//
//go:embed mermaid.min.js
var library string

//go:embed render.js
var runtime string

// Scripts returns trusted scripts and the CSP hashes that authorize only them.
func Scripts() (tags, policy string) {
	for _, script := range []string{library, runtime} {
		sum := sha256.Sum256([]byte(script))
		policy += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		tags += "<script>" + script + "</script>"
	}
	return tags, "script-src" + policy + ";"
}

func HTML() string {
	scripts, policy := Scripts()
	return `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: blob:; style-src 'unsafe-inline'; base-uri 'none'; ` + policy + `"><style>body{margin:0;width:1000px;font-family:-apple-system,sans-serif}</style><body>` + scripts + `</body>`
}
