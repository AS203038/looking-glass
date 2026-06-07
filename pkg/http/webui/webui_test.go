package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// TestConfigInjector verifies that ConfigInjector correctly serializes
// the WebConfig structure to an exported Javascript env object, setting the
// proper javascript content-type header, and correctly mapping Sentry fields.
func TestConfigInjector(t *testing.T) {
	tests := []struct {
		name       string
		cfg        utils.WebConfig
		wantSentry bool
	}{
		{
			name: "Sentry Disabled",
			cfg: utils.WebConfig{
				Title: "Test LG",
				Header: utils.HFBlock{
					Text: "Test Header",
				},
				Footer: utils.HFBlock{
					Text: "Test Footer",
				},
				GrpcURL: "/grpc",
			},
			wantSentry: false,
		},
		{
			name: "Sentry Enabled",
			cfg: utils.WebConfig{
				Title: "Test LG",
				Header: utils.HFBlock{
					Text: "Test Header",
				},
				Footer: utils.HFBlock{
					Text: "Test Footer",
				},
				GrpcURL: "/grpc",
				Sentry: utils.SentryConfig{
					Enabled:     true,
					DSN:         "https://example.com/sentry",
					Environment: "production",
					SampleRate:  0.5,
				},
			},
			wantSentry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := ConfigInjector(tt.cfg)
			req := httptest.NewRequest(http.MethodGet, "/_app/env.js", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected HTTP 200, got %d", rec.Code)
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/javascript" {
				t.Errorf("expected Content-Type application/javascript, got %q", contentType)
			}

			body := rec.Body.String()
			const prefix = "export const env = "
			if !strings.HasPrefix(body, prefix) {
				t.Fatalf("expected prefix %q, got %q", prefix, body)
			}

			jsonStr := strings.TrimPrefix(body, prefix)
			var env EnvJS
			if err := json.Unmarshal([]byte(jsonStr), &env); err != nil {
				t.Fatalf("failed to unmarshal JSON env payload: %v", err)
			}

			if env.PageTitle != tt.cfg.Title {
				t.Errorf("expected PageTitle %q, got %q", tt.cfg.Title, env.PageTitle)
			}

			if env.GrpcURL != tt.cfg.GrpcURL {
				t.Errorf("expected GrpcURL %q, got %q", tt.cfg.GrpcURL, env.GrpcURL)
			}

			if tt.wantSentry {
				if env.SentryDSN != tt.cfg.Sentry.DSN {
					t.Errorf("expected SentryDSN %q, got %q", tt.cfg.Sentry.DSN, env.SentryDSN)
				}
				if env.SentryEnv != tt.cfg.Sentry.Environment {
					t.Errorf("expected SentryEnv %q, got %q", tt.cfg.Sentry.Environment, env.SentryEnv)
				}
				if env.SentrySampleRate != tt.cfg.Sentry.SampleRate {
					t.Errorf("expected SentrySampleRate %f, got %f", tt.cfg.Sentry.SampleRate, env.SentrySampleRate)
				}
			} else {
				if env.SentryDSN != "" {
					t.Errorf("expected empty SentryDSN, got %q", env.SentryDSN)
				}
			}
		})
	}
}
