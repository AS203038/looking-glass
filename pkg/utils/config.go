package utils

import (
	"fmt"
	"os"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

var _version = ""         // computed
var release = "untracked" // set by build tools

func Version() string {
	if _version == "" {
		_version = fmt.Sprintf("%s+%x", release, time.Now().UnixNano())
	}
	return _version
}

type Config struct {
	Devices     []RouterConfig    `yaml:"devices"`
	Grpc        GrpcConfig        `yaml:"grpc"`
	Web         WebConfig         `yaml:"web"`
	SecurityTxt SecurityTxtConfig `yaml:"security.txt"`
	Redis       RedisConfig       `yaml:"redis"`
}

type RouterConfig struct {
	Name     string `yaml:"name"`
	Hostname string `yaml:"hostname"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSHKey   string `yaml:"ssh_key"`
	VRF      string `yaml:"vrf"`
	Location string `yaml:"location"`
	Source4  *IPNet `yaml:"source4"`
	Source6  *IPNet `yaml:"source6"`
	Type     string `yaml:"type"`
}

type GrpcConfig struct {
	Enabled bool      `yaml:"enabled"`
	Listen  string    `yaml:"listen"`
	TLS     TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Cert       string `yaml:"cert"`
	Key        string `yaml:"key"`
	SelfSigned bool   `yaml:"self_signed"`
}

type RedisConfig struct {
	Enabled bool   `yaml:"enabled"`
	URI     string `yaml:"uri"`
	TTL     string `yaml:"ttl"`
}

type WebConfig struct {
	Enabled   bool         `yaml:"enabled"`
	GrpcURL   string       `yaml:"grpc_url"`
	Theme     string       `yaml:"theme"`
	Title     string       `yaml:"title"`
	Header    HFBlock      `yaml:"header"`
	Footer    HFBlock      `yaml:"footer"`
	RtListMax int          `yaml:"rt_list_max"`
	Sentry    SentryConfig `yaml:"sentry"`
}

type SentryConfig struct {
	Enabled     bool    `yaml:"enabled"`
	DSN         string  `yaml:"dsn"`
	Environment string  `yaml:"environment"`
	SampleRate  float64 `yaml:"sample_rate"`
}

type HFBlock struct {
	Text  string `yaml:"text"`
	Logo  string `yaml:"logo"`
	Links []Link `yaml:"links"`
}

func (hf *HFBlock) LinksString() string {
	var pre []string
	for _, link := range hf.Links {
		pre = append(pre, link.Text+"|"+link.URL)
	}
	return strings.Join(pre, ",")
}

type Link struct {
	Text string `yaml:"text"`
	URL  string `yaml:"url"`
}

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

type SecurityTxtConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Contact            string `yaml:"contact"`
	Canonical          string `yaml:"canonical"`
	Encryption         string `yaml:"encryption"`
	Acknowledgements   string `yaml:"acknowledgements"`
	PreferredLanguages string `yaml:"preferred-languages"`
	Policy             string `yaml:"policy"`
	Hiring             string `yaml:"hiring"`
	CSAF               string `yaml:"csaf"`
	Expires            string `yaml:"expires"`
}

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
