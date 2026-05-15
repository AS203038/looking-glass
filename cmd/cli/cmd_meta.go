package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// newVersionCmd builds the `version` subcommand.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show lg-cli version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch opts.Output {
			case "json":
				return printJSON(map[string]string{
					"version":   Version,
					"goVersion": runtime.Version(),
					"os":        runtime.GOOS,
					"arch":      runtime.GOARCH,
				})
			case "raw":
				fmt.Println(Version)
				return nil
			default:
				fmt.Printf("lg-cli %s %s/%s (built with %s)\n",
					Version, runtime.GOOS, runtime.GOARCH, runtime.Version())
				return nil
			}
		},
	}
}

// newCompletionCmd builds the `completion` subcommand.
func newCompletionCmd() *cobra.Command {
	completion := &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate shell completion script",
		Long:                  "Generate a shell completion script. Run `lg-cli completion <shell> --help` for installation instructions.",
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return fmt.Errorf("unsupported shell %q", args[0])
		},
	}
	return completion
}
