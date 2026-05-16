// Package http composes the HTTP/2 listener that fronts the gRPC API,
// the optional embedded WebUI, and the security.txt endpoint.
package http

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	stdlog "log"
	"log/slog"
	"net/http"
	"strings"
	"time"

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

// redisClient is the process-global Redis client used by [cacheHandler].
var redisClient *redis.Client

// httpwriter is a recording [http.ResponseWriter] that captures the
// status code and body bytes written through it.
type httpwriter struct {
	http.ResponseWriter
	// Status records the most recent value passed to WriteHeader.
	Status int
	// Body accumulates every byte written through Write.
	Body []byte
}

// Header delegates to the wrapped ResponseWriter.
func (w *httpwriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// WriteHeader records the status code locally and forwards the call.
func (w *httpwriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write buffers the bytes locally and forwards them to the wrapped writer.
func (w *httpwriter) Write(b []byte) (int, error) {
	w.Body = append(w.Body, b...)
	return w.ResponseWriter.Write(b)
}

// CacheEntry is the on-the-wire shape of a cached response stored in Redis.
type CacheEntry struct {
	// Body is the raw response body bytes.
	Body []byte `json:"body"`
	// Status is the response status code; zero means WriteHeader was never called.
	Status int `json:"status"`
	// Header is the full set of response headers recorded by the handler.
	Header http.Header `json:"header"`
}

// cacheHandler wraps h with a Redis-backed response cache keyed by
// MD5(path+body). Cache hits set the X-Cache: HIT response header;
// cache writes are async. TTL falls back to 60s when malformed.
func cacheHandler(cfg utils.RedisConfig, h http.Handler) http.Handler {
	log := logging.Component("cache")
	ttl, err := time.ParseDuration(cfg.TTL)
	if err != nil {
		log.Warn("ttl parse failed; using default",
			slog.String("ttl", cfg.TTL),
			slog.Duration("default", 60*time.Second),
			slog.Any("err", err))
		ttl = 60 * time.Second
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bd, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(bd))
		cacheKey := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.Path+string(bd))))
		cachedResponse, err := redisClient.Get(context.Background(), cacheKey).Result()
		if err == nil {
			var cacheEntry CacheEntry
			err = json.Unmarshal([]byte(cachedResponse), &cacheEntry)
			if err == nil {
				log.Debug("cache hit",
					slog.String("key", cacheKey),
					slog.String("path", r.URL.Path),
					slog.Int("body_bytes", len(cacheEntry.Body)))
				w.Header().Set("X-Cache", "HIT")
				for k, v := range cacheEntry.Header {
					w.Header()[k] = v
				}
				if cacheEntry.Status > 0 {
					w.WriteHeader(cacheEntry.Status)
				}
				w.Write(cacheEntry.Body)
				return
			}
			log.Debug("cache entry malformed; treating as miss",
				slog.String("key", cacheKey),
				slog.Any("err", err))
		} else {
			log.Debug("cache miss",
				slog.String("key", cacheKey),
				slog.String("path", r.URL.Path))
		}

		responseWriter := &httpwriter{ResponseWriter: w}
		h.ServeHTTP(responseWriter, r)

		go func() {
			cacheEntry := CacheEntry{
				Body:   responseWriter.Body,
				Status: responseWriter.Status,
				Header: responseWriter.Header(),
			}
			cachejson, err := json.Marshal(cacheEntry)
			if err != nil {
				log.Error("marshal cache entry failed",
					slog.String("key", cacheKey),
					slog.Any("err", err))
				return
			}
			_, err = redisClient.Set(context.Background(), cacheKey, cachejson, ttl).Result()
			if err != nil {
				log.Error("store cache entry failed",
					slog.String("key", cacheKey),
					slog.Any("err", err))
				return
			}
			log.Debug("cache store ok",
				slog.String("key", cacheKey),
				slog.Int("body_bytes", len(cacheEntry.Body)),
				slog.Duration("ttl", ttl))
		}()
	})
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
		},
	})

	var handler http.Handler = mux

	if cfg.Redis.Enabled {
		opts, err := redis.ParseURL(cfg.Redis.URI)
		if err != nil {
			log.Error("redis url parse failed; cache disabled",
				slog.String("uri", cfg.Redis.URI),
				slog.Any("err", err))
		} else {
			log.Info("connecting to redis", slog.String("uri", cfg.Redis.URI))
			redisClient = redis.NewClient(opts)
			handler = cacheHandler(cfg.Redis, handler)
		}
	}

	handler = loggingHandler(corsHandler.Handler(handler))

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

	// Route net/http server errors through slog at ERROR via the
	// stdlib bridge installed by logging.Init.
	srv := &http.Server{
		Addr:     cfg.Grpc.Listen,
		ErrorLog: stdlog.Default(),
	}

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
			srv.ListenAndServeTLS("", "")
		} else {
			log.Info("using configured certificate",
				slog.String("cert", cfg.Grpc.TLS.Cert))
			srv.ListenAndServeTLS(cfg.Grpc.TLS.Cert, cfg.Grpc.TLS.Key)
		}
	} else {
		log.Info("listening", slog.String("addr", cfg.Grpc.Listen), slog.Bool("tls", false))
		srv.Handler = h2c.NewHandler(handler, &http2.Server{})
		srv.ListenAndServe()
	}
	return nil
}
