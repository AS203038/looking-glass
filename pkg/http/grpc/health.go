package grpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// healthLeaseTTL is the lifetime of a per-router probe-lease key.
const healthLeaseTTL = 70 * time.Second

// healthStateTTL is the lifetime of a published per-router probe-outcome key.
const healthStateTTL = 5 * time.Minute

// healthRedis is the optional shared coordination store; nil disables coordination.
var healthRedis *redis.Client

// nodeID identifies this process to peers via the coordination store.
var nodeID = func() string {
	host, _ := os.Hostname()
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	return fmt.Sprintf("%s-%s", host, hex.EncodeToString(rnd[:]))
}()

// healthState is the JSON shape stored under [healthStateKey].
type healthState struct {
	Healthy bool      `json:"healthy"`
	Checked time.Time `json:"checked"`
	Source  string    `json:"source"`
}

// SetRedis registers c as the shared coordination store for the
// background health-check loop. A nil c disables coordination. Must
// be called before [Mux].
func SetRedis(c *redis.Client) {
	healthRedis = c
}

// healthLeaseKey returns the Redis key reserving the next probe slot for router.
func healthLeaseKey(router string) string {
	return "lg:health:lease:" + router
}

// healthStateKey returns the Redis key storing the last probe outcome for router.
func healthStateKey(router string) string {
	return "lg:health:state:" + router
}

// tryAcquireHealthLease attempts to claim the probe slot for router.
// Returns (true, nil) when this replica holds the lease for the next
// [healthLeaseTTL]; (false, nil) when another replica holds it or
// coordination is disabled; (false, err) on Redis failure.
func tryAcquireHealthLease(ctx context.Context, router string) (bool, error) {
	if healthRedis == nil {
		return false, nil
	}
	return healthRedis.SetNX(ctx, healthLeaseKey(router), nodeID, healthLeaseTTL).Result()
}

// readHealthState returns the most recent probe outcome for router.
// The bool is false when no outcome has been published or when
// coordination is disabled.
func readHealthState(ctx context.Context, router string) (healthState, bool, error) {
	var s healthState
	if healthRedis == nil {
		return s, false, nil
	}
	raw, err := healthRedis.Get(ctx, healthStateKey(router)).Result()
	if err == redis.Nil {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return s, false, err
	}
	return s, true, nil
}

// writeHealthState publishes the probe outcome for router. A nil
// [healthRedis] is a no-op.
func writeHealthState(ctx context.Context, router string, s healthState) error {
	if healthRedis == nil {
		return nil
	}
	buf, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return healthRedis.Set(ctx, healthStateKey(router), buf, healthStateTTL).Err()
}
