package routers

import (
	"bytes"
	"embed"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/AS203038/looking-glass/server/errs"
	"github.com/AS203038/looking-glass/server/utils"
	yaml "gopkg.in/yaml.v2"
)

// _tpl_data represents the template data used in the router YAML file.
type _tpl_data struct {
	Cfg       *utils.RouterConfig // Cfg holds the router configuration.
	IP        *utils.IPNet        // IP holds the IP network information.
	Community string              // Community holds the community string.
	ASPath    string              // ASPath holds the AS path information.
}

// Yaml represents the structure of a YAML file.
type Yaml struct {
	Path     string // Path represents the file path.
	Template struct {
		Name string `yaml:"name"` // Name represents the template name.
		Ping struct {
			Any  []string `yaml:"any"`  // Any represents the list of ping targets for any IP address.
			IPv4 []string `yaml:"ipv4"` // IPv4 represents the list of ping targets for IPv4 addresses.
			IPv6 []string `yaml:"ipv6"` // IPv6 represents the list of ping targets for IPv6 addresses.
		} `yaml:"ping"` // Ping represents the ping section in the template.
		Traceroute struct {
			Any  []string `yaml:"any"`  // Any represents the list of traceroute targets for any IP address.
			IPv4 []string `yaml:"ipv4"` // IPv4 represents the list of traceroute targets for IPv4 addresses.
			IPv6 []string `yaml:"ipv6"` // IPv6 represents the list of traceroute targets for IPv6 addresses.
		} `yaml:"traceroute"` // Traceroute represents the traceroute section in the template.
		BGP struct {
			Route     []string `yaml:"route"`     // Route represents the list of BGP routes.
			Community []string `yaml:"community"` // Community represents the list of BGP communities.
			ASPath    []string `yaml:"aspath"`    // ASPath represents the list of BGP AS paths.
		} `yaml:"bgp"` // BGP represents the BGP section in the template.
	}
}

//go:embed all:*.yml
var compiledRouters embed.FS

// init is a function that is automatically called before the program starts.
// It initializes the routers by loading them from the specified directory and bundled routers.
// It reads YAML files, unmarshals them into router templates, and registers the routers.
// If the ROUTER_DIR environment variable is set, it loads routers from the specified directory.
// If the bundled routers exist, it loads them as well.
// The function logs the registration status of each router.
func init() {
	// Check for ROUTER_DIR environment variable
	rd := os.Getenv("ROUTER_DIR")
	if rd != "" {
		// Load all routers from ROUTER_DIR
		files, err := os.ReadDir(rd)
		if err != nil {
			log.Panicf("Could not Read directory %s: %+v", rd, err)
		}
		log.Printf("Loading routers from %s\n", rd)
		for _, file := range files {
			if file.IsDir() || (!strings.HasSuffix(file.Name(), ".yml") && !strings.HasSuffix(file.Name(), ".yaml")) {
				continue
			}
			// Rest of the code for processing the file
			y := &Yaml{Path: file.Name()}
			yamlFile, err := os.ReadFile(rd + "/" + y.Path)
			if err != nil {
				log.Panicf("Could not Read file %s/%s: %+v", rd, y.Path, err)
			}
			err = yaml.Unmarshal(yamlFile, &y.Template)
			if err != nil {
				log.Panicf("Could not Unmarshal file %s/%s: %+v", rd, y.Path, err)
			}
			if y.Template.Name == "" {
				log.Printf("Router name cannot be empty (%s/%s)", rd, y.Path)
				continue
			}
			register(y.Template.Name, y)
			log.Printf("Router %s (%s/%s) registered\n", y.Template.Name, rd, y.Path)
		}
	}

	// Load all bundled routers
	files, err := compiledRouters.ReadDir(".")
	if err != nil {
		log.Panicf("Could not Read builtin directory: %+v", err)
	}
	for _, file := range files {
		y := &Yaml{Path: file.Name()}
		yamlFile, err := compiledRouters.ReadFile(y.Path)
		if err != nil {
			log.Panicf("Could not Read file builtin:%s: %+v", y.Path, err)
		}
		err = yaml.Unmarshal(yamlFile, &y.Template)
		if err != nil {
			log.Panicf("Could not Unmarshal file builtin:%s: %+v", y.Path, err)
		}
		if _, ok := _routers[y.Template.Name]; !ok {
			register(y.Template.Name, y)
			log.Printf("Router %s (builtin:%s) registered\n", y.Template.Name, y.Path)
		} else {
			log.Printf("Router %s already registered\n", y.Template.Name)
		}
	}
}

// _tpl is a helper function used to generate a list of strings based on the provided template name and data.
// It takes a template name and data as input and returns a list of strings generated from the template.
// The function first determines the appropriate template to use based on the template name and the IP version in the data.
// It then iterates over the selected template(s), executes them with the provided data, and appends the generated strings to the result list.
// If no template is found for the given name, the function returns nil and an error of type `errs.OperationUnknown`.
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
	case "bgp.aspath":
		tpl = rt.Template.BGP.ASPath
	}
	if tpl == nil {
		return nil, errs.OperationUnknown
	}
	for _, t := range tpl {
		var buf bytes.Buffer
		tt, err := template.New(t).Parse(t)
		if err != nil {
			return nil, err
		}
		err = tt.Execute(&buf, data)
		if err != nil {
			return nil, err
		}
		ret = append(ret, buf.String())
	}
	return ret, nil
}

// Ping sends a ping request to the specified IP address using the provided router configuration.
// It returns a slice of strings representing the ping response and an error if any.
func (rt *Yaml) Ping(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("ping", _tpl_data{Cfg: cfg, IP: ip})
}

// Traceroute performs a traceroute operation using the provided router configuration and IP address.
// It returns a slice of strings representing the traceroute results and an error if any.
func (rt *Yaml) Traceroute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("traceroute", _tpl_data{Cfg: cfg, IP: ip})
}

// BGPRoute generates BGP route configuration based on the provided RouterConfig and IPNet.
// It returns a slice of strings representing the generated configuration and an error if any.
func (rt *Yaml) BGPRoute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("bgp.route", _tpl_data{Cfg: cfg, IP: ip})
}

// BGPCommunity returns a list of strings representing the BGP community values for the given router configuration and community.
func (rt *Yaml) BGPCommunity(cfg *utils.RouterConfig, community string) ([]string, error) {
	return rt._tpl("bgp.community", _tpl_data{Cfg: cfg, Community: community})
}

// BGPASPath returns a slice of strings representing the BGP AS path for the given router configuration and AS path string.
// It uses the "_tpl" method to render the "bgp.aspath" template with the provided configuration and AS path.
func (rt *Yaml) BGPASPath(cfg *utils.RouterConfig, aspath string) ([]string, error) {
	return rt._tpl("bgp.aspath", _tpl_data{Cfg: cfg, ASPath: aspath})
}
