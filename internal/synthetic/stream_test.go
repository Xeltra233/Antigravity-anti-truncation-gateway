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
		`\u4e16\u754c `,
		`\ud83d\ude00"`,
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
	rem, _ := decoder.Finish(1024 * 1024)
	emitted.WriteString(rem)

	expected := "Hello world\nLine 2 世界 \U0001F600"
	if emitted.String() != expected {
		t.Errorf("decoded text mismatch:\ngot:  %q\nwant: %q", emitted.String(), expected)
	}
}

func TestIncrementalJSONDecoderAlternativeKey(t *testing.T) {
	chunks := []string{
		`{"`,
		`text`,
		`": `,
		`"Alternative key content"`,
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
	rem, _ := decoder.Finish(1024 * 1024)
	emitted.WriteString(rem)

	expected := "Alternative key content"
	if emitted.String() != expected {
		t.Errorf("decoded text mismatch:\ngot:  %q\nwant: %q", emitted.String(), expected)
	}
}

func TestIncrementalJSONDecoderRawTextFallback(t *testing.T) {
	chunks := []string{
		`Hello `,
		`raw `,
		`text `,
		`stream`,
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
	rem, _ := decoder.Finish(1024 * 1024)
	emitted.WriteString(rem)

	expected := "Hello raw text stream"
	if emitted.String() != expected {
		t.Errorf("decoded text mismatch:\ngot:  %q\nwant: %q", emitted.String(), expected)
	}
}

func TestIncrementalJSONDecoderUTF8ChunkBoundarySplit(t *testing.T) {
	// Chinese character "你" = E4 BD A0, "好" = E5 A5 BD, "世" = E4 B8 96, "界" = E7 95 8C
	// Intentionally split across chunk boundaries:
	// Chunk 1: `{"content": "` + E4 BD (incomplete "你")
	// Chunk 2: A0 (completes "你") + E5 (incomplete "好")
	// Chunk 3: A5 BD (completes "好") + 世界"}
	chunk1 := `{"content": "` + string([]byte{0xE4, 0xBD})
	chunk2 := string([]byte{0xA0, 0xE5})
	chunk3 := string([]byte{0xA5, 0xBD}) + `世界"}`

	chunks := []string{chunk1, chunk2, chunk3}

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
	rem, _ := decoder.Finish(1024 * 1024)
	emitted.WriteString(rem)

	expected := "你好世界"
	if emitted.String() != expected {
		t.Errorf("UTF-8 boundary split decoded mismatch:\ngot:  %q\nwant: %q", emitted.String(), expected)
	}
	if strings.ContainsRune(emitted.String(), '\uFFFD') {
		t.Errorf("output contains replacement character U+FFFD (black question mark artifact)!")
	}
}

func TestIncrementalJSONDecoderExtremeByteByByte(t *testing.T) {
	// Extreme case: feed full JSON string 1 byte at a time!
	rawJSON := `{"content": "你好，世界！🌟🚀"}`
	decoder := NewIncrementalJSONStringDecoder()
	var emitted strings.Builder

	for i := 0; i < len(rawJSON); i++ {
		singleByteChunk := rawJSON[i : i+1]
		out, _, err := decoder.Feed(singleByteChunk)
		if err != nil {
			t.Fatalf("byte %d feed error: %v", i, err)
		}
		emitted.WriteString(out)
	}
	rem, _ := decoder.Finish(1024 * 1024)
	emitted.WriteString(rem)

	expected := "你好，世界！🌟🚀"
	if emitted.String() != expected {
		t.Errorf("byte-by-byte decoded mismatch:\ngot:  %q\nwant: %q", emitted.String(), expected)
	}
	if strings.ContainsRune(emitted.String(), '\uFFFD') {
		t.Errorf("output contains replacement character U+FFFD (black question mark artifact) in byte-by-byte feed!")
	}
}

func TestStreamTransformerSyntheticOnly(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:           "prefer",
		MaxResponseBytes:      1024 * 1024,
		StreamSideBufferBytes: 512,
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

func TestStreamTransformerDirectStreaming(t *testing.T) {
	// Test standard content streaming without synthetic tool calls
	cfg := &config.Config{
		WrapperMode:           "prefer",
		MaxResponseBytes:      1024 * 1024,
		StreamSideBufferBytes: 0, // 0 = immediate true streaming
	}
	synthName := "agw_emit_stream_dir"
	transformer := NewStreamTransformer(cfg, synthName)

	upstreamSSE := `data: {"id":"c1","object":"chat.completion.chunk","created":100,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"c1","object":"chat.completion.chunk","created":100,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello "}}]}

data: {"id":"c1","object":"chat.completion.chunk","created":100,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"world!"}}]}

data: {"id":"c1","object":"chat.completion.chunk","created":100,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`

	var downstream bytes.Buffer
	stats, err := transformer.Transform(strings.NewReader(upstreamSSE), &downstream, nil)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	if stats.SyntheticHit {
		t.Errorf("expected SyntheticHit false")
	}

	output := downstream.String()
	if !strings.Contains(output, "Hello ") || !strings.Contains(output, "world!") {
		t.Errorf("missing content chunks in output: %s", output)
	}
	if !strings.Contains(output, `"finish_reason":"stop"`) {
		t.Errorf("finish_reason should be stop: %s", output)
	}
}

func TestStreamTransformerConflictNoConcat(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:           "prefer",
		MaxResponseBytes:      1024 * 1024,
		StreamSideBufferBytes: 1024,
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

func TestStreamTransformerReasoningContent(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:           "prefer",
		MaxResponseBytes:      1024 * 1024,
		StreamSideBufferBytes: 0,
	}
	synthName := "agw_emit_stream_reason"
	transformer := NewStreamTransformer(cfg, synthName)

	upstreamSSE := `data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"Thinking step 1..."}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":" Thinking step 2."}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"agw_emit_stream_reason","arguments":"{\"content\": \"Final answer\"}"}}]}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

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
	if !strings.Contains(output, "Thinking step 1...") {
		t.Errorf("missing reasoning_content: %s", output)
	}
	if !strings.Contains(output, "Final answer") {
		t.Errorf("missing final answer: %s", output)
	}
}

func TestStreamTransformerRealToolCalls(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:           "prefer",
		MaxResponseBytes:      1024 * 1024,
		StreamSideBufferBytes: 0,
	}
	synthName := "agw_emit_stream_real"
	transformer := NewStreamTransformer(cfg, synthName)

	upstreamSSE := `data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}}]}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`

	var downstream bytes.Buffer
	stats, err := transformer.Transform(strings.NewReader(upstreamSSE), &downstream, nil)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	if stats.RealToolCallCount != 1 {
		t.Errorf("expected RealToolCallCount 1, got %d", stats.RealToolCallCount)
	}

	output := downstream.String()
	if !strings.Contains(output, "get_weather") {
		t.Errorf("missing get_weather tool call in output: %s", output)
	}
	if !strings.Contains(output, `"finish_reason":"tool_calls"`) {
		t.Errorf("finish_reason should remain tool_calls for real tools: %s", output)
	}
}

func TestStreamTransformerUsageOnlyChunk(t *testing.T) {
	cfg := &config.Config{
		WrapperMode:           "prefer",
		MaxResponseBytes:      1024 * 1024,
		StreamSideBufferBytes: 0,
	}
	synthName := "agw_emit_stream_usage"
	transformer := NewStreamTransformer(cfg, synthName)

	upstreamSSE := `data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hi"}}]}

data: {"id":"c1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	var downstream bytes.Buffer
	_, err := transformer.Transform(strings.NewReader(upstreamSSE), &downstream, nil)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	output := downstream.String()
	if !strings.Contains(output, `"total_tokens":15`) {
		t.Errorf("missing usage chunk in output: %s", output)
	}
}
