package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// routerView is the JSON shape emitted for `routers` and `info`.
type routerView struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	Healthy   bool   `json:"healthy"`
	Timestamp string `json:"timestamp"`
}

// newRoutersCmd builds the `routers <instance>` subcommand.
func newRoutersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "routers <instance>",
		Short: "List the routers exposed by a Looking Glass instance",
		Long: "List every router exposed by the chosen instance, including the " +
			"latest health-check status and timestamp. Use the ID or NAME from " +
			"this list as the <router> argument to ping/traceroute/bgp commands.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			lg, err := resolveInstance(ctx, args[0])
			if err != nil {
				return err
			}
			client := newClient(lg)

			routers, err := fetchAllRouters(ctx, client)
			if err != nil {
				return err
			}

			switch opts.Output {
			case "json":
				out := make([]routerView, 0, len(routers))
				for _, rt := range routers {
					out = append(out, routerView{
						ID:        rt.GetId(),
						Name:      rt.GetName(),
						Location:  rt.GetLocation(),
						Healthy:   rt.GetHealth().GetHealthy(),
						Timestamp: rt.GetHealth().GetTimestamp().AsTime().Format(time.RFC3339),
					})
				}
				return printJSON(out)
			case "raw":
				for _, rt := range routers {
					fmt.Printf("%d\t%s\n", rt.GetId(), rt.GetName())
				}
				return nil
			default:
				tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, colorize("ID\tNAME\tLOCATION\tHEALTH\tLAST CHECK", ansiBold))
				for _, rt := range routers {
					h := rt.GetHealth()
					healthStr := colorize("✗ unhealthy", ansiRed)
					if h.GetHealthy() {
						healthStr = colorize("✓ healthy", ansiGreen)
					}
					ts := "-"
					if h != nil && h.GetTimestamp() != nil {
						ts = h.GetTimestamp().AsTime().Format(time.RFC3339)
					}
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n",
						rt.GetId(), rt.GetName(), rt.GetLocation(), healthStr, ts)
				}
				return tw.Flush()
			}
		},
	}
}
