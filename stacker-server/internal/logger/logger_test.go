package logger

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"verbose", slog.LevelInfo},
	}

	for _, tc := range cases {
		name := tc.in
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			if got := parseLevel(tc.in); got != tc.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewJSONHandlerInProduction(t *testing.T) {
	log := New("info", "production")
	if _, ok := log.Handler().(*slog.JSONHandler); !ok {
		t.Fatalf("handler type = %T, want *slog.JSONHandler", log.Handler())
	}
}

func TestNewTextHandlerOutsideProduction(t *testing.T) {
	log := New("debug", "development")
	if _, ok := log.Handler().(*slog.TextHandler); !ok {
		t.Fatalf("handler type = %T, want *slog.TextHandler", log.Handler())
	}
}
