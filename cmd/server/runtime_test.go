package main

import (
	"bytes"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func preserveRuntimeState(t *testing.T) {
	t.Helper()
	prevProcs := runtime.GOMAXPROCS(0)
	prevMemLimit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(prevProcs)
		debug.SetMemoryLimit(prevMemLimit)
	})
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	prev, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// TestConfigureRuntimeHonorsEnvOverrides verifies that configureRuntime honors GOMAXPROCS and GOMEMLIMIT environment overrides and logs through the supplied logger.
func TestConfigureRuntimeHonorsEnvOverrides(t *testing.T) {
	preserveRuntimeState(t)
	t.Setenv("GOMAXPROCS", "2")
	t.Setenv("GOMEMLIMIT", "256MiB")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	undo := configureRuntime(logger)
	if undo == nil {
		t.Fatal("expected non-nil undo function")
	}
	undo()

	got := buf.String()
	if !strings.Contains(got, `Honoring GOMAXPROCS=\"2\"`) {
		t.Fatalf("expected log to mention Honoring GOMAXPROCS=\"2\", got %q", got)
	}
	if !strings.Contains(got, "GOMEMLIMIT=256MiB") {
		t.Fatalf("expected log to mention GOMEMLIMIT=256MiB, got %q", got)
	}
}

// TestConfigureRuntimeAutomemlimitOff verifies that AUTOMEMLIMIT=off skips modifying GOMEMLIMIT without emitting warnings or errors.
func TestConfigureRuntimeAutomemlimitOff(t *testing.T) {
	preserveRuntimeState(t)
	unsetEnvForTest(t, "GOMEMLIMIT")
	t.Setenv("AUTOMEMLIMIT", "off")
	t.Setenv("GOMAXPROCS", "2")

	before := debug.SetMemoryLimit(-1)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	undo := configureRuntime(logger)
	if undo == nil {
		t.Fatal("expected non-nil undo function")
	}
	undo()

	if after := debug.SetMemoryLimit(-1); after != before {
		t.Fatalf("expected GOMEMLIMIT to remain %d, got %d", before, after)
	}
	if got := buf.String(); strings.Contains(got, "level=WARN") || strings.Contains(got, "level=ERROR") || strings.Contains(got, "automemlimit failed") {
		t.Fatalf("expected no warnings or errors, got %q", got)
	}
}

// TestConfigureRuntimeNilLogger verifies that configureRuntime accepts a nil logger without panicking.
func TestConfigureRuntimeNilLogger(t *testing.T) {
	preserveRuntimeState(t)
	t.Setenv("GOMAXPROCS", "2")
	t.Setenv("AUTOMEMLIMIT", "off")

	undo := configureRuntime(nil)
	if undo == nil {
		t.Fatal("expected non-nil undo function")
	}
	undo()
}

// TestConfigureRuntimeDefault verifies that configureRuntime runs with unset runtime env vars and logs maxprocs output through the supplied logger.
func TestConfigureRuntimeDefault(t *testing.T) {
	preserveRuntimeState(t)
	unsetEnvForTest(t, "GOMAXPROCS")
	unsetEnvForTest(t, "GOMEMLIMIT")
	unsetEnvForTest(t, "AUTOMEMLIMIT")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	undo := configureRuntime(logger)
	if undo == nil {
		t.Fatal("expected non-nil undo function")
	}
	undo()

	if got := buf.String(); !strings.Contains(got, "maxprocs:") {
		t.Fatalf("expected maxprocs log output, got %q", got)
	}
}
