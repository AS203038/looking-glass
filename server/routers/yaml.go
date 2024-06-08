package routers

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"gitlab.as203038.net/AS203038/looking-glass/server/errs"
	"gitlab.as203038.net/AS203038/looking-glass/server/utils"
	yaml "gopkg.in/yaml.v2"
)

type _tpl_data struct {
	Cfg       *utils.RouterConfig
	IP        *utils.IPNet
	Community string
	ASPath    string
}

type Yaml struct {
	Path     string
	Template struct {
		Name string `yaml:"name"`
		Ping struct {
			Any  []string `yaml:"any"`
			IPv4 []string `yaml:"ipv4"`
			IPv6 []string `yaml:"ipv6"`
		} `yaml:"ping"`
		Traceroute struct {
			Any  []string `yaml:"any"`
			IPv4 []string `yaml:"ipv4"`
			IPv6 []string `yaml:"ipv6"`
		} `yaml:"traceroute"`
		BGP struct {
			Route     []string `yaml:"route"`
			Community []string `yaml:"community"`
			ASPath    []string `yaml:"aspath"`
		} `yaml:"bgp"`
	}
}

//go:embed all:*.yml
var compiledRouters embed.FS

func init() {
	// Load all routers
	files, err := compiledRouters.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		y := &Yaml{}
		y.Path = file.Name()
		yamlFile, err := compiledRouters.ReadFile(y.Path)
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(yamlFile, &y.Template)
		if err != nil {
			panic(err)
		}
		register(y.Template.Name, y)
		fmt.Printf("Router %s registered\n", y.Template.Name)
	}
}

func (rt *Yaml) _tpl(name string, data _tpl_data) ([]string, error) {
	var tpl []string
	var ret []string
	switch name {
	case "ping":
		if rt.Template.Ping.Any != nil {
			tpl = rt.Template.Ping.Any
		} else if data.IP.IsIPv4() {
			tpl = rt.Template.Ping.IPv4
		} else if data.IP.IsIPv6() {
			tpl = rt.Template.Ping.IPv6
		}
	case "traceroute":
		if rt.Template.Traceroute.Any != nil {
			tpl = rt.Template.Traceroute.Any
		} else if data.IP.IsIPv4() {
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

func (rt *Yaml) Ping(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("ping", _tpl_data{Cfg: cfg, IP: ip})
}

func (rt *Yaml) Traceroute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("traceroute", _tpl_data{Cfg: cfg, IP: ip})
}

func (rt *Yaml) BGPRoute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	return rt._tpl("bgp.route", _tpl_data{Cfg: cfg, IP: ip})
}

func (rt *Yaml) BGPCommunity(cfg *utils.RouterConfig, community string) ([]string, error) {
	return rt._tpl("bgp.community", _tpl_data{Cfg: cfg, Community: community})
}

func (rt *Yaml) BGPASPath(cfg *utils.RouterConfig, aspath string) ([]string, error) {
	return rt._tpl("bgp.aspath", _tpl_data{Cfg: cfg, ASPath: aspath})
}
