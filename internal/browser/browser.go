package browser

import (
	"context"
	"log/slog"
	"os"
)

// Driver controls a headless browser for coordinate-based interaction.
type Driver interface {
	Navigate(ctx context.Context, url string) error
	Click(ctx context.Context, x, y int) error
	Type(ctx context.Context, text string, delayMs int) error
	Scroll(ctx context.Context, direction string, clicks int) error
	Screenshot(ctx context.Context) (string, int, int, error)
	WaitForStable(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error)
	Title(ctx context.Context) (string, error)
	Close() error
}

type Config struct {
	Headless   bool
	Width      int
	Height     int
	ChromePath string
	NoSandbox  bool
	TimeoutMs  int
}

// ConfigFromEnv reads browser configuration from IRIS_* environment variables.
func ConfigFromEnv() Config {
	cfg := Config{
		Headless:  true,
		Width:     1920,
		Height:    1080,
		NoSandbox: true,
		TimeoutMs: 30000,
	}
	if v := os.Getenv("IRIS_HEADLESS"); v == "false" || v == "0" {
		cfg.Headless = false
	}
	if v := os.Getenv("IRIS_CHROME_PATH"); v != "" {
		cfg.ChromePath = v
	}
	if v := os.Getenv("IRIS_NO_SANDBOX"); v == "false" || v == "0" {
		cfg.NoSandbox = false
	}
	return cfg
}

// NewDriver starts a headless Chrome instance and returns a Driver.
func NewDriver(ctx context.Context, logger *slog.Logger, cfg Config) (Driver, error) {
	return newChromedpDriver(ctx, logger, cfg)
}
