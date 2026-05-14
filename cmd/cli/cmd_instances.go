package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// newInstancesCmd builds the `instances` subcommand, which prints
// every Looking Glass entry in the configured public index.
//
// The index is fetched through [fetchIndex] (with its own tight
// timeout, independent of --timeout) so a slow registry never eats
// into the RPC budget. Output respects --output: pretty renders a
// columnar table, json dumps the raw index entries, and raw prints
// one instance name per line for easy scripting.
func newInstancesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "instances",
		Short: "List Looking Glass instances from the public index",
		Long: "List every Looking Glass entry in the public index " +
			"(--index / LG_INDEX_URL). Useful for discovering what `<instance>` " +
			"values you can pass to other commands.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			idx, err := fetchIndex(ctx)
			if err != nil {
				return err
			}

			switch opts.Output {
			case "json":
				return printJSON(idx.LookingGlasses)
			case "raw":
				for _, lg := range idx.LookingGlasses {
					fmt.Println(lg.Name)
				}
				return nil
			default: // pretty
				tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, colorize("ASN\tNAME\tURL", ansiBold))
				for _, lg := range idx.LookingGlasses {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", lg.ASN, lg.Name, lg.URL)
				}
				return tw.Flush()
			}
		},
	}
}
