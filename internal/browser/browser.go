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
	DoubleClick(ctx context.Context, x, y int) error
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
	Stealth    bool
}

const envFalse = "false"

func envIsFalse(v string) bool {
	return v == envFalse || v == "0"
}

// ConfigFromEnv reads browser configuration from IRIS_* environment variables.
func ConfigFromEnv() Config {
	cfg := Config{
		Headless:  true,
		Width:     1920,
		Height:    1080,
		NoSandbox: true,
		TimeoutMs: 30000,
		Stealth:   true,
	}
	if envIsFalse(os.Getenv("IRIS_HEADLESS")) {
		cfg.Headless = false
	}
	if v := os.Getenv("IRIS_CHROME_PATH"); v != "" {
		cfg.ChromePath = v
	}
	if envIsFalse(os.Getenv("IRIS_NO_SANDBOX")) {
		cfg.NoSandbox = false
	}
	if envIsFalse(os.Getenv("IRIS_STEALTH")) {
		cfg.Stealth = false
	}
	return cfg
}

// NewDriver starts a headless Chrome instance and returns a Driver.
func NewDriver(ctx context.Context, logger *slog.Logger, cfg Config) (Driver, error) {
	return newChromedpDriver(ctx, logger, cfg)
}
