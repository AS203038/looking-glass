package logging

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// TestParseLevel verifies parseLevel accurately maps string level names to slog.Levels.
func TestParseLevel(t *testing.T) {
	cases := []struct {
		input    string
		expected slog.Level
		wantErr  bool
	}{
		{"", slog.LevelInfo, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{" debug ", slog.LevelDebug, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"err", slog.LevelError, false},
		{"unknown", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseLevel(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseLevel(%q) error = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

// TestResolveOutput verifies resolveOutput returns appropriate stdout/stderr writers or opens files.
func TestResolveOutput(t *testing.T) {
	out, err := resolveOutput("stdout")
	if err != nil || out != os.Stdout {
		t.Errorf("stdout resolve failed: %v", err)
	}

	errOut, err := resolveOutput("stderr")
	if err != nil || errOut != os.Stderr {
		t.Errorf("stderr resolve failed: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "lg-test-log-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	fileOut, err := resolveOutput(tmpFile.Name())
	if err != nil {
		t.Fatalf("file resolve failed: %v", err)
	}
	if closer, ok := fileOut.(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestInitAndLevelHandler verifies filtering and component level overrides via public Init.
func TestInitAndLevelHandler(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "lg-test-init-log-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := Config{
		Level:  "info",
		Format: "json",
		Output: tmpFile.Name(),
		Components: map[string]string{
			"db":    "debug",
			"other": "warn",
		},
	}

	err = Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger := Component("test")
	dbLogger := Component("db")

	// Log below global default level (INFO) - should be skipped
	logger.Debug("global debug message")
	
	// Log at global default level (INFO) - should be captured
	logger.Info("global info message")

	// Log debug with db component override - should be captured
	dbLogger.Debug("db debug message")

	// Log directly with inline component to cover levelHandler.Handle's inner blocks
	slog.Info("direct info db message", slog.String(ComponentKey, "db"))
	slog.Info("direct info other message", slog.String(ComponentKey, "other"))
	slog.Info("direct info no component", slog.String("other", "val"))

	// Read log file content to verify
	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "global debug message") {
		t.Errorf("global debug log was not filtered out: %s", content)
	}
	if !strings.Contains(content, "global info message") {
		t.Errorf("global info log was filtered out: %s", content)
	}
	if !strings.Contains(content, "db debug message") {
		t.Errorf("component-override debug log was filtered out: %s", content)
	}
}

// TestStdlibBridge verifies standard library logs redirection.
func TestStdlibBridge(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(inner)

	bridge := stdlibBridge{logger: logger}
	log.SetOutput(bridge)
	defer log.SetOutput(os.Stderr)

	log.Println("hello from standard library logger")

	output := buf.String()
	if !strings.Contains(output, "hello from standard library logger") {
		t.Errorf("stdlib log was not forwarded to slog: %s", output)
	}
	if !strings.Contains(output, `"component":"stdlog"`) {
		t.Errorf("stdlib log was not tagged with component: %s", output)
	}
}

// TestLoggingExtraCases exercises uncovered blocks: text format, source file:line logging, invalid paths, and handler attributes/groups.
func TestLoggingExtraCases(t *testing.T) {
	// 1. resolveOutput error
	_, err := resolveOutput("/non-existent/dir/file.log")
	if err == nil {
		t.Errorf("expected error for invalid file output path")
	}

	// 2. parseLevel error
	_, err = parseLevel("invalid-level")
	if err == nil {
		t.Errorf("expected error for invalid level name")
	}

	// 3. Init with Text Format + Source Enable
	tmpFile, err := os.CreateTemp("", "lg-test-text-log-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := Config{
		Level:  "debug",
		Format: "text",
		Source: true,
		Output: tmpFile.Name(),
	}
	err = Init(cfg)
	if err != nil {
		t.Fatalf("Init failed for text format: %v", err)
	}

	// 4. Exercise WithAttrs and WithGroup on handlers
	subLogger := Component("extra").With(slog.String("key1", "val1")).WithGroup("group1")
	subLogger.Info("message with attrs and group")

	// Read log file content to verify
	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "key1=val1") {
		t.Errorf("expected logs to contain attribute 'key1=val1', got: %s", content)
	}

	// 5. Directly test internal componentHandler types to guarantee 100% coverage
	ctx := context.Background()
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)

	// Test componentHandler WithAttrs and WithGroup directly
	hComp := &componentHandler{component: "test"}
	_ = hComp.WithAttrs([]slog.Attr{slog.String("a", "b")})
	_ = hComp.WithGroup("g")

	// Test componentHandlerWithAttrs methods
	hCompAttrs := &componentHandlerWithAttrs{component: "test", attrs: []slog.Attr{slog.String("x", "y")}}
	_ = hCompAttrs.Enabled(ctx, slog.LevelInfo)
	_ = hCompAttrs.Handle(ctx, rec)
	_ = hCompAttrs.WithAttrs([]slog.Attr{slog.String("a", "b")})
	_ = hCompAttrs.WithGroup("g")

	// Test componentHandlerWithGroup methods
	hCompGroup := &componentHandlerWithGroup{component: "test", group: "g", attrs: []slog.Attr{slog.String("x", "y")}}
	_ = hCompGroup.Enabled(ctx, slog.LevelInfo)
	_ = hCompGroup.Handle(ctx, rec)
	_ = hCompGroup.WithAttrs([]slog.Attr{slog.String("a", "b")})
	_ = hCompGroup.WithGroup("g2")
}
