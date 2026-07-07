// Package log provides a small structured logger on top of slog.
package log

import (
	"log/slog"
	"os"
	"strings"
)

// L is the package-level logger.
var L *slog.Logger

// Init configures the global logger.
func Init(format, level string) {
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	if strings.EqualFold(format, "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	L = slog.New(h)
	slog.SetDefault(L)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug wraps slog.Debug.
func Debug(msg string, args ...any) { L.Debug(msg, args...) }

// Info wraps slog.Info.
func Info(msg string, args ...any) { L.Info(msg, args...) }

// Warn wraps slog.Warn.
func Warn(msg string, args ...any) { L.Warn(msg, args...) }

// Error wraps slog.Error.
func Error(msg string, args ...any) { L.Error(msg, args...) }
