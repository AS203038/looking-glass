package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
)

// newClient builds a ConnectRPC client pointed at lg's URL.
func newClient(lg *LookingGlass) lookingglassconnect.LookingGlassServiceClient {
	verbosef("connecting to %s (%s)", lg.Name, lg.URL)
	return lookingglassconnect.NewLookingGlassServiceClient(http.DefaultClient, lg.URL)
}

// fetchAllRouters pages through GetRouters and returns every router.
func fetchAllRouters(ctx context.Context, client lookingglassconnect.LookingGlassServiceClient, instanceURL string, skipCache bool) ([]*pb.Router, error) {
	cdir := getCacheDir()
	hash := sha256.Sum256([]byte(instanceURL))
	cachePath := filepath.Join(cdir, fmt.Sprintf("routers_%x.json", hash))

	if !opts.Update && !skipCache {
		if st, err := os.Stat(cachePath); err == nil {
			if time.Since(st.ModTime()) < 1*time.Hour { // 1 hour TTL for router list
				data, err := os.ReadFile(cachePath)
				if err == nil {
					var routers []*pb.Router
					if json.Unmarshal(data, &routers) == nil {
						verbosef("loaded routers from cache (%s)", cachePath)
						return routers, nil
					}
				}
			}
		}
	}

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

	if data, err := json.Marshal(all); err == nil {
		if err := os.MkdirAll(cdir, 0755); err == nil {
			_ = os.WriteFile(cachePath, data, 0644)
		}
	}

	return all, nil
}

// resolveRouter turns a user-supplied router argument into a router ID.
// Accepts a positive integer or a case-insensitive substring of the router
// name; exact name matches win over substring matches.
func resolveRouter(
	ctx context.Context,
	client lookingglassconnect.LookingGlassServiceClient,
	instanceURL string,
	arg string,
	requireExists bool,
) (int64, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, fmt.Errorf("router argument is required")
	}

	if id, err := strconv.ParseInt(arg, 10, 64); err == nil && id > 0 {
		if !requireExists {
			return id, nil
		}
		routers, err := fetchAllRouters(ctx, client, instanceURL, false)
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

	routers, err := fetchAllRouters(ctx, client, instanceURL, false)
	if err != nil {
		return 0, fmt.Errorf("list routers for name resolution: %w", err)
	}
	needle := strings.ToLower(arg)
	for _, rt := range routers {
		if strings.ToLower(rt.GetName()) == needle {
			return rt.GetId(), nil
		}
	}
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
