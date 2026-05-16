package routers

import (
	"bytes"
	"context"
	"embed"
	"log/slog"
	"os"
	"strings"
	"text/template"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/routers/parse"
	"github.com/AS203038/looking-glass/pkg/utils"
	yaml "gopkg.in/yaml.v2"
)

// tplLog is the component-tagged logger for template rendering events.
var tplLog = logging.Component("tpl")

// yamlLog is the component-tagged logger for YAML-loader events.
var yamlLog = logging.Component("yaml")

// Exit codes for fatal YAML-loader failures.
const (
	exitRouterDirRead     = 20
	exitRouterDirReadFile = 21
	exitRouterDirParse    = 22
	exitBuiltinDirRead    = 23
	exitBuiltinReadFile   = 24
	exitBuiltinParse      = 25
)

// tplAttrs builds the slog attributes describing a single render attempt.
func tplAttrs(routerType, op string, cfg *utils.RouterConfig) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("router_type", routerType),
		slog.String("op", op),
	}
	if cfg != nil {
		attrs = append(attrs,
			slog.String("router", cfg.Name),
			slog.String("vrf", cfg.VRF))
	}
	return attrs
}

// _tpl_data is the data context handed to every vendor-template render.
type _tpl_data struct {
	// Cfg is the per-device configuration.
	Cfg *utils.RouterConfig
	// IP is the validated IP/CIDR operand.
	IP *utils.IPNet
	// Community is the RFC 1997 standard BGP community in "ASN:VALUE" form.
	Community string
	// LargeCommunity is the RFC 8092 large community in "GLOBAL:LOCAL1:LOCAL2" form.
	LargeCommunity string
	// ASPath is the sanitised AS-path regex.
	ASPath string
}

// parserSpec is the YAML projection of a per-operation parser declaration.
type parserSpec struct {
	// Kind selects the parser pipe: "raw", "textfsm", "native_json", or "builtin".
	Kind string `yaml:"kind"`
	// Template names the TextFSM template asset; used when Kind == "textfsm".
	Template string `yaml:"template"`
	// Schema selects the JSON normalisation schema; used when Kind == "native_json".
	Schema string `yaml:"schema"`
}

// Yaml is the [utils.Router] implementation that backs every shipped vendor template.
type Yaml struct {
	// Path is the source filename, used only for log messages.
	Path string
	// Template holds the unmarshalled template body.
	Template struct {
		// Name is the unique template identifier.
		Name string `yaml:"name"`
		// Ping holds the ping command sequences, keyed by IP family.
		Ping struct {
			Any  []string `yaml:"any"`
			IPv4 []string `yaml:"ipv4"`
			IPv6 []string `yaml:"ipv6"`
		} `yaml:"ping"`
		// Traceroute holds the traceroute command sequences, keyed by IP family.
		Traceroute struct {
			Any  []string `yaml:"any"`
			IPv4 []string `yaml:"ipv4"`
			IPv6 []string `yaml:"ipv6"`
		} `yaml:"traceroute"`
		// BGP holds the BGP query command sequences.
		BGP struct {
			// Summary is the neighbour-summary command sequence.
			Summary        []string `yaml:"summary"`
			Route          []string `yaml:"route"`
			Community      []string `yaml:"community"`
			LargeCommunity []string `yaml:"largecommunity"`
			ASPath         []string `yaml:"aspath"`
		} `yaml:"bgp"`
		// Parsers is the per-operation parser declaration map.
		Parsers map[string]parserSpec `yaml:"parsers"`
	}
}

// compiledRouters embeds every `*.yml` file in this package directory.
//
//go:embed all:*.yml
var compiledRouters embed.FS

// init populates the global template registry, first from the optional
// ROUTER_DIR directory (where loaded templates win on name collision)
// and then from the bundled `*.yml` files. Exits the process on any
// parse error.
func init() {
	rd := os.Getenv("ROUTER_DIR")
	if rd != "" {
		files, err := os.ReadDir(rd)
		if err != nil {
			yamlLog.Error("could not read router dir",
				slog.String("dir", rd), slog.Any("err", err))
			os.Exit(exitRouterDirRead)
		}
		yamlLog.Info("loading routers from dir", slog.String("dir", rd))
		for _, file := range files {
			if file.IsDir() || (!strings.HasSuffix(file.Name(), ".yml") && !strings.HasSuffix(file.Name(), ".yaml")) {
				continue
			}
			y := &Yaml{Path: file.Name()}
			yamlFile, err := os.ReadFile(rd + "/" + y.Path)
			if err != nil {
				yamlLog.Error("could not read router file",
					slog.String("dir", rd),
					slog.String("path", y.Path),
					slog.Any("err", err))
				os.Exit(exitRouterDirReadFile)
			}
			err = yaml.Unmarshal(yamlFile, &y.Template)
			if err != nil {
				yamlLog.Error("could not unmarshal router file",
					slog.String("dir", rd),
					slog.String("path", y.Path),
					slog.Any("err", err))
				os.Exit(exitRouterDirParse)
			}
			if y.Template.Name == "" {
				yamlLog.Error("router name cannot be empty",
					slog.String("dir", rd),
					slog.String("path", y.Path))
				continue
			}
			register(y.Template.Name, y)
			yamlLog.Info("router registered",
				slog.String("router", y.Template.Name),
				slog.String("source", rd+"/"+y.Path))
		}
	}

	files, err := compiledRouters.ReadDir(".")
	if err != nil {
		yamlLog.Error("could not read builtin dir", slog.Any("err", err))
		os.Exit(exitBuiltinDirRead)
	}
	for _, file := range files {
		y := &Yaml{Path: file.Name()}
		yamlFile, err := compiledRouters.ReadFile(y.Path)
		if err != nil {
			yamlLog.Error("could not read builtin file",
				slog.String("path", y.Path),
				slog.Any("err", err))
			os.Exit(exitBuiltinReadFile)
		}
		err = yaml.Unmarshal(yamlFile, &y.Template)
		if err != nil {
			yamlLog.Error("could not unmarshal builtin file",
				slog.String("path", y.Path),
				slog.Any("err", err))
			os.Exit(exitBuiltinParse)
		}
		if _, ok := _routers[y.Template.Name]; !ok {
			register(y.Template.Name, y)
			yamlLog.Info("router registered",
				slog.String("router", y.Template.Name),
				slog.String("source", "builtin:"+y.Path))
		} else {
			yamlLog.Warn("router already registered; skipping builtin",
				slog.String("router", y.Template.Name),
				slog.String("source", "builtin:"+y.Path))
		}
	}
}

// _tpl renders the command sequence for the named operation against data.
// Returns [errs.OperationUnknown] when the template defines no commands
// for the operation or when an individual command fails to render.
func (rt *Yaml) _tpl(name string, data _tpl_data) ([]string, error) {
	var tpl []string
	var ret []string
	switch name {
	case "ping":
		tpl = rt.Template.Ping.Any
		if data.IP.IsIPv4() {
			tpl = rt.Template.Ping.IPv4
		} else if data.IP.IsIPv6() {
			tpl = rt.Template.Ping.IPv6
		}
	case "traceroute":
		tpl = rt.Template.Traceroute.Any
		if data.IP.IsIPv4() {
			tpl = rt.Template.Traceroute.IPv4
		} else if data.IP.IsIPv6() {
			tpl = rt.Template.Traceroute.IPv6
		}
	case "bgp.summary":
		tpl = rt.Template.BGP.Summary
	case "bgp.route":
		tpl = rt.Template.BGP.Route
	case "bgp.community":
		tpl = rt.Template.BGP.Community
	case "bgp.largecommunity":
		tpl = rt.Template.BGP.LargeCommunity
	case "bgp.aspath":
		tpl = rt.Template.BGP.ASPath
	}
	if tpl == nil {
		attrs := tplAttrs(rt.Template.Name, name, data.Cfg)
		tplLog.LogAttrs(context.Background(), slog.LevelWarn, "operation not defined for router", attrs...)
		return nil, errs.OperationUnknown
	}
	for i, t := range tpl {
		var buf bytes.Buffer
		tt, err := template.New(t).Parse(t)
		if err != nil {
			attrs := tplAttrs(rt.Template.Name, name, data.Cfg)
			attrs = append(attrs,
				slog.Int("cmd_index", i),
				slog.String("raw", t),
				slog.Any("err", err))
			tplLog.LogAttrs(context.Background(), slog.LevelError, "parse failed", attrs...)
			return nil, errs.OperationUnknown
		}
		err = tt.Execute(&buf, data)
		if err != nil {
			attrs := tplAttrs(rt.Template.Name, name, data.Cfg)
			attrs = append(attrs,
				slog.Int("cmd_index", i),
				slog.String("raw", t),
				slog.Any("err", err))
			tplLog.LogAttrs(context.Background(), slog.LevelError, "execute failed", attrs...)
			return nil, errs.OperationUnknown
		}
		rendered := buf.String()
		debugAttrs := tplAttrs(rt.Template.Name, name, data.Cfg)
		debugAttrs = append(debugAttrs,
			slog.Int("cmd_index", i),
			slog.String("rendered", rendered))
		tplLog.LogAttrs(context.Background(), slog.LevelDebug, "tpl render", debugAttrs...)
		ret = append(ret, rendered)
	}
	return ret, nil
}

// Ping renders the ping command sequence for cfg and ip.
func (rt *Yaml) Ping(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("ping", _tpl_data{Cfg: cfg, IP: ip})
}

// Traceroute renders the traceroute command sequence for cfg and ip.
func (rt *Yaml) Traceroute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("traceroute", _tpl_data{Cfg: cfg, IP: ip})
}

// BGPSummary renders the neighbour-summary command sequence for cfg.
func (rt *Yaml) BGPSummary(cfg *utils.RouterConfig) ([]string, error) {
	return rt._tpl("bgp.summary", _tpl_data{Cfg: cfg})
}

// BGPRoute renders the BGP-route lookup command sequence for cfg and ip.
func (rt *Yaml) BGPRoute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("bgp.route", _tpl_data{Cfg: cfg, IP: ip})
}

// BGPCommunity renders the standard-community lookup command sequence.
func (rt *Yaml) BGPCommunity(cfg *utils.RouterConfig, community string) ([]string, error) {
	return rt._tpl("bgp.community", _tpl_data{Cfg: cfg, Community: community})
}

// BGPLargeCommunity renders the large-community lookup command sequence.
func (rt *Yaml) BGPLargeCommunity(cfg *utils.RouterConfig, largeCommunity string) ([]string, error) {
	return rt._tpl("bgp.largecommunity", _tpl_data{Cfg: cfg, LargeCommunity: largeCommunity})
}

// BGPASPath renders the AS-path regex lookup command sequence.
func (rt *Yaml) BGPASPath(cfg *utils.RouterConfig, aspath string) ([]string, error) {
	return rt._tpl("bgp.aspath", _tpl_data{Cfg: cfg, ASPath: aspath})
}

// Parser returns the parser declared for op together with its
// parser-private configuration. Templates that omit a `parsers:`
// entry return [parse.RawParser]. Unknown parser kinds fall back to raw.
func (rt *Yaml) Parser(op string) (parse.Parser, parse.Config) {
	spec, ok := rt.Template.Parsers[op]
	if !ok {
		return parse.RawParser{}, parse.Config{}
	}
	cfg := parse.Config{Template: spec.Template, Schema: spec.Schema}
	switch spec.Kind {
	case "", "raw":
		return parse.RawParser{}, parse.Config{}
	case "textfsm":
		return parse.TextFSMParser{}, cfg
	case "native_json":
		return parse.JSONParser{}, cfg
	case "builtin":
		return parse.BuiltinParser{}, cfg
	default:
		yamlLog.Warn("unknown parser kind; falling back to raw",
			slog.String("kind", spec.Kind),
			slog.String("router_type", rt.Template.Name),
			slog.String("op", op))
		return parse.RawParser{}, parse.Config{}
	}
}
