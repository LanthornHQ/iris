package tools

type ScreenshotResponse struct {
	ImageBase64 string `json:"image_base64"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

type ClickResponse struct {
	Success     bool   `json:"success"`
	ImageBase64 string `json:"image_base64,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type TypeTextResponse struct {
	Success      bool    `json:"success"`
	UIChanged    bool    `json:"ui_changed,omitempty"`
	X            int     `json:"x,omitempty"`
	Y            int     `json:"y,omitempty"`
	Width        int     `json:"width,omitempty"`
	Height       int     `json:"height,omitempty"`
	Method       string  `json:"method,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	ImageBase64  string  `json:"image_base64,omitempty"`
	ScreenWidth  int     `json:"screen_width,omitempty"`
	ScreenHeight int     `json:"screen_height,omitempty"`
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

type VerifyScreenResponse struct {
	Answer   string `json:"answer"`
	Evidence string `json:"evidence"`
	X1       int    `json:"x1"`
	Y1       int    `json:"y1"`
	X2       int    `json:"x2"`
	Y2       int    `json:"y2"`
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
