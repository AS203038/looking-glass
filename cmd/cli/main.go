// Command lg-cli is the command-line client for Looking Glass instances.
package main

import (
	"fmt"
	"os"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		_ = err
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
}
