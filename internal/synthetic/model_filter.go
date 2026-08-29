package synthetic

import (
	"regexp"
	"strings"
)

var (
	// DefaultNonTextModelRegex matches models that are clearly NOT text chat models (image, audio, embeddings, etc.)
	DefaultNonTextModelRegex = regexp.MustCompile(`(?i)(image|vision-preview|audio|tts|stt|whisper|embed|embedding|video|flux|dall-e|sdxl|diffusion|moderation)`)

	// DefaultTextModelRegex matches general text and reasoning LLM models
	DefaultTextModelRegex = regexp.MustCompile(`(?i)(gpt|claude|gemini|deepseek|qwen|llama|mistral|gemma|chat|text|instruct|reasoner|agent|plus|pro|flash|opus|sonnet|haiku|medium|low|high|lite|base|code|coder)`)
)

type ModelFilter struct {
	textRegex    *regexp.Regexp
	nonTextRegex *regexp.Regexp
}

func NewModelFilter(customTextPattern, customNonTextPattern string) *ModelFilter {
	filter := &ModelFilter{
		textRegex:    DefaultTextModelRegex,
		nonTextRegex: DefaultNonTextModelRegex,
	}

	if customTextPattern != "" {
		if r, err := regexp.Compile(customTextPattern); err == nil {
			filter.textRegex = r
		}
	}

	if customNonTextPattern != "" {
		if r, err := regexp.Compile(customNonTextPattern); err == nil {
			filter.nonTextRegex = r
		}
	}

	return filter
}

func (f *ModelFilter) IsTextModel(modelName string) bool {
	trimmed := strings.TrimSpace(modelName)
	if trimmed == "" {
		return false
	}

	// Step 1: If it matches non-text patterns (e.g. image, audio, embeddings), it is definitely NOT a text chat model
	if f.nonTextRegex != nil && f.nonTextRegex.MatchString(trimmed) {
		return false
	}

	// Step 2: If it matches recognized text model patterns, it is a text model
	if f.textRegex != nil && f.textRegex.MatchString(trimmed) {
		return true
	}

	// Step 3: Default fallback - if it doesn't match non-text patterns and contains common alphanumeric identifier, treat as text
	return true
}
