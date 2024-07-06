package server

import (
	"context"
	"io/fs"
	"log"

	"github.com/AS203038/looking-glass/server/http"
	"github.com/AS203038/looking-glass/server/routers"
	"github.com/AS203038/looking-glass/server/utils"
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
