package tools

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanthornHQ/iris/internal/browser"
)

type mockBrowserDriver struct {
	navigateCalled         int
	clickCalled            int
	doubleClickCalled      int
	typeCalled             int
	scrollCalled           int
	screenshotCalled       int
	waitCalled             int
	closeCalled            int
	lastNavigateURL        string
	lastClickX             int
	lastClickY             int
	lastDoubleClickX       int
	lastDoubleClickY       int
	lastTypeText           string
	lastTypeDelay          int
	lastScrollDir          string
	lastScrollClicks       int
	screenshotB64          string
	screenshotW            int
	screenshotH            int
	screenshotErr          error
	waitStable             bool
	waitElapsedMs          int64
	waitErr                error
	navigateErr            error
	clickErr               error
	doubleClickErr         error
	typeErr                error
	scrollErr              error
	drawMarksCalled        int
	drawMarksErr           error
	getElementCoordsCalled int
	getElementCoordsX      int
	getElementCoordsY      int
	getElementCoordsErr    error
}

func (m *mockBrowserDriver) Navigate(_ context.Context, url string) error {
	m.navigateCalled++
	m.lastNavigateURL = url
	return m.navigateErr
}

func (m *mockBrowserDriver) Title(_ context.Context) (string, error) {
	return "Mock Title", nil
}

func (m *mockBrowserDriver) Click(_ context.Context, x, y int, _ string) error {
	m.clickCalled++
	m.lastClickX = x
	m.lastClickY = y
	return m.clickErr
}

func (m *mockBrowserDriver) DoubleClick(_ context.Context, x, y int) error {
	m.doubleClickCalled++
	m.lastDoubleClickX = x
	m.lastDoubleClickY = y
	return m.doubleClickErr
}

func (m *mockBrowserDriver) Type(_ context.Context, text string, delayMs int) error {
	m.typeCalled++
	m.lastTypeText = text
	m.lastTypeDelay = delayMs
	return m.typeErr
}

func (m *mockBrowserDriver) Scroll(_ context.Context, direction string, clicks int) error {
	m.scrollCalled++
	m.lastScrollDir = direction
	m.lastScrollClicks = clicks
	return m.scrollErr
}

func (m *mockBrowserDriver) Screenshot(_ context.Context) (string, int, int, error) {
	m.screenshotCalled++
	if m.screenshotB64 != "" || m.screenshotErr != nil {
		return m.screenshotB64, m.screenshotW, m.screenshotH, m.screenshotErr
	}
	return "testbase64==", 800, 600, nil
}

func (m *mockBrowserDriver) WaitForStable(_ context.Context, _ int, _ float64) (bool, int64, error) {
	m.waitCalled++
	return m.waitStable, m.waitElapsedMs, m.waitErr
}

func (m *mockBrowserDriver) Close() error {
	m.closeCalled++
	return nil
}

func (m *mockBrowserDriver) DrawMarks(_ context.Context) ([]browser.SomElement, error) {
	m.drawMarksCalled++
	if m.drawMarksErr != nil {
		return nil, m.drawMarksErr
	}
	return []browser.SomElement{
		{
			ID:   1,
			Tag:  "button",
			Text: "Click Me",
			Role: "button",
			Type: "submit",
			Bounds: browser.SomBounds{
				X:      10,
				Y:      20,
				Width:  100,
				Height: 30,
			},
		},
	}, nil
}

func (m *mockBrowserDriver) GetElementCoords(_ context.Context, _ int) (int, int, error) {
	m.getElementCoordsCalled++
	if m.getElementCoordsErr != nil {
		return 0, 0, m.getElementCoordsErr
	}
	return m.getElementCoordsX, m.getElementCoordsY, nil
}

var testLogger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

func TestNavigate_RequiresURL(t *testing.T) {
	drv := &mockBrowserDriver{}
	tool := &Navigate{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url is required")
}

func TestNavigate_InvalidURL(t *testing.T) {
	drv := &mockBrowserDriver{}
	tool := &Navigate{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"url": "not-a-url"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid url")
}

func TestNavigate_Success(t *testing.T) {
	drv := &mockBrowserDriver{}
	tool := &Navigate{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.navigateCalled)
	assert.Equal(t, "https://example.com", drv.lastNavigateURL)

	resp, ok := result.(NavigateResponse)
	require.True(t, ok)
	assert.True(t, resp.Success)
	assert.Equal(t, "https://example.com", resp.URL)
}

func TestNavigate_DriverError(t *testing.T) {
	drv := &mockBrowserDriver{navigateErr: assert.AnError}
	tool := &Navigate{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `navigate to "https://example.com" failed`)
}

func TestScreenshot_Success(t *testing.T) {
	drv := &mockBrowserDriver{screenshotB64: "abc", screenshotW: 100, screenshotH: 200, waitStable: true, waitElapsedMs: 50}
	tool := &Screenshot{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.screenshotCalled)
	assert.Equal(t, 1, drv.drawMarksCalled)

	resp, ok := result.(ScreenshotResponse)
	require.True(t, ok)
	assert.Equal(t, "abc", resp.ImageBase64)
	assert.Equal(t, 100, resp.Width)
	assert.Equal(t, 200, resp.Height)
	assert.True(t, resp.SoMApplied)
	require.Len(t, resp.Elements, 1)
	assert.Equal(t, 1, resp.Elements[0].ID)
	assert.Equal(t, "button", resp.Elements[0].Tag)
	assert.Equal(t, "Click Me", resp.Elements[0].Text)
	assert.Equal(t, "button", resp.Elements[0].Role)
	assert.Equal(t, "submit", resp.Elements[0].Type)
}

func TestScreenshot_WithoutSoM(t *testing.T) {
	drv := &mockBrowserDriver{screenshotB64: "abc", screenshotW: 100, screenshotH: 200}
	tool := &Screenshot{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"som": false})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.screenshotCalled)
	assert.Equal(t, 0, drv.drawMarksCalled)

	resp, ok := result.(ScreenshotResponse)
	require.True(t, ok)
	assert.Equal(t, "abc", resp.ImageBase64)
	assert.False(t, resp.SoMApplied)
	assert.Empty(t, resp.Elements)
}

func TestScreenshot_WithSoMExplicit(t *testing.T) {
	drv := &mockBrowserDriver{screenshotB64: "abc", screenshotW: 100, screenshotH: 200}
	tool := &Screenshot{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"som": true})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.screenshotCalled)
	assert.Equal(t, 1, drv.drawMarksCalled)

	resp, ok := result.(ScreenshotResponse)
	require.True(t, ok)
	assert.Equal(t, "abc", resp.ImageBase64)
	assert.True(t, resp.SoMApplied)
	require.Len(t, resp.Elements, 1)
}

func TestScreenshot_DefaultSoMFails_GracefulDegradation(t *testing.T) {
	drv := &mockBrowserDriver{screenshotB64: "abc", screenshotW: 100, screenshotH: 200, drawMarksErr: assert.AnError}
	tool := &Screenshot{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.screenshotCalled)
	assert.Equal(t, 1, drv.drawMarksCalled)

	resp, ok := result.(ScreenshotResponse)
	require.True(t, ok)
	assert.Equal(t, "abc", resp.ImageBase64)
	assert.False(t, resp.SoMApplied)
	assert.Empty(t, resp.Elements)
}

func TestScreenshot_ExplicitSoMFails_HardError(t *testing.T) {
	drv := &mockBrowserDriver{screenshotB64: "abc", screenshotW: 100, screenshotH: 200, drawMarksErr: assert.AnError}
	tool := &Screenshot{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"som": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "som requested but failed to draw marks")
}

func TestScreenshot_DriverError(t *testing.T) {
	drv := &mockBrowserDriver{screenshotErr: assert.AnError}
	tool := &Screenshot{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "screenshot failed")
}

func TestClick_RequiresCoordinates(t *testing.T) {
	drv := &mockBrowserDriver{}
	tool := &Click{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"x": float64(100)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "y is required")

	_, err = tool.Execute(context.Background(), map[string]any{"y": float64(100)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "x is required")
}

func TestClick_Success(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: true, waitElapsedMs: 50}
	tool := &Click{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"x": float64(100), "y": float64(200)})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.clickCalled)
	assert.Equal(t, 100, drv.lastClickX)
	assert.Equal(t, 200, drv.lastClickY)

	resp, ok := result.(ClickResponse)
	require.True(t, ok)
	assert.True(t, resp.Success)
}

func TestClick_DoubleClick(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: true, waitElapsedMs: 50}
	tool := &Click{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"x": float64(50), "y": float64(100), "button": "double"})
	require.NoError(t, err)
	assert.Equal(t, 0, drv.clickCalled)
	assert.Equal(t, 1, drv.doubleClickCalled)
	assert.Equal(t, 50, drv.lastDoubleClickX)
	assert.Equal(t, 100, drv.lastDoubleClickY)
}

func TestClick_DriverError(t *testing.T) {
	drv := &mockBrowserDriver{clickErr: assert.AnError}
	tool := &Click{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"x": float64(100), "y": float64(200)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "click failed")
}

func TestClick_ElementID(t *testing.T) {
	drv := &mockBrowserDriver{
		getElementCoordsX: 120,
		getElementCoordsY: 240,
	}
	tool := &Click{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"element_id": float64(42)})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.getElementCoordsCalled)
	assert.Equal(t, 1, drv.clickCalled)
	assert.Equal(t, 120, drv.lastClickX)
	assert.Equal(t, 240, drv.lastClickY)

	resp, ok := result.(ClickResponse)
	require.True(t, ok)
	assert.True(t, resp.Success)
}

func TestClick_ElementIDError(t *testing.T) {
	drv := &mockBrowserDriver{
		getElementCoordsErr: assert.AnError,
	}
	tool := &Click{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"element_id": float64(42)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locating element 42")
	assert.Equal(t, 1, drv.getElementCoordsCalled)
	assert.Equal(t, 0, drv.clickCalled)
}

func TestClick_Annotate(t *testing.T) {
	// A tiny valid base64-encoded 100x100 JPEG to avoid decoding failures in drawClickDot
	tinyJPEG := "/9j/2wCEAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDIBCQkJDAsMGA0NGDIhHCEyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMv/AABEIAGQAZAMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/APf6KKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKAP//Z"

	t.Run("default_false_without_param", func(t *testing.T) {
		drv := &mockBrowserDriver{screenshotB64: tinyJPEG, screenshotW: 100, screenshotH: 100}
		tool := &Click{Logger: testLogger, Driver: drv, AnnotateDefault: false}
		result, err := tool.Execute(context.Background(), map[string]any{"x": float64(0), "y": float64(0)})
		require.NoError(t, err)
		resp := result.(ClickResponse)
		// Should not be annotated (remains tinyJPEG)
		assert.Equal(t, tinyJPEG, resp.ImageBase64)
	})

	t.Run("default_false_override_true", func(t *testing.T) {
		drv := &mockBrowserDriver{screenshotB64: tinyJPEG, screenshotW: 100, screenshotH: 100}
		tool := &Click{Logger: testLogger, Driver: drv, AnnotateDefault: false}
		result, err := tool.Execute(context.Background(), map[string]any{"x": float64(50), "y": float64(50), "annotate": true})
		require.NoError(t, err)
		resp := result.(ClickResponse)
		// Should be annotated (different from tinyJPEG)
		assert.NotEqual(t, tinyJPEG, resp.ImageBase64)
		assert.NotEmpty(t, resp.ImageBase64)
	})

	t.Run("default_true_without_param", func(t *testing.T) {
		drv := &mockBrowserDriver{screenshotB64: tinyJPEG, screenshotW: 100, screenshotH: 100}
		tool := &Click{Logger: testLogger, Driver: drv, AnnotateDefault: true}
		result, err := tool.Execute(context.Background(), map[string]any{"x": float64(50), "y": float64(50)})
		require.NoError(t, err)
		resp := result.(ClickResponse)
		// Should be annotated (different from tinyJPEG)
		assert.NotEqual(t, tinyJPEG, resp.ImageBase64)
		assert.NotEmpty(t, resp.ImageBase64)
	})

	t.Run("default_true_override_false", func(t *testing.T) {
		drv := &mockBrowserDriver{screenshotB64: tinyJPEG, screenshotW: 100, screenshotH: 100}
		tool := &Click{Logger: testLogger, Driver: drv, AnnotateDefault: true}
		result, err := tool.Execute(context.Background(), map[string]any{"x": float64(50), "y": float64(50), "annotate": false})
		require.NoError(t, err)
		resp := result.(ClickResponse)
		// Should not be annotated (remains tinyJPEG)
		assert.Equal(t, tinyJPEG, resp.ImageBase64)
	})
}

func TestScroll_Success(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: true, waitElapsedMs: 50}
	tool := &Scroll{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"direction": "up", "clicks": float64(5)})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.scrollCalled)
	assert.Equal(t, "up", drv.lastScrollDir)
	assert.Equal(t, 5, drv.lastScrollClicks)

	resp, ok := result.(ScrollResponse)
	require.True(t, ok)
	assert.True(t, resp.Success)
}

func TestScroll_Defaults(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: true, waitElapsedMs: 50}
	tool := &Scroll{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.scrollCalled)
	assert.Equal(t, "down", drv.lastScrollDir)
	assert.Equal(t, 3, drv.lastScrollClicks)
}

func TestScroll_DriverError(t *testing.T) {
	drv := &mockBrowserDriver{scrollErr: assert.AnError}
	tool := &Scroll{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"direction": "down"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scroll failed")
}

func TestWaitForStable_Success(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: true, waitElapsedMs: 200}
	tool := &WaitForStable{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"timeout_ms": float64(5000)})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.waitCalled)

	resp, ok := result.(WaitForStableResponse)
	require.True(t, ok)
	assert.True(t, resp.Stable)
	assert.Equal(t, int64(200), resp.ElapsedMs)
}

func TestWaitForStable_DefaultTimeout(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: false, waitElapsedMs: 5000}
	tool := &WaitForStable{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.waitCalled)

	resp, ok := result.(WaitForStableResponse)
	require.True(t, ok)
	assert.False(t, resp.Stable)
}

func TestWaitForStable_ExceedsMax(t *testing.T) {
	drv := &mockBrowserDriver{}
	tool := &WaitForStable{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"timeout_ms": float64(5000000)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestWaitForStable_DriverError(t *testing.T) {
	drv := &mockBrowserDriver{waitErr: assert.AnError}
	tool := &WaitForStable{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{"timeout_ms": float64(5000)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wait_for_stable")
}

func TestSleep_RequiredDuration(t *testing.T) {
	tool := &Sleep{Logger: testLogger}

	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration_ms is required")
}

func TestSleep_NegativeDuration(t *testing.T) {
	tool := &Sleep{Logger: testLogger}

	_, err := tool.Execute(context.Background(), map[string]any{"duration_ms": float64(-1)})
	require.Error(t, err)
}

func TestSleep_Success(t *testing.T) {
	tool := &Sleep{Logger: testLogger}

	result, err := tool.Execute(context.Background(), map[string]any{"duration_ms": float64(50)})
	require.NoError(t, err)

	resp, ok := result.(SleepResponse)
	require.True(t, ok)
	assert.True(t, resp.Success)
}

func TestGetDatetime_Success(t *testing.T) {
	tool := &GetDatetime{Logger: testLogger}

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)

	resp, ok := result.(GetDatetimeResponse)
	require.True(t, ok)
	assert.NotEmpty(t, resp.Datetime)
	assert.NotEmpty(t, resp.Date)
	assert.NotEmpty(t, resp.Time)
	assert.NotEmpty(t, resp.Weekday)
}

func TestTypeText_RequiredText(t *testing.T) {
	drv := &mockBrowserDriver{}
	tool := &TypeText{Logger: testLogger, Driver: drv}

	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text is required")
}

func TestTypeText_SimpleType(t *testing.T) {
	drv := &mockBrowserDriver{waitStable: true, waitElapsedMs: 50}
	tool := &TypeText{Logger: testLogger, Driver: drv}

	result, err := tool.Execute(context.Background(), map[string]any{"text": "hello"})
	require.NoError(t, err)
	assert.Equal(t, 1, drv.typeCalled)
	assert.Equal(t, "hello", drv.lastTypeText)

	resp, ok := result.(TypeTextResponse)
	require.True(t, ok)
	assert.True(t, resp.Success)
}

func TestIntArg(t *testing.T) {
	_, err := intArg(map[string]any{}, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "x is required")

	v, err := intArg(map[string]any{"x": float64(42)}, "x")
	require.NoError(t, err)
	assert.Equal(t, 42, v)

	_, err = intArg(map[string]any{"x": float64(-1)}, "x")
	require.Error(t, err)
}

func TestOptIntArg(t *testing.T) {
	assert.Equal(t, 10, optIntArg(map[string]any{}, "delay_ms", 10))
	assert.Equal(t, 5, optIntArg(map[string]any{"delay_ms": float64(5)}, "delay_ms", 10))
}

func TestDriverInterface(_ *testing.T) {
	var _ browser.Driver = &mockBrowserDriver{}
}
