package imagectx

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTransformer_StandardMode(t *testing.T) {
	cfg := DefaultPipelineConfig()
	inputJSON := `{"model":"gemini-3.7-flash-high","messages":[{"role":"user","content":"hello"}]}`
	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeStandard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("expected fallback false for standard mode")
	}
	if string(out) != inputJSON {
		t.Errorf("expected output identical to input in standard mode")
	}
}

func TestTransformer_AllImageMode(t *testing.T) {
	cfg := DefaultPipelineConfig()
	inputJSON := `{"model":"gemini-3.7-flash-high","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hello"}]}`
	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeAllImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("unexpected fallback")
	}

	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	msgs := root["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	for i, m := range msgs {
		mMap := m.(map[string]any)
		parts := mMap["content"].([]any)
		p0 := parts[0].(map[string]any)
		if p0["type"] != "image_url" {
			t.Errorf("msg %d expected type image_url, got %v", i, p0["type"])
		}
	}
}

func TestTransformer_CurrentTurnOnlyMode(t *testing.T) {
	cfg := DefaultPipelineConfig()
	inputJSON := `{"model":"gemini-3.7-flash-high","messages":[{"role":"system","content":"sys"},{"role":"user","content":"old user"},{"role":"assistant","content":"old reply"},{"role":"user","content":"new user"}]}`
	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeCurrentTurnOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("unexpected fallback")
	}

	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	msgs := root["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// 0: system -> text
	m0 := msgs[0].(map[string]any)
	if m0["content"] != "sys" {
		t.Errorf("expected system content to remain string sys, got %v", m0["content"])
	}

	// 1: old user -> text
	m1 := msgs[1].(map[string]any)
	if m1["content"] != "old user" {
		t.Errorf("expected old user content to remain string, got %v", m1["content"])
	}

	// 2: old assistant -> text
	m2 := msgs[2].(map[string]any)
	if m2["content"] != "old reply" {
		t.Errorf("expected old assistant content to remain string, got %v", m2["content"])
	}

	// 3: new user -> image_url
	m3 := msgs[3].(map[string]any)
	parts3 := m3["content"].([]any)
	p0 := parts3[0].(map[string]any)
	if p0["type"] != "image_url" {
		t.Errorf("expected last user turn to be image_url, got %v", p0["type"])
	}
}

func TestTransformer_ToolResponseProtection(t *testing.T) {
	cfg := DefaultPipelineConfig()
	inputJSON := `{
		"model":"gemini-3.7-flash-high",
		"messages":[
			{"role":"user","content":"run tool"},
			{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"query","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"{\"result\":\"success\"}"}
		]
	}`
	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeAllImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("unexpected fallback")
	}

	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	msgs := root["messages"].([]any)
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" {
		t.Errorf("expected role tool, got %v", toolMsg["role"])
	}
	if toolMsg["content"] != "{\"result\":\"success\"}" {
		t.Errorf("expected tool content to remain native string, got %v", toolMsg["content"])
	}
}

func TestTransformer_NativeImagePreservation(t *testing.T) {
	cfg := DefaultPipelineConfig()
	nativeURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	inputJSON := `{
		"model":"gemini-3.7-flash-high",
		"messages":[
			{"role":"user","content":[
				{"type":"text","text":"check this image:"},
				{"type":"image_url","image_url":{"url":"` + nativeURL + `","detail":"high"}}
			]}
		]
	}`

	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeAllImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("unexpected fallback")
	}

	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	msgs := root["messages"].([]any)
	parts := msgs[0].(map[string]any)["content"].([]any)

	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (1 rasterized text + 1 native image), got %d", len(parts))
	}

	// Part 0: rasterized text
	p0 := parts[0].(map[string]any)
	if p0["type"] != "image_url" {
		t.Errorf("expected part 0 to be image_url, got %v", p0["type"])
	}

	// Part 1: native image untouched
	p1 := parts[1].(map[string]any)
	if p1["type"] != "image_url" {
		t.Errorf("expected part 1 to be image_url, got %v", p1["type"])
	}
	imgURLMap := p1["image_url"].(map[string]any)
	if imgURLMap["url"] != nativeURL {
		t.Errorf("expected native image URL to be perfectly preserved, got %v", imgURLMap["url"])
	}
}

func TestTransformer_ImageGenBypass(t *testing.T) {
	cfg := DefaultPipelineConfig()
	inputJSON := `{"model":"gemini-3.1-flash-image","messages":[{"role":"user","content":"draw a red apple"}]}`
	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeAllImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("unexpected fallback")
	}
	if string(out) != inputJSON {
		t.Errorf("expected image generation request to bypass transformation completely")
	}
}

func TestTransformer_LimitsFallback(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.MaxPages = 1 // strict limit
	cfg.FallbackOnError = true

	longText := strings.Repeat("Very long line for testing fallback limit.\n", 100)
	inputJSON := `{"model":"gemini-3.7-flash-high","messages":[{"role":"user","content":"` + strings.ReplaceAll(longText, "\n", "\\n") + `"}]}`

	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeAllImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb {
		t.Errorf("expected fallback to be true when exceeding max pages")
	}
	if string(out) != inputJSON {
		t.Errorf("expected fallback output to equal original input JSON")
	}
}

func TestTransformer_SillyTavernDatabasePluginCompatibility(t *testing.T) {
	cfg := DefaultPipelineConfig()
	// Simulates SillyTavern RAG / Vector DB plugin injecting system memory context into current turn + a message with missing role
	inputJSON := `{
		"model":"gemini-3.7-flash-high",
		"messages":[
			{"role":"system","content":"Global World Info"},
			{"role":"user","content":"turn 1 user"},
			{"role":"assistant","content":"turn 1 assistant reply"},
			{"role":"system","content":"[Vector DB Memory Injection for Turn 2] User prefers tea over coffee."},
			{"content":"turn 2 user query"}
		]
	}`

	out, fb, err := TransformRequest([]byte(inputJSON), cfg, ModeCurrentTurnOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb {
		t.Errorf("unexpected fallback")
	}

	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	msgs := root["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	// 0: Global World Info (History turn <= lastAssistantIdx) -> text
	m0 := msgs[0].(map[string]any)
	if m0["content"] != "Global World Info" {
		t.Errorf("expected history system to remain string, got %v", m0["content"])
	}

	// 1: Turn 1 User (History) -> text
	m1 := msgs[1].(map[string]any)
	if m1["content"] != "turn 1 user" {
		t.Errorf("expected history user to remain string, got %v", m1["content"])
	}

	// 2: Turn 1 Assistant (History lastAssistantIdx) -> text
	m2 := msgs[2].(map[string]any)
	if m2["content"] != "turn 1 assistant reply" {
		t.Errorf("expected history assistant to remain string, got %v", m2["content"])
	}

	// 3: Injected System Memory in Current Turn (Turn 2 > lastAssistantIdx) -> image_url with role SYSTEM
	m3 := msgs[3].(map[string]any)
	parts3, ok := m3["content"].([]any)
	if !ok || len(parts3) == 0 {
		t.Fatalf("expected injected system to be converted to parts, got %v", m3["content"])
	}
	p3 := parts3[0].(map[string]any)
	if p3["type"] != "image_url" {
		t.Errorf("expected injected system in current turn to be image_url, got %v", p3["type"])
	}

	// 4: Missing role in Current Turn -> auto-filled to 'user' and converted to image_url
	m4 := msgs[4].(map[string]any)
	if m4["role"] != "user" {
		t.Errorf("expected missing role to be auto-filled to 'user', got %v", m4["role"])
	}
	parts4, ok := m4["content"].([]any)
	if !ok || len(parts4) == 0 {
		t.Fatalf("expected missing role message to be converted to parts, got %v", m4["content"])
	}
	p4 := parts4[0].(map[string]any)
	if p4["type"] != "image_url" {
		t.Errorf("expected current turn user message to be image_url, got %v", p4["type"])
	}
}
