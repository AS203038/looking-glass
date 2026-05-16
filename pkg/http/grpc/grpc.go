// Package grpc wires the LookingGlassService into a [http.ServeMux]
// and drives the background router health-check loop.
package grpc

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/grpchealth"
	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
)

// Health is the process-wide [grpchealth.Checker].
var Health = grpchealth.NewStaticChecker(lookingglassconnect.LookingGlassServiceName)

// Mux mounts the LookingGlassService and the gRPC health endpoint onto
// mux, marks the service as Serving, and launches the [healthcheck] loop.
func Mux(ctx context.Context, mux *http.ServeMux, rts utils.RouterMap) {
	mux.Handle(lookingglassconnect.NewLookingGlassServiceHandler(NewLookingGlassService(ctx, rts)))
	mux.Handle(grpchealth.NewHandler(Health))
	Health.SetStatus(lookingglassconnect.LookingGlassServiceName, grpchealth.StatusServing)
	go healthcheck(ctx, rts)
}

// healthcheck periodically probes every configured router (one sweep
// per minute) and updates its gRPC health status on state changes.
// The function returns when ctx is cancelled.
func healthcheck(ctx context.Context, rts utils.RouterMap) {
	log := logging.Component("health")
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Debug("healthcheck tick", slog.Int("routers", len(rts)))
			for _, r := range rts {
				o := r.HealthCheck.Healthy
				probeStart := time.Now()
				err := r.Healthcheck()
				probeDur := time.Since(probeStart)
				if err == nil {
					log.Debug("healthcheck probe ok",
						slog.String("router", r.Config.Name),
						slog.Duration("duration", probeDur))
					if !o {
						Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusServing)
						log.Info("router healthy", slog.String("router", r.Config.Name))
					}
				} else {
					log.Debug("healthcheck probe failed",
						slog.String("router", r.Config.Name),
						slog.Duration("duration", probeDur),
						slog.Any("err", err))
					if o {
						Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusNotServing)
						log.Warn("router unhealthy",
							slog.String("router", r.Config.Name),
							slog.Any("err", err))
					}
				}
			}
		}
	}
}
