package synthetic

import (
	"encoding/json"
	"strings"
	"testing"

	"antigravity-gateway/internal/config"
)

func TestRequestInjectorBasic(t *testing.T) {
	cfg := &config.Config{
		ControlMessagePosition: "tail",
		SyntheticToolPrefix:    "agw_emit_",
		SyntheticToolStrict:    false,
		WrapperMode:            "prefer",
	}

	inj := NewRequestInjector(cfg)

	rawReq := []byte(`{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "user", "content": "hello"}
		]
	}`)

	res, err := inj.Inject(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.OriginalModel != "gpt-4o-mini" {
		t.Errorf("expected model gpt-4o-mini, got %s", res.OriginalModel)
	}
	if !strings.HasPrefix(res.SyntheticToolName, "agw_emit_") {
		t.Errorf("expected prefix agw_emit_, got %s", res.SyntheticToolName)
	}
	if len(res.SyntheticToolName) != len("agw_emit_")+24 {
		t.Errorf("expected 24 hex characters for nonce, got %s", res.SyntheticToolName)
	}

	var parsed map[string]any
	if err := json.Unmarshal(res.TransformedBody, &parsed); err != nil {
		t.Fatalf("failed to unmarshal transformed body: %v", err)
	}

	// Verify tools array
	tools, ok := parsed["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", parsed["tools"])
	}
	t0 := tools[0].(map[string]any)
	fn := t0["function"].(map[string]any)
	if fn["name"] != res.SyntheticToolName {
		t.Errorf("tool name mismatch: %v vs %s", fn["name"], res.SyntheticToolName)
	}

	// Verify messages array
	msgs, ok := parsed["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	lastMsg := msgs[1].(map[string]any)
	if lastMsg["role"] != "user" || !strings.Contains(lastMsg["content"].(string), res.SyntheticToolName) {
		t.Errorf("control message not injected properly: %+v", lastMsg)
	}
}

func TestRequestInjectorPreservesRealTools(t *testing.T) {
	cfg := &config.Config{
		ControlMessageRole:     "developer",
		ControlMessagePosition: "head",
		SyntheticToolPrefix:    "agw_emit_",
		WrapperMode:            "prefer",
	}

	inj := NewRequestInjector(cfg)

	rawReq := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "check weather"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "get weather",
					"parameters": {"type": "object"}
				}
			},
			{
				"type": "function",
				"function": {
					"name": "calculate",
					"description": "calc",
					"parameters": {"type": "object"}
				}
			}
		],
		"tool_choice": "none"
	}`)

	res, err := inj.Inject(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(res.TransformedBody, &parsed); err != nil {
		t.Fatalf("failed to unmarshal transformed body: %v", err)
	}

	tools := parsed["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools (2 real + 1 synthetic), got %d", len(tools))
	}

	// Check tool 0 is get_weather
	fn0 := tools[0].(map[string]any)["function"].(map[string]any)
	if fn0["name"] != "get_weather" {
		t.Errorf("first tool modified: %v", fn0["name"])
	}

	// Check tool 1 is calculate
	fn1 := tools[1].(map[string]any)["function"].(map[string]any)
	if fn1["name"] != "calculate" {
		t.Errorf("second tool modified: %v", fn1["name"])
	}

	// Check tool 2 is synthetic tool
	fn2 := tools[2].(map[string]any)["function"].(map[string]any)
	if fn2["name"] != res.SyntheticToolName {
		t.Errorf("synthetic tool mismatch: %v vs %s", fn2["name"], res.SyntheticToolName)
	}

	// Check tool_choice none -> auto
	if parsed["tool_choice"] != "auto" {
		t.Errorf("expected tool_choice auto, got %v", parsed["tool_choice"])
	}

	// Check messages head position and developer role
	msgs := parsed["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	firstMsg := msgs[0].(map[string]any)
	if firstMsg["role"] != "developer" {
		t.Errorf("expected developer role, got %s", firstMsg["role"])
	}
}

func TestRequestInjectorWrapperModeOff(t *testing.T) {
	cfg := &config.Config{
		WrapperMode: "off",
	}

	inj := NewRequestInjector(cfg)
	rawReq := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`)

	res, err := inj.Inject(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.SyntheticToolName != "" {
		t.Errorf("expected empty synthetic tool name in off mode, got %s", res.SyntheticToolName)
	}
	if string(res.TransformedBody) != string(rawReq) {
		t.Errorf("expected verbatim body in off mode")
	}
}
