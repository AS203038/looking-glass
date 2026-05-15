// Package grpc wires the LookingGlassService into a [http.ServeMux]
// and drives the background router health-check loop.
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
