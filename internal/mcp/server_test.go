package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestProcessRequestInitialize(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion 2024-11-05, got %v", result["protocolVersion"])
	}
}

func TestProcessRequestPing(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "ping",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestProcessRequestUnknownMethod(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "unknown/method",
	})

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}

func TestProcessRequestToolsList(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/list",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestProcessRequestInvalidJSONRPC(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)

	for _, version := range []string{"1.0", "", "3.0"} {
		resp := s.processRequest(context.TODO(), &Request{
			JSONRPC: version,
			ID:      10,
			Method:  "ping",
		})
		if resp == nil {
			t.Fatalf("expected error response for jsonrpc=%q, got nil", version)
		}
		if resp.Error == nil {
			t.Errorf("expected error for jsonrpc=%q, got success", version)
		}
		if resp.Error.Code != -32600 {
			t.Errorf("expected code -32600 for jsonrpc=%q, got %d", version, resp.Error.Code)
		}
	}
}

func TestProcessRequestNotificationNoResponse(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)

	for _, method := range []string{
		"notifications/initialized",
		"notifications/cancelled",
		"notifications/progress",
		"notifications/roots/list_changed",
		"notifications/tools/list_changed",
	} {
		resp := s.processRequest(context.TODO(), &Request{
			JSONRPC: "2.0",
			ID:      nil,
			Method:  method,
		})
		if resp != nil {
			t.Errorf("expected nil response for notification %q, got %+v", method, resp)
		}
	}
}

func TestProcessRequestNotificationWithIDNil(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      nil,
		Method:  "initialize",
	})
	if resp != nil {
		t.Errorf("expected nil response for notification (id=nil), got %+v", resp)
	}
}

func TestProcessRequestParamsWithNilArguments(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	params := json.RawMessage(`{"name":"test_tool"}`)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params:  params,
	})
	if resp == nil {
		t.Fatal("expected response for tools/call with missing arguments")
	}
	if resp.Error == nil {
		t.Error("expected error for unknown tool, got success")
	}
}
