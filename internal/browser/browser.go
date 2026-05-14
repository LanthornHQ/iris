package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

// SomBounds represents the bounding box of a Set-of-Mark element in viewport pixels.
type SomBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// SomElement carries structured textual and spatial metadata for a Set-of-Mark element.
type SomElement struct {
	ID        string    `json:"id"`
	Tag       string    `json:"tag"`
	Text      string    `json:"text"`
	AriaLabel string    `json:"aria_label,omitempty"`
	Role      string    `json:"role,omitempty"`
	Type      string    `json:"type,omitempty"`
	Bounds    SomBounds `json:"bounds"`
}

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
	DrawMarks(ctx context.Context) ([]SomElement, error)
	ClearMarks(ctx context.Context) error
	GetElementCoords(ctx context.Context, id string) (int, int, error)
	PageTree(ctx context.Context) (string, error)
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
	Headless            bool
	Width               int
	Height              int
	ChromePath          string
	NoSandbox           bool
	TimeoutMs           int
	BypassGoogleConsent bool
	InitialCookies      []CookieDef
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
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Headless:  true,
		Width:     defaultWidth,
		Height:    defaultHeight,
		NoSandbox: true,
		TimeoutMs: defaultTimeoutMs,
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
	if os.Getenv("IRIS_BYPASS_GOOGLE_CONSENT") == "1" {
		cfg.BypassGoogleConsent = true
	}
	if v := os.Getenv("IRIS_WINDOW_WIDTH"); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			cfg.Width = val
		}
	}
	if v := os.Getenv("IRIS_WINDOW_HEIGHT"); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			cfg.Height = val
		}
	}
	if v := os.Getenv("IRIS_INITIAL_COOKIES"); v != "" {
		var cookies []CookieDef
		if err := json.Unmarshal([]byte(v), &cookies); err != nil {
			return cfg, fmt.Errorf("parsing IRIS_INITIAL_COOKIES: %w", err)
		}
		cfg.InitialCookies = cookies
	}
	return cfg, nil
}

// NewDriver starts a headless Chrome instance and returns a Driver.
func NewDriver(ctx context.Context, logger *slog.Logger, cfg Config) (Driver, error) {
	return newChromedpDriver(ctx, logger, cfg)
}
