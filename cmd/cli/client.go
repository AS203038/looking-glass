package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
)

// newClient builds a ConnectRPC client pointed at the given Looking Glass URL.
//
// The same http.DefaultClient is reused; per-call timeouts are enforced via
// the context returned by cmdContext().
func newClient(lg *LookingGlass) lookingglassconnect.LookingGlassServiceClient {
	verbosef("connecting to %s (%s)", lg.Name, lg.URL)
	return lookingglassconnect.NewLookingGlassServiceClient(http.DefaultClient, lg.URL)
}

// fetchAllRouters pages through GetRouters and returns every router the
// instance exposes. The remote service may return results in multiple pages;
// we follow next_page until empty.
func fetchAllRouters(ctx context.Context, client lookingglassconnect.LookingGlassServiceClient) ([]*pb.Router, error) {
	const pageSize = 1024
	var (
		all  []*pb.Router
		page uint32 = 1
	)
	for {
		resp, err := client.GetRouters(ctx, connect.NewRequest(&pb.GetRoutersRequest{
			Limit:     pageSize,
			PageToken: page,
		}))
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Msg.GetRouters()...)
		next := resp.Msg.GetNextPage()
		if next == 0 || next == page {
			break
		}
		page = next
	}
	return all, nil
}

// resolveRouter turns a user-supplied router argument into an *int64* router
// ID. The argument may be:
//   - a positive integer: used directly (no network round-trip)
//   - any other string: matched as a case-insensitive substring against the
//     instance's router names; an exact name match wins over substring
//     matches; ambiguous substring matches return an error listing candidates.
//
// For numeric IDs we skip the GetRouters lookup to keep simple scripted
// invocations cheap. Pass `requireExists=true` to force validation.
func resolveRouter(
	ctx context.Context,
	client lookingglassconnect.LookingGlassServiceClient,
	arg string,
	requireExists bool,
) (int64, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, fmt.Errorf("router argument is required")
	}

	// Numeric fast path.
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil && id > 0 {
		if !requireExists {
			return id, nil
		}
		// Validate the ID exists.
		routers, err := fetchAllRouters(ctx, client)
		if err != nil {
			return 0, fmt.Errorf("list routers for validation: %w", err)
		}
		for _, rt := range routers {
			if rt.GetId() == id {
				return id, nil
			}
		}
		return 0, fmt.Errorf("router id %d not found on this instance", id)
	}

	// Name match: needs GetRouters.
	routers, err := fetchAllRouters(ctx, client)
	if err != nil {
		return 0, fmt.Errorf("list routers for name resolution: %w", err)
	}
	needle := strings.ToLower(arg)

	// Pass 1: exact case-insensitive name match.
	for _, rt := range routers {
		if strings.ToLower(rt.GetName()) == needle {
			return rt.GetId(), nil
		}
	}

	// Pass 2: substring match.
	var matches []*pb.Router
	for _, rt := range routers {
		if strings.Contains(strings.ToLower(rt.GetName()), needle) {
			matches = append(matches, rt)
		}
	}
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("no router on this instance matches %q (try `lg-cli routers <instance>`)", arg)
	case 1:
		return matches[0].GetId(), nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, fmt.Sprintf("%d:%s", m.GetId(), m.GetName()))
		}
		return 0, fmt.Errorf("router %q is ambiguous; matches %d routers: %s",
			arg, len(matches), strings.Join(names, ", "))
	}
}
