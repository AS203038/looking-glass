// Command server is the Looking Glass HTTP/2 daemon.
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

// webemned holds the embedded WebUI tree rooted at `dist/`.
//
//go:embed all:dist
var webemned embed.FS

// main is the process entry point.
func main() {
	web, err := fs.Sub(webemned, "dist")
	if err != nil {
		log.Panicln(err)
	}
	Start(context.Background(), web)
}

// Start parses config.yaml, builds the router catalogue, and starts
// the HTTP/2 listener with the supplied embedded WebUI filesystem.
func Start(ctx context.Context, web fs.FS) {
	cfg, err := utils.ParseConfigYaml("config.yaml")
	if err != nil {
		log.Fatalf("ERROR: Failed to parse config: %v\n", err)
	}
	rm := routers.CreateRouterMap(cfg)
	http.ListenAndServe(ctx, cfg, rm, web)
	log.Println("NOTICE: Goodbye, World!")
}
