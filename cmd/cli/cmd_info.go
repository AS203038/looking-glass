package main

import (
	"fmt"
	"os"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

// newInfoCmd builds the `info <instance>` subcommand.
func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <instance>",
		Short: "Show hostname and version of a Looking Glass instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			lg, err := resolveInstance(ctx, args[0])
			if err != nil {
				return err
			}
			client := newClient(lg)

			resp, err := client.GetInfo(ctx, connect.NewRequest(&emptypb.Empty{}))
			if err != nil {
				return err
			}
			info := resp.Msg

			switch opts.Output {
			case "json":
				return printJSON(map[string]string{
					"instance": lg.Name,
					"url":      lg.URL,
					"hostname": info.GetHostname(),
					"version":  info.GetVersion(),
				})
			case "raw":
				fmt.Println(info.GetVersion())
				return nil
			default:
				tw := newTabWriter(os.Stdout)
				fmt.Fprintf(tw, "%sInstance:%s\t%s\n", cBold, cReset, lg.Name)
				fmt.Fprintf(tw, "%sURL:%s\t%s\n", cBold, cReset, lg.URL)
				fmt.Fprintf(tw, "%sHostname:%s\t%s\n", cBold, cReset, info.GetHostname())
				fmt.Fprintf(tw, "%sVersion:%s\t%s\n", cBold, cReset, info.GetVersion())
				return tw.Flush()
			}
		},
	}
}
