// mdpreview is a sandbox-inheriting helper used by the Quick Look extension.
package main

import (
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
	fmt.Print(preview.HTML(source))
}
