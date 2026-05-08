package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	goframeagent "github.com/sevigo/goframe/agent"

	"github.com/LanthornHQ/iris/internal/metrics"
)

const (
	defaultToolTimeout       = 30 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 60 * time.Second
	defaultWriteTimeout      = 5 * time.Minute
	defaultIdleTimeout       = 120 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
)

// Tool is implemented by every browser-control action.
type Tool interface {
	Name() string
	Description() string
	ParametersSchema() map[string]any
	Execute(ctx context.Context, args map[string]any) (any, error)
}

type Server struct {
	registry    *goframeagent.Registry
	logger      *slog.Logger
	apiKey      string
	version     string
	toolTimeout time.Duration

	toolCount atomic.Int32
}

func NewServer(logger *slog.Logger, version string) *Server {
	toolTimeout := defaultToolTimeout
	if v := os.Getenv("IRIS_TOOL_TIMEOUT"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			toolTimeout = time.Duration(d) * time.Second
		}
	}
	return &Server{
		registry:    goframeagent.NewRegistry(),
		logger:      logger,
		apiKey:      os.Getenv("IRIS_API_KEY"),
		version:     version,
		toolTimeout: toolTimeout,
	}
}

func (s *Server) RegisterTool(tool Tool) {
	s.registry.MustRegisterTool(tool)
	s.toolCount.Add(1)
}

func (s *Server) ListTools() []ToolInfo {
	defs := s.registry.Definitions()
	tools := make([]ToolInfo, 0, len(defs))
	for _, def := range defs {
		fn, ok := def["function"].(map[string]any)
		if !ok {
			s.logger.Warn("skipping malformed tool definition", "def_keys", keysOf(def))
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		if name == "" {
			s.logger.Warn("skipping tool with empty name")
			continue
		}
		tools = append(tools, ToolInfo{
			Name:        name,
			Description: desc,
			InputSchema: params,
		})
	}
	return tools
}

// ToolNames returns just the names of all registered tools.
func (s *Server) ToolNames() []string {
	tools := s.ListTools()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, s.toolTimeout)
	defer cancel()

	s.logger.DebugContext(ctx, "tool execution starting", "tool", name, "timeout", s.toolTimeout)
	start := time.Now()
	result, err := s.registry.Execute(ctx, name, args)
	elapsed := time.Since(start)
	if err != nil {
		s.logger.ErrorContext(ctx, "tool call failed",
			"tool", name,
			"elapsed_ms", elapsed.Milliseconds(),
			"error", err)
		metrics.ToolCallsTotal.WithLabelValues(name, "error").Inc()
		metrics.ToolCallDuration.WithLabelValues(name).Observe(elapsed.Seconds())
		return nil, err
	}
	s.logger.InfoContext(ctx, "tool call completed",
		"tool", name,
		"elapsed_ms", elapsed.Milliseconds(),
		"result_type", fmt.Sprintf("%T", result))
	metrics.ToolCallsTotal.WithLabelValues(name, "success").Inc()
	metrics.ToolCallDuration.WithLabelValues(name).Observe(elapsed.Seconds())
	return result, nil
}

func (s *Server) RunHTTP(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.requireAuth(s.handleMCP))
	mux.HandleFunc("/health", s.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	hs := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	authStatus := "disabled"
	if s.apiKey != "" {
		authStatus = "enabled"
	}
	s.logger.InfoContext(ctx, "iris MCP server starting",
		"transport", "http",
		"endpoint", fmt.Sprintf("http://%s/mcp", listener.Addr()),
		"health", fmt.Sprintf("http://%s/health", listener.Addr()),
		"auth", authStatus)

	errCh := make(chan error, 1)
	go func() {
		if err := hs.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()
		_ = hs.Shutdown(shutCtx)
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"tools":   s.toolCount.Load(),
		"version": s.version,
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" && !s.authenticate(r) {
			s.logger.Warn("HTTP request unauthorized", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) authenticate(r *http.Request) bool {
	key := []byte(s.apiKey)
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), key) == 1 {
			return true
		}
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Api-Key")), key) == 1 {
		return true
	}
	return false
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.logger.Warn("HTTP request rejected", "method", r.Method, "remote", r.RemoteAddr)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const maxRequestBytes = 4 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("HTTP request parse error", "remote", r.RemoteAddr, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"error": map[string]any{
				"code":    -32700,
				"message": "Parse error",
			},
		})
		return
	}

	s.logger.Info("HTTP request", "remote", r.RemoteAddr, "method", req.Method, "id", req.ID,
		"tool", extractToolName(req))

	resp := s.processRequest(r.Context(), &req)

	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func extractToolName(req Request) string {
	if req.Method != "tools/call" || req.Params == nil {
		return ""
	}
	var p struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(req.Params, &p) == nil {
		return p.Name
	}
	return ""
}

func (s *Server) processRequest(ctx context.Context, req *Request) *Response {
	s.logger.DebugContext(ctx, "received MCP request", "method", req.Method, "id", req.ID,
		"params_len", len(req.Params))

	if req.JSONRPC != "2.0" {
		s.logger.WarnContext(ctx, "invalid JSON-RPC version", "jsonrpc", req.JSONRPC, "id", req.ID)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32600, Message: "invalid request: jsonrpc must be \"2.0\""},
		}
	}

	if req.ID == nil {
		s.logger.DebugContext(ctx, "received notification, ignoring", "method", req.Method)
		return nil
	}

	switch req.Method {
	case "initialize":
		s.logger.InfoContext(ctx, "MCP initialize", "id", req.ID)
		resp := &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]any{
					"name":    "iris",
					"version": s.version,
				},
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
			},
		}
		s.logger.DebugContext(ctx, "MCP initialize response sent", "id", req.ID, "version", s.version)
		return resp

	case "ping":
		s.logger.DebugContext(ctx, "MCP ping", "id", req.ID)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}

	case "tools/list":
		s.logger.DebugContext(ctx, "MCP tools/list", "id", req.ID)
		tools := s.ListTools()
		toolNames := make([]string, len(tools))
		for i, t := range tools {
			toolNames[i] = t.Name
		}
		s.logger.DebugContext(ctx, "MCP tools/list response", "id", req.ID, "tools", toolNames)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": tools,
			},
		}

	case "tools/call":
		return s.handleToolCall(ctx, req)

	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			s.logger.DebugContext(ctx, "received notification", "method", req.Method)
			return nil
		}
		s.logger.WarnContext(ctx, "unknown MCP method", "method", req.Method, "id", req.ID)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

func (s *Server) handleToolCall(ctx context.Context, req *Request) *Response {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.logger.ErrorContext(ctx, "MCP tools/call: invalid params", "id", req.ID, "error", err)
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: -32602, Message: "invalid params"},
			}
		}
	}

	s.logger.InfoContext(ctx, "MCP tools/call invoked", "id", req.ID, "tool", params.Name, "arg_keys", keysOf(params.Arguments))

	result, err := s.CallTool(ctx, params.Name, params.Arguments)
	if err != nil {
		s.logger.ErrorContext(ctx, "MCP tools/call failed", "id", req.ID, "tool", params.Name, "error", err)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32603, Message: err.Error()},
		}
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		s.logger.ErrorContext(ctx, "failed to marshal tool result", "tool", params.Name, "error", marshalErr)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32603, Message: fmt.Sprintf("failed to marshal result: %v", marshalErr)},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(resultJSON)},
			},
		},
	}
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
