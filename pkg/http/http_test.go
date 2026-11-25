package http

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AS203038/looking-glass/pkg/http/grpc"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/redis/go-redis/v9"
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

	// /_app/env.js must bypass the binary-version ETag shortcut so config edits take effect.
	reqEnv := httptest.NewRequest("GET", "/_app/env.js", nil)
	reqEnv.Header.Set("If-None-Match", etag)
	recEnv := httptest.NewRecorder()
	logged.ServeHTTP(recEnv, reqEnv)
	if recEnv.Code != http.StatusTeapot {
		t.Errorf("expected /_app/env.js to reach inner handler (418), got %d", recEnv.Code)
	}
}

// TestSecurityHeadersHandler verifies that baseline HTTP security headers are added to responses.
func TestSecurityHeadersHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeadersHandler(inner)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q, want strict-origin-when-cross-origin", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
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

func TestRedactRedisURI(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "no credentials",
			raw:  "redis://localhost:6379/0",
			want: "redis://localhost:6379/0",
		},
		{
			name: "non-sensitive query",
			raw:  "redis://redis-svc:6379/0?protocol=3",
			want: "redis://redis-svc:6379/0?protocol=3",
		},
		{
			name: "password only",
			raw:  "redis://:supersecret@localhost:6379/0",
			want: "redis://:xxxxx@localhost:6379/0",
		},
		{
			name: "user and password with tls and query",
			raw:  "rediss://alice:supersecret@redis.example.com:6380/2?protocol=3",
			want: "rediss://alice:xxxxx@redis.example.com:6380/2?protocol=3",
		},
		{
			name: "bare userinfo without colon",
			raw:  "redis://supersecret@localhost:6379/0",
			want: "redis://xxxxx@localhost:6379/0",
		},
		{
			name: "unix socket with credentials",
			raw:  "unix://alice:supersecret@/var/run/redis.sock?db=1",
			want: "unix://alice:xxxxx@/var/run/redis.sock?db=1",
		},
		{
			name: "ipv6 host with password",
			raw:  "redis://:supersecret@[::1]:6379/0",
			want: "redis://:xxxxx@[::1]:6379/0",
		},
		{
			name: "unparseable uri with space",
			raw:  "redis://:my secret@localhost:6379/0",
			want: "<redacted>",
		},
		{
			name: "unparseable uri with invalid escape",
			raw:  "redis://:my%zzsecret@localhost:6379/0",
			want: "<redacted>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactRedisURI(tc.raw)
			if got != tc.want {
				t.Errorf("redactRedisURI(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}

	t.Run("sensitive query parameter", func(t *testing.T) {
		got := redactRedisURI("redis://localhost:6379/0?password=supersecret&protocol=3")
		if strings.Contains(got, "supersecret") {
			t.Fatalf("expected secret to be redacted, got %q", got)
		}
		if !strings.Contains(got, "password=xxxxx") {
			t.Errorf("expected password=xxxxx in %q", got)
		}
		if !strings.Contains(got, "protocol=3") {
			t.Errorf("expected protocol=3 in %q", got)
		}
	})
}

func TestSanitizeRedisParseErr(t *testing.T) {
	t.Run("unparseable uri with space", func(t *testing.T) {
		_, err := redis.ParseURL("redis://:mysecret pw@localhost:6379/0")
		if err == nil {
			t.Fatal("expected parse error")
		}
		sanitized := sanitizeRedisParseErr(err).Error()
		if strings.Contains(sanitized, "mysecret") || strings.Contains(sanitized, "pw") {
			t.Fatalf("sanitized error leaked credentials: %q", sanitized)
		}
		if !strings.Contains(sanitized, "<redacted>") {
			t.Errorf("expected <redacted> in %q", sanitized)
		}
	})

	t.Run("unparseable uri with invalid escape", func(t *testing.T) {
		_, err := redis.ParseURL("redis://:my%zzsecret@localhost:6379/0")
		if err == nil {
			t.Fatal("expected parse error")
		}
		sanitized := sanitizeRedisParseErr(err).Error()
		for _, forbidden := range []string{"my", "secret", "%zz"} {
			if strings.Contains(sanitized, forbidden) {
				t.Fatalf("sanitized error leaked %q: %q", forbidden, sanitized)
			}
		}
		if !strings.Contains(sanitized, "<redacted>") {
			t.Errorf("expected <redacted> in %q", sanitized)
		}
		if !strings.Contains(sanitized, "invalid URL escape") {
			t.Errorf("expected invalid URL escape in %q", sanitized)
		}
	})

	t.Run("parseable url with invalid scheme", func(t *testing.T) {
		_, err := redis.ParseURL("http://:mysecret@localhost:6379/0")
		if err == nil {
			t.Fatal("expected parse error")
		}
		sanitized := sanitizeRedisParseErr(err).Error()
		if strings.Contains(sanitized, "mysecret") {
			t.Fatalf("sanitized error leaked credentials: %q", sanitized)
		}
	})
}

type syncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestListenAndServeRedactsRedisPassword(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("welcome")},
	}

	prev := slog.Default()
	defer slog.SetDefault(prev)
	defer func() {
		grpc.SetRedis(nil)
		grpc.SetRPCCache(nil, 0)
	}()

	cases := []struct {
		name         string
		uri          string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "valid uri with password",
			uri:          "redis://alice:top-secret-pw@127.0.0.1:6379/0?protocol=3",
			wantContains: []string{"connecting to redis", "redis://alice:xxxxx@127.0.0.1:6379/0?protocol=3"},
			wantAbsent:   []string{"top-secret-pw"},
		},
		{
			name:         "parseable invalid-scheme uri with password",
			uri:          "http://:top-secret-pw@127.0.0.1:6379/0",
			wantContains: []string{"redis url parse failed", "http://:xxxxx@127.0.0.1:6379/0"},
			wantAbsent:   []string{"top-secret-pw"},
		},
		{
			name:         "unparseable uri with invalid escape and password",
			uri:          "redis://:top%zzsecret@127.0.0.1:6379/0",
			wantContains: []string{"redis url parse failed", "<redacted>"},
			wantAbsent:   []string{"top", "%zz", "secret"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &syncLogBuffer{}
			slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))

			cfg := &utils.Config{}
			cfg.Grpc.Listen = "127.0.0.1:0"
			cfg.Redis.Enabled = true
			cfg.Redis.URI = tc.uri

			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				errCh <- ListenAndServe(ctx, cfg, utils.RouterMap{}, mockFS)
			}()

			select {
			case err := <-errCh:
				if err != nil && err != http.ErrServerClosed {
					t.Fatalf("ListenAndServe() error = %v", err)
				}
			case <-time.After(100 * time.Millisecond):
				cancel()
				if err := <-errCh; err != nil && err != http.ErrServerClosed {
					t.Fatalf("ListenAndServe() shutdown error = %v", err)
				}
			}

			logs := buf.String()
			for _, want := range tc.wantContains {
				if !strings.Contains(logs, want) {
					t.Errorf("expected logs to contain %q, got:\n%s", want, logs)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(logs, absent) {
					t.Errorf("expected logs NOT to contain %q, got:\n%s", absent, logs)
				}
			}
		})
	}
}
