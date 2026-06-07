package http

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// TestHttpWriter verifies that httpwriter correctly wraps ResponseWriter and intercepts status codes.
func TestHttpWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := &httpwriter{ResponseWriter: rec, Status: 200}

	writer.WriteHeader(http.StatusNotFound)
	if writer.Status != http.StatusNotFound {
		t.Errorf("writer.Status = %d, want %d", writer.Status, http.StatusNotFound)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("rec.Code = %d, want %d", rec.Code, http.StatusNotFound)
	}

	if writer.Unwrap() != rec {
		t.Errorf("Unwrap did not return the original ResponseWriter")
	}
}

// TestSecurityTxtInjector verifies that the security.txt injector correctly serves the structured fields.
func TestSecurityTxtInjector(t *testing.T) {
	cfg := utils.SecurityTxtConfig{
		Enabled:            true,
		Contact:            "mailto:security@example.com",
		Encryption:         "https://keybase.io/example",
		Acknowledgements:   "https://example.com/hall-of-fame",
		PreferredLanguages: "en, fr",
		Policy:             "https://example.com/security-policy",
		Hiring:             "https://example.com/jobs",
	}

	handler := SecurityTxtInjector(cfg)
	req := httptest.NewRequest("GET", "/.well-known/security.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %q", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Contact: mailto:security@example.com") {
		t.Errorf("security.txt body does not contain Contact line: %q", body)
	}
	if !strings.Contains(body, "Preferred-Languages: en, fr") {
		t.Errorf("security.txt body does not contain Preferred-Languages line: %q", body)
	}
}

// TestLoggingHandler verifies that the access logging and ETag logic works under normal operations.
func TestLoggingHandler(t *testing.T) {
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("short body"))
	})

	logged := loggingHandler(mockHandler)

	req := httptest.NewRequest("GET", "/test-path", nil)
	rec := httptest.NewRecorder()

	logged.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status 418 Teapot, got %d", rec.Code)
	}

	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Errorf("expected non-empty ETag header")
	}

	reqCached := httptest.NewRequest("GET", "/test-path", nil)
	reqCached.Header.Set("If-None-Match", etag)
	recCached := httptest.NewRecorder()

	logged.ServeHTTP(recCached, reqCached)

	if recCached.Code != http.StatusNotModified {
		t.Errorf("expected 304 Not Modified, got %d", recCached.Code)
	}
}

// TestListenAndServe exercises all major configuration paths in ListenAndServe.
func TestListenAndServe(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("welcome")},
	}

	tests := []struct {
		name    string
		setup   func() (*utils.Config, utils.RouterMap, fs.FS)
		wantErr bool
	}{
		{
			name: "Standard Cleartext with Redis parse error and invalid TTL",
			setup: func() (*utils.Config, utils.RouterMap, fs.FS) {
				cfg := &utils.Config{}
				cfg.Grpc.Listen = "127.0.0.1:0"
				cfg.Redis.Enabled = true
				cfg.Redis.URI = "redis://invalid uri with space"
				cfg.Redis.TTL = "invalid-ttl-duration"
				return cfg, utils.RouterMap{}, mockFS
			},
			wantErr: false,
		},
		{
			name: "Standard Cleartext with Redis parsed and Sentry error",
			setup: func() (*utils.Config, utils.RouterMap, fs.FS) {
				cfg := &utils.Config{}
				cfg.Grpc.Listen = "127.0.0.1:0"
				cfg.Redis.Enabled = true
				cfg.Redis.URI = "redis://localhost:6379/0"
				cfg.Redis.TTL = "30s"
				cfg.Web.Enabled = true
				cfg.Web.Sentry.Enabled = true
				cfg.Web.Sentry.DSN = "ftp://invalid-sentry"
				cfg.SecurityTxt.Enabled = true
				cfg.Grpc.Enabled = true
				return cfg, utils.RouterMap{}, mockFS
			},
			wantErr: false,
		},
		{
			name: "TLS SelfSigned Enabled",
			setup: func() (*utils.Config, utils.RouterMap, fs.FS) {
				cfg := &utils.Config{}
				cfg.Grpc.Listen = "127.0.0.1:0"
				cfg.Grpc.TLS.Enabled = true
				cfg.Grpc.TLS.SelfSigned = true
				return cfg, utils.RouterMap{}, mockFS
			},
			wantErr: false,
		},
		{
			name: "TLS Configured Certs (expect error due to missing cert files)",
			setup: func() (*utils.Config, utils.RouterMap, fs.FS) {
				cfg := &utils.Config{}
				cfg.Grpc.Listen = "127.0.0.1:0"
				cfg.Grpc.TLS.Enabled = true
				cfg.Grpc.TLS.SelfSigned = false
				cfg.Grpc.TLS.Cert = "nonexistent_cert.pem"
				cfg.Grpc.TLS.Key = "nonexistent_key.pem"
				return cfg, utils.RouterMap{}, mockFS
			},
			wantErr: true,
		},
		{
			name: "BMP Enabled with Empty Listen and Sentry Enabled with Valid DSN",
			setup: func() (*utils.Config, utils.RouterMap, fs.FS) {
				cfg := &utils.Config{}
				cfg.Grpc.Listen = "127.0.0.1:0"
				cfg.Redis.Enabled = true
				cfg.Redis.URI = "redis://localhost:6379/0"
				cfg.Redis.TTL = "30s"
				cfg.Bmp.Enabled = true
				cfg.Bmp.Listen = ""
				cfg.Web.Enabled = true
				cfg.Web.Sentry.Enabled = true
				cfg.Web.Sentry.DSN = "https://public@sentry.example.com/1"
				return cfg, utils.RouterMap{}, mockFS
			},
			wantErr: false,
		},
		{
			name: "BMP Enabled with Specified Listen Port",
			setup: func() (*utils.Config, utils.RouterMap, fs.FS) {
				cfg := &utils.Config{}
				cfg.Grpc.Listen = "127.0.0.1:0"
				cfg.Redis.Enabled = true
				cfg.Redis.URI = "redis://localhost:6379/0"
				cfg.Redis.TTL = "30s"
				cfg.Bmp.Enabled = true
				cfg.Bmp.Listen = "127.0.0.1:0"
				return cfg, utils.RouterMap{}, mockFS
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, rts, webfs := tt.setup()

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- ListenAndServe(ctx, cfg, rts, webfs)
			}()

			select {
			case err := <-errCh:
				if (err != nil) != tt.wantErr {
					t.Errorf("ListenAndServe() returned error %v, wantErr %v", err, tt.wantErr)
				}
			case <-time.After(150 * time.Millisecond):
				cancel()
				err := <-errCh
				if err != nil && err != http.ErrServerClosed && !tt.wantErr {
					t.Errorf("ListenAndServe() failed on shutdown: %v", err)
				}
			}
		})
	}
}
