package utils

import (
	"fmt"
	"os"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// _version is the cached, fully-qualified version string produced
// by [Version] on first call. It is computed lazily so that the
// nanosecond suffix reflects process start time, not import time.
var _version = "" // computed

// release holds the human-readable release identifier. It is set at
// build time via `-ldflags "-X .../utils.release=<value>"` and falls
// back to "untracked" for development builds produced by `go run`
// or a bare `go build` without ldflags.
var release = "untracked" // set by build tools

// Version returns a process-stable version string of the form
// "<release>+<nanos-hex>". The nanosecond suffix is used as the
// HTTP ETag for static assets so that every server start
// invalidates browser caches, even when the underlying release
// identifier has not changed.
func Version() string {
	if _version == "" {
		_version = fmt.Sprintf("%s+%x", release, time.Now().UnixNano())
	}
	return _version
}

// Config is the root structure parsed from `config.yaml` at startup.
// It is the single source of truth for every operator-tunable
// behaviour of the server; there is no other configuration channel
// (apart from the ROUTER_DIR environment variable consumed by the
// router-template loader).
type Config struct {
	// Devices is the list of routers the server will expose. Each
	// entry produces one [RouterInstance] at startup.
	Devices []RouterConfig `yaml:"devices"`
	// Grpc controls the HTTP/2 listener that serves the gRPC and
	// (optional) WebUI surfaces.
	Grpc GrpcConfig `yaml:"grpc"`
	// Web controls the embedded SvelteKit WebUI and its runtime
	// configuration payload.
	Web WebConfig `yaml:"web"`
	// SecurityTxt populates the RFC 9116 /.well-known/security.txt
	// endpoint when enabled.
	SecurityTxt SecurityTxtConfig `yaml:"security.txt"`
	// Redis configures the optional response cache shared across
	// horizontally-scaled replicas.
	Redis RedisConfig `yaml:"redis"`
}

// RouterConfig describes a single managed device. Credentials and
// source addresses are operator-provided; Type names a registered
// router template (see pkg/routers).
type RouterConfig struct {
	// Name is the human-readable identifier shown in the UI.
	Name string `yaml:"name"`
	// Hostname is the SSH target ("host" or "host:port").
	Hostname string `yaml:"hostname"`
	// Username is the SSH login.
	Username string `yaml:"username"`
	// Password is the SSH password. Ignored when SSHKey is set.
	Password string `yaml:"password"`
	// SSHKey is the path to a private key on the server filesystem.
	// When non-empty it takes precedence over Password.
	SSHKey string `yaml:"ssh_key"`
	// VRF is the routing instance interpolated into vendor templates
	// that scope commands to a non-default VRF.
	VRF string `yaml:"vrf"`
	// Location is the free-form site label used to group routers in
	// the WebUI router picker.
	Location string `yaml:"location"`
	// Source4 is the IPv4 source address bound to ping/traceroute
	// invocations. Defaults to 127.0.0.1 when omitted.
	Source4 *IPNet `yaml:"source4"`
	// Source6 is the IPv6 source address bound to ping/traceroute
	// invocations. Defaults to ::1 when omitted.
	Source6 *IPNet `yaml:"source6"`
	// Type is the registered router-template name (e.g. "frrouting",
	// "cisco_ios", "juniper_junos"). Resolved against the registry
	// in pkg/routers at startup.
	Type string `yaml:"type"`
}

// GrpcConfig controls the network listener that serves both the
// gRPC/ConnectRPC API and (when enabled) the embedded WebUI.
type GrpcConfig struct {
	// Enabled toggles the gRPC service mount. When false the WebUI
	// can still be served but the LookingGlassService is not
	// reachable.
	Enabled bool `yaml:"enabled"`
	// Listen is the bind address in "host:port" form.
	Listen string `yaml:"listen"`
	// TLS configures TLS termination. When disabled the listener
	// runs in h2c (HTTP/2 cleartext) mode.
	TLS TLSConfig `yaml:"tls"`
}

// TLSConfig configures TLS termination for the HTTP/2 listener.
type TLSConfig struct {
	// Enabled toggles TLS. When false the listener accepts h2c.
	Enabled bool `yaml:"enabled"`
	// Cert is the path to a PEM-encoded certificate chain.
	// Ignored when SelfSigned is true.
	Cert string `yaml:"cert"`
	// Key is the path to a PEM-encoded private key.
	// Ignored when SelfSigned is true.
	Key string `yaml:"key"`
	// SelfSigned causes the server to generate a fresh, in-memory
	// certificate at startup via [GenerateSelfSignedPair]. Useful
	// for local development.
	SelfSigned bool `yaml:"self_signed"`
}

// RedisConfig configures the optional shared response cache.
// Cache keys are MD5 hashes of `<path><body>` and entries are stored
// as JSON-encoded [http.CacheEntry] values.
type RedisConfig struct {
	// Enabled toggles the cache middleware.
	Enabled bool `yaml:"enabled"`
	// URI is a redis:// or rediss:// URL consumed by go-redis's
	// ParseURL helper.
	URI string `yaml:"uri"`
	// TTL is a Go [time.ParseDuration] string controlling how long
	// cached responses live before re-execution. Defaults to 60s
	// when malformed.
	TTL string `yaml:"ttl"`
}

// WebConfig controls the embedded WebUI and the runtime configuration
// payload (`/_app/env.js`) it loads on boot.
type WebConfig struct {
	// Enabled toggles serving of the embedded SvelteKit static
	// bundle plus the runtime env injector.
	Enabled bool `yaml:"enabled"`
	// GrpcURL is the gRPC-Web endpoint the WebUI dials. Leave empty
	// to use the same origin as the page.
	GrpcURL string `yaml:"grpc_url"`
	// Title is the document title shown in the browser tab.
	Title string `yaml:"title"`
	// Header populates the top-of-page brand block.
	Header HFBlock `yaml:"header"`
	// Footer populates the bottom-of-page link block.
	Footer HFBlock `yaml:"footer"`
	// Sentry configures the optional browser-side Sentry SDK.
	Sentry SentryConfig `yaml:"sentry"`
}

// SentryConfig describes the (optional) Sentry SDK configuration
// shared between the Go server's HTTP middleware and the WebUI.
type SentryConfig struct {
	// Enabled toggles both server-side capture and the runtime
	// flag emitted to the WebUI.
	Enabled bool `yaml:"enabled"`
	// DSN is the Sentry project DSN.
	DSN string `yaml:"dsn"`
	// Environment is a free-form tag attached to every event.
	Environment string `yaml:"environment"`
	// SampleRate is the traces-sample-rate in the range [0, 1].
	SampleRate float64 `yaml:"sample_rate"`
}

// HFBlock describes a header- or footer-style content block: a
// short text/logo plus a list of branded links.
type HFBlock struct {
	// Text is the plain-text label rendered next to Logo.
	Text string `yaml:"text"`
	// Logo is a URL to a logo image. May be empty.
	Logo string `yaml:"logo"`
	// Links is the ordered list of branded links rendered in the
	// block. See [Link].
	Links []Link `yaml:"links"`
}

// LinksString serialises the link list to the "name|href,name|href"
// shape consumed by the WebUI's runtime env parser. Kept here so the
// Go and TypeScript sides agree on a single grammar.
func (hf *HFBlock) LinksString() string {
	var pre []string
	for _, link := range hf.Links {
		pre = append(pre, link.Text+"|"+link.URL)
	}
	return strings.Join(pre, ",")
}

// Link is one entry in an [HFBlock]'s link list.
type Link struct {
	// Text is the visible link label.
	Text string `yaml:"text"`
	// URL is the link target. Rendered verbatim into the
	// HTML href attribute.
	URL string `yaml:"url"`
}

// ParseConfigYaml reads and unmarshals the configuration file at
// path, then runs [ValidateConfig] to apply defaults and strip
// malformed device entries. Returns the populated [Config] or the
// first IO/parse error encountered.
func ParseConfigYaml(path string) (*Config, error) {
	var config Config
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return nil, err
	}
	ValidateConfig(&config)
	return &config, nil
}

// SecurityTxtConfig holds the fields exposed via RFC 9116
// /.well-known/security.txt. Empty fields are still emitted as
// blank values so that consumers see a deterministic shape.
type SecurityTxtConfig struct {
	// Enabled toggles the /.well-known/security.txt handler.
	Enabled bool `yaml:"enabled"`
	// Contact is one or more contact URIs (mailto:, tel:, https:).
	Contact string `yaml:"contact"`
	// Canonical is the canonical URL where this security.txt lives.
	Canonical string `yaml:"canonical"`
	// Encryption is a URL pointing to a PGP/OpenPGP key.
	Encryption string `yaml:"encryption"`
	// Acknowledgements is a URL listing security-researcher credits.
	Acknowledgements string `yaml:"acknowledgements"`
	// PreferredLanguages is a comma-separated list of RFC 5646 tags.
	PreferredLanguages string `yaml:"preferred-languages"`
	// Policy is a URL to the organisation's vulnerability policy.
	Policy string `yaml:"policy"`
	// Hiring is a URL to a security-jobs page.
	Hiring string `yaml:"hiring"`
	// CSAF is a URL to a CSAF provider-metadata document.
	CSAF string `yaml:"csaf"`
	// Expires is the RFC 3339 expiration timestamp. When empty the
	// String method synthesises "one year from now".
	Expires string `yaml:"expires"`
}

// String renders the config as an RFC 9116 security.txt document.
// When Expires is empty an automatic expiration one year in the
// future is generated so that the served document is never stale by
// RFC 9116 standards.
func (s *SecurityTxtConfig) String() string {
	exp := s.Expires
	if exp == "" {
		exp = time.Now().AddDate(1, 0, 0).Format(time.RFC3339)
	}
	return strings.Join([]string{
		"Contact: " + s.Contact,
		"Expires: " + exp,
		"Encryption: " + s.Encryption,
		"Acknowledgements: " + s.Acknowledgements,
		"Preferred-Languages: " + s.PreferredLanguages,
		"Canonical: " + s.Canonical,
		"Policy: " + s.Policy,
		"Hiring: " + s.Hiring,
		"CSAF: " + s.CSAF,
	}, "\n")
}

// ValidateConfig normalises a freshly-parsed [Config] in place:
//
//   - Devices missing a Hostname are dropped (a hostname-less router
//     is unusable and would only fail at runtime).
//   - Devices missing a Source4 or Source6 receive loopback defaults
//     so that vendor templates which interpolate `{{.Cfg.Source4}}`
//     still render successfully.
//
// The function never returns an error; malformed device entries are
// silently discarded so the server can still start with whatever
// remains valid.
func ValidateConfig(c *Config) {
	for k, v := range c.Devices {
		if v.Hostname == "" {
			c.Devices = append(c.Devices[:k], c.Devices[k+1:]...)
		}
		if v.Source4 == nil {
			c.Devices[k].Source4, _ = NewIPNET("127.0.0..1")
		}
		if v.Source6 == nil {
			c.Devices[k].Source6, _ = NewIPNET("::1")
		}
	}
}
