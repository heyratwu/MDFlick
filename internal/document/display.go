package document

import (
	"net/url"
	"strings"
	"unicode"
)

type LinkAction uint8

const (
	IgnoreLink LinkAction = iota
	ScrollToAnchor
	OpenExternal
)

// DisplayPolicy describes adapter capabilities, independent of UI/WebKit APIs.
// Functions return values so one adapter cannot mutate another's defaults.
type DisplayPolicy struct {
	LocalImages, RemoteImages bool
	ExternalLinks             bool
	Mermaid                   bool
}

func NativePolicy() DisplayPolicy {
	return DisplayPolicy{LocalImages: true, RemoteImages: true, ExternalLinks: true, Mermaid: true}
}
func QuickLookPolicy() DisplayPolicy { return DisplayPolicy{Mermaid: true} }

func (p DisplayPolicy) Link(dest string) LinkAction {
	if strings.ContainsAny(dest, "\\\x00\r\n") || strings.HasPrefix(dest, "//") {
		return IgnoreLink
	}
	if strings.HasPrefix(dest, "#") {
		if _, err := url.PathUnescape(dest[1:]); err == nil {
			return ScrollToAnchor
		}
		return IgnoreLink
	}
	u, err := url.Parse(dest)
	if err != nil || !p.ExternalLinks {
		return IgnoreLink
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host != "" {
			return OpenExternal
		}
	case "mailto":
		if u.Opaque != "" || u.Path != "" {
			return OpenExternal
		}
	}
	return IgnoreLink
}

func (p DisplayPolicy) AllowsImage(source string) bool {
	if source == "" || strings.ContainsRune(source, '\x00') || strings.HasPrefix(source, "//") {
		return false
	}
	u, err := url.Parse(source)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return p.RemoteImages && u.Host != ""
	case "", "file":
		return p.LocalImages
	default:
		return false
	}
}

// Generated Quick Look documents have an about:blank base. Navigation never
// escapes that document, even when a new URL is supplied outside the sanitizer.
func (p DisplayPolicy) AllowsPreviewNavigation(dest string) bool {
	return dest == "about:blank" || strings.HasPrefix(dest, "about:blank#")
}

// HeadingAnchor matches the pinned native Markdown renderer's heading naming.
func HeadingAnchor(text string) string {
	var result strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case r == ' ':
			result.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			result.WriteRune(r)
		}
	}
	return result.String()
}
