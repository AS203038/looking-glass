package utils

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AS203038/looking-glass/pkg/logging"
	yaml "gopkg.in/yaml.v2"
)

// _version is the cached version string produced by [Version].
var _version = ""

// release is the build's release identifier, set via ldflags at build time.
var release = "untracked"

// Version returns the process version string. For ldflags-tagged
// builds this is the bare value of [release], which is stable across
// replicas of the same build so the RPC cache namespace, HTTP ETag,
// and Sentry release roll-up agree cluster-wide. For untagged
// development builds (release == "untracked") a per-process nanos
// suffix is appended so the HTTP ETag still rotates across local
// restarts and the operator's browser picks up edits.
func Version() string {
	if _version == "" {
		if release == "untracked" {
			_version = fmt.Sprintf("%s+%x", release, time.Now().UnixNano())
		} else {
			_version = release
		}
	}
	return _version
}

// Config is the root structure parsed from config.yaml at startup.
type Config struct {
	// Devices is the list of routers the server will expose.
	Devices []RouterConfig `yaml:"devices"`
	// Grpc controls the HTTP/2 listener.
	Grpc GrpcConfig `yaml:"grpc"`
	// Web controls the embedded WebUI.
	Web WebConfig `yaml:"web"`
	// SecurityTxt populates the /.well-known/security.txt endpoint.
	SecurityTxt SecurityTxtConfig `yaml:"security.txt"`
	// Redis configures the optional response cache.
	Redis RedisConfig `yaml:"redis"`
	// Logging controls the slog-based event stream.
	Logging logging.Config `yaml:"logging"`
}

// RouterConfig describes a single managed device.
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
	SSHKey string `yaml:"ssh_key"`
	// VRF is the routing instance interpolated into vendor templates.
	VRF string `yaml:"vrf"`
	// Location is the free-form site label.
	Location string `yaml:"location"`
	// Source4 is the IPv4 source address bound to ping/traceroute.
	Source4 *IPNet `yaml:"source4"`
	// Source6 is the IPv6 source address bound to ping/traceroute.
	Source6 *IPNet `yaml:"source6"`
	// Type is the registered router-template name.
	Type string `yaml:"type"`
}

// GrpcConfig controls the gRPC listener.
type GrpcConfig struct {
	// Enabled toggles the gRPC service mount.
	Enabled bool `yaml:"enabled"`
	// Listen is the bind address in "host:port" form.
	Listen string `yaml:"listen"`
	// TLS configures TLS termination.
	TLS TLSConfig `yaml:"tls"`
}

// TLSConfig configures TLS termination.
type TLSConfig struct {
	// Enabled toggles TLS.
	Enabled bool `yaml:"enabled"`
	// Cert is the path to a PEM-encoded certificate chain.
	Cert string `yaml:"cert"`
	// Key is the path to a PEM-encoded private key.
	Key string `yaml:"key"`
	// SelfSigned causes the server to generate a fresh in-memory certificate at startup.
	SelfSigned bool `yaml:"self_signed"`
}

// RedisConfig configures the optional shared response cache.
type RedisConfig struct {
	// Enabled toggles the cache middleware.
	Enabled bool `yaml:"enabled"`
	// URI is a redis:// or rediss:// URL.
	URI string `yaml:"uri"`
	// TTL is a time.ParseDuration string controlling cache lifetime.
	TTL string `yaml:"ttl"`
}

// WebConfig controls the embedded WebUI.
type WebConfig struct {
	// Enabled toggles serving of the embedded WebUI bundle.
	Enabled bool `yaml:"enabled"`
	// GrpcURL is the gRPC-Web endpoint the WebUI dials.
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

// SentryConfig describes the optional Sentry SDK configuration.
type SentryConfig struct {
	// Enabled toggles both server-side capture and the WebUI flag.
	Enabled bool `yaml:"enabled"`
	// DSN is the Sentry project DSN.
	DSN string `yaml:"dsn"`
	// Environment is a free-form tag attached to every event.
	Environment string `yaml:"environment"`
	// SampleRate is the traces-sample-rate in [0, 1].
	SampleRate float64 `yaml:"sample_rate"`
}

// HFBlock describes a header or footer content block.
type HFBlock struct {
	// Text is the plain-text label.
	Text string `yaml:"text"`
	// Logo is a URL to a logo image.
	Logo string `yaml:"logo"`
	// Links is the ordered list of branded links.
	Links []Link `yaml:"links"`
}

// LinksString serialises the link list to "name|href,name|href" form.
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
	// URL is the link target.
	URL string `yaml:"url"`
}

// ParseConfigYaml reads and unmarshals the configuration file at path and runs [ValidateConfig].
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

// SecurityTxtConfig holds the fields exposed via /.well-known/security.txt.
type SecurityTxtConfig struct {
	// Enabled toggles the /.well-known/security.txt handler.
	Enabled bool `yaml:"enabled"`
	// Contact is one or more contact URIs.
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
	// Expires is the RFC 3339 expiration timestamp; empty synthesises one year from now.
	Expires string `yaml:"expires"`
}

// String renders the config as an RFC 9116 security.txt document.
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

// ValidateConfig normalises a freshly-parsed [Config] in place: drops devices
// without a Hostname and supplies loopback defaults for missing Source4/Source6.
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
