package server

import (
	"context"
	"io/fs"
	"log"

	"gitlab.as203038.net/AS203038/looking-glass/server/http"
	"gitlab.as203038.net/AS203038/looking-glass/server/routers"
	"gitlab.as203038.net/AS203038/looking-glass/server/utils"
)

func Start(ctx context.Context, web fs.FS) {
	cfg, err := utils.ParseConfigYaml("config.yaml")
	if err != nil {
		log.Fatalf("Failed to parse config: %v\n", err)
	}
	rm := routers.CreateRouterMap(cfg)
	http.ListenAndServe(ctx, cfg, rm, web)
	log.Println("Goodbye, World!")
}
