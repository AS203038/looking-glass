package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// ANSI colour escapes (gated by isColorEnabled).
const (
	ansiReset = "\x1b[0m"
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
	ansiDim   = "\x1b[2m"
	ansiBold  = "\x1b[1m"
)

// isTTY returns true if `f` is an interactive terminal. We probe via Stat()
// and check the character-device mode bit to avoid pulling in a dep.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// isColorEnabled returns true if pretty output should emit ANSI colour. We
// honour --no-color (and the NO_COLOR / LG_NO_COLOR env vars baked into the
// default), and additionally turn colour off when stdout is not a TTY (e.g.
// piped or redirected).
func isColorEnabled() bool {
	if opts.NoColor {
		return false
	}
	return isTTY(os.Stdout)
}

// colorize wraps `s` in `code` + reset if colour is enabled.
func colorize(s, code string) string {
	if !isColorEnabled() {
		return s
	}
	return code + s + ansiReset
}

// opResult is the canonical shape we emit for ping/traceroute/bgp queries.
type opResult struct {
	Result    string `json:"result"`
	Timestamp string `json:"timestamp"`
}

// printOpResult writes the result of a single-router operation according to
// the global --output mode.
//
// Modes:
//   - raw    : just the result body, no timestamp, no trailing newline added
//   - json   : {"result": "...", "timestamp": "..."}\n
//   - pretty : result body, then (unless --quiet) a dim "ts: <RFC3339>" footer
//     on stderr so stdout stays clean for piping.
func printOpResult(result string, ts time.Time) error {
	switch opts.Output {
	case "raw":
		_, err := io.WriteString(os.Stdout, result)
		return err
	case "json":
		return json.NewEncoder(os.Stdout).Encode(opResult{
			Result:    result,
			Timestamp: ts.Format(time.RFC3339),
		})
	default: // pretty
		if _, err := io.WriteString(os.Stdout, result); err != nil {
			return err
		}
		// Ensure result ends with a newline before the footer.
		if len(result) == 0 || result[len(result)-1] != '\n' {
			fmt.Fprintln(os.Stdout)
		}
		if !opts.Quiet {
			fmt.Fprintln(os.Stderr, colorize(
				fmt.Sprintf("ts: %s", ts.Format(time.RFC3339)), ansiDim,
			))
		}
		return nil
	}
}

// printJSON writes `v` as a JSON object to stdout (used by `instances`,
// `routers`, `info` in --output json mode).
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
