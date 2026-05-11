package tools

import "github.com/LanthornHQ/iris/internal/browser"

type ScreenshotResponse struct {
	ImageBase64 string               `json:"image_base64"`
	Width       int                  `json:"width"`
	Height      int                  `json:"height"`
	SoMApplied  bool                 `json:"som_applied"`
	Elements    []browser.SomElement `json:"elements,omitempty"`
	PageTree    string               `json:"page_tree,omitempty"`
}

type ClickResponse struct {
	Success     bool   `json:"success"`
	ImageBase64 string `json:"image_base64,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type TypeTextResponse struct {
	Success      bool   `json:"success"`
	ImageBase64  string `json:"image_base64,omitempty"`
	ScreenWidth  int    `json:"screen_width,omitempty"`
	ScreenHeight int    `json:"screen_height,omitempty"`
}

type NavigateResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
}

type ScrollResponse struct {
	Success      bool   `json:"success"`
	ImageBase64  string `json:"image_base64,omitempty"`
	ScreenWidth  int    `json:"screen_width,omitempty"`
	ScreenHeight int    `json:"screen_height,omitempty"`
}

type WaitForStableResponse struct {
	Stable    bool  `json:"stable"`
	ElapsedMs int64 `json:"elapsed_ms"`
}

type GetDatetimeResponse struct {
	Datetime string `json:"datetime"`
	Date     string `json:"date"`
	Time     string `json:"time"`
	Weekday  string `json:"weekday"`
}

type SleepResponse struct {
	Success bool `json:"success"`
}
