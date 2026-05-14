package main

import (
	"connectrpc.com/connect"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/spf13/cobra"
)

// newPingCmd builds the `ping <instance> <router> <target>`
// subcommand. The router argument is resolved through
// [resolveRouter] so users can supply either a numeric ID or a
// substring of the router's name.
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
			routerID, err := resolveRouter(ctx, client, args[1], false)
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
			return printOpResult(string(resp.Msg.GetResult()), resp.Msg.GetTimestamp().AsTime())
		},
	}
}

// newTracerouteCmd builds the `traceroute` subcommand (also
// reachable via the `trace` alias). Argument shape and resolution
// rules mirror [newPingCmd].
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
			routerID, err := resolveRouter(ctx, client, args[1], false)
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
			return printOpResult(string(resp.Msg.GetResult()), resp.Msg.GetTimestamp().AsTime())
		},
	}
}
