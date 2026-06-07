package routers

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/utils"
)

// TestYamlRendering verifies that YAML command templates render correctly for both IPv4 and IPv6.
func TestYamlRendering(t *testing.T) {
	frrRouter := Get("frrouting")
	if frrRouter == nil {
		t.Fatal("frrouting template not found in registry")
	}

	aristaRouter := Get("arista_eos")
	if aristaRouter == nil {
		t.Fatal("arista_eos template not found in registry")
	}

	cfg := &utils.RouterConfig{
		Name:    "test-router",
		VRF:     "MGMT",
		Source4: &utils.IPNet{IP: "192.0.2.1"},
		Source6: &utils.IPNet{IP: "2001:db8::1"},
	}

	ip4, err := utils.NewIPNET("1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}

	ip6, err := utils.NewIPNET("2606:4700:4700::1111")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Ping Rendering", func(t *testing.T) {
		cmds, err := frrRouter.Ping(cfg, ip4)
		if err != nil {
			t.Fatalf("Ping IPv4 rendering failed: %v", err)
		}
		if len(cmds) != 1 || !strings.Contains(cmds[0], "-I 192.0.2.1 1.1.1.1") {
			t.Errorf("Unexpected Ping IPv4 commands: %v", cmds)
		}

		cmds, err = frrRouter.Ping(cfg, ip6)
		if err != nil {
			t.Fatalf("Ping IPv6 rendering failed: %v", err)
		}
		if len(cmds) != 1 || !strings.Contains(cmds[0], "-I 2001:db8::1 2606:4700:4700::1111") {
			t.Errorf("Unexpected Ping IPv6 commands: %v", cmds)
		}
	})

	t.Run("Traceroute Rendering", func(t *testing.T) {
		cmds, err := frrRouter.Traceroute(cfg, ip4)
		if err != nil {
			t.Fatalf("Traceroute IPv4 rendering failed: %v", err)
		}
		if len(cmds) != 1 || !strings.Contains(cmds[0], "-s 192.0.2.1 1.1.1.1") {
			t.Errorf("Unexpected Traceroute IPv4 commands: %v", cmds)
		}
	})

	t.Run("BGP Summary Rendering", func(t *testing.T) {
		cmds, err := frrRouter.BGPSummary(cfg)
		if err != nil {
			t.Fatalf("BGPSummary rendering failed: %v", err)
		}
		if len(cmds) != 2 || !strings.Contains(cmds[0], "show bgp vrf MGMT ipv4 unicast summary json") {
			t.Errorf("Unexpected BGPSummary commands: %v", cmds)
		}
	})

	t.Run("BGP Route Rendering", func(t *testing.T) {
		cmds, err := frrRouter.BGPRoute(cfg, ip4)
		if err != nil {
			t.Fatalf("BGPRoute rendering failed: %v", err)
		}
		if len(cmds) != 1 || !strings.Contains(cmds[0], "show bgp vrf MGMT ipv4 unicast 1.1.1.1 json") {
			t.Errorf("Unexpected BGPRoute commands: %v", cmds)
		}
	})

	t.Run("BGP Community Rendering", func(t *testing.T) {
		cmds, err := frrRouter.BGPCommunity(cfg, "65000:100")
		if err != nil {
			t.Fatalf("BGPCommunity rendering failed: %v", err)
		}
		if len(cmds) != 2 || !strings.Contains(cmds[0], "community 65000:100 json") {
			t.Errorf("Unexpected BGPCommunity commands: %v", cmds)
		}
	})

	t.Run("BGP Large Community Rendering", func(t *testing.T) {
		cmds, err := frrRouter.BGPLargeCommunity(cfg, "65000:100:200")
		if err != nil {
			t.Fatalf("BGPLargeCommunity rendering failed: %v", err)
		}
		if len(cmds) != 2 || !strings.Contains(cmds[0], "large-community 65000:100:200 json") {
			t.Errorf("Unexpected BGPLargeCommunity commands: %v", cmds)
		}
	})

	t.Run("BGP ASPath Rendering", func(t *testing.T) {
		cmds, err := frrRouter.BGPASPath(cfg, "_65000$")
		if err != nil {
			t.Fatalf("BGPASPath rendering failed: %v", err)
		}
		if len(cmds) != 2 || !strings.Contains(cmds[0], "regexp _65000$ json") {
			t.Errorf("Unexpected BGPASPath commands: %v", cmds)
		}
	})

	t.Run("Parser Selection", func(t *testing.T) {
		frrConcrete, ok := frrRouter.(*Yaml)
		if !ok {
			t.Fatal("expected frrouting router to be of concrete type *Yaml")
		}

		p, pcfg := frrConcrete.Parser("ping")
		if p.Name() != "builtin" {
			t.Errorf("Expected builtin parser for frrouting ping, got %q", p.Name())
		}
		_ = pcfg

		p, _ = frrConcrete.Parser("bgp.summary")
		if p.Name() != "native_json" {
			t.Errorf("Expected native_json parser for frrouting bgp.summary, got %q", p.Name())
		}

		p, _ = frrConcrete.Parser("nonexistent")
		if p.Name() != "raw" {
			t.Errorf("Expected raw fallback parser for nonexistent operation, got %q", p.Name())
		}
	})

	t.Run("BGPPeerRoutes Rendering", func(t *testing.T) {
		birdRouter := Get("bird")
		if birdRouter == nil {
			t.Fatal("bird template not found in registry")
		}
		cmds, err := birdRouter.BGPPeerRoutes(cfg, "192.0.2.10", "peer_member", "received")
		if err != nil {
			t.Fatalf("BGPPeerRoutes received rendering failed: %v", err)
		}
		if len(cmds) != 1 || !strings.Contains(cmds[0], "protocol peer_member all") {
			t.Errorf("Unexpected BGPPeerRoutes commands: %v", cmds)
		}

		_, _ = birdRouter.BGPPeerRoutes(cfg, "192.0.2.10", "peer_member", "rejected")
		_, _ = birdRouter.BGPPeerRoutes(cfg, "192.0.2.10", "peer_member", "accepted")
		_, _ = birdRouter.BGPPeerRoutes(cfg, "192.0.2.10", "peer_member", "advertised")
	})

	t.Run("Custom Yaml and fallbacks", func(t *testing.T) {
		y := &Yaml{}
		y.Template.Name = "custom-mock"
		y.Template.Parsers = map[string]parserSpec{
			"testop":     {Kind: "invalid-kind"},
			"textfsm_op": {Kind: "textfsm", Template: "my_template"},
			"raw_op":     {Kind: "raw"},
			"empty_op":   {Kind: ""},
		}
		p, _ := y.Parser("testop")
		if p.Name() != "raw" {
			t.Errorf("expected raw parser for invalid-kind, got %q", p.Name())
		}
		p, _ = y.Parser("textfsm_op")
		if p.Name() != "textfsm" {
			t.Errorf("expected textfsm, got %q", p.Name())
		}
		p, _ = y.Parser("raw_op")
		if p.Name() != "raw" {
			t.Errorf("expected raw, got %q", p.Name())
		}
		p, _ = y.Parser("empty_op")
		if p.Name() != "raw" {
			t.Errorf("expected raw for empty kind, got %q", p.Name())
		}
	})
}

// TestCustomRouterDirDirect verifies custom template directory loading.
func TestCustomRouterDirDirect(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lg-router-dir-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mockYaml := `
name: custom_mock_vendor
ping:
  any:
    - ping -c1 {{.IP.IP}}
`
	err = os.WriteFile(tmpDir+"/custom_mock_vendor.yml", []byte(mockYaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	emptyNameYaml := `
name: ""
`
	err = os.WriteFile(tmpDir+"/empty_name.yml", []byte(emptyNameYaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(tmpDir+"/invalid_ext.txt", []byte("some content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(tmpDir+"/sub_dir", 0755)
	if err != nil {
		t.Fatal(err)
	}

	loadCustomDir(tmpDir)
}

// TestLoadBuiltinDuplicate verifies skip behavior on duplicate template loading.
func TestLoadBuiltinDuplicate(t *testing.T) {
	loadBuiltin()
}

// Test_tpl_ErrorsAndFuncs verifies template rendering error paths and functions.
func Test_tpl_ErrorsAndFuncs(t *testing.T) {
	frr := Get("frrouting")
	frrConcrete, ok := frr.(*Yaml)
	if !ok {
		t.Fatal("expected concrete *Yaml type")
	}

	cfg := &utils.RouterConfig{Name: "test"}
	_, err := frrConcrete._tpl("nonexistent_op", _tpl_data{Cfg: cfg})
	if !errors.Is(err, errs.OperationUnknown) {
		t.Errorf("expected OperationUnknown for missing op, got %v", err)
	}

	ip4, _ := utils.NewIPNET("1.1.1.1")

	badParseYaml := &Yaml{}
	badParseYaml.Template.Name = "bad-parse"
	badParseYaml.Template.Ping.IPv4 = []string{"ping {{invalid syntax}"}
	_, err = badParseYaml._tpl("ping", _tpl_data{Cfg: cfg, IP: ip4})
	if !errors.Is(err, errs.OperationUnknown) {
		t.Errorf("expected OperationUnknown for parse error, got %v", err)
	}

	badExecYaml := &Yaml{}
	badExecYaml.Template.Name = "bad-exec"
	badExecYaml.Template.Ping.IPv4 = []string{"ping {{bird_community .Cfg}}"}
	_, err = badExecYaml._tpl("ping", _tpl_data{Cfg: cfg, IP: ip4})
	if !errors.Is(err, errs.OperationUnknown) {
		t.Errorf("expected OperationUnknown for execution error, got %v", err)
	}

	funcsYaml := &Yaml{}
	funcsYaml.Template.Name = "funcs-router"
	funcsYaml.Template.BGP.Community = []string{"community {{bird_community \"65000:100\"}} community_bad {{bird_community \"invalid\"}}"}
	funcsYaml.Template.BGP.LargeCommunity = []string{"large {{bird_large_community \"65000:100:200\"}} large_bad {{bird_large_community \"invalid\"}}"}

	cmds, err := funcsYaml._tpl("bgp.community", _tpl_data{Cfg: cfg})
	if err != nil {
		t.Fatalf("bgp.community rendering failed: %v", err)
	}
	if len(cmds) != 1 || cmds[0] != "community (65000, 100) community_bad invalid" {
		t.Errorf("unexpected bgp.community rendered: %v", cmds)
	}

	cmds, err = funcsYaml._tpl("bgp.largecommunity", _tpl_data{Cfg: cfg})
	if err != nil {
		t.Fatalf("bgp.largecommunity rendering failed: %v", err)
	}
	if len(cmds) != 1 || cmds[0] != "large (65000, 100, 200) large_bad invalid" {
		t.Errorf("unexpected bgp.largecommunity rendered: %v", cmds)
	}
}

// TestRegisterEmptyNameSubprocess verifies name validation failure in a subprocess.
func TestRegisterEmptyNameSubprocess(t *testing.T) {
	if os.Getenv("RUN_EMPTY_NAME_TEST") == "1" {
		register("", &Yaml{})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRegisterEmptyNameSubprocess")
	cmd.Env = append(os.Environ(), "RUN_EMPTY_NAME_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 10 {
			t.Errorf("expected exit code 10, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

// TestRegisterDuplicateSubprocess verifies duplicate registration failure in a subprocess.
func TestRegisterDuplicateSubprocess(t *testing.T) {
	if os.Getenv("RUN_DUPLICATE_TEST") == "1" {
		register("frrouting", &Yaml{})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRegisterDuplicateSubprocess")
	cmd.Env = append(os.Environ(), "RUN_DUPLICATE_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 11 {
			t.Errorf("expected exit code 11, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

// TestLoadCustomDirReadErrorSubprocess verifies directory read failure in a subprocess.
func TestLoadCustomDirReadErrorSubprocess(t *testing.T) {
	if os.Getenv("RUN_DIR_READ_TEST") == "1" {
		loadCustomDir("/nonexistent/directory/that/does/not/exist")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadCustomDirReadErrorSubprocess")
	cmd.Env = append(os.Environ(), "RUN_DIR_READ_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 20 {
			t.Errorf("expected exit code 20, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

// TestLoadCustomDirParseErrorSubprocess verifies template parsing failure in a subprocess.
func TestLoadCustomDirParseErrorSubprocess(t *testing.T) {
	if os.Getenv("RUN_DIR_PARSE_TEST") == "1" {
		tmpDir, err := os.MkdirTemp("", "lg-router-dir-bad-")
		if err != nil {
			os.Exit(99)
		}
		defer os.RemoveAll(tmpDir)
		os.WriteFile(tmpDir+"/bad_yaml.yml", []byte("invalid: [yaml"), 0644)
		loadCustomDir(tmpDir)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadCustomDirParseErrorSubprocess")
	cmd.Env = append(os.Environ(), "RUN_DIR_PARSE_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 22 {
			t.Errorf("expected exit code 22, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

// TestLoadCustomDirReadFileErrorSubprocess verifies file read failure in a subprocess.
func TestLoadCustomDirReadFileErrorSubprocess(t *testing.T) {
	if os.Getenv("RUN_DIR_READ_FILE_TEST") == "1" {
		tmpDir, err := os.MkdirTemp("", "lg-router-dir-bad-read-")
		if err != nil {
			os.Exit(99)
		}
		defer os.RemoveAll(tmpDir)
		file := tmpDir + "/bad_read.yml"
		os.WriteFile(file, []byte("name: test"), 0644)
		os.Chmod(file, 0000)
		loadCustomDir(tmpDir)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadCustomDirReadFileErrorSubprocess")
	cmd.Env = append(os.Environ(), "RUN_DIR_READ_FILE_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 21 {
			t.Errorf("expected exit code 21, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

// TestBGPPeerRoutesQueryTypes verifies BGP peer routing operation query types.
func TestBGPPeerRoutesQueryTypes(t *testing.T) {
	frr := Get("frrouting")
	cfg := &utils.RouterConfig{Name: "test"}

	types := []string{"received", "accepted", "rejected", "advertised"}
	for _, ty := range types {
		_, err := frr.BGPPeerRoutes(cfg, "1.1.1.1", "peer_name", ty)
		if err != nil && !errors.Is(err, errs.OperationUnknown) {
			t.Errorf("unexpected error for %q: %v", ty, err)
		}
	}

	_, err := frr.BGPPeerRoutes(cfg, "1.1.1.1", "peer_name", "invalid_type")
	if !errors.Is(err, errs.OperationUnknown) {
		t.Errorf("expected OperationUnknown for invalid peer route type, got %v", err)
	}
}

// Test_tpl_AllCases verifies template rendering across all switch paths.
func Test_tpl_AllCases(t *testing.T) {
	y := &Yaml{}
	y.Template.Name = "all-cases-router"
	y.Template.Ping.Any = []string{"ping {{.IP.IP}}"}
	y.Template.Ping.IPv4 = []string{"ping -4 {{.IP.IP}}"}
	y.Template.Ping.IPv6 = []string{"ping -6 {{.IP.IP}}"}
	y.Template.Traceroute.Any = []string{"trace {{.IP.IP}}"}
	y.Template.Traceroute.IPv4 = []string{"trace -4 {{.IP.IP}}"}
	y.Template.Traceroute.IPv6 = []string{"trace -6 {{.IP.IP}}"}
	y.Template.BGP.Summary = []string{"show bgp summary"}
	y.Template.BGP.Route = []string{"show bgp route {{.IP.IP}}"}
	y.Template.BGP.Community = []string{"show bgp community {{.Community}}"}
	y.Template.BGP.LargeCommunity = []string{"show bgp large {{.LargeCommunity}}"}
	y.Template.BGP.ASPath = []string{"show bgp aspath {{.ASPath}}"}
	y.Template.BGP.PeerRoutes.Received = []string{"show bgp peer {{.PeerIP}} received"}
	y.Template.BGP.PeerRoutes.Accepted = []string{"show bgp peer {{.PeerIP}} accepted"}
	y.Template.BGP.PeerRoutes.Rejected = []string{"show bgp peer {{.PeerIP}} rejected"}
	y.Template.BGP.PeerRoutes.Advertised = []string{"show bgp peer {{.PeerIP}} advertised"}

	cfg := &utils.RouterConfig{Name: "test"}
	ip4, _ := utils.NewIPNET("1.1.1.1")
	ip6, _ := utils.NewIPNET("2001:db8::1")

	check := func(op string, ip *utils.IPNet, qType string) {
		var cmds []string
		var err error
		if op == "ping" {
			cmds, err = y.Ping(cfg, ip)
		} else if op == "traceroute" {
			cmds, err = y.Traceroute(cfg, ip)
		} else if op == "bgp.summary" {
			cmds, err = y.BGPSummary(cfg)
		} else if op == "bgp.route" {
			cmds, err = y.BGPRoute(cfg, ip)
		} else if op == "bgp.community" {
			cmds, err = y.BGPCommunity(cfg, "65000:100")
		} else if op == "bgp.largecommunity" {
			cmds, err = y.BGPLargeCommunity(cfg, "65000:100:200")
		} else if op == "bgp.aspath" {
			cmds, err = y.BGPASPath(cfg, "203038")
		} else if op == "bgp.peer_routes" {
			cmds, err = y.BGPPeerRoutes(cfg, "1.1.1.1", "peer_name", qType)
		}
		if err != nil {
			t.Errorf("op %s (%s) failed: %v", op, qType, err)
		}
		if len(cmds) != 1 {
			t.Errorf("expected 1 command, got %v", cmds)
		}
	}

	check("ping", ip4, "")
	check("ping", ip6, "")
	check("traceroute", ip4, "")
	check("traceroute", ip6, "")
	check("bgp.summary", nil, "")
	check("bgp.route", ip4, "")
	check("bgp.community", nil, "")
	check("bgp.largecommunity", nil, "")
	check("bgp.aspath", nil, "")
	check("bgp.peer_routes", nil, "received")
	check("bgp.peer_routes", nil, "accepted")
	check("bgp.peer_routes", nil, "rejected")
	check("bgp.peer_routes", nil, "advertised")
}

type mockBadDirFS struct{}

func (mockBadDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return nil, errors.New("read dir error")
}

func (mockBadDirFS) ReadFile(name string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

// TestLoadBuiltinDirReadErrorSubprocess verifies builtin directory read failure in a subprocess.
func TestLoadBuiltinDirReadErrorSubprocess(t *testing.T) {
	if os.Getenv("RUN_BUILTIN_DIR_READ_TEST") == "1" {
		compiledRoutersFS = mockBadDirFS{}
		loadBuiltin()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadBuiltinDirReadErrorSubprocess")
	cmd.Env = append(os.Environ(), "RUN_BUILTIN_DIR_READ_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 23 {
			t.Errorf("expected exit code 23, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

type mockBadFileFS struct{}

func (mockBadFileFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{mockDirEntry{name: "bad_file.yml"}}, nil
}

func (mockBadFileFS) ReadFile(name string) ([]byte, error) {
	return nil, errors.New("read file error")
}

type mockDirEntry struct {
	fs.DirEntry
	name string
}

func (m mockDirEntry) Name() string { return m.name }
func (m mockDirEntry) IsDir() bool  { return false }

// TestLoadBuiltinReadFileErrorSubprocess verifies builtin file read failure in a subprocess.
func TestLoadBuiltinReadFileErrorSubprocess(t *testing.T) {
	if os.Getenv("RUN_BUILTIN_READ_FILE_TEST") == "1" {
		compiledRoutersFS = mockBadFileFS{}
		loadBuiltin()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadBuiltinReadFileErrorSubprocess")
	cmd.Env = append(os.Environ(), "RUN_BUILTIN_READ_FILE_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 24 {
			t.Errorf("expected exit code 24, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}

type mockBadParseFS struct{}

func (mockBadParseFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{mockDirEntry{name: "bad_parse.yml"}}, nil
}

func (mockBadParseFS) ReadFile(name string) ([]byte, error) {
	return []byte("invalid: [yaml"), nil
}

// TestLoadBuiltinParseErrorSubprocess verifies builtin YAML parsing failure in a subprocess.
func TestLoadBuiltinParseErrorSubprocess(t *testing.T) {
	if os.Getenv("RUN_BUILTIN_PARSE_TEST") == "1" {
		compiledRoutersFS = mockBadParseFS{}
		loadBuiltin()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadBuiltinParseErrorSubprocess")
	cmd.Env = append(os.Environ(), "RUN_BUILTIN_PARSE_TEST=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 25 {
			t.Errorf("expected exit code 25, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error, got %v", err)
	}
}
