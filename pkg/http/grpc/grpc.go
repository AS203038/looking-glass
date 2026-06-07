// Package grpc wires the LookingGlassService into a [http.ServeMux]
// and drives the background router health-check loop.
package grpc

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/grpchealth"
	"github.com/AS203038/looking-glass/pkg/bmp"
	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
)

// Health is the process-wide [grpchealth.Checker].
var Health = grpchealth.NewStaticChecker(lookingglassconnect.LookingGlassServiceName)

var healthcheckInterval = time.Minute

// Mux mounts the LookingGlassService and the gRPC health endpoint onto
// mux, marks the service as Serving, and launches the [healthcheck] loop.
func Mux(ctx context.Context, mux *http.ServeMux, rts utils.RouterMap) {
	mux.Handle(lookingglassconnect.NewLookingGlassServiceHandler(NewLookingGlassService(ctx, rts)))
	mux.Handle(grpchealth.NewHandler(Health))
	Health.SetStatus(lookingglassconnect.LookingGlassServiceName, grpchealth.StatusServing)
	go healthcheck(ctx, rts)
}

// healthcheck runs one [probeRouterOnce] sweep per minute over rts
// and returns when ctx is cancelled.
func healthcheck(ctx context.Context, rts utils.RouterMap) {
	log := logging.Component("health")
	if healthRedis != nil {
		log.Info("healthcheck coordinated via redis",
			slog.String("node", nodeID),
			slog.Duration("lease_ttl", healthLeaseTTL),
			slog.Duration("state_ttl", healthStateTTL))
	} else {
		log.Info("healthcheck uncoordinated; every replica probes every router")
	}
	ticker := time.NewTicker(healthcheckInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Debug("healthcheck tick", slog.Int("routers", len(rts)))
			for _, r := range rts {
				probeRouterOnce(ctx, log, r)
			}
		}
	}
}

// probeLocal performs a single local probe on r. It returns whether the probe succeeded (is healthy) and any associated error.
func probeLocal(ctx context.Context, r *utils.RouterInstance) (bool, error) {
	r.HealthCheck.Checked = time.Now()
	if r.Config.HasSSHCredentials() {
		probeErr := r.Healthcheck()
		healthy := probeErr == nil
		if !healthy && bmp.IsBMPActive(ctx, healthRedis, r.Config.Name) {
			r.HealthCheck.Healthy = true
			return true, nil
		}
		// r.HealthCheck.Healthy is already set by r.Healthcheck()
		return healthy, probeErr
	}

	// No SSH credentials configured. We rely solely on BMP activity.
	if bmp.IsBMPActive(ctx, healthRedis, r.Config.Name) {
		r.HealthCheck.Healthy = true
		return true, nil
	}
	r.HealthCheck.Healthy = false
	return false, errors.New("no SSH credentials configured and BMP is inactive")
}

// probeRouterOnce runs one health sweep for r. If [healthRedis] is
// configured the probe is gated by [tryAcquireHealthLease]: the lease
// holder probes and publishes via [writeHealthState], everyone else
// reads via [readHealthState]. Without [healthRedis] r is probed
// locally. The resulting Healthy bit is applied to r's
// [utils.HealthCheck] record and the gRPC health checker.
func probeRouterOnce(ctx context.Context, log *slog.Logger, r *utils.RouterInstance) {
	prev := r.HealthCheck.Healthy

	leased, err := tryAcquireHealthLease(ctx, r.Config.Name)
	if err != nil {
		log.Warn("healthcheck lease acquire failed; probing locally",
			slog.String("router", r.Config.Name),
			slog.Any("err", err))
	}

	if healthRedis == nil || leased {
		probeStart := time.Now()
		healthy, probeErr := probeLocal(ctx, r)
		probeDur := time.Since(probeStart)
		if healthy {
			log.Debug("healthcheck probe ok",
				slog.String("router", r.Config.Name),
				slog.Duration("duration", probeDur))
		} else {
			log.Debug("healthcheck probe failed",
				slog.String("router", r.Config.Name),
				slog.Duration("duration", probeDur),
				slog.Any("err", probeErr))
		}
		if werr := writeHealthState(ctx, r.Config.Name, healthState{
			Healthy: healthy,
			Checked: time.Now(),
			Source:  nodeID,
		}); werr != nil {
			log.Warn("healthcheck state publish failed",
				slog.String("router", r.Config.Name),
				slog.Any("err", werr))
		}
		applyHealthTransition(log, r, prev, healthy, probeErr, "local")
		return
	}

	state, ok, err := readHealthState(ctx, r.Config.Name)
	if err != nil {
		log.Warn("healthcheck state read failed; probing locally",
			slog.String("router", r.Config.Name),
			slog.Any("err", err))
		probeStart := time.Now()
		healthy, probeErr := probeLocal(ctx, r)
		probeDur := time.Since(probeStart)
		log.Debug("healthcheck fallback probe complete",
			slog.String("router", r.Config.Name),
			slog.Duration("duration", probeDur),
			slog.Bool("healthy", healthy))
		applyHealthTransition(log, r, prev, healthy, probeErr, "fallback")
		return
	}
	if !ok {
		log.Debug("healthcheck state not yet published; skipping",
			slog.String("router", r.Config.Name))
		return
	}
	log.Debug("healthcheck adopted peer state",
		slog.String("router", r.Config.Name),
		slog.String("peer", state.Source),
		slog.Bool("healthy", state.Healthy),
		slog.Time("checked", state.Checked))
	r.HealthCheck.Healthy = state.Healthy
	r.HealthCheck.Checked = state.Checked
	applyHealthTransition(log, r, prev, state.Healthy, nil, "peer:"+state.Source)
}

// applyHealthTransition flips the gRPC health status for r when its
// healthy bit changed between prev and now, emitting source and err
// as context on the transition log line.
func applyHealthTransition(log *slog.Logger, r *utils.RouterInstance, prev, now bool, err error, source string) {
	if prev == now {
		return
	}
	if now {
		Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusServing)
		log.Info("router healthy",
			slog.String("router", r.Config.Name),
			slog.String("source", source))
		return
	}
	Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusNotServing)
	log.Warn("router unhealthy",
		slog.String("router", r.Config.Name),
		slog.String("source", source),
		slog.Any("err", err))
}
