package utils

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestVersion verifies that the Version function behaves correctly in untracked and tagged modes.
func TestVersion(t *testing.T) {
	_version = "" // Reset cached version
	v1 := Version()
	if !strings.HasPrefix(v1, "untracked+") {
		t.Errorf("expected dev version prefix untracked+, got %q", v1)
	}

	// Mock release build
	_version = ""
	release = "1.9.0"
	v2 := Version()
	if v2 != "1.9.0" {
		t.Errorf("expected version to be exactly release build %q, got %q", "1.9.0", v2)
	}

	// Restore default
	_version = ""
	release = "untracked"
}

// TestSecurityTxtConfigString verifies that the security.txt rendering works correctly.
func TestSecurityTxtConfigString(t *testing.T) {
	s := &SecurityTxtConfig{
		Contact:            "mailto:security@example.com",
		Encryption:         "https://keybase.io/example",
		Acknowledgements:   "https://example.com/thanks",
		PreferredLanguages: "en, fr",
		Canonical:          "https://example.com/.well-known/security.txt",
		Policy:             "https://example.com/policy",
		Hiring:             "https://example.com/jobs",
		CSAF:               "https://example.com/csaf",
	}

	rendered := s.String()
	if !strings.Contains(rendered, "Contact: mailto:security@example.com") {
		t.Errorf("missing Contact line in rendered output")
	}
	if !strings.Contains(rendered, "Acknowledgements: https://example.com/thanks") {
		t.Errorf("missing Acknowledgements line in rendered output")
	}
	if !strings.Contains(rendered, "Preferred-Languages: en, fr") {
		t.Errorf("missing Preferred-Languages line in rendered output")
	}
}

// TestLinksString verifies serializing of links.
func TestLinksString(t *testing.T) {
	hf := HFBlock{
		Links: []Link{
			{Text: "GitHub", URL: "https://github.com"},
			{Text: "Privacy", URL: "/privacy"},
		},
	}
	serialized := hf.LinksString()
	expected := "GitHub|https://github.com,Privacy|/privacy"
	if serialized != expected {
		t.Errorf("LinksString() = %q, want %q", serialized, expected)
	}
}

// TestParseConfigYaml verifies parsing config file and validating it.
func TestParseConfigYaml(t *testing.T) {
	// 1. Invalid path
	_, err := ParseConfigYaml("non-existent-config.yaml")
	if err == nil {
		t.Errorf("expected error parsing non-existent file")
	}

	// 1b. Test ParseConfigYaml with invalid YAML syntax
	invalidYaml := `
devices:
  - name: "Router1"
  this-is-not-valid-yaml-!
`
	tmpInvalid, err := os.CreateTemp("", "lg-config-invalid-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpInvalid.Name())

	if _, err := tmpInvalid.Write([]byte(invalidYaml)); err != nil {
		t.Fatal(err)
	}
	tmpInvalid.Close()

	_, err = ParseConfigYaml(tmpInvalid.Name())
	if err == nil {
		t.Errorf("expected error unmarshaling invalid YAML")
	}

	// 2. Valid config content
	validYaml := `
devices:
  - name: "Router1"
    hostname: "10.0.0.1"
    type: "frrouting"
  - name: "RouterInvalid"
    type: "arista_eos" # host is empty, should be removed
grpc:
  enabled: true
  listen: "0.0.0.0:443"
`
	tmp, err := os.CreateTemp("", "lg-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write([]byte(validYaml)); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	cfg, err := ParseConfigYaml(tmp.Name())
	if err != nil {
		t.Fatalf("unexpected error parsing valid yaml: %v", err)
	}

	// RouterInvalid must have been removed since Hostname is empty, leaving only 1 device
	if len(cfg.Devices) != 1 {
		t.Errorf("len(Devices) = %d, want 1", len(cfg.Devices))
	}

	d := cfg.Devices[0]
	if d.Name != "Router1" {
		t.Errorf("expected device name Router1, got %q", d.Name)
	}

	// Check fallback loopback Source IPs
	if d.Source4 == nil || d.Source4.IP != "127.0.0.1" {
		t.Errorf("expected fallback Source4 127.0.0.1, got %v", d.Source4)
	}
	if d.Source6 == nil || d.Source6.IP != "::1" {
		t.Errorf("expected fallback Source6 ::1, got %v", d.Source6)
	}
}

// TestRouterConfigHasSSHCredentials verifies that HasSSHCredentials accurately identifies whether required credentials are set.
func TestRouterConfigHasSSHCredentials(t *testing.T) {
	tests := []struct {
		name     string
		config   RouterConfig
		expected bool
	}{
		{
			name:     "no credentials at all",
			config:   RouterConfig{Name: "rt1"},
			expected: false,
		},
		{
			name:     "username only",
			config:   RouterConfig{Name: "rt2", Username: "user1"},
			expected: true,
		},
		{
			name:     "password only",
			config:   RouterConfig{Name: "rt3", Password: "pass"},
			expected: false,
		},
		{
			name:     "ssh key only",
			config:   RouterConfig{Name: "rt4", SSHKey: "/path/to/key"},
			expected: false,
		},
		{
			name:     "username and password",
			config:   RouterConfig{Name: "rt5", Username: "user1", Password: "pass"},
			expected: true,
		},
		{
			name:     "username and ssh key",
			config:   RouterConfig{Name: "rt6", Username: "user1", SSHKey: "/path/to/key"},
			expected: true,
		},
		{
			name:     "all credentials",
			config:   RouterConfig{Name: "rt7", Username: "user1", Password: "pass", SSHKey: "/path/to/key"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.HasSSHCredentials(); got != tt.expected {
				t.Errorf("RouterConfig.HasSSHCredentials() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestCommandTimeoutConfig verifies YAML parsing, inheritance from grpc.command_timeout,
// per-device overrides, and rejection of negative or invalid durations.
func TestCommandTimeoutConfig(t *testing.T) {
	writeTmp := func(t *testing.T, body string) string {
		t.Helper()
		f, err := os.CreateTemp("", "lg-config-timeout-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(f.Name()) })
		if _, err := f.WriteString(body); err != nil {
			t.Fatal(err)
		}
		f.Close()
		return f.Name()
	}

	cfg, err := ParseConfigYaml(writeTmp(t, `
grpc:
  enabled: true
  command_timeout: 50s
devices:
  - name: "inherited"
    hostname: "10.0.0.1:22"
  - name: "overridden"
    hostname: "10.0.0.2:22"
    command_timeout: 2m
`))
	if err != nil {
		t.Fatalf("ParseConfigYaml valid timeout config: %v", err)
	}
	if cfg.Grpc.CommandTimeout != 50*time.Second {
		t.Errorf("Grpc.CommandTimeout = %v, want 50s", cfg.Grpc.CommandTimeout)
	}
	if got := cfg.Devices[0].CommandTimeout; got != 50*time.Second {
		t.Errorf("inherited device CommandTimeout = %v, want 50s", got)
	}
	if got := cfg.Devices[1].CommandTimeout; got != 2*time.Minute {
		t.Errorf("overridden device CommandTimeout = %v, want 2m", got)
	}

	// Unset everywhere leaves device CommandTimeout at 0 and commandTimeout() returns default.
	cfgDefault, err := ParseConfigYaml(writeTmp(t, `
devices:
  - name: "defaulted"
    hostname: "10.0.0.3:22"
`))
	if err != nil {
		t.Fatalf("ParseConfigYaml default timeout config: %v", err)
	}
	if got := cfgDefault.Devices[0].CommandTimeout; got != 0 {
		t.Errorf("defaulted device CommandTimeout = %v, want 0", got)
	}
	if got := cfgDefault.Devices[0].commandTimeout(60 * time.Second); got != 60*time.Second {
		t.Errorf("commandTimeout(60s) = %v, want 60s", got)
	}

	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "negative grpc.command_timeout",
			yaml: "grpc:\n  command_timeout: -5s\ndevices: []\n",
			want: "grpc.command_timeout must not be negative",
		},
		{
			name: "negative device command_timeout",
			yaml: "devices:\n  - name: \"rt1\"\n    hostname: \"10.0.0.1:22\"\n    command_timeout: -1s\n",
			want: `device "rt1": command_timeout must not be negative`,
		},
		{
			name: "invalid duration string",
			yaml: "grpc:\n  command_timeout: not-a-duration\ndevices: []\n",
			want: "cannot unmarshal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfigYaml(writeTmp(t, tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ParseConfigYaml err = %v, want substring %q", err, tc.want)
			}
		})
	}
}
