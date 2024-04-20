package webui

import (
	"encoding/json"
	"net/http"

	"gitlab.as203038.net/AS203038/looking-glass/server/utils"
)

type EnvJS struct {
	Theme       string `json:"PUBLIC_THEME"`
	PageTitle   string `json:"PUBLIC_PAGE_TITLE"`
	HeaderText  string `json:"PUBLIC_HEADER_TEXT"`
	HeaderLinks string `json:"PUBLIC_HEADER_LINKS"`
	HeaderLogo  string `json:"PUBLIC_HEADER_LOGO"`
	FooterText  string `json:"PUBLIC_FOOTER_TEXT"`
	FooterLinks string `json:"PUBLIC_FOOTER_LINKS"`
	FooterLogo  string `json:"PUBLIC_FOOTER_LOGO"`
	GrpcURL     string `json:"PUBLIC_GRPC_URL"`
}

func ConfigInjector(cfg utils.WebConfig) http.Handler {
	envjson, err := json.Marshal(EnvJS{
		Theme:       cfg.Theme,
		PageTitle:   cfg.Title,
		HeaderText:  cfg.Header.Text,
		HeaderLinks: cfg.Header.LinksString(),
		HeaderLogo:  cfg.Header.Logo,
		FooterText:  cfg.Footer.Text,
		FooterLinks: cfg.Footer.LinksString(),
		FooterLogo:  cfg.Footer.Logo,
		GrpcURL:     cfg.GrpcURL,
	})
	if err != nil {
		panic(err)
	}
	envjs := "export const env = " + string(envjson)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(envjs))
	})
}
