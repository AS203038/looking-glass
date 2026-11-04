package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/utils"
)

type mockInternalErrorRouter struct{ mockGrpcRouter }

func (mockInternalErrorRouter) Traceroute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, fmt.Errorf("dial tcp 198.51.100.7:22: %w", errs.ConnectionFailed)
}

// TestMuxErrorsHaveCodeAndMessage sends RPCs over HTTP to the mounted service and
// verifies that each failure reaches the client with its code and the sentinel text only.
func TestMuxErrorsHaveCodeAndMessage(t *testing.T) {
	rts := utils.RouterMap{}
	for i, r := range []utils.Router{mockGrpcRouter{}, mockInternalErrorRouter{}, mockPoolExhaustedRouter{}} {
		rts = append(rts, &utils.RouterInstance{
			Config:      &utils.RouterConfig{Name: fmt.Sprintf("router%d", i+1)},
			Router:      r,
			HealthCheck: &utils.HealthCheck{Healthy: true},
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	mux := http.NewServeMux()
	done := Mux(ctx, mux, rts)
	t.Cleanup(func() { cancel(); <-done })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name, body, code, message string
		status                    int
	}{
		{"empty target", `{"routerId":1,"target":""}`, "invalid_argument", "invalid IP", http.StatusBadRequest},
		{"unknown router", `{"routerId":9,"target":"192.0.2.1"}`, "not_found", "router unknown", http.StatusNotFound},
		{"internal error", `{"routerId":2,"target":"192.0.2.1"}`, "unavailable", "execution error", http.StatusServiceUnavailable},
		{"pool exhausted", `{"routerId":3,"target":"192.0.2.1"}`, "resource_exhausted", "ssh pool exhausted", http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/lookingglass.v0.LookingGlassService/Traceroute", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			var got struct{ Code, Message string }
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("body %q: %v", raw, err)
			}
			if resp.StatusCode != tc.status || got.Code != tc.code || got.Message != tc.message {
				t.Errorf("got %d %q %q, want %d %q %q", resp.StatusCode, got.Code, got.Message, tc.status, tc.code, tc.message)
			}
			if strings.Contains(string(raw), "198.51.100.7") {
				t.Errorf("body %q has the internal error text", raw)
			}
		})
	}
}

// TestPublicErrorsCoverEveryErrsSentinel verifies that each error variable in pkg/errs has a code in publicErrors.
func TestPublicErrorsCoverEveryErrsSentinel(t *testing.T) {
	files, err := filepath.Glob("../../errs/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("pkg/errs sources: %v %v", files, err)
	}
	mapped := map[string]bool{}
	for _, p := range publicErrors {
		mapped[p.err.Error()] = true
	}
	for _, f := range files {
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "New" {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && !mapped[strings.Trim(lit.Value, `"`)] {
				t.Errorf("%s: errors.New(%s) has no entry in publicErrors", filepath.Base(f), lit.Value)
			}
			return true
		})
	}
}
