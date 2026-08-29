package synthetic

import (
	"bytes"
	"strings"
	"testing"

	"antigravity-gateway/internal/config"
)

func TestIncrementalJSONDecoder(t *testing.T) {
	// Simulate arbitrary byte slices
	chunks := []string{
		`{"`,
		`content`,
		`": `,
		`"Hello `,
		`world\n`,
		`Line 2 `,
		`\u4e16\u754c"`,
		`}`,
	}

	decoder := NewIncrementalJSONStringDecoder()
	var emitted strings.Builder
	for _, c := range chunks {
		out, done, err := decoder.Feed(c)
		if err != nil {
			t.Fatalf("feed error: %v", err)
		}
		emitted.WriteString(out)
		if done {
			break
		}
	}

	expected := "Hello world\nLine 2 世界"
	if emitted.String() != expected {
		t.Errorf("decoded text mismatch:\ngot:  %q\nwant: %q", emitted.String(), expected)
	}
}

func TestStreamTransformerSyntheticOnly(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:      "prefer",
		MaxResponseBytes: 1024 * 1024,
	}
	synthName := "agw_emit_stream123"
	transformer := NewStreamTransformer(cfg, synthName)

	upstreamSSE := `data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":100,"model":"gemini-3.5-flash-low","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_s","type":"function","function":{"name":"agw_emit_stream123","arguments":"{\"content\": \""}}]}}]}

data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":100,"model":"gemini-3.5-flash-low","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"Streamed message content."}}]}}]}

data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":100,"model":"gemini-3.5-flash-low","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"}"}}]}}]}

data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":100,"model":"gemini-3.5-flash-low","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"total_tokens":50}}

data: [DONE]
`

	var downstream bytes.Buffer
	stats, err := transformer.Transform(strings.NewReader(upstreamSSE), &downstream, nil)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	if !stats.SyntheticHit {
		t.Errorf("expected SyntheticHit true")
	}

	output := downstream.String()
	if !strings.Contains(output, "Streamed message content.") {
		t.Errorf("output missing content: %s", output)
	}
	if strings.Contains(output, synthName) {
		t.Errorf("synthetic tool name leaked to downstream output: %s", output)
	}
	if !strings.Contains(output, `"finish_reason":"stop"`) {
		t.Errorf("finish_reason should be normalized to stop: %s", output)
	}
	if !strings.Contains(output, "data: [DONE]\n\n") {
		t.Errorf("missing [DONE] marker at end: %s", output)
	}
}

func TestStreamTransformerConflictNoConcat(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:      "prefer",
		MaxResponseBytes: 1024 * 1024,
	}
	synthName := "agw_emit_stream_conf"
	transformer := NewStreamTransformer(cfg, synthName)

	upstreamSSE := `data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Standard preamble that MUST be dropped."}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"agw_emit_stream_conf","arguments":"{\"content\": \"Real Answer\"}"}}]}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`

	var downstream bytes.Buffer
	stats, err := transformer.Transform(strings.NewReader(upstreamSSE), &downstream, nil)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	if !stats.ContentConflict {
		t.Errorf("expected ContentConflict true")
	}

	output := downstream.String()
	if strings.Contains(output, "Standard preamble") {
		t.Errorf("standard preamble leaked / concatenated into stream: %s", output)
	}
	if !strings.Contains(output, "Real Answer") {
		t.Errorf("output missing Real Answer: %s", output)
	}
}
