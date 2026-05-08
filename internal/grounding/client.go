package grounding

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registers PNG decoder needed by decodeImage
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/LanthornHQ/iris/internal/metrics"
)

type Image struct {
	Base64 string
	Width  int
	Height int
}

type BoundingBox struct {
	X1 int
	Y1 int
	X2 int
	Y2 int
}

const (
	TypeActionType  = "type"
	TypeActionClick = "click"

	maxLogResponseLen  = 1000
	maxLogBodyLen      = 1000
	maxErrBodyLen      = 500
	maxErrRawLen       = 200
	defaultJPEGQuality = 85
	minRegexMatchLen   = 3
)

type Config struct {
	BaseURL          string
	ModelName        string
	VerifyModelName  string
	APIKey           string
	TimeoutSec       int
	BBoxScale        int
	MaxDim           int
	PointClickRadius int
}

func getEnvInt(key string) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// ConfigFromEnv reads grounding configuration from IRIS_GROUNDING_* environment variables.
func ConfigFromEnv() Config {
	return Config{
		BaseURL:          os.Getenv("IRIS_GROUNDING_URL"),
		ModelName:        os.Getenv("IRIS_GROUNDING_MODEL"),
		VerifyModelName:  os.Getenv("IRIS_VERIFY_MODEL"),
		APIKey:           os.Getenv("IRIS_GROUNDING_API_KEY"),
		TimeoutSec:       getEnvInt("IRIS_GROUNDING_TIMEOUT"),
		MaxDim:           getEnvInt("IRIS_GROUNDING_MAX_DIM"),
		BBoxScale:        getEnvInt("IRIS_GROUNDING_BBOX_SCALE"),
		PointClickRadius: getEnvInt("IRIS_GROUNDING_POINT_CLICK_RADIUS"),
	}
}

type Client struct {
	mu               sync.RWMutex
	baseURL          string
	modelName        string
	verifyModelName  string
	apiKey           string
	HTTPClient       *http.Client
	Logger           *slog.Logger
	BBoxScale        int
	MaxDim           int
	PointClickRadius int
}

func normalizeBaseURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(u, "/v1/chat/completions") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/chat/completions"
	}
	return u + "/v1/chat/completions"
}

// SetVerifyModel overrides the model used for verify_screen calls.
func (c *Client) SetVerifyModel(modelName string) {
	if c == nil || modelName == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verifyModelName = modelName
}

// NewClient creates a grounding HTTP client for the vision model.
func NewClient(cfg Config, logger *slog.Logger) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8000/v1/chat/completions"
	} else {
		cfg.BaseURL = normalizeBaseURL(cfg.BaseURL)
	}
	if cfg.ModelName == "" {
		cfg.ModelName = "vocaela/KV-Ground-4B-BaseGuiOwl1.5-0228"
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 60
	}
	if cfg.BBoxScale <= 0 {
		cfg.BBoxScale = 1000
	}
	if cfg.MaxDim <= 0 {
		cfg.MaxDim = 1280
	}
	if cfg.PointClickRadius <= 0 {
		cfg.PointClickRadius = 15
	}

	return &Client{
		baseURL:          cfg.BaseURL,
		modelName:        cfg.ModelName,
		verifyModelName:  cfg.VerifyModelName,
		apiKey:           cfg.APIKey,
		HTTPClient:       &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second},
		Logger:           logger,
		BBoxScale:        cfg.BBoxScale,
		MaxDim:           cfg.MaxDim,
		PointClickRadius: cfg.PointClickRadius,
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imageURLContent struct {
	Type     string   `json:"type"`
	ImageURL imageURL `json:"image_url"`
}

type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type chatResponse struct {
	Error   chatResponseError `json:"error"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type chatResponseError struct {
	raw string
}

func (e *chatResponseError) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &e.raw)
	}
	var obj struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	e.raw = obj.Message
	return nil
}

func (e *chatResponseError) String() string {
	return e.raw
}

const groundingSystemPrompt = `You are a UI Grounding Agent. Your task is to identify the exact center coordinates of requested elements on a screenshot.
The coordinate system uses a 0-1000 range, where [0, 0] is the top-left and [1000, 1000] is the bottom-right.
Respond ONLY with valid JSON in this exact format, no other text:
{"description":"<brief description>","point":[x,y]}
Rules:
- point is the center [x,y] of the target element.
- Coordinates are normalized to 0-1000 range.
- Do NOT wrap in markdown code fences.
- Do NOT include any text outside the JSON object.`

func buildUserPrompt(target string, intent string) string {
	switch intent {
	case TypeActionType:
		return fmt.Sprintf("Look at this screenshot. Find the text input field associated with '%s'. Describe its appearance and location, then output the center coordinates.", target)
	case TypeActionClick:
		return fmt.Sprintf("Look at this screenshot. Find the '%s' element. Describe what you see and where it is, then output the exact center coordinates.", target)
	default:
		return fmt.Sprintf("Look at this screenshot. Find the '%s' element. Describe its appearance and location, then output the center coordinates.", target)
	}
}

const verifySystemPrompt = `You are a visual verification model. Given a screenshot and a question, answer based on what you see.
Respond ONLY with valid JSON in this exact format, no other text:
{"answer":"<yes or no>","evidence":"<brief explanation>","bbox":[x1,y1,x2,y2]}
Rules:
- bbox is the bounding box of the area that supports your answer, in normalized coordinates (0-1000).
- x1,y1 is the top-left corner, x2,y2 is the bottom-right corner.
- bbox MUST be a single flat array with exactly 4 integers.
- Do NOT wrap in markdown code fences.
- Do NOT include any text outside the JSON object.`

func buildVerifyUserPrompt(question string) string {
	return fmt.Sprintf("Question: %s\nAnswer truthfully based only on what you see in the screenshot.", question)
}

// Verify asks the vision model a yes/no question about a screenshot.
func (c *Client) Verify(ctx context.Context, question string, img Image) (string, string, BoundingBox, error) {
	c.mu.RLock()
	modelName := c.verifyModelName
	if modelName == "" {
		modelName = c.modelName
	}
	c.mu.RUnlock()

	start := time.Now()
	c.Logger.InfoContext(ctx, "verify: starting",
		"question", question,
		"model", modelName,
		"image_base64_len", len(img.Base64),
		"image_size", fmt.Sprintf("%dx%d", img.Width, img.Height))

	jpegB64, resizedW, resizedH, err := c.prepareImage(img.Base64)
	if err != nil {
		return "", "", BoundingBox{}, fmt.Errorf("verify: prepare image: %w", err)
	}

	c.Logger.DebugContext(ctx, "verify: image prepared",
		"resized", fmt.Sprintf("%dx%d", resizedW, resizedH),
		"jpeg_base64_len", len(jpegB64))

	dataURL := fmt.Sprintf("data:image/jpeg;base64,%s", jpegB64)
	content, err := c.callModel(ctx, modelName, verifySystemPrompt, buildVerifyUserPrompt(question), dataURL)
	if err != nil {
		return "", "", BoundingBox{}, err
	}

	elapsed := time.Since(start)
	c.Logger.InfoContext(ctx, "verify: model responded",
		"question", question,
		"elapsed_ms", elapsed.Milliseconds(),
		"response_len", len(content),
		"raw_response", truncate(content, maxLogResponseLen))

	ans, ev, b := parseVerifyResponse(content)

	c.Logger.InfoContext(ctx, "verify: parsed response",
		"question", question,
		"answer", ans,
		"evidence", ev,
		"bbox", fmt.Sprintf("%v", b))

	if b != nil {
		nx1, ny1, nx2, ny2 := b[0], b[1], b[2], b[3]
		x1, y1, x2, y2 := c.denormalizeBBox(nx1, ny1, nx2, ny2, img.Width, img.Height, resizedW, resizedH)
		c.Logger.InfoContext(ctx, "verify: bbox mapped",
			"normalized", fmt.Sprintf("[%d,%d,%d,%d]", nx1, ny1, nx2, ny2),
			"pixels", fmt.Sprintf("[%d,%d,%d,%d]", x1, y1, x2, y2),
			"resized", fmt.Sprintf("%dx%d", resizedW, resizedH),
			"original", fmt.Sprintf("%dx%d", img.Width, img.Height))
		if x2 > x1 && y2 > y1 {
			return ans, ev, BoundingBox{X1: x1, Y1: y1, X2: x2, Y2: y2}, nil
		}
	}

	return ans, ev, BoundingBox{}, nil
}

func parseVerifyResponse(text string) (string, string, []int) {
	var answer, evidence string

	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var result struct {
		Answer   string `json:"answer"`
		Evidence string `json:"evidence"`
		BBox     []int  `json:"bbox"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err == nil {
		return strings.ToLower(strings.TrimSpace(result.Answer)), strings.TrimSpace(result.Evidence), result.BBox
	}

	answerRegexp := regexp.MustCompile(`"answer"\s*:\s*"([^"]*)"`)
	evidenceRegexp := regexp.MustCompile(`"evidence"\s*:\s*"([^"]*)"`)
	if m := answerRegexp.FindStringSubmatch(cleaned); len(m) > 1 {
		answer = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := evidenceRegexp.FindStringSubmatch(cleaned); len(m) > 1 {
		evidence = strings.TrimSpace(m[1])
	}
	if answer == "" {
		answer = cleaned
	}
	return answer, evidence, nil
}

func stripModelPrefix(model string) string {
	for _, prefix := range []string{"openai/", "ollama/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	return model
}

func (c *Client) doHTTPPost(ctx context.Context, modelName string, reqBody chatRequest, baseURL, apiKey string) ([]byte, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("grounding: marshal request: %w", err)
	}

	c.Logger.DebugContext(ctx, "grounding: sending HTTP request",
		"model", modelName,
		"url", baseURL,
		"api_key_set", apiKey != "",
		"request_body_len", len(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("grounding: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		c.Logger.ErrorContext(ctx, "grounding: HTTP request failed",
			"model", modelName,
			"url", baseURL,
			"error", err)
		return nil, fmt.Errorf("grounding: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grounding: read response body: %w", err)
	}

	c.Logger.DebugContext(ctx, "grounding: HTTP response received",
		"model", modelName,
		"status_code", resp.StatusCode,
		"response_body_len", len(body))

	if resp.StatusCode != http.StatusOK {
		c.Logger.ErrorContext(ctx, "grounding: non-200 response",
			"status", resp.StatusCode,
			"body", truncate(string(body), maxLogBodyLen))
		return nil, fmt.Errorf("grounding: HTTP %d: %s", resp.StatusCode, truncate(string(body), maxErrBodyLen))
	}

	return body, nil
}

func (c *Client) callModel(ctx context.Context, modelName, systemPrompt, userPrompt, dataURL string) (string, error) {
	modelName = stripModelPrefix(modelName)
	start := time.Now()
	defer func() {
		metrics.GroundingRequestDuration.Observe(time.Since(start).Seconds())
	}()

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: []any{
			textContent{Type: "text", Text: userPrompt},
			imageURLContent{Type: "image_url", ImageURL: imageURL{URL: dataURL, Detail: "high"}},
		}},
	}

	reqBody := chatRequest{
		Model:       modelName,
		Messages:    messages,
		Temperature: 0,
	}

	c.mu.RLock()
	baseURL := c.baseURL
	apiKey := c.apiKey
	c.mu.RUnlock()

	body, err := c.doHTTPPost(ctx, modelName, reqBody, baseURL, apiKey)
	if err != nil {
		return "", err
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		c.Logger.ErrorContext(ctx, "grounding: failed to decode response",
			"error", err,
			"raw_body", truncate(string(body), maxLogBodyLen))
		return "", fmt.Errorf("grounding: decode response: %w (raw: %s)", err, truncate(string(body), maxErrRawLen))
	}

	if chatResp.Error.String() != "" {
		c.Logger.ErrorContext(ctx, "grounding: model returned error",
			"error", chatResp.Error.String(),
			"raw_body", truncate(string(body), maxLogBodyLen))
		return "", fmt.Errorf("grounding: model error: %s", chatResp.Error.String())
	}

	if len(chatResp.Choices) == 0 {
		c.Logger.WarnContext(ctx, "grounding: no choices in response",
			"raw_body", truncate(string(body), maxLogBodyLen))
		return "", errors.New("grounding: no choices in response")
	}

	c.Logger.DebugContext(ctx, "grounding: model content received",
		"model", modelName,
		"content_len", len(chatResp.Choices[0].Message.Content),
		"elapsed_ms", time.Since(start).Milliseconds())

	content := stripThinkingTags(chatResp.Choices[0].Message.Content)
	return content, nil
}

var thinkingTagRe = regexp.MustCompile(`(?s)<thinking>.*?</thinking>\s*`)

func stripThinkingTags(s string) string {
	if !strings.Contains(s, "<thinking>") {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(thinkingTagRe.ReplaceAllString(s, ""))
}

// Ground locates a target element via the vision model and returns its bounding box.
func (c *Client) Ground(ctx context.Context, target string, intent string, img Image) (BoundingBox, error) {
	start := time.Now()
	c.Logger.InfoContext(ctx, "grounding: starting",
		"target", target,
		"model", c.modelName,
		"intent", intent,
		"image_size", fmt.Sprintf("%dx%d", img.Width, img.Height))

	jpegB64, _, _, err := c.prepareImage(img.Base64)
	if err != nil {
		return BoundingBox{}, fmt.Errorf("grounding: prepare image: %w", err)
	}

	dataURL := fmt.Sprintf("data:image/jpeg;base64,%s", jpegB64)
	userPrompt := buildUserPrompt(target, intent)

	content, err := c.callModel(ctx, c.modelName, groundingSystemPrompt, userPrompt, dataURL)
	if err != nil {
		return BoundingBox{}, err
	}

	elapsed := time.Since(start)
	c.Logger.InfoContext(ctx, "grounding: model responded",
		"target", target,
		"elapsed_ms", elapsed.Milliseconds(),
		"response_len", len(content),
		"raw_response", truncate(content, maxLogResponseLen))

	nx, ny, pErr := parsePoint(content)
	if pErr != nil {
		c.Logger.ErrorContext(ctx, "grounding: failed to parse point from model response",
			"target", target,
			"raw_response", truncate(content, maxErrBodyLen),
			"error", pErr)
		return BoundingBox{}, pErr
	}

	c.Logger.InfoContext(ctx, "grounding: parsed normalized point",
		"target", target,
		"normalized", fmt.Sprintf("[%d,%d]", nx, ny))

	scaleX := float64(img.Width) / float64(c.BBoxScale)
	scaleY := float64(img.Height) / float64(c.BBoxScale)
	px := int(math.Round(float64(nx) * scaleX))
	py := int(math.Round(float64(ny) * scaleY))

	c.Logger.InfoContext(ctx, "grounding: point mapped to pixels",
		"target", target,
		"pixel", fmt.Sprintf("(%d,%d)", px, py))

	r := c.PointClickRadius
	return BoundingBox{
		X1: max(0, px-r),
		Y1: max(0, py-r),
		X2: min(img.Width, px+r),
		Y2: min(img.Height, py+r),
	}, nil
}

func (c *Client) denormalizeBBox(nx1, ny1, nx2, ny2, imgW, imgH, resizedW, resizedH int) (int, int, int, int) {
	resX1 := int(math.Round((float64(nx1) / float64(c.BBoxScale)) * float64(resizedW)))
	resY1 := int(math.Round((float64(ny1) / float64(c.BBoxScale)) * float64(resizedH)))
	resX2 := int(math.Round((float64(nx2) / float64(c.BBoxScale)) * float64(resizedW)))
	resY2 := int(math.Round((float64(ny2) / float64(c.BBoxScale)) * float64(resizedH)))

	scaleX := float64(imgW) / float64(resizedW)
	scaleY := float64(imgH) / float64(resizedH)

	x1 := int(math.Round(float64(resX1) * scaleX))
	y1 := int(math.Round(float64(resY1) * scaleY))
	x2 := int(math.Round(float64(resX2) * scaleX))
	y2 := int(math.Round(float64(resY2) * scaleY))

	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 > imgW {
		x2 = imgW
	}
	if y2 > imgH {
		y2 = imgH
	}

	return x1, y1, x2, y2
}

func (c *Client) prepareImage(imageBase64 string) (string, int, int, error) {
	pngData, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return "", 0, 0, fmt.Errorf("decode base64: %w", err)
	}

	img, err := decodeImage(pngData)
	if err != nil {
		return "", 0, 0, fmt.Errorf("decode PNG: %w", err)
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	resized := resizeImage(img, c.MaxDim)
	resW := resized.Bounds().Dx()
	resH := resized.Bounds().Dy()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: defaultJPEGQuality}); err != nil {
		return "", 0, 0, fmt.Errorf("encode JPEG: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	c.Logger.Info("grounding: image prepared",
		"original", fmt.Sprintf("%dx%d", origW, origH),
		"resized", fmt.Sprintf("%dx%d", resW, resH),
		"jpeg_bytes", buf.Len(),
		"base64_len", len(encoded))

	return encoded, resW, resH, nil
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image.Decode: %w", err)
	}
	return img, nil
}

func resizeImage(img image.Image, maxDim int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w <= maxDim && h <= maxDim {
		return img
	}

	scale := float64(maxDim) / float64(max(w, h))
	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)

	resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, xdraw.Src, nil)
	return resized
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var pointPatterns = []*regexp.Regexp{
	regexp.MustCompile(`"point"\s*:\s*\[\s*(\d+)\s*,\s*(\d+)\s*\]`),
	regexp.MustCompile(`\[\s*(\d+)\s*,\s*(\d+)\s*\]`),
}

func parsePoint(text string) (int, int, error) {
	primary := pointPatterns[0].FindStringSubmatch(text)
	if len(primary) >= minRegexMatchLen {
		x, errX := strconv.Atoi(primary[1])
		y, errY := strconv.Atoi(primary[2])
		if errX == nil && errY == nil {
			return x, y, nil
		}
	}

	fallback := pointPatterns[1].FindStringSubmatch(text)
	if len(fallback) >= minRegexMatchLen {
		loc := pointPatterns[1].FindStringIndex(text)
		if loc != nil {
			after := text[loc[1]:]
			if len(after) > 0 && after[0] == ',' {
				return 0, 0, fmt.Errorf("grounding: response looks like a bbox, not a point: %s", truncate(text, maxErrRawLen))
			}
		}
		x, errX := strconv.Atoi(fallback[1])
		y, errY := strconv.Atoi(fallback[2])
		if errX == nil && errY == nil {
			return x, y, nil
		}
	}

	return 0, 0, fmt.Errorf("grounding: could not parse point coordinates from response: %s", truncate(text, maxErrRawLen))
}
