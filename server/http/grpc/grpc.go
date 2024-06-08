package grpc

import (
	"context"
	"log"
	"net/http"
	"time"
	"unsafe"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"gitlab.as203038.net/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
	"gitlab.as203038.net/AS203038/looking-glass/server/utils"
)

var Health = grpchealth.NewStaticChecker(lookingglassconnect.LookingGlassServiceName)

func NewLogInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			start := time.Now()

			// Call the original handler
			r, e := next(ctx, req)

			// Log the request
			n := 200
			if e != nil {
				n = 500
			}
			p := req.Header().Get("X-Forwarded-For")
			if p == "" {
				p = req.Peer().Addr
			}
			log.Printf("%s \"%s %s %s\" %d %d \"%s\" \"%s\" %s",
				p,
				req.HTTPMethod(),
				req.Spec().Procedure,
				req.Peer().Protocol,
				n,
				unsafe.Sizeof(req.Any()),
				req.Header().Get("Referer"),
				req.Header().Get("User-Agent"),
				time.Since(start),
			)
			return r, e
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func Mux(ctx context.Context, mux *http.ServeMux, rts utils.RouterMap) {
	interceptors := connect.WithInterceptors(NewLogInterceptor())
	mux.Handle(lookingglassconnect.NewLookingGlassServiceHandler(NewLookingGlassService(ctx, rts), interceptors))
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
						log.Printf("Router %s is healthy", r.Config.Name)
					}
				} else {
					if o {
						Health.SetStatus(lookingglassconnect.LookingGlassServiceName+"/"+r.Config.Name, grpchealth.StatusNotServing)
						log.Printf("Router %s is unhealthy: %s", r.Config.Name, err)
					}
				}
			}
		}
	}
}
