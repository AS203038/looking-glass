// Package logging provides a process-wide slog-based event stream with
// configurable level, format, output sink, and per-component overrides.
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// ComponentKey is the slog attribute name carrying the subsystem label.
const ComponentKey = "component"

// Config is the YAML-projected logging configuration.
type Config struct {
	// Level is the default minimum level: debug, info, warn, error.
	Level string `yaml:"level"`
	// Format selects the output encoder: json (default) or text.
	Format string `yaml:"format"`
	// Output selects the sink: stdout (default), stderr, or a file path.
	Output string `yaml:"output"`
	// Source toggles AddSource (file:line annotations) on records.
	Source bool `yaml:"source"`
	// Components maps component names to a per-component minimum level.
	Components map[string]string `yaml:"components"`
}

// defaultLevel is the level applied when Config.Level is empty.
const defaultLevel = slog.LevelInfo

// parseLevel maps a string level name onto a [slog.Level].
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error", "err":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q", s)
	}
}

// resolveOutput opens the sink described by name. The caller must close
// the returned [io.Writer] when it implements [io.Closer] and is not
// stdout/stderr.
func resolveOutput(name string) (io.Writer, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "stdout":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	default:
		f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open log file %q: %w", name, err)
		}
		return f, nil
	}
}

// Init configures the process-global slog default logger from cfg and
// routes the stdlib log package through the same handler at INFO.
func Init(cfg Config) error {
	lvl, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}
	comps := make(map[string]slog.Level, len(cfg.Components))
	for k, v := range cfg.Components {
		l, err := parseLevel(v)
		if err != nil {
			return fmt.Errorf("component %q: %w", k, err)
		}
		comps[k] = l
	}
	out, err := resolveOutput(cfg.Output)
	if err != nil {
		return err
	}
	opts := &slog.HandlerOptions{
		AddSource: cfg.Source,
		Level:     slog.LevelDebug, // outer filter is applied by levelHandler
	}
	var inner slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "", "json":
		inner = slog.NewJSONHandler(out, opts)
	case "text":
		inner = slog.NewTextHandler(out, opts)
	default:
		return fmt.Errorf("unknown log format %q", cfg.Format)
	}
	h := &levelHandler{
		inner:      inner,
		defaultLvl: lvl,
		components: comps,
	}
	logger := slog.New(h)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(stdlibBridge{logger: logger})
	return nil
}

// initOnce ensures the package's safety-net default handler is installed
// exactly once, prior to any user-provided Init call.
var initOnce sync.Once

// install applies the safety-net JSON-at-INFO handler used before Init
// is called (e.g. while parsing the configuration file itself).
func install() {
	initOnce.Do(func() {
		h := &levelHandler{
			inner:      slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
			defaultLvl: defaultLevel,
		}
		slog.SetDefault(slog.New(h))
		log.SetFlags(0)
		log.SetOutput(stdlibBridge{logger: slog.Default()})
	})
}

// Component returns a [*slog.Logger] tagged with component=name. The
// returned logger inherits the per-component level threshold configured
// via [Config.Components] and resolves the underlying handler via
// [slog.Default] at call time, so loggers captured before [Init] still
// pick up the configured handler once Init runs.
func Component(name string) *slog.Logger {
	install()
	return slog.New(&componentHandler{component: name})
}

// componentHandler is a thin wrapper that resolves the underlying
// handler from [slog.Default] at every call. This decouples loggers
// captured at package-init time from the safety-net default, so a
// [Component] call returning a logger before [Init] runs still emits
// through the configured handler afterwards.
type componentHandler struct {
	component string
}

// current returns the active handler bound with the component attribute.
func (h *componentHandler) current() slog.Handler {
	return slog.Default().Handler().WithAttrs(
		[]slog.Attr{slog.String(ComponentKey, h.component)})
}

// Enabled forwards to the current handler.
func (h *componentHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.current().Enabled(ctx, lvl)
}

// Handle forwards to the current handler.
func (h *componentHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.current().Handle(ctx, r)
}

// WithAttrs returns a handler that pre-binds attrs in addition to the
// component label.
func (h *componentHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &componentHandlerWithAttrs{component: h.component, attrs: attrs}
}

// WithGroup returns a handler that opens a named group on top of the
// component label.
func (h *componentHandler) WithGroup(name string) slog.Handler {
	return &componentHandlerWithGroup{component: h.component, group: name}
}

// componentHandlerWithAttrs is the [componentHandler] flavour returned
// after a [WithAttrs] call.
type componentHandlerWithAttrs struct {
	component string
	attrs     []slog.Attr
}

func (h *componentHandlerWithAttrs) current() slog.Handler {
	return slog.Default().Handler().
		WithAttrs([]slog.Attr{slog.String(ComponentKey, h.component)}).
		WithAttrs(h.attrs)
}
func (h *componentHandlerWithAttrs) Enabled(ctx context.Context, l slog.Level) bool {
	return h.current().Enabled(ctx, l)
}
func (h *componentHandlerWithAttrs) Handle(ctx context.Context, r slog.Record) error {
	return h.current().Handle(ctx, r)
}
func (h *componentHandlerWithAttrs) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &componentHandlerWithAttrs{component: h.component, attrs: merged}
}
func (h *componentHandlerWithAttrs) WithGroup(name string) slog.Handler {
	return &componentHandlerWithGroup{component: h.component, attrs: h.attrs, group: name}
}

// componentHandlerWithGroup is the [componentHandler] flavour returned
// after a [WithGroup] call.
type componentHandlerWithGroup struct {
	component string
	attrs     []slog.Attr
	group     string
}

func (h *componentHandlerWithGroup) current() slog.Handler {
	hh := slog.Default().Handler().
		WithAttrs([]slog.Attr{slog.String(ComponentKey, h.component)})
	if len(h.attrs) > 0 {
		hh = hh.WithAttrs(h.attrs)
	}
	return hh.WithGroup(h.group)
}
func (h *componentHandlerWithGroup) Enabled(ctx context.Context, l slog.Level) bool {
	return h.current().Enabled(ctx, l)
}
func (h *componentHandlerWithGroup) Handle(ctx context.Context, r slog.Record) error {
	return h.current().Handle(ctx, r)
}
func (h *componentHandlerWithGroup) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &nh
}
func (h *componentHandlerWithGroup) WithGroup(name string) slog.Handler {
	nh := *h
	nh.group = name
	return &nh
}

// init installs the safety-net handler at package load time so any
// slog call issued before [Init] still produces structured output.
func init() {
	install()
}

// levelHandler filters records by the default level and any
// per-component level override.
type levelHandler struct {
	inner      slog.Handler
	defaultLvl slog.Level
	components map[string]slog.Level
	component  string
}

// thresholdFor returns the level threshold to apply when the record's
// component is comp (empty string falls back to defaultLvl).
func (h *levelHandler) thresholdFor(comp string) slog.Level {
	if comp != "" {
		if l, ok := h.components[comp]; ok {
			return l
		}
	}
	return h.defaultLvl
}

// Enabled reports whether the handler will emit a record at level.
func (h *levelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.thresholdFor(h.component)
}

// Handle filters the record by the component-scoped threshold and
// forwards surviving records to the inner handler.
func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	threshold := h.thresholdFor(h.component)
	if h.component == "" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == ComponentKey {
				if l, ok := h.components[a.Value.String()]; ok {
					threshold = l
				}
				return false
			}
			return true
		})
	}
	if r.Level < threshold {
		return nil
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new handler binding attrs onto the inner handler
// and remembering the component label when present.
func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	for _, a := range attrs {
		if a.Key == ComponentKey {
			nh.component = a.Value.String()
			break
		}
	}
	nh.inner = h.inner.WithAttrs(attrs)
	return &nh
}

// WithGroup returns a new handler scoping attrs under name.
func (h *levelHandler) WithGroup(name string) slog.Handler {
	nh := *h
	nh.inner = h.inner.WithGroup(name)
	return &nh
}

// stdlibBridge adapts [log.Logger] writes onto a [*slog.Logger] at INFO.
type stdlibBridge struct {
	logger *slog.Logger
}

// Write forwards the trimmed line as a single slog INFO record tagged
// component=stdlog.
func (b stdlibBridge) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	b.logger.LogAttrs(context.Background(), slog.LevelInfo, msg,
		slog.String(ComponentKey, "stdlog"))
	return len(p), nil
}
