// Command kiln is the Kiln toolchain: check, render, serve and document a
// Kiln program.
package main

import (
	"os"

	"github.com/chrisnordrum/kiln/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
