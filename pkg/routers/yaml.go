package routers

import (
	"bytes"
	"embed"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/utils"
	yaml "gopkg.in/yaml.v2"
)

// tplLogTag builds a credential-free identifier for a single render
// attempt, suitable for prefixing operator-facing log lines. The
// router type and operation are always included; the device name
// and VRF are added when a [utils.RouterConfig] is available so
// failures can be traced back to a specific configured device
// without leaking secrets.
func tplLogTag(routerType, op string, cfg *utils.RouterConfig) string {
	tag := "router_type=" + routerType + " op=" + op
	if cfg != nil {
		tag += " router=" + cfg.Name + " vrf=" + cfg.VRF
	}
	return tag
}

// _tpl_data is the data context handed to every vendor-template
// render. Template authors reference fields via `{{.Cfg.…}}`,
// `{{.IP.…}}`, `{{.Community}}`, etc.; unused fields are left at
// their zero values for operations that do not need them.
type _tpl_data struct {
	// Cfg is the per-device configuration (credentials, source
	// addresses, VRF, location, …).
	Cfg *utils.RouterConfig
	// IP is the validated IP/CIDR operand for ping, traceroute,
	// and bgp.route operations.
	IP *utils.IPNet
	// Community is the RFC 1997 standard BGP community in
	// "ASN:VALUE" form, populated for bgp.community lookups.
	Community string
	// LargeCommunity is the RFC 8092 large community in
	// "GLOBAL:LOCAL1:LOCAL2" form, populated for bgp.largecommunity
	// lookups.
	LargeCommunity string
	// ASPath is the (already sanitised) AS-path regex used by
	// bgp.aspath lookups. See [utils.SanitizeASPathRegex].
	ASPath string
}

// Yaml is the [utils.Router] implementation that backs every shipped
// vendor template. A single Yaml instance is parsed from disk at
// process start and stored in the registry; it is treated as
// immutable thereafter.
type Yaml struct {
	// Path is the source filename, used only for log messages.
	Path string
	// Template holds the unmarshalled template body. The nested
	// anonymous struct mirrors the YAML schema exactly so that
	// `yaml.Unmarshal` populates it directly.
	Template struct {
		// Name is the unique template identifier referenced from
		// [utils.RouterConfig.Type].
		Name string `yaml:"name"`
		// Ping holds the command sequences for ping operations,
		// keyed by IP family. When a family-specific list is empty
		// the family-agnostic `any` list is used instead.
		Ping struct {
			Any  []string `yaml:"any"`
			IPv4 []string `yaml:"ipv4"`
			IPv6 []string `yaml:"ipv6"`
		} `yaml:"ping"`
		// Traceroute mirrors the Ping shape, for traceroute
		// operations.
		Traceroute struct {
			Any  []string `yaml:"any"`
			IPv4 []string `yaml:"ipv4"`
			IPv6 []string `yaml:"ipv6"`
		} `yaml:"traceroute"`
		// BGP holds the four BGP query command sequences. These
		// are family-agnostic at the top level: where a vendor's
		// CLI requires different commands per family the template
		// uses `{{if eq .IP.Family "ipv4"}}…{{end}}` conditionals.
		BGP struct {
			Route          []string `yaml:"route"`
			Community      []string `yaml:"community"`
			LargeCommunity []string `yaml:"largecommunity"`
			ASPath         []string `yaml:"aspath"`
		} `yaml:"bgp"`
	}
}

// compiledRouters embeds every `*.yml` file in this package directory
// into the binary so that all shipped templates are available
// without any filesystem dependency.
//
//go:embed all:*.yml
var compiledRouters embed.FS

// init populates the global template registry. Two sources are
// consulted, in order:
//
//  1. The directory named by the ROUTER_DIR environment variable, if
//     set. Templates discovered here win — they fully replace any
//     bundled template with the same name. This is the supported
//     escape hatch for operators who need to ship a custom or
//     in-house vendor profile without forking the project.
//  2. The embedded `*.yml` files in this package. Each is registered
//     under its `name:` field unless an external template with the
//     same name was already loaded in step 1, in which case a
//     warning is logged and the embedded copy is ignored.
//
// Any parse / read / unmarshal error in this function panics
// because a malformed template renders the router that depends on
// it permanently inoperable; failing at startup is preferable to
// surfacing template errors on every request.
func init() {
	rd := os.Getenv("ROUTER_DIR")
	if rd != "" {
		files, err := os.ReadDir(rd)
		if err != nil {
			log.Panicf("ERROR: Could not Read directory %s: %+v", rd, err)
		}
		log.Printf("NOTICE: Loading routers from %s\n", rd)
		for _, file := range files {
			if file.IsDir() || (!strings.HasSuffix(file.Name(), ".yml") && !strings.HasSuffix(file.Name(), ".yaml")) {
				continue
			}
			y := &Yaml{Path: file.Name()}
			yamlFile, err := os.ReadFile(rd + "/" + y.Path)
			if err != nil {
				log.Panicf("ERROR: Could not Read file %s/%s: %+v", rd, y.Path, err)
			}
			err = yaml.Unmarshal(yamlFile, &y.Template)
			if err != nil {
				log.Panicf("ERROR: Could not Unmarshal file %s/%s: %+v", rd, y.Path, err)
			}
			if y.Template.Name == "" {
				log.Printf("ERROR: Router name cannot be empty (%s/%s)", rd, y.Path)
				continue
			}
			register(y.Template.Name, y)
			log.Printf("NOTICE: Router %s (%s/%s) registered\n", y.Template.Name, rd, y.Path)
		}
	}

	files, err := compiledRouters.ReadDir(".")
	if err != nil {
		log.Panicf("ERROR: Could not Read builtin directory: %+v", err)
	}
	for _, file := range files {
		y := &Yaml{Path: file.Name()}
		yamlFile, err := compiledRouters.ReadFile(y.Path)
		if err != nil {
			log.Panicf("ERROR: Could not Read file builtin:%s: %+v", y.Path, err)
		}
		err = yaml.Unmarshal(yamlFile, &y.Template)
		if err != nil {
			log.Panicf("ERROR: Could not Unmarshal file builtin:%s: %+v", y.Path, err)
		}
		if _, ok := _routers[y.Template.Name]; !ok {
			register(y.Template.Name, y)
			log.Printf("NOTICE: Router %s (builtin:%s) registered\n", y.Template.Name, y.Path)
		} else {
			log.Printf("WARNING: Router %s already registered\n", y.Template.Name)
		}
	}
}

// _tpl renders the command sequence for the named operation against
// data, returning one entry per command in the order declared by
// the template.
//
// Operation routing:
//
//   - "ping" / "traceroute" pick the family-specific list when the
//     operand has a definite [utils.IPFamily]; the family-agnostic
//     `any` list otherwise.
//   - "bgp.route", "bgp.community", "bgp.largecommunity",
//     "bgp.aspath" map directly onto the matching template field.
//
// Returns [errs.OperationUnknown] when the template does not define
// any commands for the requested operation, or when an individual
// command fails to parse or execute as a [text/template]. The
// underlying parse/execute error is logged server-side; clients
// only see the sentinel.
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
		log.Printf("TPL: operation not defined for router (%s)",
			tplLogTag(rt.Template.Name, name, data.Cfg))
		return nil, errs.OperationUnknown
	}
	for i, t := range tpl {
		var buf bytes.Buffer
		tt, err := template.New(t).Parse(t)
		if err != nil {
			log.Printf("TPL: parse failed (%s cmd_index=%d raw=%q): %v",
				tplLogTag(rt.Template.Name, name, data.Cfg), i, t, err)
			return nil, errs.OperationUnknown
		}
		err = tt.Execute(&buf, data)
		if err != nil {
			log.Printf("TPL: execute failed (%s cmd_index=%d raw=%q): %v",
				tplLogTag(rt.Template.Name, name, data.Cfg), i, t, err)
			return nil, errs.OperationUnknown
		}
		ret = append(ret, buf.String())
	}
	return ret, nil
}

// Ping renders the ping command sequence for the supplied router
// configuration and target. The family-specific template is used
// when available, falling back to the family-agnostic `any` list.
func (rt *Yaml) Ping(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("ping", _tpl_data{Cfg: cfg, IP: ip})
}

// Traceroute renders the traceroute command sequence for the
// supplied router configuration and target. Family selection is
// identical to [Yaml.Ping].
func (rt *Yaml) Traceroute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("traceroute", _tpl_data{Cfg: cfg, IP: ip})
}

// BGPRoute renders the BGP-route lookup command sequence. The
// template typically embeds `{{.IP.Family}}` so that exactly one
// command is issued for the operand's family rather than blindly
// querying both v4 and v6.
func (rt *Yaml) BGPRoute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("bgp.route", _tpl_data{Cfg: cfg, IP: ip})
}

// BGPCommunity renders the standard-community (RFC 1997) lookup
// command sequence for the supplied "ASN:VALUE" community string.
// Community-style queries are family-agnostic by nature, so most
// templates emit two commands here (one per family).
func (rt *Yaml) BGPCommunity(cfg *utils.RouterConfig, community string) ([]string, error) {
	return rt._tpl("bgp.community", _tpl_data{Cfg: cfg, Community: community})
}

// BGPLargeCommunity renders the RFC 8092 large-community lookup
// command sequence for the supplied
// "GLOBAL:LOCAL1:LOCAL2" community string.
func (rt *Yaml) BGPLargeCommunity(cfg *utils.RouterConfig, largeCommunity string) ([]string, error) {
	return rt._tpl("bgp.largecommunity", _tpl_data{Cfg: cfg, LargeCommunity: largeCommunity})
}

// BGPASPath renders the AS-path regex lookup command sequence.
// The aspath string must already have been validated by
// [utils.SanitizeASPathRegex] before reaching this method.
func (rt *Yaml) BGPASPath(cfg *utils.RouterConfig, aspath string) ([]string, error) {
	return rt._tpl("bgp.aspath", _tpl_data{Cfg: cfg, ASPath: aspath})
}
