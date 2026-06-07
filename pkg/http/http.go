// Package http composes the HTTP/2 listener that fronts the gRPC API,
// the optional embedded WebUI, and the security.txt endpoint.
package http

import (
	"context"
	"crypto/tls"
	"io/fs"
	stdlog "log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AS203038/looking-glass/pkg/bmp"
	"github.com/AS203038/looking-glass/pkg/http/grpc"
	"github.com/AS203038/looking-glass/pkg/http/webui"
	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/redis/go-redis/v9"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// httpwriter is a recording [http.ResponseWriter] that captures the
// status code written through it for the access log.
type httpwriter struct {
	http.ResponseWriter
	// Status records the most recent value passed to WriteHeader.
	Status int
}

// Header delegates to the wrapped ResponseWriter.
func (w *httpwriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// Unwrap returns the wrapped ResponseWriter.
func (w *httpwriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// WriteHeader records the status code locally and forwards the call.
func (w *httpwriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

// loggingHandler wraps h with a structured slog access log and emits
// the process version as an ETag for static asset 304s.
func loggingHandler(h http.Handler) http.Handler {
	log := logging.Component("httpaccess")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wr := &httpwriter{ResponseWriter: w}
		v := utils.Version()
		if v != "untracked" && r.Header.Get("If-None-Match") == v {
			wr.WriteHeader(http.StatusNotModified)
		} else {
			if v != "untracked" {
				wr.Header().Set("Etag", v)
			}
			h.ServeHTTP(wr, r)
		}

		remote := r.Header.Get("X-Forwarded-For")
		if remote == "" {
			remote = r.RemoteAddr
		}
		log.Info("http access",
			slog.String("remote", remote),
			slog.String("method", r.Method),
			slog.String("uri", r.RequestURI),
			slog.String("proto", r.Proto),
			slog.Int("status", wr.Status),
			slog.Int64("content_length", r.ContentLength),
			slog.String("referer", r.Referer()),
			slog.String("user_agent", r.UserAgent()),
			slog.Duration("duration", time.Since(start)),
			slog.String("cache", wr.Header().Get("X-Cache")),
		)
	})
}

// SecurityTxtInjector returns an [http.Handler] that serves cfg as a
// plain-text RFC 9116 security.txt document.
func SecurityTxtInjector(cfg utils.SecurityTxtConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(cfg.String()))
	})
}

// ListenAndServe builds the middleware stack and starts the HTTP/2
// listener described by cfg. It blocks for the lifetime of the listener.
func ListenAndServe(ctx context.Context, cfg *utils.Config, rts utils.RouterMap, webfs fs.FS) error {
	log := logging.Component("http")

	if cfg.Redis.Enabled {
		opts, err := redis.ParseURL(cfg.Redis.URI)
		if err != nil {
			log.Error("redis url parse failed; cache and health-coordination disabled",
				slog.String("uri", cfg.Redis.URI),
				slog.Any("err", err))
		} else {
			log.Info("connecting to redis", slog.String("uri", cfg.Redis.URI))
			client := redis.NewClient(opts)
			grpc.SetRedis(client)
			ttl, perr := time.ParseDuration(cfg.Redis.TTL)
			if perr != nil {
				log.Warn("ttl parse failed; using default",
					slog.String("ttl", cfg.Redis.TTL),
					slog.Duration("default", 60*time.Second),
					slog.Any("err", perr))
				ttl = 60 * time.Second
			}
			grpc.SetRPCCache(client, ttl)

			if cfg.Bmp.Enabled {
				listen := cfg.Bmp.Listen
				if listen == "" {
					listen = ":11019"
				}
				_ = bmp.StartBMPListener(ctx, listen, client, rts)
			}
		}
	}

	mux := http.NewServeMux()
	if cfg.Grpc.Enabled {
		grpc.Mux(ctx, mux, rts)
	}
	if cfg.SecurityTxt.Enabled {
		mux.Handle("/.well-known/security.txt", SecurityTxtInjector(cfg.SecurityTxt))
	}
	if cfg.Web.Enabled {
		mux.Handle("/_app/env.js", webui.ConfigInjector(cfg.Web))
		mux.Handle("/", http.FileServerFS(webfs))
	}
	corsHandler := cors.New(cors.Options{
		Debug: false,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
		},
		AllowedHeaders: []string{
			"Authorization",
			"Accept-Encoding",
			"Content-Encoding",
			"Content-Type",
			"Connect-Protocol-Version",
			"Connect-Timeout-Ms",
			"Connect-Accept-Encoding",  // Unused in web browsers, but added for future-proofing.
			"Connect-Content-Encoding", // Unused in web browsers, but added for future-proofing.
			"Grpc-Timeout",             // Used for gRPC-web.
			"X-Grpc-Web",               // Used for gRPC-web.
			"X-User-Agent",             // Used for gRPC-web.
			"baggage",                  // Used for Sentry distributed tracing.
			"sentry-trace",             // Used for Sentry distributed tracing.
		},
		ExposedHeaders: []string{
			"Content-Encoding",         // Unused in web browsers, but added for future-proofing.
			"Connect-Content-Encoding", // Unused in web browsers, but added for future-proofing.
			"Grpc-Status",              // Required for gRPC-web.
			"Grpc-Message",             // Required for gRPC-web.
			"X-Cache",                  // Cache hit indicator emitted by RPC handlers.
		},
	})

	handler := loggingHandler(corsHandler.Handler(mux))

	if cfg.Web.Sentry.Enabled {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.Web.Sentry.DSN,
			EnableTracing:    true,
			TracesSampleRate: cfg.Web.Sentry.SampleRate,
			Release:          strings.Split(utils.Version(), "+")[0],
			Environment:      cfg.Web.Sentry.Environment,
		})
		if err != nil {
			log.Warn("sentry init failed; sentry middleware disabled",
				slog.Any("err", err))
		} else {
			log.Info("sentry initialized")
			handler = sentryhttp.New(
				sentryhttp.Options{
					Repanic:         true,
					WaitForDelivery: true,
				},
			).Handle(handler)
		}
	}

	srv := &http.Server{

		Addr:     cfg.Grpc.Listen,
		ErrorLog: stdlog.Default(),
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	if cfg.Grpc.TLS.Enabled {
		log.Info("listening", slog.String("addr", cfg.Grpc.Listen), slog.Bool("tls", true))
		srv.Handler = handler
		if cfg.Grpc.TLS.SelfSigned {
			log.Info("using self-signed certificate")
			key, crt, err := utils.GenerateSelfSignedPair()
			if err != nil {
				panic(err)
			}
			srv.TLSConfig = &tls.Config{
				Certificates: []tls.Certificate{{
					Certificate: [][]byte{crt},
					PrivateKey:  key,
				}},
			}
			return srv.ListenAndServeTLS("", "")
		} else {
			log.Info("using configured certificate",
				slog.String("cert", cfg.Grpc.TLS.Cert))
			return srv.ListenAndServeTLS(cfg.Grpc.TLS.Cert, cfg.Grpc.TLS.Key)
		}
	} else {
		log.Info("listening", slog.String("addr", cfg.Grpc.Listen), slog.Bool("tls", false))
		srv.Handler = h2c.NewHandler(handler, &http2.Server{})
		return srv.ListenAndServe()
	}
}
