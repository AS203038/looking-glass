// Command server is the Looking Glass HTTP/2 daemon.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"os"

	"github.com/AS203038/looking-glass/pkg/http"
	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/routers"
	"github.com/AS203038/looking-glass/pkg/utils"
)

// Exit codes carry a distinct value per failure mode so the cause is
// recoverable from the shell without scraping log output.
const (
	exitEmbedFS    = 2
	exitConfigLoad = 3
	exitLoggingCfg = 4
)

// webemned holds the embedded WebUI tree rooted at `dist/`.
//
//go:embed all:dist
var webemned embed.FS

// main is the process entry point.
func main() {
	web, err := fs.Sub(webemned, "dist")
	if err != nil {
		slog.Error("embedded webui filesystem unavailable", slog.Any("err", err))
		os.Exit(exitEmbedFS)
	}
	Start(context.Background(), web)
}

// Start parses config.yaml, builds the router catalogue, and starts
// the HTTP/2 listener with the supplied embedded WebUI filesystem.
func Start(ctx context.Context, web fs.FS) {
	cfg, err := utils.ParseConfigYaml("config.yaml")
	if err != nil {
		slog.Error("config parse failed",
			slog.String("path", "config.yaml"),
			slog.Any("err", err))
		os.Exit(exitConfigLoad)
	}
	if err := logging.Init(cfg.Logging); err != nil {
		slog.Error("logging init failed", slog.Any("err", err))
		os.Exit(exitLoggingCfg)
	}
	serverLog := logging.Component("server")
	serverLog.Info("server starting",
		slog.String("version", utils.Version()),
		slog.Int("pid", os.Getpid()),
		slog.Int("devices", len(cfg.Devices)))
	rm := routers.CreateRouterMap(cfg)
	http.ListenAndServe(ctx, cfg, rm, web)
	serverLog.Info("server shutting down")
}
