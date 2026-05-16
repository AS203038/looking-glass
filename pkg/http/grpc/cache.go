package grpc

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"log/slog"
	"strconv"
	"time"

	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

// rpcCacheClient is the optional shared response cache; nil disables caching.
var rpcCacheClient *redis.Client

// rpcCacheTTL is the lifetime of every cached RPC response.
var rpcCacheTTL time.Duration

// rpcCacheLog is the slog logger used for cache get/set diagnostics.
var rpcCacheLog *slog.Logger

// rpcCacheKeyPrefix is the namespace prefix shared by every cached key.
// utils.Version() is folded in so a deploy invalidates every entry.
var rpcCacheKeyPrefix = func() string {
	return "lg:rpc:" + utils.Version() + ":"
}

// SetRPCCache registers c as the shared RPC response cache and ttl
// as the per-entry lifetime. A nil c disables caching. Must be called
// before [Mux].
func SetRPCCache(c *redis.Client, ttl time.Duration) {
	rpcCacheClient = c
	rpcCacheTTL = ttl
	rpcCacheLog = logging.Component("rpccache")
}

// rpcCacheGet fetches and unmarshals the cached protobuf message
// stored under key into dst. Returns true on a cache hit, false on
// miss, on a disabled cache, or on any Redis or unmarshal failure
// (all failures are logged at debug and treated as a miss).
func rpcCacheGet(ctx context.Context, key string, dst proto.Message) bool {
	if rpcCacheClient == nil {
		return false
	}
	raw, err := rpcCacheClient.Get(ctx, key).Bytes()
	if err == redis.Nil {
		rpcCacheLog.Debug("cache miss", slog.String("key", key))
		return false
	}
	if err != nil {
		rpcCacheLog.Debug("cache get failed",
			slog.String("key", key),
			slog.Any("err", err))
		return false
	}
	if err := proto.Unmarshal(raw, dst); err != nil {
		rpcCacheLog.Debug("cache entry unmarshal failed",
			slog.String("key", key),
			slog.Any("err", err))
		return false
	}
	rpcCacheLog.Debug("cache hit",
		slog.String("key", key),
		slog.Int("bytes", len(raw)))
	return true
}

// rpcCacheSet marshals msg and writes it under key with the
// configured TTL. Fire-and-forget; errors are logged at debug. A
// disabled cache is a no-op.
func rpcCacheSet(ctx context.Context, key string, msg proto.Message) {
	if rpcCacheClient == nil {
		return
	}
	buf, err := proto.Marshal(msg)
	if err != nil {
		rpcCacheLog.Debug("cache marshal failed",
			slog.String("key", key),
			slog.Any("err", err))
		return
	}
	if err := rpcCacheClient.Set(ctx, key, buf, rpcCacheTTL).Err(); err != nil {
		rpcCacheLog.Debug("cache set failed",
			slog.String("key", key),
			slog.Any("err", err))
		return
	}
	rpcCacheLog.Debug("cache store ok",
		slog.String("key", key),
		slog.Int("bytes", len(buf)),
		slog.Duration("ttl", rpcCacheTTL))
}

// rpcCacheHash returns a stable hex digest of s, used to bound the
// length of cache keys whose input is operator-supplied (e.g. an
// arbitrary AS-path regex).
func rpcCacheHash(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// pingCacheKey builds the cache key for [LookingGlassService.Ping].
func pingCacheKey(routerID int64, target string) string {
	return rpcCacheKeyPrefix() + "ping:" + strconv.FormatInt(routerID, 10) + ":" + target
}

// tracerouteCacheKey builds the cache key for [LookingGlassService.Traceroute].
func tracerouteCacheKey(routerID int64, target string) string {
	return rpcCacheKeyPrefix() + "traceroute:" + strconv.FormatInt(routerID, 10) + ":" + target
}

// bgpSummaryCacheKey builds the cache key for [LookingGlassService.BGPSummary].
func bgpSummaryCacheKey(routerID int64) string {
	return rpcCacheKeyPrefix() + "bgpsummary:" + strconv.FormatInt(routerID, 10)
}

// bgpRouteCacheKey builds the cache key for [LookingGlassService.BGPRoute].
func bgpRouteCacheKey(routerID int64, target string) string {
	return rpcCacheKeyPrefix() + "bgproute:" + strconv.FormatInt(routerID, 10) + ":" + target
}

// bgpCommunityCacheKey builds the cache key for [LookingGlassService.BGPCommunity].
func bgpCommunityCacheKey(routerID int64, community string) string {
	return rpcCacheKeyPrefix() + "bgpcommunity:" + strconv.FormatInt(routerID, 10) + ":" + community
}

// bgpLargeCommunityCacheKey builds the cache key for [LookingGlassService.BGPLargeCommunity].
func bgpLargeCommunityCacheKey(routerID int64, community string) string {
	return rpcCacheKeyPrefix() + "bgplargecommunity:" + strconv.FormatInt(routerID, 10) + ":" + community
}

// bgpASPathCacheKey builds the cache key for [LookingGlassService.BGPASPath].
// The pattern is hashed to keep keys bounded.
func bgpASPathCacheKey(routerID int64, pattern string) string {
	return rpcCacheKeyPrefix() + "bgpaspath:" + strconv.FormatInt(routerID, 10) + ":" + rpcCacheHash(pattern)
}
