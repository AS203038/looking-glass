package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// globalOpts holds flags that apply to every subcommand. They are wired up on
// the root command's PersistentFlags and read by leaf commands via the package
// variable so we don't have to thread them through every constructor.
type globalOpts struct {
	IndexURL string
	Output   string // pretty | json | raw
	Timeout  time.Duration
	NoColor  bool
	Quiet    bool
	Verbose  bool
}

var opts = &globalOpts{}

const (
	defaultIndexURL = "https://raw.githubusercontent.com/AS203038/looking-glass/main/public_index.yaml"
	defaultTimeout  = 60 * time.Second
)

// envOr returns the value of `env` if set and non-empty, otherwise `def`.
func envOr(env, def string) string {
	if v, ok := os.LookupEnv(env); ok && v != "" {
		return v
	}
	return def
}

// newRootCmd builds the top-level cobra command.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "lg-cli",
		Short: "Command-line client for Looking Glass (BGP/network looking-glass) instances",
		Long: strings.TrimSpace(`
lg-cli is a command-line client for Looking Glass instances exposed over
ConnectRPC. It can list instances from the public index, enumerate routers,
and execute ping / traceroute / BGP queries against a chosen router.

Instance argument accepts:
  - a public-index name (case-insensitive, e.g. as203038)
  - an ASN, with or without the "AS" prefix (e.g. 203038, AS203038)
  - a full URL (e.g. https://lg.example.com)

Router argument accepts:
  - a numeric router ID (e.g. 1)
  - a case-insensitive substring of the router name (resolved against the
    instance's GetRouters response; ambiguous matches are listed and rejected)
`),
		SilenceUsage:  true,
		SilenceErrors: false,
		// Validate global flag combinations once, before any subcommand runs.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch opts.Output {
			case "pretty", "json", "raw":
			default:
				return fmt.Errorf("invalid --output %q: want one of pretty|json|raw", opts.Output)
			}
			if opts.Quiet && opts.Verbose {
				return fmt.Errorf("--quiet and --verbose are mutually exclusive")
			}
			return nil
		},
	}

	// Persistent flags + env-var fallbacks. Cobra evaluates defaults at
	// command-build time, so we resolve env vars here.
	root.PersistentFlags().StringVar(&opts.IndexURL, "index",
		envOr("LG_INDEX_URL", defaultIndexURL),
		"URL of the public Looking Glass index (env: LG_INDEX_URL)")
	root.PersistentFlags().StringVarP(&opts.Output, "output", "o",
		envOr("LG_OUTPUT", "pretty"),
		"output format: pretty | json | raw (env: LG_OUTPUT)")
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout",
		parseDurationOr(envOr("LG_TIMEOUT", ""), defaultTimeout),
		"request timeout (env: LG_TIMEOUT)")
	root.PersistentFlags().BoolVar(&opts.NoColor, "no-color",
		envOr("LG_NO_COLOR", "") != "" || envOr("NO_COLOR", "") != "",
		"disable ANSI colour in pretty output (env: LG_NO_COLOR or NO_COLOR)")
	root.PersistentFlags().BoolVarP(&opts.Quiet, "quiet", "q", false,
		"suppress trailing timestamp footer in pretty output")
	root.PersistentFlags().BoolVarP(&opts.Verbose, "verbose", "v", false,
		"verbose diagnostics to stderr")

	root.AddCommand(
		newInstancesCmd(),
		newInfoCmd(),
		newRoutersCmd(),
		newPingCmd(),
		newTracerouteCmd(),
		newBGPCmd(),
		newVersionCmd(),
		newCompletionCmd(),
	)
	return root
}

func parseDurationOr(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// cmdContext returns a context that:
//   - has the global --timeout deadline applied
//   - is cancelled on SIGINT / SIGTERM so connections close promptly
//
// The returned cancel MUST be called by the caller (defer cancel()).
func cmdContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	// Wrap both cancels into one so the caller only needs to defer one.
	return sigCtx, func() {
		stop()
		cancel()
	}
}

// verbosef writes a diagnostic to stderr when --verbose is set.
func verbosef(format string, a ...any) {
	if !opts.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "lg-cli: "+format+"\n", a...)
}
