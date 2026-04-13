package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/oslog"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
		wantErr  bool
	}{
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
		wantErr  bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"1", true, false},
		{"0", false, false},
		{"", false, false},
		{"yes", false, true},
		{"no", false, true},
		{"invalid", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseBool(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBool(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewHandler(t *testing.T) {
	tmpFile := t.TempDir() + "/test.log"

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "json to file",
			cfg: Config{
				Level:      slog.LevelInfo,
				Format:     "json",
				Sink:       "file",
				FilePath:   tmpFile,
				MaxBytes:   1024,
				MaxBackups: 2,
			},
			wantErr: false,
		},
		{
			name: "text to stderr",
			cfg: Config{
				Level:      slog.LevelDebug,
				Format:     "text",
				Sink:       "stderr",
				MaxBytes:   1024,
				MaxBackups: 2,
			},
			wantErr: false,
		},
		{
			name: "both sink",
			cfg: Config{
				Level:      slog.LevelWarn,
				Format:     "json",
				Sink:       "both",
				FilePath:   tmpFile,
				MaxBytes:   1024,
				MaxBackups: 2,
				StdErr:     true,
			},
			wantErr: false,
		},
		{
			name: "invalid sink",
			cfg: Config{
				Level:      slog.LevelInfo,
				Format:     "json",
				Sink:       "invalid",
				MaxBytes:   1024,
				MaxBackups: 2,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, closeFn, err := newHandler(tt.cfg, oslog.NewFactory())
			if (err != nil) != tt.wantErr {
				t.Errorf("newHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && handler == nil {
				t.Error("newHandler() returned nil handler")
			}
			if closeFn != nil {
				if err := closeFn(); err != nil {
					t.Fatalf("closeFn() error = %v", err)
				}
			}
		})
	}
}

func TestFactoryBuildFileSink(t *testing.T) {
	tmpFile := t.TempDir() + "/test_file.log"

	factory := oslog.NewFactory()
	sink, err := factory.Build(middleware.LogSinkOptions{
		Target:     "file",
		FilePath:   tmpFile,
		MaxBytes:   1024,
		MaxBackups: 2,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if sink == nil {
		t.Fatal("Build() returned nil sink")
	}

	handler := slog.NewJSONHandler(sink.Writer(), &slog.HandlerOptions{Level: slog.LevelInfo})
	if err := handler.Handle(context.Background(), slog.Record{
		Level:   slog.LevelInfo,
		Message: "test message",
	}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Errorf("log file was not created at %s", tmpFile)
	}
}

func TestParseInt64(t *testing.T) {
	got, err := parseInt64("1024")
	if err != nil {
		t.Fatalf("parseInt64() error = %v", err)
	}
	if got != 1024 {
		t.Fatalf("parseInt64() = %d, want 1024", got)
	}
}

func TestParseInt(t *testing.T) {
	got, err := parseInt("5")
	if err != nil {
		t.Fatalf("parseInt() error = %v", err)
	}
	if got != 5 {
		t.Fatalf("parseInt() = %d, want 5", got)
	}
}

func TestTeeHandler(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	left := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	right := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	handler := newTeeHandler(left, right)

	record := slog.Record{
		Level:   slog.LevelInfo,
		Message: "tee test",
	}

	err := handler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("teeHandler.Handle() error = %v", err)
	}

	if buf1.Len() == 0 {
		t.Error("left buffer is empty")
	}
	if buf2.Len() == 0 {
		t.Error("right buffer is empty")
	}
}

func TestTeeHandlerEnabled(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	left := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelError})
	right := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug})

	handler := newTeeHandler(left, right)

	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("teeHandler.Enabled() should return true when left is enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("teeHandler.Enabled() should return true when right is enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("teeHandler.Enabled() should return true because right handler enables info")
	}
}

func TestTeeHandlerWithAttrs(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	left := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	right := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	handler := newTeeHandler(left, right).WithAttrs([]slog.Attr{
		{Key: "service", Value: slog.StringValue("test")},
	})

	record := slog.Record{
		Level:   slog.LevelInfo,
		Message: "with attrs test",
	}

	err := handler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("teeHandler.WithAttrs().Handle() error = %v", err)
	}

	if buf1.Len() == 0 {
		t.Error("left buffer is empty after WithAttrs")
	}
	if buf2.Len() == 0 {
		t.Error("right buffer is empty after WithAttrs")
	}
}

func TestTeeHandlerWithGroup(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	left := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	right := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	handler := newTeeHandler(left, right).WithGroup("testgroup")

	record := slog.Record{
		Level:   slog.LevelInfo,
		Message: "with group test",
	}

	err := handler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("teeHandler.WithGroup().Handle() error = %v", err)
	}

	if buf1.Len() == 0 {
		t.Error("left buffer is empty after WithGroup")
	}
	if buf2.Len() == 0 {
		t.Error("right buffer is empty after WithGroup")
	}
}

func TestLoadSetting(t *testing.T) {
	if err := os.Setenv("TEST_MATRIX_VAR", "from_env"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("TEST_MATRIX_VAR") }()

	t.Run("env var takes precedence", func(t *testing.T) {
		got := loadSetting(nil, "some.config.key", "TEST_MATRIX_VAR", "fallback")
		if got != "from_env" {
			t.Errorf("loadSetting() = %q, want %q", got, "from_env")
		}
	})

	t.Run("fallback when no env and no config manager", func(t *testing.T) {
		got := loadSetting(nil, "some.config.key", "NONEXISTENT_VAR", "fallback")
		if got != "fallback" {
			t.Errorf("loadSetting() = %q, want %q", got, "fallback")
		}
	})

	t.Run("env var with whitespace is trimmed", func(t *testing.T) {
		if err := os.Setenv("TEST_WHITESPACE_VAR", "  trimmed  "); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("TEST_WHITESPACE_VAR") }()

		got := loadSetting(nil, "key", "TEST_WHITESPACE_VAR", "fallback")
		if got != "trimmed" {
			t.Errorf("loadSetting() = %q, want %q", got, "trimmed")
		}
	})
}
