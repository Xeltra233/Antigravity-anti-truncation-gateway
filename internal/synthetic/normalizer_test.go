package synthetic

import (
	"encoding/json"
	"testing"

	"antigravity-gateway/internal/config"
)

func TestNormalizeNonStreamingSyntheticOnly(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:      "prefer",
		MaxResponseBytes: 1024 * 1024,
	}
	normalizer := NewResponseNormalizer(cfg)

	synthTool := "agw_emit_test123"
	upstreamResp := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gemini-3.5-flash-low",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_1",
					"type": "function",
					"function": {
						"name": "agw_emit_test123",
						"arguments": "{\"content\": \"This is the final response.\"}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
	}`)

	normalized, stats, err := normalizer.NormalizeNonStreaming(upstreamResp, synthTool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stats.SyntheticHit || stats.SyntheticCallCount != 1 || stats.RealToolCallCount != 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	var resp map[string]any
	if err := json.Unmarshal(normalized, &resp); err != nil {
		t.Fatalf("failed to unmarshal normalized JSON: %v", err)
	}

	choices := resp["choices"].([]any)
	c0 := choices[0].(map[string]any)
	msg := c0["message"].(map[string]any)

	if msg["content"] != "This is the final response." {
		t.Errorf("content mismatch: got %v", msg["content"])
	}
	if _, exists := msg["tool_calls"]; exists {
		t.Errorf("tool_calls should be removed when only synthetic tool is present")
	}
	if c0["finish_reason"] != "stop" {
		t.Errorf("finish_reason should be converted to stop, got %v", c0["finish_reason"])
	}
	if resp["model"] != "gemini-3.5-flash-low" {
		t.Errorf("model should be preserved verbatim, got %v", resp["model"])
	}
}

func TestNormalizeNonStreamingRealAndSyntheticMixed(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:      "prefer",
		MaxResponseBytes: 1024 * 1024,
	}
	normalizer := NewResponseNormalizer(cfg)

	synthTool := "agw_emit_abc"
	upstreamResp := []byte(`{
		"id": "chatcmpl-456",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_real",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"location\": \"Tokyo\"}"
						}
					},
					{
						"id": "call_synth",
						"type": "function",
						"function": {
							"name": "agw_emit_abc",
							"arguments": "{\"content\": \"Weather query result.\"}"
						}
					}
				]
			},
			"finish_reason": "tool_calls"
		}]
	}`)

	normalized, stats, err := normalizer.NormalizeNonStreaming(upstreamResp, synthTool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stats.SyntheticHit || stats.RealToolCallCount != 1 || stats.SyntheticCallCount != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	var resp map[string]any
	_ = json.Unmarshal(normalized, &resp)
	choices := resp["choices"].([]any)
	c0 := choices[0].(map[string]any)
	msg := c0["message"].(map[string]any)

	if msg["content"] != "Weather query result." {
		t.Errorf("expected synthetic content, got %v", msg["content"])
	}

	toolCalls, ok := msg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected 1 real tool call preserved, got %v", msg["tool_calls"])
	}
	fn := toolCalls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Errorf("real tool call modified: %v", fn["name"])
	}
	if c0["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason should stay tool_calls, got %v", c0["finish_reason"])
	}
}

func TestNormalizeNonStreamingContentConflictNoConcat(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:      "prefer",
		MaxResponseBytes: 1024 * 1024,
	}
	normalizer := NewResponseNormalizer(cfg)

	synthTool := "agw_emit_xyz"
	// Upstream mistakenly returned standard content AND synthetic tool call
	upstreamResp := []byte(`{
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Standard preamble text that should NOT be concatenated.",
				"tool_calls": [{
					"id": "call_1",
					"type": "function",
					"function": {
						"name": "agw_emit_xyz",
						"arguments": "{\"content\": \"The authoritative synthetic response.\"}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}]
	}`)

	normalized, stats, err := normalizer.NormalizeNonStreaming(upstreamResp, synthTool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stats.ContentConflict {
		t.Errorf("expected ContentConflict to be true")
	}

	var resp map[string]any
	_ = json.Unmarshal(normalized, &resp)
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)

	// Must be ONLY the synthetic response, NEVER concatenated!
	expected := "The authoritative synthetic response."
	if msg["content"] != expected {
		t.Errorf("expected strictly %q without concatenation, got %q", expected, msg["content"])
	}
}
