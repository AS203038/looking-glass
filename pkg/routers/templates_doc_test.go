package routers

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// TestRouterTemplatesDocExamplesCompile verifies that each complete template
// (a YAML block with a top-level name) in docs/router-templates.md compiles.
func TestRouterTemplatesDocExamplesCompile(t *testing.T) {
	doc, err := os.ReadFile("../../docs/router-templates.md")
	if err != nil {
		t.Fatal(err)
	}
	named := regexp.MustCompile(`(?m)^name:`)
	blocks := regexp.MustCompile("(?ms)^```yaml\n(.*?)^```$").FindAllStringSubmatch(strings.ReplaceAll(string(doc), "\r\n", "\n"), -1)
	checked := 0
	for _, b := range blocks {
		if !named.MatchString(b[1]) {
			continue
		}
		checked++
		y := &Yaml{Path: "docs/router-templates.md"}
		if err := yaml.Unmarshal([]byte(b[1]), &y.Template); err != nil {
			t.Errorf("template %q: %v", y.Template.Name, err)
			continue
		}
		if err := y.compile(); err != nil {
			t.Errorf("template %q: %v", y.Template.Name, err)
		}
	}
	if checked == 0 {
		t.Fatal("no complete template in docs/router-templates.md")
	}
}
