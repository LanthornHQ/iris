package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer() *Server {
	return NewServer(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), "test", 30*time.Second)
}

// --- Health Endpoint ---

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer()

	handler := http.HandlerFunc(s.handleHealth)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
	assert.NotEmpty(t, body["version"])
}

// --- Auth Middleware ---

func TestRequireAuth_NoKeyConfigured(t *testing.T) {
	s := newTestServer()
	s.apiKey = "" // no auth required

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, called, "handler should be called when no API key is configured")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_ValidKey(t *testing.T) {
	s := newTestServer()
	s.apiKey = "test-secret-key"

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, called, "handler should be called with valid Bearer token")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_ValidKeyXApiKey(t *testing.T) {
	s := newTestServer()
	s.apiKey = "test-secret-key"

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Api-Key", "test-secret-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, called, "handler should be called with valid X-Api-Key")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_InvalidKey(t *testing.T) {
	s := newTestServer()
	s.apiKey = "test-secret-key"

	called := false
	handler := s.requireAuth(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.False(t, called, "handler should NOT be called with invalid API key")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	s := newTestServer()
	s.apiKey = "test-secret-key"

	called := false
	handler := s.requireAuth(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.False(t, called, "handler should NOT be called without auth header")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Tool Timeout ---

type slowTool struct{}

func (s *slowTool) Name() string        { return "slow_tool" }
func (s *slowTool) Description() string { return "A tool that takes too long" }
func (s *slowTool) ParametersSchema() map[string]any {
	return map[string]any{"type": "object"}
}
func (s *slowTool) Execute(ctx context.Context, _ map[string]any) (any, error) {
	select {
	case <-time.After(10 * time.Second):
		return map[string]any{"ok": true}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("tool cancelled: %w", ctx.Err())
	}
}

func TestToolTimeout_Enforced(t *testing.T) {
	s := newTestServer()
	s.toolTimeout = 100 * time.Millisecond
	s.RegisterTool(&slowTool{})

	start := time.Now()
	_, err := s.CallTool(context.Background(), "slow_tool", map[string]any{})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	assert.Less(t, elapsed, 2*time.Second, "tool should have been cancelled well before 2s")
}

type fastTool struct{}

func (f *fastTool) Name() string        { return "fast_tool" }
func (f *fastTool) Description() string { return "A fast tool" }
func (f *fastTool) ParametersSchema() map[string]any {
	return map[string]any{"type": "object"}
}
func (f *fastTool) Execute(_ context.Context, _ map[string]any) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestToolTimeout_DoesNotAffectFastTools(t *testing.T) {
	s := newTestServer()
	s.toolTimeout = 5 * time.Second
	s.RegisterTool(&fastTool{})

	result, err := s.CallTool(context.Background(), "fast_tool", map[string]any{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// --- HTTP MCP Endpoint ---

func TestHandleMCP_MethodNotAllowed(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()
	s.handleMCP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleMCP_InvalidJSON(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleMCP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
}

func TestHandleMCP_ValidInitialize(t *testing.T) {
	s := newTestServer()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	s.handleMCP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "2.0", resp["jsonrpc"])
	assert.Nil(t, resp["error"])

	result, ok := resp["result"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "2024-11-05", result["protocolVersion"])
}

// --- HTTP Integration: Auth + Health together ---

func TestHTTPIntegration_HealthBypassesAuth(t *testing.T) {
	s := newTestServer()
	s.apiKey = "secret"

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.requireAuth(s.handleMCP))
	mux.HandleFunc("/health", s.handleHealth)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Health should work without auth
	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// MCP without auth should fail
	mcpResp, err := http.Post(ts.URL+"/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, mcpResp.StatusCode)
	mcpResp.Body.Close()

	// MCP with auth should succeed
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer secret")
	authResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, authResp.StatusCode)
	authResp.Body.Close()

	// MCP with X-Api-Key should also succeed
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req2.Header.Set("X-Api-Key", "secret")
	authResp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, authResp2.StatusCode)

	body, _ := io.ReadAll(authResp2.Body)
	authResp2.Body.Close()
	var result map[string]any
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)
	assert.Nil(t, result["error"], "ping should succeed")
}
