package utils

import (
	"encoding/xml"
	"testing"
	"unicode/utf8"
)

// FuzzNetconfSpecRender verifies that any command renders a well-formed RPC
// and that a command of valid XML characters reaches the device unchanged.
func FuzzNetconfSpecRender(f *testing.F) {
	spec, err := NewNetconfSpec(`<command format="text">{{.Command}}</command>`, "output", "")
	if err != nil {
		f.Fatal(err)
	}
	seeds := []string{
		"show bgp summary",
		`show route "<&>" 'x'`,
		"</command><rpc>",
		"]]>]]>",
		"\n#4\nabcd\n##\n",
		"\x00\x1b",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, cmd string) {
		body, err := spec.Render(cmd)
		if err != nil {
			t.Fatalf("Render(%q): %v", cmd, err)
		}
		if err := checkXML(body); err != nil {
			t.Fatalf("Render(%q) = %q is not well-formed: %v", cmd, body, err)
		}
		if !utf8.ValidString(cmd) || !netconfTestXMLChars(cmd) {
			return
		}
		var got struct {
			Command string `xml:",chardata"`
		}
		if err := xml.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("Unmarshal(%q): %v", body, err)
		}
		if got.Command != cmd {
			t.Errorf("command = %q, want %q", got.Command, cmd)
		}
	})
}

// netconfTestXMLChars reports whether s holds only characters that XML 1.0
// allows; EscapeText replaces the others.
func netconfTestXMLChars(s string) bool {
	for _, r := range s {
		ok := r == 0x09 || r == 0x0A || r == 0x0D ||
			(r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF)
		if !ok {
			return false
		}
	}
	return true
}
