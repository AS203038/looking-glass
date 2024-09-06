package main

import (
	"context"
	"embed"
	"io/fs"
	"log"

	"github.com/AS203038/looking-glass/pkg/http"
	"github.com/AS203038/looking-glass/pkg/routers"
	"github.com/AS203038/looking-glass/pkg/utils"
)

//go:embed all:dist
var webemned embed.FS

func main() {
	web, err := fs.Sub(webemned, "dist")
	if err != nil {
		log.Panicln(err)
	}
	Start(context.Background(), web)
}

func Start(ctx context.Context, web fs.FS) {
	cfg, err := utils.ParseConfigYaml("config.yaml")
	if err != nil {
		log.Fatalf("ERROR: Failed to parse config: %v\n", err)
	}
	rm := routers.CreateRouterMap(cfg)
	http.ListenAndServe(ctx, cfg, rm, web)
	log.Println("NOTICE: Goodbye, World!")
}
