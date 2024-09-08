package webui

import (
	"encoding/json"
	"net/http"

	"github.com/AS203038/looking-glass/pkg/utils"
)

type EnvJS struct {
	Theme            string  `json:"PUBLIC_THEME"`
	PageTitle        string  `json:"PUBLIC_PAGE_TITLE"`
	HeaderText       string  `json:"PUBLIC_HEADER_TEXT"`
	HeaderLinks      string  `json:"PUBLIC_HEADER_LINKS"`
	HeaderLogo       string  `json:"PUBLIC_HEADER_LOGO"`
	FooterText       string  `json:"PUBLIC_FOOTER_TEXT"`
	FooterLinks      string  `json:"PUBLIC_FOOTER_LINKS"`
	FooterLogo       string  `json:"PUBLIC_FOOTER_LOGO"`
	GrpcURL          string  `json:"PUBLIC_GRPC_URL"`
	LGVersion        string  `json:"PUBLIC_LG_VERSION"`
	SentryDSN        string  `json:"PUBLIC_SENTRY_DSN"`
	SentryEnv        string  `json:"PUBLIC_SENTRY_ENV"`
	SentrySampleRate float64 `json:"PUBLIC_SENTRY_SAMPLE_RATE"`
	RtListMax        int     `json:"PUBLIC_RT_LIST_MAX"`
}

func ConfigInjector(cfg utils.WebConfig) http.Handler {
	envobj := EnvJS{
		Theme:       cfg.Theme,
		PageTitle:   cfg.Title,
		HeaderText:  cfg.Header.Text,
		HeaderLinks: cfg.Header.LinksString(),
		HeaderLogo:  cfg.Header.Logo,
		FooterText:  cfg.Footer.Text,
		FooterLinks: cfg.Footer.LinksString(),
		FooterLogo:  cfg.Footer.Logo,
		GrpcURL:     cfg.GrpcURL,
		LGVersion:   utils.Version(),
		RtListMax:   cfg.RtListMax,
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
