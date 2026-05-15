// Package webui hosts the runtime-config injector for the embedded WebUI.
package webui

import (
	"encoding/json"
	"net/http"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// EnvJS is the JSON shape serialised into `/_app/env.js`'s `env` object.
type EnvJS struct {
	// PageTitle is the document title shown in the browser tab.
	PageTitle string `json:"PUBLIC_PAGE_TITLE"`
	// HeaderText is the brand-bar text label.
	HeaderText string `json:"PUBLIC_HEADER_TEXT"`
	// HeaderLinks is the serialised header link list in "name|href,name|href" form.
	HeaderLinks string `json:"PUBLIC_HEADER_LINKS"`
	// HeaderLogo is the URL of the header logo image.
	HeaderLogo string `json:"PUBLIC_HEADER_LOGO"`
	// FooterText is the bottom-of-page text label.
	FooterText string `json:"PUBLIC_FOOTER_TEXT"`
	// FooterLinks is the serialised footer link list in "name|href,name|href" form.
	FooterLinks string `json:"PUBLIC_FOOTER_LINKS"`
	// FooterLogo is the URL of the footer logo image.
	FooterLogo string `json:"PUBLIC_FOOTER_LOGO"`
	// GrpcURL is the gRPC-Web endpoint the WebUI dials.
	GrpcURL string `json:"PUBLIC_GRPC_URL"`
	// LGVersion is the server's [utils.Version] string.
	LGVersion string `json:"PUBLIC_LG_VERSION"`
	// SentryDSN, when non-empty, enables the browser Sentry SDK.
	SentryDSN string `json:"PUBLIC_SENTRY_DSN"`
	// SentryEnv is the Sentry environment tag.
	SentryEnv string `json:"PUBLIC_SENTRY_ENV"`
	// SentrySampleRate is the traces-sample-rate in [0, 1].
	SentrySampleRate float64 `json:"PUBLIC_SENTRY_SAMPLE_RATE"`
}

// ConfigInjector builds the `/_app/env.js` handler. The response body
// is precomputed from cfg at construction time and reused per request.
func ConfigInjector(cfg utils.WebConfig) http.Handler {
	envobj := EnvJS{
		PageTitle:   cfg.Title,
		HeaderText:  cfg.Header.Text,
		HeaderLinks: cfg.Header.LinksString(),
		HeaderLogo:  cfg.Header.Logo,
		FooterText:  cfg.Footer.Text,
		FooterLinks: cfg.Footer.LinksString(),
		FooterLogo:  cfg.Footer.Logo,
		GrpcURL:     cfg.GrpcURL,
		LGVersion:   utils.Version(),
	}
	if cfg.Sentry.Enabled {
		envobj.SentryDSN = cfg.Sentry.DSN
		envobj.SentryEnv = cfg.Sentry.Environment
		envobj.SentrySampleRate = cfg.Sentry.SampleRate
	}
	envjson, err := json.Marshal(envobj)
	if err != nil {
		panic(err)
	}
	envjs := "export const env = " + string(envjson)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(envjs))
	})
}
