// mdpreview is a sandbox-inheriting helper used by the Quick Look extension.
package main

import (
	"context"
	"fmt"
	"mdflick/internal/document"
	"mdflick/internal/preview"
	"os"
)

func main() {
	source, err := document.Read(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	html, err := preview.Render(context.Background(), source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(html)
}
