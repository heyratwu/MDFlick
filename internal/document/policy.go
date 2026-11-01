package document

import (
	"context"
	"errors"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// Code values are stable across the Go/C/Swift boundary. Messages live here so
// the two presentation adapters report the same validation failure.
type Code int

const (
	OK                   Code = 0
	TooLarge             Code = 1
	InvalidUTF8          Code = 2
	UnsupportedExtension Code = 3
	NotRegular           Code = 4
	InvalidArgument      Code = 5
	InternalFailure      Code = 6
	IOFailure            Code = 7
	ResourceLimit        Code = 8
	Cancelled            Code = 9
	TimedOut             Code = 10
)

func (c Code) Error() string {
	switch c {
	case TooLarge:
		return "文件超过 32 MB，无法预览"
	case InvalidUTF8:
		return "请使用 UTF-8 编码的 Markdown 文件"
	case UnsupportedExtension:
		return "仅支持 .md 和 .markdown 文件"
	case NotRegular:
		return "请选择普通 Markdown 文件"
	case InvalidArgument:
		return "无效的预览请求"
	case IOFailure:
		return "无法读取 Markdown 文件"
	case ResourceLimit:
		return "文档渲染结果超过资源限制"
	case Cancelled:
		return "预览请求已取消"
	case TimedOut:
		return "生成预览超时"
	case OK:
		return ""
	default:
		return "无法生成 Markdown 预览"
	}
}
func CodeOf(err error) Code {
	if err == nil {
		return OK
	}
	if errors.Is(err, context.Canceled) {
		return Cancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return TimedOut
	}
	var code Code
	if errors.As(err, &code) {
		return code
	}
	return IOFailure
}

// IsMermaid uses the same language token as MyGo's native Code callback.
// Incomplete fences stay as source in both presentation adapters.
func IsMermaid(b *parser.Block) bool {
	return b.Kind == parser.CodeBlock && b.Fenced && !b.Open && b.Language() == "mermaid"
}
func HasMermaid(d *parser.Document) bool {
	for i := 0; i < d.Len(); i++ {
		if IsMermaid(d.Block(i)) {
			return true
		}
	}
	return false
}
func DetectMermaid(source string) bool { return HasMermaid(parser.Parse(source)) }
