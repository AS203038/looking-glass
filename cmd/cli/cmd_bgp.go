package main

import (
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/spf13/cobra"
)

// newBGPCmd builds the `bgp` parent command.
func newBGPCmd() *cobra.Command {
	bgp := &cobra.Command{
		Use:   "bgp",
		Short: "BGP query commands (route, community, aspath, summary)",
	}
	bgp.AddCommand(
		newBGPSummaryCmd(),
		newBGPRouteCmd(),
		newBGPCommunityCmd(),
		newBGPASPathCmd(),
	)
	return bgp
}

// newBGPSummaryCmd builds the `bgp summary` subcommand.
func newBGPSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "summary <instance> <router>",
		Short: "Show BGP neighbour summary on a router",
		Args:  cobra.ExactArgs(2),
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

			resp, err := client.BGPSummary(ctx, connect.NewRequest(&pb.BGPSummaryRequest{
				RouterId: routerID,
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

// newBGPRouteCmd builds the `bgp route` subcommand.
func newBGPRouteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "route <instance> <router> <prefix>",
		Short: "Look up a BGP route by prefix",
		Long:  "Look up the BGP route(s) for <prefix> on the chosen router. <prefix> can be an IPv4 or IPv6 address or CIDR (e.g. 8.8.8.0/24, 2001:db8::/32).",
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

			resp, err := client.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{
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

// newBGPCommunityCmd builds the `bgp community` subcommand.
func newBGPCommunityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "community <instance> <router> <community>",
		Short: "Look up BGP routes by community",
		Long: strings.TrimSpace(`
Look up BGP routes tagged with <community>. The format is auto-detected
from the number of colon-separated fields:

  ASN:VALUE                 — standard community (RFC 1997)
  GLOBAL:LOCAL1:LOCAL2      — large community  (RFC 8092)

Examples:
  lg-cli bgp community as203038 1 65000:100
  lg-cli bgp community as203038 1 214503:8:3607
`),
		Args: cobra.ExactArgs(3),
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

			parts := strings.Split(args[2], ":")
			switch len(parts) {
			case 2:
				asn, err := strconv.ParseInt(parts[0], 10, 32)
				if err != nil {
					return fmt.Errorf("invalid community ASN %q: %w", parts[0], err)
				}
				val, err := strconv.ParseInt(parts[1], 10, 32)
				if err != nil {
					return fmt.Errorf("invalid community value %q: %w", parts[1], err)
				}
				resp, err := client.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
					RouterId: routerID,
					Community: &pb.BGPCommunity{
						Asn:   int32(asn),
						Value: int32(val),
					},
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

			case 3:
				global, err := strconv.ParseUint(parts[0], 10, 32)
				if err != nil {
					return fmt.Errorf("invalid large-community global %q: %w", parts[0], err)
				}
				local1, err := strconv.ParseUint(parts[1], 10, 32)
				if err != nil {
					return fmt.Errorf("invalid large-community local1 %q: %w", parts[1], err)
				}
				local2, err := strconv.ParseUint(parts[2], 10, 32)
				if err != nil {
					return fmt.Errorf("invalid large-community local2 %q: %w", parts[2], err)
				}
				resp, err := client.BGPLargeCommunity(ctx, connect.NewRequest(&pb.BGPLargeCommunityRequest{
					RouterId: routerID,
					Community: &pb.BGPLargeCommunity{
						GlobalAdmin: uint32(global),
						LocalData1:  uint32(local1),
						LocalData2:  uint32(local2),
					},
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

			default:
				return fmt.Errorf(
					"invalid community %q: expected ASN:VALUE (standard) or GLOBAL:LOCAL1:LOCAL2 (large)",
					args[2])
			}
		},
	}
}

// newBGPASPathCmd builds the `bgp aspath` subcommand (alias `as-path`).
func newBGPASPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "aspath <instance> <router> <regex>",
		Aliases: []string{"as-path"},
		Short:   "Look up BGP routes matching an AS-path regex",
		Long:    "Filter BGP routes by AS-path. <regex> uses the router-native regex syntax (vendor-specific).",
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

			resp, err := client.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{
				RouterId: routerID,
				Pattern:  args[2],
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
