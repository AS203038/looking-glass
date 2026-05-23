package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newInstancesCmd builds the `instances` subcommand.
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
			default:
				tw := newTabWriter(os.Stdout)
				fmt.Fprintln(tw, cBold+"ASN\tNAME\tURL"+cReset)
				for _, lg := range idx.LookingGlasses {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", lg.ASN, lg.Name, lg.URL)
				}
				return tw.Flush()
			}
		},
	}
}
