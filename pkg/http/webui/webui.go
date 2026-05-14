// Package webui hosts the runtime-config injector for the embedded
// SvelteKit WebUI.
//
// The static SvelteKit bundle is compiled at build time with no
// knowledge of the deployed instance's configuration (router list,
// branding, Sentry DSN, …). At runtime the WebUI fetches
// `/_app/env.js` first, which evaluates an ES module exporting a
// single `env` object. [ConfigInjector] is the handler that
// generates this module from the operator's [utils.WebConfig].
package webui

import (
	"encoding/json"
	"net/http"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// EnvJS is the JSON shape serialised into `/_app/env.js`'s exported
// `env` object. The PUBLIC_* prefix matches SvelteKit's
// `$env/dynamic/public` convention; the WebUI's
// `webui/src/lib/env.ts` parses each field with the same key.
//
// New fields added here must be added to the TypeScript
// `PublicEnv` interface in the same commit, otherwise the WebUI
// will silently ignore them.
type EnvJS struct {
	// PageTitle is the document title shown in the browser tab.
	PageTitle string `json:"PUBLIC_PAGE_TITLE"`
	// HeaderText is the brand-bar text label.
	HeaderText string `json:"PUBLIC_HEADER_TEXT"`
	// HeaderLinks is the serialised header link list in
	// "name|href,name|href" form. See [utils.HFBlock.LinksString].
	HeaderLinks string `json:"PUBLIC_HEADER_LINKS"`
	// HeaderLogo is the URL of the header logo image.
	HeaderLogo string `json:"PUBLIC_HEADER_LOGO"`
	// FooterText is the bottom-of-page text label.
	FooterText string `json:"PUBLIC_FOOTER_TEXT"`
	// FooterLinks is the serialised footer link list in
	// "name|href,name|href" form. See [utils.HFBlock.LinksString].
	FooterLinks string `json:"PUBLIC_FOOTER_LINKS"`
	// FooterLogo is the URL of the footer logo image.
	FooterLogo string `json:"PUBLIC_FOOTER_LOGO"`
	// GrpcURL is the gRPC-Web endpoint the WebUI dials. Empty
	// means "same origin as the page".
	GrpcURL string `json:"PUBLIC_GRPC_URL"`
	// LGVersion mirrors [utils.Version] so the WebUI can show the
	// running server's release in its footer and tag client-side
	// Sentry events.
	LGVersion string `json:"PUBLIC_LG_VERSION"`
	// SentryDSN, when non-empty, triggers lazy initialisation of
	// the browser Sentry SDK.
	SentryDSN string `json:"PUBLIC_SENTRY_DSN"`
	// SentryEnv is the Sentry environment tag (e.g. "production",
	// "staging").
	SentryEnv string `json:"PUBLIC_SENTRY_ENV"`
	// SentrySampleRate is the traces-sample-rate in the range
	// [0, 1].
	SentrySampleRate float64 `json:"PUBLIC_SENTRY_SAMPLE_RATE"`
}

// ConfigInjector builds the `/_app/env.js` handler.
//
// The handler is fully derived at construction time — the response
// body is precomputed and reused for every request, so this hot
// path performs neither JSON serialisation nor reflection at
// request time. The trade-off is that the response is fixed for the
// lifetime of the process; restart the server to pick up
// configuration changes.
//
// Sentry-specific fields are only populated when
// [utils.SentryConfig.Enabled] is true; the zero values are emitted
// otherwise, which the WebUI interprets as "Sentry disabled".
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
