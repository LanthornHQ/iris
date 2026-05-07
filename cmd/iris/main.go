package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/LanthornHQ/iris/internal/browser"
	"github.com/LanthornHQ/iris/internal/grounding"
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

	browserCfg := browser.ConfigFromEnv()
	driver, err := browser.NewDriver(ctx, logger, browserCfg)
	if err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}
	defer driver.Close()

	var groundingClient *grounding.Client
	groundingCfg := grounding.ConfigFromEnv()
	if groundingCfg.BaseURL != "" {
		groundingClient = grounding.NewClient(groundingCfg, logger)
		logger.Info("grounding model configured",
			"grounding_url", groundingCfg.BaseURL,
			"grounding_model", groundingCfg.ModelName)
	} else {
		logger.Warn("grounding model not configured; set IRIS_GROUNDING_URL to enable vision-grounded tools")
	}

	server := mcp.NewServer(logger, version)
	registry := tools.NewToolRegistry(logger, driver, groundingClient)
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
