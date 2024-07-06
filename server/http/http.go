package http

import (
	"context"
	"crypto/tls"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/AS203038/looking-glass/server/http/grpc"
	"github.com/AS203038/looking-glass/server/http/webui"
	"github.com/AS203038/looking-glass/server/utils"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type httpwriter struct {
	http.ResponseWriter
	Status int
}

func (w *httpwriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *httpwriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *httpwriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

func HTTPHandler(h http.Handler) http.Handler {
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
		log.Printf("%s \"%s %s %s\" %d %d \"%s\" \"%s\" %s",
			p,
			r.Method,
			r.RequestURI,
			r.Proto,
			wr.Status,
			r.ContentLength,
			r.Referer(),
			r.UserAgent(),
			time.Since(start),
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
		mux.Handle("/.well-known/security.txt", HTTPHandler(SecurityTxtInjector(cfg.SecurityTxt)))
	}
	if cfg.Web.Enabled {
		mux.Handle("/_app/env.js", HTTPHandler(webui.ConfigInjector(cfg.Web)))
		mux.Handle("/", HTTPHandler(http.FileServerFS(webfs)))
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
	handler := corsHandler.Handler(mux)
	srv := &http.Server{
		Addr:     cfg.Grpc.Listen,
		ErrorLog: log.Default(),
	}

	if cfg.Grpc.TLS.Enabled {
		log.Printf("Listening on %s with TLS", cfg.Grpc.Listen)
		srv.Handler = handler
		if cfg.Grpc.TLS.SelfSigned {
			log.Printf("Using self-signed certificate")
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
			log.Printf("Using certificate %s", cfg.Grpc.TLS.Cert)
			srv.ListenAndServeTLS(cfg.Grpc.TLS.Cert, cfg.Grpc.TLS.Key)
		}
	} else {
		log.Printf("Listening on %s", cfg.Grpc.Listen)
		srv.Handler = h2c.NewHandler(handler, &http2.Server{})
		srv.ListenAndServe()
	}
	return nil
}
