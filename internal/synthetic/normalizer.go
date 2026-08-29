package synthetic

import (
	"encoding/json"
	"fmt"
	"strings"

	"antigravity-gateway/internal/config"
)

type NormalizeStats struct {
	SyntheticHit       bool
	SyntheticRepaired  bool
	ContentConflict    bool
	RealToolCallCount  int
	SyntheticCallCount int
}

type ResponseNormalizer struct {
	cfg *config.Config
}

func NewResponseNormalizer(cfg *config.Config) *ResponseNormalizer {
	return &ResponseNormalizer{cfg: cfg}
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   any            `json:"usage,omitempty"`
	// Extra top-level fields preserved via raw json if needed
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
	NativeFinish any           `json:"native_finish_reason,omitempty"`
}

type OpenAIMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content"` // string or nil
	ReasoningContent any              `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIFunctionCallDesc `json:"function"`
}

type OpenAIFunctionCallDesc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// NormalizeNonStreaming normalizes an upstream Chat Completion JSON response.
func (n *ResponseNormalizer) NormalizeNonStreaming(rawResp []byte, syntheticToolName string) ([]byte, *NormalizeStats, error) {
	stats := &NormalizeStats{}

	// If WRAPPER_MODE is "off" or no synthetic tool name, return as-is
	if n.cfg.WrapperMode == "off" || syntheticToolName == "" {
		return rawResp, stats, nil
	}

	var respMap map[string]any
	if err := json.Unmarshal(rawResp, &respMap); err != nil {
		return nil, nil, fmt.Errorf("invalid upstream response JSON: %w", err)
	}

	rawChoices, ok := respMap["choices"].([]any)
	if !ok || len(rawChoices) == 0 {
		return rawResp, stats, nil
	}

	for i, cRaw := range rawChoices {
		cMap, ok := cRaw.(map[string]any)
		if !ok {
			continue
		}

		mMap, ok := cMap["message"].(map[string]any)
		if !ok {
			continue
		}

		var realCalls []any
		var syntheticCalls []map[string]any

		if tcList, ok := mMap["tool_calls"].([]any); ok {
			for _, tcRaw := range tcList {
				tcMap, ok := tcRaw.(map[string]any)
				if !ok {
					continue
				}
				fnMap, ok := tcMap["function"].(map[string]any)
				if !ok {
					continue
				}
				name, _ := fnMap["name"].(string)

				if name == syntheticToolName {
					syntheticCalls = append(syntheticCalls, tcMap)
				} else {
					realCalls = append(realCalls, tcMap)
				}
			}
		}

		stats.RealToolCallCount += len(realCalls)
		stats.SyntheticCallCount += len(syntheticCalls)

		if len(syntheticCalls) > 0 {
			stats.SyntheticHit = true

			// Parse and concatenate synthetic calls
			var syntheticContentBuilder strings.Builder
			for _, sCall := range syntheticCalls {
				fnMap := sCall["function"].(map[string]any)
				argsStr, _ := fnMap["arguments"].(string)

				res, err := RepairSyntheticArguments(argsStr, int(n.cfg.MaxResponseBytes))
				if err != nil {
					return nil, stats, fmt.Errorf("failed to decode synthetic tool arguments: %w", err)
				}
				if res.Repaired {
					stats.SyntheticRepaired = true
				}
				syntheticContentBuilder.WriteString(res.Content)
			}

			finalSyntheticContent := syntheticContentBuilder.String()

			// Check standard content: MUST NOT CONCATENATE if standard content exists!
			if origContent, exists := mMap["content"]; exists && origContent != nil {
				if strContent, ok := origContent.(string); ok && strings.TrimSpace(strContent) != "" {
					stats.ContentConflict = true
					// Intentionally discard standard content to maintain single source of truth
				}
			}

			mMap["role"] = "assistant"
			mMap["content"] = finalSyntheticContent

			if len(realCalls) > 0 {
				mMap["tool_calls"] = realCalls
				cMap["finish_reason"] = "tool_calls"
			} else {
				delete(mMap, "tool_calls")
				finishReason, _ := cMap["finish_reason"].(string)
				if finishReason == "tool_calls" {
					cMap["finish_reason"] = "stop"
				}
			}
		} else {
			// No synthetic tool calls
			if n.cfg.WrapperMode == "required" && len(realCalls) == 0 {
				return nil, stats, fmt.Errorf("synthetic tool call required but not present in upstream response")
			}
		}

		cMap["message"] = mMap
		rawChoices[i] = cMap
	}

	respMap["choices"] = rawChoices

	normalized, err := json.Marshal(respMap)
	if err != nil {
		return nil, stats, fmt.Errorf("failed to marshal normalized response: %w", err)
	}

	return normalized, stats, nil
}
