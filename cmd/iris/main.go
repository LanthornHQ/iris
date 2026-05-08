package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/LanthornHQ/iris/internal/browser"
	"github.com/LanthornHQ/iris/internal/mcp"
	"github.com/LanthornHQ/iris/internal/tools"
)

// Version is set at build time via -ldflags="-X main.version=...".
var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "iris: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if arg == "-v" || arg == "-version" || arg == "--version" {
				fmt.Fprintf(os.Stdout, "iris %s\n", version)
				return nil
			}
		}
	}

	dotenvErr := godotenv.Load()

	logLevel := slog.LevelDebug
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
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if dotenvErr != nil {
		logger.Warn("error loading .env file", "error", dotenvErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	if apiKey, set := os.LookupEnv("IRIS_API_KEY"); set && apiKey == "" {
		return errors.New("IRIS_API_KEY is set but empty; unset it to disable auth or provide a non-empty key")
	}

	browserCfg := browser.ConfigFromEnv()
	driver, err := browser.NewDriver(ctx, logger, browserCfg)
	if err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}
	defer driver.Close()

	server := mcp.NewServer(logger, version)
	registry := tools.NewToolRegistry(logger, driver)
	registry.RegisterAll(server)

	addr := os.Getenv("IRIS_ADDR")
	if addr == "" {
		addr = "0.0.0.0:3000"
	}

	logger.Info("iris configuration",
		"version", version,
		"addr", addr,
		"tool_timeout", os.Getenv("IRIS_TOOL_TIMEOUT"),
		"headless", browserCfg.Headless,
		"window_size", fmt.Sprintf("%dx%d", browserCfg.Width, browserCfg.Height))

	logger.Info("iris starting", "addr", addr, "version", version)
	return server.RunHTTP(ctx, addr)
}
