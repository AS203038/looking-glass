package utils

import (
	"os"
	"strings"
	"testing"
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
