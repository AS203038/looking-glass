package bmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisBMPPeersPrefix               = "lg:bmp:peers:"
	redisBMPRibPrefix                 = "lg:bmp:rib:"
	redisBMPIndexASPrefix             = "lg:bmp:index:as:"
	redisBMPIndexCommunityPrefix      = "lg:bmp:index:community:"
	redisBMPIndexLargeCommunityPrefix = "lg:bmp:index:large_community:"
)

// PeerInfo records BGP peer session status retrieved via BMP.
type PeerInfo struct {
	IP               string `json:"peer_ip"`
	ASN              uint32 `json:"peer_asn"`
	State            string `json:"state"`
	UptimeSeconds    uint64 `json:"uptime_seconds"`
	BGPID            string `json:"bgp_id"`
	Timestamp        int64  `json:"timestamp"`
	PrefixesReceived uint64 `json:"prefixes_received"`
	PrefixesAccepted uint64 `json:"prefixes_accepted"`
}

// RouteInfo holds the BGP path attributes for a prefix.
type RouteInfo struct {
	Prefix           string   `json:"prefix"`
	ASPath           []uint32 `json:"as_path"`
	NextHop          string   `json:"next_hop"`
	Communities      []string `json:"communities"`
	LargeCommunities []string `json:"large_communities"`
	Timestamp        int64    `json:"timestamp"`
	Accepted         bool     `json:"accepted"`
	Bestpath         bool     `json:"bestpath"`
	Rejected         bool     `json:"rejected"`
}

// UpdateBMPPeerStatus writes/updates a peer's connectivity status in Redis.
func UpdateBMPPeerStatus(ctx context.Context, rdb *redis.Client, router string, ip string, asn uint32, state string, bgpID string) error {
	if rdb == nil {
		return nil
	}
	key := redisBMPPeersPrefix + router

	var peer PeerInfo
	val, err := rdb.HGet(ctx, key, ip).Result()
	if err == nil {
		_ = json.Unmarshal([]byte(val), &peer)
	}

	peer.IP = ip
	peer.ASN = asn
	peer.State = state
	peer.BGPID = bgpID
	peer.Timestamp = time.Now().Unix()

	if state == "established" {
		cleanPeerIndexes(ctx, rdb, router, ip)
		_ = rdb.Del(ctx, redisBMPRibPrefix+router+":"+ip)

		if peer.UptimeSeconds == 0 {
			peer.UptimeSeconds = 1
		}
	} else {
		peer.UptimeSeconds = 0
		peer.PrefixesReceived = 0
		peer.PrefixesAccepted = 0
		cleanPeerIndexes(ctx, rdb, router, ip)
		_ = rdb.Del(ctx, redisBMPRibPrefix+router+":"+ip)
	}

	raw, _ := json.Marshal(peer)
	return rdb.HSet(ctx, key, ip, string(raw)).Err()
}

// UpdateBMPRib processes BGP updates (announcements and withdrawals) in Redis.
func UpdateBMPRib(ctx context.Context, rdb *redis.Client, router string, peerIP string, update *BGPUpdate) error {
	if rdb == nil {
		return nil
	}
	ribKey := redisBMPRibPrefix + router + ":" + peerIP
	peersKey := redisBMPPeersPrefix + router

	now := time.Now().Unix()

	allPrefixes := append(append([]string{}, update.WithdrawnPrefixes...), update.AnnouncedPrefixes...)

	existingRoutes := make(map[string]RouteInfo)
	if len(allPrefixes) > 0 {
		vals, err := rdb.HMGet(ctx, ribKey, allPrefixes...).Result()
		if err == nil {
			for i, val := range vals {
				if val != nil {
					if valStr, ok := val.(string); ok && valStr != "" {
						var ri RouteInfo
						if err := json.Unmarshal([]byte(valStr), &ri); err == nil {
							existingRoutes[allPrefixes[i]] = ri
						}
					}
				}
			}
		}
	}

	if len(update.WithdrawnPrefixes) > 0 {
		pipe := rdb.Pipeline()
		fields := make([]string, len(update.WithdrawnPrefixes))
		for i, prefix := range update.WithdrawnPrefixes {
			fields[i] = prefix
			if ri, exists := existingRoutes[prefix]; exists {
				deleteRouteIndex(ctx, pipe, router, peerIP, prefix, ri)
			}
		}
		pipe.HDel(ctx, ribKey, fields...)
		_, _ = pipe.Exec(ctx)
	}

	if len(update.AnnouncedPrefixes) > 0 {
		pipe := rdb.Pipeline()
		for _, prefix := range update.AnnouncedPrefixes {
			if oldRi, exists := existingRoutes[prefix]; exists {
				deleteRouteIndex(ctx, pipe, router, peerIP, prefix, oldRi)
			}

			ri := RouteInfo{
				Prefix:           prefix,
				ASPath:           update.ASPath,
				NextHop:          update.NextHop,
				Communities:      update.Communities,
				LargeCommunities: update.LargeCommunities,
				Timestamp:        now,
				Accepted:         true,
				Bestpath:         true,
				Rejected:         false,
			}
			raw, _ := json.Marshal(ri)
			pipe.HSet(ctx, ribKey, prefix, string(raw))

			addRouteIndex(ctx, pipe, router, peerIP, prefix, ri)
		}
		_, _ = pipe.Exec(ctx)
	}

	count, err := rdb.HLen(ctx, ribKey).Result()
	if err == nil {
		var peer PeerInfo
		val, err := rdb.HGet(ctx, peersKey, peerIP).Result()
		if err == nil {
			_ = json.Unmarshal([]byte(val), &peer)
			peer.PrefixesReceived = uint64(count)
			peer.PrefixesAccepted = uint64(count)
			raw, _ := json.Marshal(peer)
			_ = rdb.HSet(ctx, peersKey, peerIP, string(raw)).Err()
		}
	}

	return nil
}

// GetBMPPeers retrieves all BGP peers for a given router from Redis.
func GetBMPPeers(ctx context.Context, rdb *redis.Client, router string) ([]PeerInfo, error) {
	if rdb == nil {
		return nil, nil
	}
	key := redisBMPPeersPrefix + router
	res, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var peers []PeerInfo
	for _, val := range res {
		var peer PeerInfo
		if err := json.Unmarshal([]byte(val), &peer); err == nil {
			peers = append(peers, peer)
		}
	}
	return peers, nil
}

// GetBMPPeerRoutes retrieves RIB routes for a specific peer.
func GetBMPPeerRoutes(ctx context.Context, rdb *redis.Client, router string, peerIP string, queryType string) ([]RouteInfo, error) {
	if rdb == nil {
		return nil, nil
	}
	ribKey := redisBMPRibPrefix + router + ":" + peerIP
	res, err := rdb.HGetAll(ctx, ribKey).Result()
	if err != nil {
		return nil, err
	}
	var routes []RouteInfo
	for _, val := range res {
		var r RouteInfo
		if err := json.Unmarshal([]byte(val), &r); err == nil {
			if queryType == "rejected" && !r.Rejected {
				continue
			}
			if queryType == "accepted" && !r.Accepted {
				continue
			}
			if queryType == "bestpath" && !r.Bestpath {
				continue
			}
			routes = append(routes, r)
		}
	}
	return routes, nil
}

// GetBMPPeerRoutesPaginated retrieves a cursor-paginated chunk of routes received from a peer, matching the requested queryType.
func GetBMPPeerRoutesPaginated(ctx context.Context, rdb *redis.Client, router string, peerIP string, queryType string, cursor uint64, limit int64) ([]RouteInfo, uint64, error) {
	if rdb == nil {
		return nil, 0, nil
	}
	ribKey := redisBMPRibPrefix + router + ":" + peerIP

	if limit <= 0 {
		limit = 2000
	}
	if limit > 50000 {
		limit = 50000
	}

	keys, nextCursor, err := rdb.HScan(ctx, ribKey, cursor, "*", limit).Result()
	if err != nil {
		return nil, 0, err
	}

	var routes []RouteInfo
	for i := 0; i < len(keys); i += 2 {
		if i+1 >= len(keys) {
			break
		}
		valStr := keys[i+1]
		var r RouteInfo
		if err := json.Unmarshal([]byte(valStr), &r); err == nil {
			if queryType == "rejected" && !r.Rejected {
				continue
			}
			if queryType == "accepted" && !r.Accepted {
				continue
			}
			if queryType == "bestpath" && !r.Bestpath {
				continue
			}
			routes = append(routes, r)
		}
	}

	return routes, nextCursor, nil
}

// FindBMPRoutesByPrefix searches all peer RIBs on a router to locate a specific destination prefix.
func FindBMPRoutesByPrefix(ctx context.Context, rdb *redis.Client, router string, targetPrefix string) ([]RouteInfo, error) {
	if rdb == nil {
		return nil, nil
	}
	peers, err := GetBMPPeers(ctx, rdb, router)
	if err != nil {
		return nil, err
	}

	var routes []RouteInfo

	if strings.Contains(targetPrefix, "/") {
		for _, p := range peers {
			ribKey := redisBMPRibPrefix + router + ":" + p.IP
			val, err := rdb.HGet(ctx, ribKey, targetPrefix).Result()
			if err == nil {
				var r RouteInfo
				if err := json.Unmarshal([]byte(val), &r); err == nil {
					routes = append(routes, r)
				}
			}
		}
	}

	if len(routes) == 0 {
		candidates := generateIPPrefixes(targetPrefix)
		if len(candidates) > 0 {
			for _, p := range peers {
				ribKey := redisBMPRibPrefix + router + ":" + p.IP
				res, err := rdb.HMGet(ctx, ribKey, candidates...).Result()
				if err == nil {
					for _, val := range res {
						if val != nil {
							if valStr, ok := val.(string); ok && valStr != "" {
								var r RouteInfo
								if err := json.Unmarshal([]byte(valStr), &r); err == nil {
									routes = append(routes, r)
									break
								}
							}
						}
					}
				}
			}
		}
	}

	return routes, nil
}

// generateIPPrefixes generates the possible enclosing CIDR candidates for a given IP address.
func generateIPPrefixes(target string) []string {
	ip := net.ParseIP(target)
	if ip == nil {
		if targetIP, _, err := net.ParseCIDR(target); err == nil {
			ip = targetIP
		} else {
			return nil
		}
	}

	var prefixes []string
	if ip.To4() != nil {
		ipv4 := ip.To4()
		for maskLen := 32; maskLen >= 0; maskLen-- {
			mask := net.CIDRMask(maskLen, 32)
			netIP := ipv4.Mask(mask)
			prefixes = append(prefixes, fmt.Sprintf("%s/%d", netIP.String(), maskLen))
		}
	} else {
		for maskLen := 128; maskLen >= 0; maskLen-- {
			mask := net.CIDRMask(maskLen, 128)
			netIP := ip.Mask(mask)
			prefixes = append(prefixes, fmt.Sprintf("%s/%d", netIP.String(), maskLen))
		}
	}
	return prefixes
}

// IsBMPActive reports whether a router has any active BMP peer telemetry in Redis.
func IsBMPActive(ctx context.Context, rdb *redis.Client, router string) bool {
	if rdb == nil {
		return false
	}
	key := redisBMPPeersPrefix + router
	count, err := rdb.HLen(ctx, key).Result()
	return err == nil && count > 0
}

// ipContains reports whether cidr prefix contains target ip/prefix.
func ipContains(cidr string, target string) bool {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(target)
	if ip == nil {
		targetIP, _, err := net.ParseCIDR(target)
		if err == nil {
			ip = targetIP
		}
	}
	if ip != nil {
		return ipnet.Contains(ip)
	}
	return strings.HasPrefix(cidr, target) || strings.HasPrefix(target, cidr)
}

// cleanPeerIndexes removes all index entries associated with a BGP peer.
func cleanPeerIndexes(ctx context.Context, rdb *redis.Client, router string, ip string) {
	ribKey := redisBMPRibPrefix + router + ":" + ip
	cursor := uint64(0)
	for {
		keys, nextCursor, err := rdb.HScan(ctx, ribKey, cursor, "*", 1000).Result()
		if err != nil {
			break
		}
		pipe := rdb.Pipeline()
		for i := 0; i < len(keys); i += 2 {
			if i+1 >= len(keys) {
				break
			}
			var r RouteInfo
			if err := json.Unmarshal([]byte(keys[i+1]), &r); err == nil {
				member := fmt.Sprintf("%s:%s", ip, r.Prefix)
				for _, asn := range r.ASPath {
					pipe.SRem(ctx, fmt.Sprintf("%s%s:%d", redisBMPIndexASPrefix, router, asn), member)
				}
				for _, comm := range r.Communities {
					pipe.SRem(ctx, fmt.Sprintf("%s%s:%s", redisBMPIndexCommunityPrefix, router, comm), member)
				}
				for _, lc := range r.LargeCommunities {
					pipe.SRem(ctx, fmt.Sprintf("%s%s:%s", redisBMPIndexLargeCommunityPrefix, router, lc), member)
				}
			}
		}
		_, _ = pipe.Exec(ctx)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// deleteRouteIndex removes index entries for a single prefix from Redis Sets.
func deleteRouteIndex(ctx context.Context, pipe redis.Pipeliner, router string, peerIP string, prefix string, ri RouteInfo) {
	member := fmt.Sprintf("%s:%s", peerIP, prefix)
	for _, asn := range ri.ASPath {
		pipe.SRem(ctx, fmt.Sprintf("%s%s:%d", redisBMPIndexASPrefix, router, asn), member)
	}
	for _, comm := range ri.Communities {
		pipe.SRem(ctx, fmt.Sprintf("%s%s:%s", redisBMPIndexCommunityPrefix, router, comm), member)
	}
	for _, lc := range ri.LargeCommunities {
		pipe.SRem(ctx, fmt.Sprintf("%s%s:%s", redisBMPIndexLargeCommunityPrefix, router, lc), member)
	}
}

// addRouteIndex adds index entries for a single prefix to Redis Sets.
func addRouteIndex(ctx context.Context, pipe redis.Pipeliner, router string, peerIP string, prefix string, ri RouteInfo) {
	member := fmt.Sprintf("%s:%s", peerIP, prefix)
	for _, asn := range ri.ASPath {
		pipe.SAdd(ctx, fmt.Sprintf("%s%s:%d", redisBMPIndexASPrefix, router, asn), member)
	}
	for _, comm := range ri.Communities {
		pipe.SAdd(ctx, fmt.Sprintf("%s%s:%s", redisBMPIndexCommunityPrefix, router, comm), member)
	}
	for _, lc := range ri.LargeCommunities {
		pipe.SAdd(ctx, fmt.Sprintf("%s%s:%s", redisBMPIndexLargeCommunityPrefix, router, lc), member)
	}
}

// FindBMPRoutesByCommunity returns all routes tagged with the supplied community across all peers on a router.
func FindBMPRoutesByCommunity(ctx context.Context, rdb *redis.Client, router string, community string) ([]RouteInfo, error) {
	if rdb == nil {
		return nil, nil
	}
	key := fmt.Sprintf("%s%s:%s", redisBMPIndexCommunityPrefix, router, community)
	members, err := rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return fetchRoutesFromIndexMembers(ctx, rdb, router, members)
}

// FindBMPRoutesByLargeCommunity returns all routes tagged with the supplied large community across all peers on a router.
func FindBMPRoutesByLargeCommunity(ctx context.Context, rdb *redis.Client, router string, largeCommunity string) ([]RouteInfo, error) {
	if rdb == nil {
		return nil, nil
	}
	key := fmt.Sprintf("%s%s:%s", redisBMPIndexLargeCommunityPrefix, router, largeCommunity)
	members, err := rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return fetchRoutesFromIndexMembers(ctx, rdb, router, members)
}

// FindBMPRoutesByASPath returns all routes matching the ASPath regex across all peers on a router.
func FindBMPRoutesByASPath(ctx context.Context, rdb *redis.Client, router string, pattern string) ([]RouteInfo, error) {
	if rdb == nil {
		return nil, nil
	}
	asns := extractASNsFromPattern(pattern)
	var members []string
	var err error
	if len(asns) > 0 {
		var keys []string
		for _, asn := range asns {
			keys = append(keys, fmt.Sprintf("%s%s:%d", redisBMPIndexASPrefix, router, asn))
		}
		if len(keys) == 1 {
			members, err = rdb.SMembers(ctx, keys[0]).Result()
		} else {
			members, err = rdb.SInter(ctx, keys...).Result()
		}
		if err != nil {
			return nil, err
		}
	} else {
		peers, err := GetBMPPeers(ctx, rdb, router)
		if err != nil {
			return nil, err
		}
		var allRoutes []RouteInfo
		for _, p := range peers {
			routes, err := GetBMPPeerRoutes(ctx, rdb, router, p.IP, "accepted")
			if err == nil {
				allRoutes = append(allRoutes, routes...)
			}
		}
		return matchASPathRegex(pattern, allRoutes)
	}

	routes, err := fetchRoutesFromIndexMembers(ctx, rdb, router, members)
	if err != nil {
		return nil, err
	}
	return matchASPathRegex(pattern, routes)
}

var asnRegex = regexp.MustCompile(`[0-9]+`)

func extractASNsFromPattern(pattern string) []uint32 {
	matches := asnRegex.FindAllString(pattern, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[uint32]bool)
	var asns []uint32
	for _, m := range matches {
		var asn uint32
		if _, err := fmt.Sscanf(m, "%d", &asn); err == nil {
			if !seen[asn] {
				seen[asn] = true
				asns = append(asns, asn)
			}
		}
	}
	return asns
}

func matchASPathRegex(pattern string, routes []RouteInfo) ([]RouteInfo, error) {
	translated := strings.ReplaceAll(pattern, "_", `(?:^|\s|$)`)
	re, err := regexp.Compile(translated)
	if err != nil {
		return nil, err
	}

	var matched []RouteInfo
	for _, r := range routes {
		var pathParts []string
		for _, asn := range r.ASPath {
			pathParts = append(pathParts, fmt.Sprintf("%d", asn))
		}
		pathStr := strings.Join(pathParts, " ")
		if re.MatchString(pathStr) {
			matched = append(matched, r)
		}
	}
	return matched, nil
}

// fetchRoutesFromIndexMembers retrieves RouteInfo objects from the peer RIB hashes given a list of "peerIP:prefix" members.
func fetchRoutesFromIndexMembers(ctx context.Context, rdb *redis.Client, router string, members []string) ([]RouteInfo, error) {
	if len(members) == 0 {
		return nil, nil
	}

	byPeer := make(map[string][]string)
	for _, m := range members {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			peerIP := parts[0]
			prefix := parts[1]
			byPeer[peerIP] = append(byPeer[peerIP], prefix)
		}
	}

	var routes []RouteInfo
	for peerIP, prefixes := range byPeer {
		ribKey := redisBMPRibPrefix + router + ":" + peerIP
		vals, err := rdb.HMGet(ctx, ribKey, prefixes...).Result()
		if err != nil {
			continue
		}
		for _, val := range vals {
			if val != nil {
				if valStr, ok := val.(string); ok && valStr != "" {
					var r RouteInfo
					if err := json.Unmarshal([]byte(valStr), &r); err == nil {
						routes = append(routes, r)
					}
				}
			}
		}
	}
	return routes, nil
}
