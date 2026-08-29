package synthetic

import (
	"testing"

	"antigravity-gateway/internal/config"
)

func TestModelFilterClassification(t *testing.T) {
	filter := NewModelFilter("", "")

	tests := []struct {
		model    string
		wantText bool
	}{
		// Standard text / LLM chat models -> true
		{"gpt-4o", true},
		{"gpt-4o-mini", true},
		{"claude-3-5-sonnet-20241022", true},
		{"claude-opus-4-6-thinking", true},
		{"gemini-3.5-flash-low", true},
		{"gemini-3.7-flash-high", true},
		{"deepseek-chat", true},
		{"deepseek-reasoner", true},
		{"qwen-2.5-72b-instruct", true},
		{"llama-3.3-70b-instruct", true},
		{"gpt-oss-120b-medium", true},

		// Non-text models (image, vision, audio, tts, whisper, embed) -> false
		{"gemini-3.1-flash-image", false},
		{"dall-e-3", false},
		{"text-embedding-3-small", false},
		{"text-embedding-ada-002", false},
		{"whisper-1", false},
		{"tts-1", false},
		{"tts-1-hd", false},
		{"flux-schnell", false},
		{"sdxl-turbo", false},
		{"stable-diffusion-3", false},
		{"omni-moderation-latest", false},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			got := filter.IsTextModel(tc.model)
			if got != tc.wantText {
				t.Errorf("IsTextModel(%q) = %v, want %v", tc.model, got, tc.wantText)
			}
		})
	}
}

func TestInjectorNonTextModelPassthrough(t *testing.T) {
	cfg := &config.Config{
		WrapperMode: "prefer",
	}
	inj := NewRequestInjector(cfg)

	// Test non-text model: gemini-3.1-flash-image
	imageReq := []byte(`{
		"model": "gemini-3.1-flash-image",
		"messages": [{"role": "user", "content": "draw a cat"}]
	}`)

	res, err := inj.Inject(imageReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.SyntheticToolName != "" {
		t.Errorf("expected empty synthetic tool name for non-text model, got %s", res.SyntheticToolName)
	}
	if string(res.TransformedBody) != string(imageReq) {
		t.Errorf("expected transformed body to be verbatim identical to original body for non-text model")
	}

	// Test text model: gemini-3.5-flash-low
	textReq := []byte(`{
		"model": "gemini-3.5-flash-low",
		"messages": [{"role": "user", "content": "hello"}]
	}`)

	resText, err := inj.Inject(textReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resText.SyntheticToolName == "" {
		t.Errorf("expected synthetic tool to be generated for text chat model")
	}
}
