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

type mockImageTool struct{}

func (t *mockImageTool) Name() string { return "image_tool" }

func (t *mockImageTool) Description() string { return "returns a fake image" }

func (t *mockImageTool) ParametersSchema() map[string]any { return map[string]any{} }

func (t *mockImageTool) Execute(_ context.Context, _ map[string]any) (any, error) {
	return map[string]any{
		"success":      true,
		"image_base64": "fake_base64_data",
		"width":        100,
		"height":       200,
	}, nil
}

func TestHandleToolCallWithImageSeparation(t *testing.T) {
	s := NewServer(slog.New(slog.NewJSONHandler(os.Stderr, nil)), "test", 30*time.Second)
	s.RegisterTool(&mockImageTool{})

	params := json.RawMessage(`{"name":"image_tool","arguments":{}}`)
	resp := s.processRequest(context.TODO(), &Request{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params:  params,
	})

	if resp == nil {
		t.Fatal("expected response for tools/call")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	var decodedResult struct {
		Content []map[string]any `json:"content"`
	}
	if err := json.Unmarshal(data, &decodedResult); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	content := decodedResult.Content

	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks (text and image), got %d", len(content))
	}

	// Verify text block (metadata)
	textBlock := content[0]
	if textBlock["type"] != "text" {
		t.Errorf("expected first block type to be 'text', got %v", textBlock["type"])
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(textBlock["text"].(string)), &meta); err != nil {
		t.Fatalf("failed to unmarshal text block: %v", err)
	}
	if meta["image_base64"] != nil {
		t.Errorf("expected 'image_base64' to be removed from metadata, but it exists")
	}
	if meta["success"] != true || meta["width"].(float64) != 100 {
		t.Errorf("metadata missing fields: %+v", meta)
	}

	// Verify image block
	imageBlock := content[1]
	if imageBlock["type"] != "image" {
		t.Errorf("expected second block type to be 'image', got %v", imageBlock["type"])
	}
	if imageBlock["data"] != "fake_base64_data" {
		t.Errorf("expected image data 'fake_base64_data', got %v", imageBlock["data"])
	}
	if imageBlock["mimeType"] != "image/jpeg" {
		t.Errorf("expected mimeType 'image/jpeg', got %v", imageBlock["mimeType"])
	}
}
