package browser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp/kb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDriver struct {
	navigateCalled         int
	clickCalled            int
	typeCalled             int
	scrollCalled           int
	screenshotFn           func(ctx context.Context) (string, int, int, error)
	waitForStableFn        func(ctx context.Context, timeoutMs int, threshold float64) (bool, int64, error)
	closeCalled            int
	lastClickX             int
	lastClickY             int
	lastTypeText           string
	lastScrollDir          string
	lastScrollClicks       int
	lastNavigateURL        string
	drawMarksCalled        int
	getElementCoordsCalled int
}

func (m *mockDriver) Navigate(_ context.Context, url string) error {
	m.navigateCalled++
	m.lastNavigateURL = url
	return nil
}

func (m *mockDriver) Title(_ context.Context) (string, error) {
	return "Mock Title", nil
}

func (m *mockDriver) Click(_ context.Context, x, y int, _ string) error {
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

func (m *mockDriver) DrawMarks(_ context.Context) ([]SomElement, error) {
	m.drawMarksCalled++
	return []SomElement{
		{
			ID:   "A",
			Tag:  "button",
			Text: "Click Me",
			Role: "button",
			Type: "submit",
			Bounds: SomBounds{
				X:      10,
				Y:      20,
				Width:  100,
				Height: 30,
			},
		},
	}, nil
}

func (m *mockDriver) ClearMarks(_ context.Context) error { return nil }

func (m *mockDriver) GetElementCoords(_ context.Context, _ string) (int, int, error) {
	m.getElementCoordsCalled++
	return 0, 0, nil
}

func (m *mockDriver) PageTree(_ context.Context) (string, error) {
	return "A] button \"Click Me\"", nil
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
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

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.False(t, cfg.Headless)
	assert.Equal(t, "/usr/bin/chromium", cfg.ChromePath)
	assert.False(t, cfg.NoSandbox)
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	t.Setenv("IRIS_HEADLESS", "0")
	t.Setenv("IRIS_NO_SANDBOX", "0")
	t.Setenv("IRIS_WINDOW_WIDTH", "1024")
	t.Setenv("IRIS_WINDOW_HEIGHT", "768")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.False(t, cfg.Headless)
	assert.False(t, cfg.NoSandbox)
	assert.Equal(t, 1024, cfg.Width)
	assert.Equal(t, 768, cfg.Height)
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
	err := md.Click(context.Background(), 100, 200, "left")
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

func TestToBase62(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{1, "A"},
		{2, "B"},
		{3, "C"},
		{25, "Y"},
		{26, "Z"},
		{27, "a"},
		{51, "y"},
		{52, "z"},
		{61, "8"},
		{62, "9"},
		{63, "AA"},
		{199, "CM"},
		{200, "CN"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d->%s", tt.input, tt.expected), func(t *testing.T) {
			assert.Equal(t, tt.expected, toBase62(tt.input))
		})
	}
}

func TestToBase62_MatchesJSReference(t *testing.T) {
	expected := map[int]string{
		1: "A", 2: "B", 3: "C", 26: "Z", 27: "a",
		51: "y", 52: "z", 63: "AA", 200: "CN",
	}
	for n, want := range expected {
		got := toBase62(n)
		assert.Equal(t, want, got, "toBase62(%d) mismatch: Go=%q, expected=%q", n, got, want)
	}
}

func TestNewDriver_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewDriver(ctx, logger, Config{Headless: true, NoSandbox: true})
	require.Error(t, err)
}

func TestParseSpecialKeys(t *testing.T) {
	seg := func(keys string, mod ...input.Modifier) keySegment {
		if len(mod) > 0 {
			return keySegment{keys: keys, modifier: mod[0]}
		}
		return keySegment{keys: keys}
	}

	tests := []struct {
		input    string
		expected []keySegment
	}{
		{
			input:    "hello",
			expected: []keySegment{seg("h"), seg("e"), seg("l"), seg("l"), seg("o")},
		},
		{
			input: "Tokyo weather{Enter}",
			expected: []keySegment{
				seg("T"), seg("o"), seg("k"), seg("y"), seg("o"), seg(" "),
				seg("w"), seg("e"), seg("a"), seg("t"), seg("h"), seg("e"), seg("r"),
				seg("\r"),
			},
		},
		{
			input:    "{{curly}}",
			expected: []keySegment{seg("{"), seg("c"), seg("u"), seg("r"), seg("l"), seg("y"), seg("}")},
		},
		{
			input:    "{Tab}next",
			expected: []keySegment{seg("\t"), seg("n"), seg("e"), seg("x"), seg("t")},
		},
		{
			input:    "{Ctrl+A}{Backspace}",
			expected: []keySegment{seg("a", input.ModifierCtrl), seg("\b")},
		},
		{
			input: "case{enter}test",
			expected: []keySegment{
				seg("c"), seg("a"), seg("s"), seg("e"), seg("\r"),
				seg("t"), seg("e"), seg("s"), seg("t"),
			},
		},
		{
			input:    "{Shift-Tab}",
			expected: []keySegment{seg("\t", input.ModifierShift)},
		},
		{
			input:    "{Ctrl+C}{Ctrl+V}",
			expected: []keySegment{seg("c", input.ModifierCtrl), seg("v", input.ModifierCtrl)},
		},
		{
			input:    "{Up}{Down}{Left}{Right}",
			expected: []keySegment{seg(kb.ArrowUp), seg(kb.ArrowDown), seg(kb.ArrowLeft), seg(kb.ArrowRight)},
		},
		{
			input:    "{Home}{End}{PageUp}{PageDown}",
			expected: []keySegment{seg(kb.Home), seg(kb.End), seg(kb.PageUp), seg(kb.PageDown)},
		},
		{
			input:    "{unknown}x",
			expected: []keySegment{seg("{"), seg("u"), seg("n"), seg("k"), seg("n"), seg("o"), seg("w"), seg("n"), seg("}"), seg("x")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseSpecialKeys(tt.input))
		})
	}
}
