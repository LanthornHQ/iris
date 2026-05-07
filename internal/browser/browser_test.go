package browser

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDriver struct {
	navigateCalled   int
	clickCalled      int
	typeCalled       int
	scrollCalled     int
	screenshotFn     func(ctx context.Context) (string, int, int, error)
	waitForStableFn  func(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error)
	closeCalled      int
	lastClickX       int
	lastClickY       int
	lastTypeText     string
	lastScrollDir    string
	lastScrollClicks int
	lastNavigateURL  string
}

func (m *mockDriver) Navigate(_ context.Context, url string) error {
	m.navigateCalled++
	m.lastNavigateURL = url
	return nil
}

func (m *mockDriver) Title(_ context.Context) (string, error) {
	return "Mock Title", nil
}

func (m *mockDriver) Click(_ context.Context, x, y int) error {
	m.clickCalled++
	m.lastClickX = x
	m.lastClickY = y
	return nil
}

func (m *mockDriver) DoubleClick(_ context.Context, _, _ int) error {
	return nil
}

func (m *mockDriver) Type(_ context.Context, text string, _ int) error {
	m.typeCalled++
	m.lastTypeText = text
	return nil
}

func (m *mockDriver) Scroll(_ context.Context, direction string, clicks int) error {
	m.scrollCalled++
	m.lastScrollDir = direction
	m.lastScrollClicks = clicks
	return nil
}

func (m *mockDriver) Screenshot(ctx context.Context) (string, int, int, error) {
	if m.screenshotFn != nil {
		return m.screenshotFn(ctx)
	}
	return "base64data", 1920, 1080, nil
}

func (m *mockDriver) WaitForStable(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error) {
	if m.waitForStableFn != nil {
		return m.waitForStableFn(ctx, timeoutMs, threshold)
	}
	return true, 100, nil
}

func (m *mockDriver) Close() error {
	m.closeCalled++
	return nil
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	cfg := ConfigFromEnv()
	assert.True(t, cfg.Headless)
	assert.Equal(t, 1920, cfg.Width)
	assert.Equal(t, 1080, cfg.Height)
	assert.True(t, cfg.NoSandbox)
	assert.Equal(t, 30000, cfg.TimeoutMs)
	assert.Empty(t, cfg.ChromePath)
}

func TestConfigFromEnv_Custom(t *testing.T) {
	t.Setenv("IRIS_HEADLESS", "false")
	t.Setenv("IRIS_CHROME_PATH", "/usr/bin/chromium")
	t.Setenv("IRIS_NO_SANDBOX", "false")

	cfg := ConfigFromEnv()
	assert.False(t, cfg.Headless)
	assert.Equal(t, "/usr/bin/chromium", cfg.ChromePath)
	assert.False(t, cfg.NoSandbox)
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	t.Setenv("IRIS_HEADLESS", "0")
	t.Setenv("IRIS_NO_SANDBOX", "0")

	cfg := ConfigFromEnv()
	assert.False(t, cfg.Headless)
	assert.False(t, cfg.NoSandbox)
}

func TestNewDriver_Interface(_ *testing.T) {
	var _ Driver = &mockDriver{}
	var _ Driver = (*chromedpDriver)(nil)
}

func TestMockDriver_Navigate(t *testing.T) {
	md := &mockDriver{}
	err := md.Navigate(context.Background(), "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, 1, md.navigateCalled)
	assert.Equal(t, "https://example.com", md.lastNavigateURL)
}

func TestMockDriver_Click(t *testing.T) {
	md := &mockDriver{}
	err := md.Click(context.Background(), 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 1, md.clickCalled)
	assert.Equal(t, 100, md.lastClickX)
	assert.Equal(t, 200, md.lastClickY)
}

func TestMockDriver_Type(t *testing.T) {
	md := &mockDriver{}
	err := md.Type(context.Background(), "hello", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, md.typeCalled)
	assert.Equal(t, "hello", md.lastTypeText)
}

func TestMockDriver_Scroll(t *testing.T) {
	md := &mockDriver{}
	err := md.Scroll(context.Background(), "down", 3)
	require.NoError(t, err)
	assert.Equal(t, 1, md.scrollCalled)
	assert.Equal(t, "down", md.lastScrollDir)
	assert.Equal(t, 3, md.lastScrollClicks)
}

func TestMockDriver_Screenshot(t *testing.T) {
	md := &mockDriver{}
	b64, w, h, err := md.Screenshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "base64data", b64)
	assert.Equal(t, 1920, w)
	assert.Equal(t, 1080, h)
}

func TestMockDriver_WaitForStable(t *testing.T) {
	md := &mockDriver{}
	stable, elapsed, err := md.WaitForStable(context.Background(), 5000, 0.01)
	require.NoError(t, err)
	assert.True(t, stable)
	assert.Equal(t, int64(100), elapsed)
}

func TestMockDriver_Close(t *testing.T) {
	md := &mockDriver{}
	err := md.Close()
	require.NoError(t, err)
	assert.Equal(t, 1, md.closeCalled)
}

func TestNewDriver_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewDriver(ctx, logger, Config{Headless: true, NoSandbox: true})
	require.Error(t, err)
}
