package grounding

import (
	"encoding/json"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("IRIS_GROUNDING_URL", "http://example.com:9999/v1/chat")
	t.Setenv("IRIS_GROUNDING_MODEL", "test-model")
	t.Setenv("IRIS_GROUNDING_TIMEOUT", "42")
	t.Setenv("IRIS_GROUNDING_MAX_DIM", "2048")
	t.Setenv("IRIS_GROUNDING_BBOX_SCALE", "2000")

	cfg := ConfigFromEnv()
	if cfg.BaseURL != "http://example.com:9999/v1/chat" {
		t.Errorf("BaseURL = %q, want http://example.com:9999/v1/chat", cfg.BaseURL)
	}
	if cfg.ModelName != "test-model" {
		t.Errorf("ModelName = %q, want test-model", cfg.ModelName)
	}
	if cfg.TimeoutSec != 42 {
		t.Errorf("TimeoutSec = %d, want 42", cfg.TimeoutSec)
	}
	if cfg.MaxDim != 2048 {
		t.Errorf("MaxDim = %d, want 2048", cfg.MaxDim)
	}
	if cfg.BBoxScale != 2000 {
		t.Errorf("BBoxScale = %d, want 2000", cfg.BBoxScale)
	}
}

func TestConfigFromEnv_Empty(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty", cfg.BaseURL)
	}
	if cfg.ModelName != "" {
		t.Errorf("ModelName = %q, want empty", cfg.ModelName)
	}
	if cfg.TimeoutSec != 0 {
		t.Errorf("TimeoutSec = %d, want 0", cfg.TimeoutSec)
	}
}

func TestConfigFromEnv_InvalidValues(t *testing.T) {
	t.Setenv("IRIS_GROUNDING_URL", "http://valid.url")
	t.Setenv("IRIS_GROUNDING_TIMEOUT", "not-a-number")
	t.Setenv("IRIS_GROUNDING_MAX_DIM", "-5")

	cfg := ConfigFromEnv()
	if cfg.BaseURL != "http://valid.url" {
		t.Errorf("BaseURL = %q, want http://valid.url", cfg.BaseURL)
	}
	if cfg.TimeoutSec != 0 {
		t.Errorf("TimeoutSec = %d, want 0 for invalid input", cfg.TimeoutSec)
	}
	if cfg.MaxDim != 0 {
		t.Errorf("MaxDim = %d, want 0 for negative input", cfg.MaxDim)
	}
}

func TestParseVerifyResponse(t *testing.T) {
	tests := []struct {
		input        string
		wantAnswer   string
		wantEvidence string
		wantBBox     []int
	}{
		{`{"answer":"yes","evidence":"The display shows the number 7","bbox":[100,200,300,400]}`, "yes", "The display shows the number 7", []int{100, 200, 300, 400}},
		{`{"answer":"no","evidence":"The display shows 42","bbox":[50,60,150,160]}`, "no", "The display shows 42", []int{50, 60, 150, 160}},
		{`{"answer":"yes","evidence":"Display reads 3"}`, "yes", "Display reads 3", nil},
		{"```json\n{\"answer\":\"yes\",\"evidence\":\"You can see the button\",\"bbox\":[10,20,30,40]}\n```", "yes", "You can see the button", []int{10, 20, 30, 40}},
		{`{"answer": "Yes", "evidence": "Visible"}`, "yes", "Visible", nil},
	}

	for _, tt := range tests {
		ans, ev, bbox := parseVerifyResponse(tt.input)
		if ans != tt.wantAnswer {
			t.Errorf("parseVerifyResponse(%q) answer = %q, want %q", tt.input, ans, tt.wantAnswer)
		}
		if ev != tt.wantEvidence {
			t.Errorf("parseVerifyResponse(%q) evidence = %q, want %q", tt.input, ev, tt.wantEvidence)
		}
		if !bboxSliceEqual(bbox, tt.wantBBox) {
			t.Errorf("parseVerifyResponse(%q) bbox = %v, want %v", tt.input, bbox, tt.wantBBox)
		}
	}
}

func bboxSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStripModelPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openai/kv-ground-8b-baseguiowl1.5-0315", "kv-ground-8b-baseguiowl1.5-0315"},
		{"ollama/kimi-k2.6:cloud", "kimi-k2.6:cloud"},
		{"openai/gpt-4o", "gpt-4o"},
		{"kv-ground-8b-baseguiowl1.5-0315", "kv-ground-8b-baseguiowl1.5-0315"},
		{"openai/", ""},
		{"ollama/", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripModelPrefix(tt.input)
		if got != tt.want {
			t.Errorf("stripModelPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestChatResponseErrorString(t *testing.T) {
	var e chatResponseError
	if e.String() != "" {
		t.Errorf("expected empty string for zero value, got %q", e.String())
	}

	if err := json.Unmarshal([]byte(`"something went wrong"`), &e); err != nil {
		t.Fatalf("unmarshal string error: %v", err)
	}
	if e.String() != "something went wrong" {
		t.Errorf("got %q, want %q", e.String(), "something went wrong")
	}

	var e2 chatResponseError
	if err := json.Unmarshal([]byte(`{"message":"model overloaded"}`), &e2); err != nil {
		t.Fatalf("unmarshal object error: %v", err)
	}
	if e2.String() != "model overloaded" {
		t.Errorf("got %q, want %q", e2.String(), "model overloaded")
	}
}

func TestParsePoint(t *testing.T) {
	tests := []struct {
		input   string
		wantX   int
		wantY   int
		wantErr bool
	}{
		{`{"description":"destination field","point":[604,283]}`, 604, 283, false},
		{`{"point":[500,300]}`, 500, 300, false},
		{`The point is [750,420] in the image.`, 750, 420, false},
		{`no point here`, 0, 0, true},
		{`[100]`, 0, 0, true},
		{`{"bbox":[100,200,300,400]}`, 0, 0, true},
		{`{"description":"field","bbox":[604,283,800,500]}`, 0, 0, true},
	}

	for _, tt := range tests {
		x, y, err := parsePoint(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parsePoint(%q) expected error, got (%d,%d)", tt.input, x, y)
			}
		} else {
			if err != nil {
				t.Errorf("parsePoint(%q) unexpected error: %v", tt.input, err)
			}
			if x != tt.wantX || y != tt.wantY {
				t.Errorf("parsePoint(%q) = (%d,%d), want (%d,%d)", tt.input, x, y, tt.wantX, tt.wantY)
			}
		}
	}
}

func TestStripThinkingTags(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`<thinking>reasoning here</thinking>{"point":[500,300]}`, `{"point":[500,300]}`},
		{`<thinking>let me think</thinking>{"description":"btn","bbox":[10,20,30,40]}`, `{"description":"btn","bbox":[10,20,30,40]}`},
		{`no thinking tags here`, `no thinking tags here`},
		{`<thinking>multi\nline</thinking>result`, `result`},
		{`before<thinking>middle</thinking>after`, `beforeafter`},
	}

	for _, tt := range tests {
		got := stripThinkingTags(tt.input)
		if got != tt.want {
			t.Errorf("stripThinkingTags(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
