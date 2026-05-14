// Package http composes the HTTP/2 listener that fronts every public
// surface of the server: the gRPC/ConnectRPC API, the optional
// embedded SvelteKit WebUI, and the RFC 9116 security.txt endpoint.
//
// The package is also the home of the request middleware chain. The
// chain is intentionally built bottom-up in [ListenAndServe] so that
// the order of `handler = … (handler)` assignments matches the order
// in which the wrappers see each request. The full chain, from
// outermost to innermost, is:
//
//	Sentry (optional) → Logging → CORS → Redis cache (optional)
//	→ ServeMux → handler-specific code
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
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/AS203038/looking-glass/pkg/http/grpc"
	"github.com/AS203038/looking-glass/pkg/http/webui"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/redis/go-redis/v9"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// redisClient is the lazily-initialised, process-global Redis client
// used by [cacheHandler]. It is created in [ListenAndServe] when the
// operator has enabled the Redis cache and is nil otherwise; callers
// must therefore only dereference it from within the cache
// middleware, which is only installed when initialisation succeeded.
var redisClient *redis.Client

// httpwriter is a tee-style [http.ResponseWriter] that records the
// status code and body bytes as they are written to the underlying
// writer. It is wrapped around the real ResponseWriter so the
// caching and logging middleware can observe a response without
// having to re-execute the handler.
type httpwriter struct {
	http.ResponseWriter
	// Status records the most recent value passed to WriteHeader,
	// or zero if the handler never explicitly set it.
	Status int
	// Body accumulates every byte written through Write, in order.
	// Mainly used by the cache middleware to assemble the value
	// later stored in Redis.
	Body []byte
}

// Header delegates to the wrapped ResponseWriter.
func (w *httpwriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// WriteHeader records the status code locally before forwarding the
// call to the wrapped ResponseWriter.
func (w *httpwriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write appends the bytes to the local buffer and forwards them to
// the wrapped ResponseWriter. The return value is taken from the
// underlying Write so handlers see the real wire-level outcome.
func (w *httpwriter) Write(b []byte) (int, error) {
	w.Body = append(w.Body, b...)
	return w.ResponseWriter.Write(b)
}

// CacheEntry is the on-the-wire shape of a cached response stored
// in Redis. The full response — status code, headers and body — is
// preserved so a cache hit can replay the original outcome exactly.
type CacheEntry struct {
	// Body is the raw response body bytes.
	Body []byte `json:"body"`
	// Status is the response status code; zero means the handler
	// never explicitly called WriteHeader, in which case the
	// replay handler does not call it either (preserving Go's
	// implicit-200 semantics).
	Status int `json:"status"`
	// Header is the full set of response headers as recorded by
	// the original handler.
	Header http.Header `json:"header"`
}

// cacheHandler wraps h with a Redis-backed response cache.
//
// Caching strategy:
//
//   - The cache key is the MD5 of `path + request_body`, which makes
//     POST-style gRPC requests cacheable (their bodies fully
//     determine the response) while keeping the key short and
//     opaque.
//   - On a cache hit the recorded headers, status and body are
//     replayed verbatim and the response is decorated with
//     `X-Cache: HIT` so operators can distinguish cached and live
//     responses in the access log.
//   - On a cache miss the handler is run as usual; the response is
//     captured via [httpwriter] and persisted asynchronously so the
//     client never waits for the Redis write to complete.
//
// The TTL is parsed from the operator-supplied [utils.RedisConfig];
// a malformed TTL falls back to 60 seconds rather than failing
// startup.
func cacheHandler(cfg utils.RedisConfig, h http.Handler) http.Handler {
	ttl, err := time.ParseDuration(cfg.TTL)
	if err != nil {
		log.Println("WARNING: Failed to parse TTL:", err, "using default of 60 seconds")
		ttl = 60 * time.Second
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bd, _ := io.ReadAll(r.Body)
		// Restore the body so the downstream handler can still
		// read the request payload.
		r.Body = io.NopCloser(bytes.NewReader(bd))
		cacheKey := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.Path+string(bd))))
		cachedResponse, err := redisClient.Get(context.Background(), cacheKey).Result()
		if err == nil {
			var cacheEntry CacheEntry
			err = json.Unmarshal([]byte(cachedResponse), &cacheEntry)
			if err == nil {
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
		}

		// Cache miss: run the handler against a recording writer
		// so we can persist the response after replying.
		responseWriter := &httpwriter{ResponseWriter: w}
		h.ServeHTTP(responseWriter, r)

		// Persist asynchronously: the cache write is best-effort
		// and must never block the request hot path.
		go func() {
			cacheEntry := CacheEntry{
				Body:   responseWriter.Body,
				Status: responseWriter.Status,
				Header: responseWriter.Header(),
			}
			cachejson, err := json.Marshal(cacheEntry)
			if err != nil {
				log.Println("ERROR: Failed to marshal cache entry:", err)
				return
			}
			_, err = redisClient.Set(context.Background(), cacheKey, cachejson, ttl).Result()
			if err != nil {
				log.Println("ERROR: Failed to cache response:", err)
			}
		}()
	})
}

// loggingHandler wraps h with an Apache Common Log Format access
// log and a process-version ETag for static assets.
//
// ETag handling: when [utils.Version] reports a tracked release
// (i.e. anything other than the "untracked" sentinel) the version
// is sent as the response ETag, and matching `If-None-Match`
// requests short-circuit to 304 Not Modified without invoking the
// inner handler. This is the headline optimisation for the embedded
// WebUI — browsers re-validate cheaply across page loads.
//
// The access log line additionally records the X-Cache value set by
// [cacheHandler] (HIT / blank for miss) so cache-rate analytics can
// be derived from the log stream alone.
func loggingHandler(h http.Handler) http.Handler {
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

		// Prefer X-Forwarded-For when set (deployments behind a
		// reverse proxy); fall back to the direct peer address.
		p := r.Header.Get("X-Forwarded-For")
		if p == "" {
			p = r.RemoteAddr
		}
		c := wr.Header().Get("X-Cache")
		log.Printf("%s \"%s %s %s\" %d %d \"%s\" \"%s\" %s %s",
			p,
			r.Method,
			r.RequestURI,
			r.Proto,
			wr.Status,
			r.ContentLength,
			r.Referer(),
			r.UserAgent(),
			time.Since(start),
			c,
		)
	})
}

// SecurityTxtInjector returns an [http.Handler] that serves the
// supplied [utils.SecurityTxtConfig] as a plain-text RFC 9116
// security.txt document. The handler is only mounted at
// `/.well-known/security.txt` when [utils.SecurityTxtConfig.Enabled]
// is true.
func SecurityTxtInjector(cfg utils.SecurityTxtConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(cfg.String()))
	})
}

// ListenAndServe builds the full middleware stack and starts the
// HTTP/2 listener described by cfg.
//
// The function is the canonical wiring of the server: it constructs
// the mux, mounts each enabled surface (gRPC, security.txt, WebUI),
// composes the middleware chain in the order documented at the top
// of this file, and finally enters either TLS or h2c serve mode
// depending on [utils.GrpcConfig.TLS].
//
// The function blocks for the lifetime of the listener and only
// returns once the underlying server stops; it never returns a
// non-nil error in the current implementation (failures inside
// ListenAndServe are logged and either ignored or panic).
//
// Parameters:
//
//   - ctx is forwarded to the gRPC mux so handler implementations
//     can hang their own deadlines off it.
//   - cfg is the parsed configuration; see [utils.Config].
//   - rts is the materialised router catalogue.
//   - webfs is the embedded WebUI filesystem (typically the result
//     of an `embed.FS.Sub` to drop the `dist/` prefix).
func ListenAndServe(ctx context.Context, cfg *utils.Config, rts utils.RouterMap, webfs fs.FS) error {
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
		// AllowedOrigins defaults to "*" so the WebUI can be
		// served from a different origin than the gRPC endpoint
		// (e.g. during local development with `vite dev`).
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
		// Construct the Redis client once and wrap the handler.
		// Parse failures disable the cache (logged) but never
		// abort startup — the server is still useful without it.
		opts, err := redis.ParseURL(cfg.Redis.URI)
		if err != nil {
			log.Println("ERROR: Failed to parse Redis URL:", err, "disabling Redis cache")
		} else {
			log.Println("NOTICE: Connecting to Redis at", cfg.Redis.URI)
			redisClient = redis.NewClient(opts)
			handler = cacheHandler(cfg.Redis, handler)
		}
	}

	handler = loggingHandler(corsHandler.Handler(handler))

	if cfg.Web.Sentry.Enabled {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:                cfg.Web.Sentry.DSN,
			EnableTracing:      true,
			TracesSampleRate:   cfg.Web.Sentry.SampleRate,
			ProfilesSampleRate: 1.0,
			Release:            strings.Split(utils.Version(), "+")[0],
			Environment:        cfg.Web.Sentry.Environment,
		})
		if err != nil {
			log.Println("WARNING: Failed to initialize Sentry:", err, "disabling Sentry middleware")
		} else {
			log.Println("NOTICE: Sentry initialized")
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
		ErrorLog: log.Default(),
	}

	if cfg.Grpc.TLS.Enabled {
		log.Printf("NOTICE: Listening on %s with TLS", cfg.Grpc.Listen)
		srv.Handler = handler
		if cfg.Grpc.TLS.SelfSigned {
			log.Printf("NOTICE: Using self-signed certificate")
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
			log.Printf("NOTICE: Using certificate %s", cfg.Grpc.TLS.Cert)
			srv.ListenAndServeTLS(cfg.Grpc.TLS.Cert, cfg.Grpc.TLS.Key)
		}
	} else {
		// h2c lets us speak HTTP/2 without TLS, which is required
		// for gRPC clients that don't transparently downgrade.
		log.Printf("NOTICE: Listening on %s", cfg.Grpc.Listen)
		srv.Handler = h2c.NewHandler(handler, &http2.Server{})
		srv.ListenAndServe()
	}
	return nil
}
