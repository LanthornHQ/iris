package browser

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
)

// Driver controls a headless browser for coordinate-based interaction.
type Driver interface {
	Navigate(ctx context.Context, url string) error
	Click(ctx context.Context, x, y int, button string) error
	DoubleClick(ctx context.Context, x, y int) error
	Type(ctx context.Context, text string, delayMs int) error
	Scroll(ctx context.Context, direction string, clicks int) error
	Screenshot(ctx context.Context) (string, int, int, error)
	WaitForStable(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error)
	Title(ctx context.Context) (string, error)
	Close() error
}

// CookieDef represents a browser cookie to inject at startup.
type CookieDef struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type Config struct {
	Headless       bool
	Width          int
	Height         int
	ChromePath     string
	NoSandbox      bool
	TimeoutMs      int
	Stealth        bool
	InitialCookies []CookieDef
}

const (
	envFalse         = "false"
	defaultWidth     = 1920
	defaultHeight    = 1080
	defaultTimeoutMs = 30000
)

func envIsFalse(v string) bool {
	return v == envFalse || v == "0"
}

// ConfigFromEnv reads browser configuration from IRIS_* environment variables.
func ConfigFromEnv() Config {
	cfg := Config{
		Headless:  true,
		Width:     defaultWidth,
		Height:    defaultHeight,
		NoSandbox: true,
		TimeoutMs: defaultTimeoutMs,
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
	if v := os.Getenv("IRIS_INITIAL_COOKIES"); v != "" {
		var cookies []CookieDef
		if err := json.Unmarshal([]byte(v), &cookies); err == nil {
			cfg.InitialCookies = cookies
		}
	}
	return cfg
}

// NewDriver starts a headless Chrome instance and returns a Driver.
func NewDriver(ctx context.Context, logger *slog.Logger, cfg Config) (Driver, error) {
	return newChromedpDriver(ctx, logger, cfg)
}
