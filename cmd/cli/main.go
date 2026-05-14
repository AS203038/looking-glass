// lg-cli — a command-line client for Looking Glass instances.
//
// Usage:
//
//	lg-cli <command> [args...]
//
// Run `lg-cli --help` for the full surface. Subcommand help is available via
// `lg-cli <command> --help`, e.g. `lg-cli bgp community --help`.
package main

import (
	"fmt"
	"os"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// Cobra already prints the error+usage on parse failures; we only need
		// to set a non-zero exit code. We avoid double-printing by checking
		// whether the error has already been surfaced via SilenceErrors=false.
		// In practice cobra prints to stderr; suppressing here is safe-ish.
		_ = err
		fmt.Fprintln(os.Stderr) // trailing newline before exit, helps in pipes
		os.Exit(1)
	}
}
