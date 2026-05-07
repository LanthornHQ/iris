package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/LanthornHQ/iris/internal/browser"
)

type Navigate struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func (t *Navigate) Name() string { return "navigate" }

func (t *Navigate) Description() string {
	return `Navigate the browser to a URL. Replaces open_url — the browser is always running.

Parameters:
  - url (string, required): The full URL to navigate to, e.g. "https://example.com".

Returns: {success: bool, url: string, title: string}
  - success (bool): true when navigation completed without error.
  - url (string): The URL that was navigated to.
  - title (string): The page title after navigation.

Preconditions:
  - The URL must include a scheme (https:// or http://).

Failure modes:
  - Invalid URL.
  - Navigation timeout.
  - DNS resolution failure.`
}

func (t *Navigate) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to navigate to (must include scheme, e.g. https://)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *Navigate) Execute(ctx context.Context, args map[string]any) (any, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return nil, errors.New("url is required")
	}
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid url %q: must include scheme and host (e.g. https://example.com)", rawURL)
	}

	t.Logger.InfoContext(ctx, "navigate starting", "url", rawURL)
	start := time.Now()

	if err := t.Driver.Navigate(ctx, rawURL); err != nil {
		return nil, fmt.Errorf("navigate to %q failed: %w", rawURL, err)
	}

	title, titleErr := t.Driver.Title(ctx)
	if titleErr != nil {
		t.Logger.WarnContext(ctx, "navigate: failed to get title", "error", titleErr)
	}

	elapsed := time.Since(start)
	t.Logger.InfoContext(ctx, "navigate completed", "url", rawURL, "title", title, "elapsed_ms", elapsed.Milliseconds())

	return NavigateResponse{
		Success: true,
		URL:     rawURL,
		Title:   title,
	}, nil
}
