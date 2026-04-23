package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	defaultLevel      = "info"
	defaultFormat     = "json"
	defaultSink       = "file"
	defaultFile       = "logs/matrix-runtime.jsonl"
	defaultMaxBytes   = int64(10 * 1024 * 1024)
	defaultMaxBackups = 5
)

// Config holds resolved logging configuration.
type Config struct {
	Level      slog.Level
	Format     string
	Sink       string
	FilePath   string
	MaxBytes   int64
	MaxBackups int
	StdErr     bool
	ACPWire    bool
}

// Runtime holds the initialized logger, its config, and a close function.
type Runtime struct {
	Logger *slog.Logger
	Config Config
	Close  func() error
}

// Bootstrap initializes logging using the default factory.
func Bootstrap(cfgMgr *config.Manager) (*Runtime, error) {
	return BootstrapWithFactory(cfgMgr, nil)
}

// ResolveConfig loads and returns the logging configuration.
func ResolveConfig(cfgMgr *config.Manager) (Config, error) {
	return loadConfig(cfgMgr)
}

// BootstrapWithFactory initializes logging with a custom sink factory.
func BootstrapWithFactory(cfgMgr *config.Manager, sinkFactory middleware.LogSinkFactory) (*Runtime, error) {
	cfg, err := loadConfig(cfgMgr)
	if err != nil {
		return nil, err
	}

	handler, closeFn, err := newHandler(cfg, sinkFactory)
	if err != nil {
		return nil, err
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return &Runtime{
		Logger: logger,
		Config: cfg,
		Close:  closeFn,
	}, nil
}

func loadConfig(cfgMgr *config.Manager) (Config, error) {
	cfg := defaultConfig()
	if err := loadCoreConfig(cfgMgr, &cfg); err != nil {
		return Config{}, err
	}
	if err := loadFileConfig(cfgMgr, &cfg); err != nil {
		return Config{}, err
	}
	if err := loadFlagConfig(cfgMgr, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Sink == "stderr" || cfg.Sink == "both" {
		cfg.StdErr = true
	}
	return cfg, nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid logging level %q", level)
	}
}

func parseBool(raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func parseInt64(raw string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
}

func parseInt(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	return n, nil
}

func newHandler(cfg Config, sinkFactory middleware.LogSinkFactory) (slog.Handler, func() error, error) {
	opts := &slog.HandlerOptions{Level: cfg.Level}

	build := func(w io.Writer) slog.Handler {
		if cfg.Format == "text" {
			return slog.NewTextHandler(w, opts)
		}
		return slog.NewJSONHandler(w, opts)
	}

	if sinkFactory == nil {
		return nil, nil, fmt.Errorf("log sink factory is required")
	}

	sink, err := sinkFactory.Build(middleware.LogSinkOptions{
		Target:     cfg.Sink,
		FilePath:   cfg.FilePath,
		MaxBytes:   cfg.MaxBytes,
		MaxBackups: cfg.MaxBackups,
	})
	if err != nil {
		return nil, nil, err
	}
	return build(sink.Writer()), sink.Close, nil
}

type teeHandler struct {
	left  slog.Handler
	right slog.Handler
}

func newTeeHandler(left, right slog.Handler) slog.Handler {
	return &teeHandler{left: left, right: right}
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.left.Enabled(ctx, level) || h.right.Enabled(ctx, level)
}

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.left.Enabled(ctx, r.Level) {
		if err := h.left.Handle(ctx, r.Clone()); err != nil {
			return err
		}
	}
	if h.right.Enabled(ctx, r.Level) {
		if err := h.right.Handle(ctx, r.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{
		left:  h.left.WithAttrs(attrs),
		right: h.right.WithAttrs(attrs),
	}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{
		left:  h.left.WithGroup(name),
		right: h.right.WithGroup(name),
	}
}
