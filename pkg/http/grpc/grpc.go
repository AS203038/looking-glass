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

var Health = grpchealth.NewStaticChecker(lookingglassconnect.LookingGlassServiceName)

func Mux(ctx context.Context, mux *http.ServeMux, rts utils.RouterMap) {
	mux.Handle(lookingglassconnect.NewLookingGlassServiceHandler(NewLookingGlassService(ctx, rts)))
	mux.Handle(grpchealth.NewHandler(Health))
	Health.SetStatus(lookingglassconnect.LookingGlassServiceName, grpchealth.StatusServing)
	go healthcheck(ctx, rts)
}

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
