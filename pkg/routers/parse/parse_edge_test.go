package parse

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// TestBuiltinParserEdgeCases verifies parseLinuxPing and parseLinuxTraceroute edge behavior.
func TestBuiltinParserEdgeCases(t *testing.T) {
	res1 := parseLinuxPing([]byte("completely invalid ping output"))
	if res1 != nil {
		t.Errorf("expected nil result on invalid ping output")
	}

	res2 := parseLinuxPing([]byte("--- 1.1.1.1 ping statistics ---\n5 packets transmitted, 5 received"))
	if res2 == nil || res2.PacketsSent != 5 {
		t.Errorf("expected 5 packets sent, got %+v", res2)
	}

	res3 := parseLinuxPing([]byte("--- statistics ---\n5 packets transmitted, 5 received\nrtt min/avg/max = foo/bar/baz ms"))
	if res3 != nil && res3.RttMinMs != 0 {
		t.Errorf("expected 0 min rtt on invalid rtt line, got %f", res3.RttMinMs)
	}

	res4 := parseLinuxTraceroute([]byte("completely invalid traceroute output"))
	if res4 != nil {
		t.Errorf("expected nil result on invalid traceroute output")
	}

	res5 := parseLinuxTraceroute([]byte("traceroute to 1.1.1.1 (1.1.1.1)\n 1  \n 2  \n"))
	if res5 == nil || len(res5.Hops) != 2 {
		t.Errorf("expected 2 hops, got %+v", res5)
	}
}

// TestJSONParserEdgeCasesDirect verifies JSON helper function boundary inputs.
func TestJSONParserEdgeCasesDirect(t *testing.T) {
	paths := &pb.BGPPaths{}
	ok1 := tryFRRSinglePrefixJSON([]byte("invalid json"), paths)
	if ok1 {
		t.Errorf("expected tryFRRSinglePrefixJSON to fail on invalid json")
	}

	ok2 := tryFRRSinglePrefixJSON([]byte("{}"), paths)
	if ok2 {
		t.Errorf("expected tryFRRSinglePrefixJSON to fail on empty json {}")
	}

	bp := frrBestpath(nil)
	if bp {
		t.Errorf("expected bestpath false on nil message")
	}

	bp2 := frrBestpath(json.RawMessage(`{"overall": "invalid-type"}`))
	if bp2 {
		t.Errorf("expected bestpath false on invalid overall type")
	}

	splits := splitJSONObjects([]byte("{unclosed"))
	if len(splits) != 0 {
		t.Errorf("expected 0 splits on unclosed object, got %d", len(splits))
	}

	splits2 := splitJSONObjects([]byte("{\"brace_in_string\": \"}\"}"))
	if len(splits2) != 1 {
		t.Errorf("expected 1 split, got %d", len(splits2))
	}
}

// TestTextFSMParserEdgeCasesDirect verifies TextFSM record conversion functions.
func TestTextFSMParserEdgeCasesDirect(t *testing.T) {
	mockMap := map[string]interface{}{
		"empty":   "",
		"invalid": "invalid-value",
		"number":  "123",
		"float":   "12.3",
		"list":    "a,b,c",
		"as_list": "65000 65001",
	}

	if got := recVal(mockMap, "nonexistent"); got != nil {
		t.Errorf("recVal: expected nil for nonexistent key, got %v", got)
	}

	if got := recString(mockMap, "nonexistent"); got != "" {
		t.Errorf("recString: expected empty string, got %q", got)
	}
	mockMap["slice"] = []string{"first", "second"}
	if got := recString(mockMap, "slice"); got != "first" {
		t.Errorf("expected first, got %q", got)
	}
	mockMap["empty_slice"] = []string{}
	if got := recString(mockMap, "empty_slice"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	if got := recUint32(mockMap, "invalid"); got != 0 {
		t.Errorf("recUint32: expected 0 on invalid, got %d", got)
	}
	if got := recUint32(mockMap, "nonexistent"); got != 0 {
		t.Errorf("recUint32: expected 0 on nonexistent, got %d", got)
	}

	if got := recUint64(mockMap, "invalid"); got != 0 {
		t.Errorf("recUint64: expected 0 on invalid, got %d", got)
	}
	if got := recUint64(mockMap, "nonexistent"); got != 0 {
		t.Errorf("recUint64: expected 0 on nonexistent, got %d", got)
	}

	if got := recFloat32(mockMap, "invalid"); got != 0 {
		t.Errorf("recFloat32: expected 0 on invalid, got %f", got)
	}
	if got := recFloat32(mockMap, "nonexistent"); got != 0 {
		t.Errorf("recFloat32: expected 0 on nonexistent, got %f", got)
	}

	if got := recAgeSeconds(mockMap, "invalid"); got != 0 {
		t.Errorf("recAgeSeconds: expected 0 on invalid age string, got %d", got)
	}
	if got := recAgeSeconds(mockMap, "nonexistent"); got != 0 {
		t.Errorf("recAgeSeconds: expected 0 on nonexistent, got %d", got)
	}

	mockMap["bad_age"] = "10x"
	if got := recAgeSeconds(mockMap, "bad_age"); got != 0 {
		t.Errorf("recAgeSeconds: expected 0 on bad age suffix, got %d", got)
	}

	gotList := recUint32List(mockMap, "list")
	if len(gotList) != 0 {
		t.Errorf("recUint32List: expected empty list on non-numeric strings, got %v", gotList)
	}

	if got := columnDeclared(map[string]interface{}(nil), "nonexistent"); got {
		t.Errorf("columnDeclared: expected false for nil records")
	}
	if got := columnDeclared(mockMap, "nonexistent"); got {
		t.Errorf("columnDeclared: expected false, got true")
	}
	mockMap["UPPER"] = "val"
	if got := columnDeclared(mockMap, "upper"); !got {
		t.Errorf("columnDeclared: expected true for uppercase match")
	}

	rMap := []map[string]interface{}{mockMap}
	r1 := tfsmPingResult(rMap)
	if r1.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED {
		t.Errorf("expected PARSE_FAILED for mismatched op on tfsmPingResult")
	}

	r2 := tfsmTracerouteResult(rMap)
	if r2.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Errorf("expected OK for tfsmTracerouteResult on missing columns, got %v", r2.Status)
	}

	r3 := tfsmBGPSummaryResult(rMap)
	if r3.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Errorf("expected OK for tfsmBGPSummaryResult on missing columns, got %v", r3.Status)
	}

	r4 := tfsmBGPPathsResult(rMap)
	if r4.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Errorf("expected OK for tfsmBGPPathsResult on missing columns, got %v", r4.Status)
	}
}

// TestTextFSMTemplateROUTER_DIR verifies template overlay loading in TextFSM.
func TestTextFSMTemplateROUTER_DIR(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lg-parse-dir-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.Mkdir(tmpDir+"/textfsm", 0755)
	if err != nil {
		t.Fatal(err)
	}

	mockTfsm := "Value Required target (\\S+)\n\nStart\n  ^.* -> Record"
	err = os.WriteFile(tmpDir+"/textfsm/custom_ping.textfsm", []byte(mockTfsm), 0644)
	if err != nil {
		t.Fatal(err)
	}

	os.Setenv("ROUTER_DIR", tmpDir)
	defer os.Unsetenv("ROUTER_DIR")

	templateCache = sync.Map{}

	body, err := readTemplateBody("custom_ping")
	if err != nil || body != mockTfsm {
		t.Errorf("expected success reading custom template, got body %q, err %v", body, err)
	}

	_, err = resolveTemplate("custom_ping")
	if err != nil {
		t.Errorf("expected successful resolve of custom template, got err %v", err)
	}
}

// TestTextFSMParserParseError verifies TextFSM template compilation failures.
func TestTextFSMParserParseError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lg-parse-dir-bad-fsm-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.Mkdir(tmpDir+"/textfsm", 0755)
	if err != nil {
		t.Fatal(err)
	}

	badTfsm := `Value Required target (\S+)
Value malformed syntax invalid`
	err = os.WriteFile(tmpDir+"/textfsm/bad_fsm.textfsm", []byte(badTfsm), 0644)
	if err != nil {
		t.Fatal(err)
	}

	os.Setenv("ROUTER_DIR", tmpDir)
	defer os.Unsetenv("ROUTER_DIR")

	templateCache = sync.Map{}

	_, err = resolveTemplate("bad_fsm")
	if err == nil {
		t.Errorf("expected error parsing malformed TextFSM template")
	}
}

// TestJSONParserSinglePrefixShape verifies single-prefix flat JSON mapping.
func TestJSONParserSinglePrefixShape(t *testing.T) {
	singleJSON := `{"prefix": "1.1.1.0/24", "paths": [{"valid": true, "bestpath": {"overall": true}, "nexthops": [{"ip": "10.0.0.2", "afi": "ipv4"}]}]}`
	paths := &pb.BGPPaths{}
	ok := tryFRRSinglePrefixJSON([]byte(singleJSON), paths)
	if !ok || len(paths.Paths) != 1 || paths.Paths[0].Prefix != "1.1.1.0/24" {
		t.Errorf("expected single prefix mapping success, got %+v", paths)
	}

	singleJSONNetwork := `{"network": "2.2.2.0/24", "paths": [{"valid": true, "bestpath": {"overall": true}, "nexthops": [{"ip": "10.0.0.2", "afi": "ipv4"}]}]}`
	paths2 := &pb.BGPPaths{}
	ok2 := tryFRRSinglePrefixJSON([]byte(singleJSONNetwork), paths2)
	if !ok2 || len(paths2.Paths) != 1 || paths2.Paths[0].Prefix != "2.2.2.0/24" {
		t.Errorf("expected single prefix mapping success via network, got %+v", paths2)
	}
}
