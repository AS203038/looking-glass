// Command server is the Looking Glass HTTP/2 daemon.
//
// It parses `config.yaml` from the working directory, materialises
// the configured routers into a [utils.RouterMap], and starts the
// listener wired up by pkg/http. The embedded SvelteKit WebUI is
// served straight from the `dist/` directory baked into the binary
// at build time.
//
// The build is structured so that the WebUI's static output ends up
// in `cmd/server/dist/` (see `webui/svelte.config.js` adapter-static
// configuration); the `//go:embed all:dist` directive then folds
// that tree into the resulting Go binary, producing a single,
// self-contained artefact.
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

// webemned holds the embedded WebUI tree (a SvelteKit
// adapter-static build) rooted at `dist/`. The path prefix is
// stripped via [fs.Sub] before being handed to the HTTP layer.
//
//go:embed all:dist
var webemned embed.FS

// main is the process entry point. It strips the `dist/` prefix
// from the embedded filesystem and hands control to [Start].
func main() {
	web, err := fs.Sub(webemned, "dist")
	if err != nil {
		log.Panicln(err)
	}
	Start(context.Background(), web)
}

// Start is the runnable core of the server, separated from [main]
// so test code can drive it with an arbitrary context and an
// alternative WebUI filesystem (e.g. one served from disk during
// integration tests).
//
// The function:
//
//  1. Parses `config.yaml` from the current working directory,
//     terminating the process via log.Fatalf on failure.
//  2. Builds the router catalogue via [routers.CreateRouterMap].
//  3. Hands control to [http.ListenAndServe], which blocks for the
//     lifetime of the listener.
//
// Cancelling ctx is the supported shutdown mechanism, though the
// current listener implementation does not yet propagate
// cancellation back to the HTTP server (a TODO for graceful
// shutdown work).
func Start(ctx context.Context, web fs.FS) {
	cfg, err := utils.ParseConfigYaml("config.yaml")
	if err != nil {
		log.Fatalf("ERROR: Failed to parse config: %v\n", err)
	}
	rm := routers.CreateRouterMap(cfg)
	http.ListenAndServe(ctx, cfg, rm, web)
	log.Println("NOTICE: Goodbye, World!")
}
