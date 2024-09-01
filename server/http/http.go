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
	"time"

	"github.com/AS203038/looking-glass/server/http/grpc"
	"github.com/AS203038/looking-glass/server/http/webui"
	"github.com/AS203038/looking-glass/server/utils"
	"github.com/redis/go-redis/v9"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var redisClient *redis.Client

type httpwriter struct {
	http.ResponseWriter
	Status int
	Body   []byte
}

func (w *httpwriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *httpwriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *httpwriter) Write(b []byte) (int, error) {
	w.Body = append(w.Body, b...)
	return w.ResponseWriter.Write(b)
}

type CacheEnrty struct {
	Body   []byte      `json:"body"`
	Status int         `json:"status"`
	Header http.Header `json:"header"`
}

func cacheHandler(cfg utils.RedisConfig, h http.Handler) http.Handler {
	ttl, err := time.ParseDuration(cfg.TTL)
	if err != nil {
		log.Println("WARNING: Failed to parse TTL:", err, "using default of 60 seconds")
		ttl = 60 * time.Second
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bd, _ := io.ReadAll(r.Body)
		// reset the body so it can be read again
		r.Body = io.NopCloser(bytes.NewReader(bd))
		cacheKey := fmt.Sprintf("%x", md5.Sum([]byte(r.URL.Path+string(bd))))
		cachedResponse, err := redisClient.Get(context.Background(), cacheKey).Result()
		if err == nil {
			var cacheEntry CacheEnrty
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

		// If the response is not cached, execute the handler and cache the response
		responseWriter := &httpwriter{ResponseWriter: w}
		h.ServeHTTP(responseWriter, r)

		go func() {
			cacheEntry := CacheEnrty{
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
		}() // Don't wait for the cache to complete
	})
}

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

		// Log the request
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

func SecurityTxtInjector(cfg utils.SecurityTxtConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(cfg.String()))
	})
}

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
		// AllowedOrigins: []string{"*"},
		AllowedHeaders: []string{
			"Authorization",
			"Accept-Encoding",
			"Content-Encoding",
			"Content-Type",
			"Connect-Protocol-Version",
			"Connect-Timeout-Ms",
			"Connect-Accept-Encoding",  // Unused in web browsers, but added for future-proofing
			"Connect-Content-Encoding", // Unused in web browsers, but added for future-proofing
			"Grpc-Timeout",             // Used for gRPC-web
			"X-Grpc-Web",               // Used for gRPC-web
			"X-User-Agent",             // Used for gRPC-web
		},
		ExposedHeaders: []string{
			"Content-Encoding",         // Unused in web browsers, but added for future-proofing
			"Connect-Content-Encoding", // Unused in web browsers, but added for future-proofing
			"Grpc-Status",              // Required for gRPC-web
			"Grpc-Message",             // Required for gRPC-web
		},
	})

	var handler http.Handler = mux

	if cfg.Redis.Enabled {
		// Create a Redis client
		opts, err := redis.ParseURL(cfg.Redis.URI)
		if err != nil {
			log.Println("ERROR: Failed to parse Redis URL:", err)
		} else {
			log.Println("NOTICE: Connecting to Redis at", cfg.Redis.URI)
			redisClient = redis.NewClient(opts)
			// Wrap the existing handler with the cache middleware
			handler = cacheHandler(cfg.Redis, handler)
		}
	}

	handler = loggingHandler(corsHandler.Handler(handler))
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
		log.Printf("NOTICE: Listening on %s", cfg.Grpc.Listen)
		srv.Handler = h2c.NewHandler(handler, &http2.Server{})
		srv.ListenAndServe()
	}
	return nil
}
