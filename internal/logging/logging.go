package logging

import (
	"log/slog"
	"os"
	"strings"
)

// SetupLogger initializes the global structured logger based on IRIS_LOG_LEVEL.
func SetupLogger() *slog.Logger {
	logLevel := slog.LevelDebug
	var invalidLogLevelWarning string
	if v := os.Getenv("IRIS_LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			logLevel = slog.LevelDebug
		case "info":
			logLevel = slog.LevelInfo
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		default:
			invalidLogLevelWarning = v
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if invalidLogLevelWarning != "" {
		logger.Warn("invalid IRIS_LOG_LEVEL, falling back to debug", "value", invalidLogLevelWarning)
	}

	return logger
}
