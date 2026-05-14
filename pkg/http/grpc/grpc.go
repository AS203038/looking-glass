// Package grpc wires the LookingGlassService implementation into a
// [http.ServeMux] and drives the background router health-check
// loop that backs both the per-service [grpchealth] status and the
// WebUI's "router is up" badges.
package grpc

import (
	"context"
	"log"
	"net/http"
	"time"

	"connectrpc.com/grpchealth"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
)

// Health is the process-wide [grpchealth.Checker] used by the
// standard gRPC health protocol. The top-level service status is
// initialised to Serving in [Mux]; per-router sub-statuses are
// updated by the background [healthcheck] loop.
var Health = grpchealth.NewStaticChecker(lookingglassconnect.LookingGlassServiceName)

// Mux mounts the LookingGlassService and the gRPC health endpoint
// onto the supplied mux, marks the service as Serving, and launches
// the per-router health-check goroutine.
//
// The supplied ctx controls the lifetime of the health-check
// goroutine: cancelling it stops the ticker and returns. ctx is
// not propagated into RPC handlers (each handler receives its own
// per-request context from ConnectRPC).
func Mux(ctx context.Context, mux *http.ServeMux, rts utils.RouterMap) {
	mux.Handle(lookingglassconnect.NewLookingGlassServiceHandler(NewLookingGlassService(ctx, rts)))
	mux.Handle(grpchealth.NewHandler(Health))
	Health.SetStatus(lookingglassconnect.LookingGlassServiceName, grpchealth.StatusServing)
	go healthcheck(ctx, rts)
}

// healthcheck periodically probes every configured router and
// updates its gRPC health status when the reachable/unreachable
// state changes.
//
// Cadence: one full sweep per minute. State changes are logged at
// NOTICE / WARNING level so operators get an audible signal when a
// router flaps; unchanged states are silent to keep logs bounded.
//
// The function returns when ctx is cancelled and otherwise loops
// forever; it is intended to be launched as a goroutine from [Mux].
func healthcheck(ctx context.Context, rts utils.RouterMap) {
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, r := range rts {
				o := r.HealthCheck.Healthy
				if err := r.Healthcheck(); err == nil {
					if !o {
						Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusServing)
						log.Printf("NOTICE: Router %s is healthy", r.Config.Name)
					}
				} else {
					if o {
						Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusNotServing)
						log.Printf("WARNING: Router %s is unhealthy: %s", r.Config.Name, err)
					}
				}
			}
		}
	}
}
