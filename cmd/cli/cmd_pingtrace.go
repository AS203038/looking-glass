package main

import (
	"connectrpc.com/connect"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/spf13/cobra"
)

// newPingCmd builds the `ping` subcommand.
func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping <instance> <router> <target>",
		Short: "Run a ping from a router to a target",
		Long:  "Issue an ICMP echo from the chosen router to <target>. <target> may be an IPv4 or IPv6 address (or hostname, depending on the router template).",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			lg, err := resolveInstance(ctx, args[0])
			if err != nil {
				return err
			}
			client := newClient(lg)
			routerID, err := resolveRouter(ctx, client, lg.URL, args[1], false)
			if err != nil {
				return err
			}

			resp, err := client.Ping(ctx, connect.NewRequest(&pb.PingRequest{
				RouterId: routerID,
				Target:   args[2],
			}))
			if err != nil {
				return err
			}
			msg := resp.Msg
			var parsed any
			if msg.GetParsed() != nil {
				parsed = msg.GetParsed()
			}
			return printParsedResult(
				string(msg.GetResult()), msg.GetTimestamp().AsTime(),
				parsed, msg.GetParserKind(), msg.GetParseStatus())
		},
	}
}

// newTracerouteCmd builds the `traceroute` subcommand (alias `trace`).
func newTracerouteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "traceroute <instance> <router> <target>",
		Aliases: []string{"trace"},
		Short:   "Run a traceroute from a router to a target",
		Args:    cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			lg, err := resolveInstance(ctx, args[0])
			if err != nil {
				return err
			}
			client := newClient(lg)
			routerID, err := resolveRouter(ctx, client, lg.URL, args[1], false)
			if err != nil {
				return err
			}

			resp, err := client.Traceroute(ctx, connect.NewRequest(&pb.TracerouteRequest{
				RouterId: routerID,
				Target:   args[2],
			}))
			if err != nil {
				return err
			}
			msg := resp.Msg
			var parsed any
			if msg.GetParsed() != nil {
				parsed = msg.GetParsed()
			}
			return printParsedResult(
				string(msg.GetResult()), msg.GetTimestamp().AsTime(),
				parsed, msg.GetParserKind(), msg.GetParseStatus())
		},
	}
}
